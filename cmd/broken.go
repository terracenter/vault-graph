package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
)

// BrokenLink es la forma uniforme de un link roto.
type BrokenLink struct {
	FromPath string `json:"from_path"`
	ToPath   string `json:"to_path"`
	Type     string `json:"type"`
}

// walkMarkdownFiles recolecta todos los archivos .md en vaultPath.
func walkMarkdownFiles(vaultPath string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(vaultPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".obsidian" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if strings.HasSuffix(path, "Planes/LLM-Wiki/graphrag") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		relPath, err := filepath.Rel(vaultPath, path)
		if err != nil {
			return err
		}
		files = append(files, strings.ReplaceAll(relPath, "\\", "/"))
		return nil
	})
	return files, err
}

// brokenKuzu detecta wikilinks cuyo destino no existe como nota (archivo .md).
//
// Escanea el vault filesystem: lee cada .md, extrae wikilinks, resuelve paths,
// reporta como rotos los que apuntan a archivos inexistentes.
//
// NO usa Kuzu para esta detección porque el sync puede haber creado nodos
// fantasma vía MERGE para destinos no-existentes al momento de escribir aristas.
func brokenKuzu(kuzuPath string, vaultPath string) ([]BrokenLink, error) {
	if vaultPath == "" {
		return nil, fmt.Errorf("brokenKuzu requiere vaultPath")
	}

	// Recoger todos los paths .md existentes.
	paths, err := walkMarkdownFiles(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("walk vault: %w", err)
	}
	known := make(map[string]bool, len(paths))
	for _, p := range paths {
		known[p] = true
	}

	// Para cada archivo, extraer wikilinks y resolver destinos.
	var broken []BrokenLink
	for _, from := range paths {
		wikis, err := extractWikilinks(filepath.Join(vaultPath, from))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: extract %s: %v\n", from, err)
			continue
		}
		for _, link := range wikis {
			resolved := resolveWikilink(vaultPath, from, link)
			if resolved == "" && !strings.HasSuffix(strings.ToLower(link), ".md") {
				// El destino no se resolvió — intentar agregar .md como fallback
				// (el regex captura el texto sin extensión, igual que sync.go).
				resolved = resolveWikilink(vaultPath, from, link+".md")
			}
			if resolved == "" {
				// Sigue sin resolverse — wikilink roto. Reportamos con el nombre crudo.
				target := strings.TrimSuffix(link, ".md") + ".md"
				broken = append(broken, BrokenLink{
					FromPath: from,
					ToPath:   target,
					Type:     "wikilink-unresolvable",
				})
				continue
			}
			if !known[resolved] {
				// Resuelto a un path que no existe como .md en el vault.
				broken = append(broken, BrokenLink{
					FromPath: from,
					ToPath:   resolved,
					Type:     "wikilink-target-missing",
				})
			}
		}
	}

	// Retornar lista vacía justificada si no hay rotos.
	if broken == nil {
		broken = []BrokenLink{}
	}
	return broken, nil
}

// brokenAGE eliminado en cleanup final. El CLI es 100% Kuzu.

var brokenCmd = &cobra.Command{
	Use:   "broken",
	Short: "Lista wikilinks no resueltos",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if cfg.KuzuPath == "" {
			return fmt.Errorf("broken requiere KUZU_PATH (Kuzu backend); AGE no soportado desde 2026-08-12")
		}

		// Verificar que la DB Kuzu existe (obligatorio: requiere una DB sincronizada).
		if !kuzu.FileExists(cfg.KuzuPath) {
			return fmt.Errorf("no se encontró base de datos Kuzu en %s; ejecuta 'vg sync' primero", cfg.KuzuPath)
		}

		links, err := brokenKuzu(cfg.KuzuPath, cfg.VaultPath)
		if err != nil {
			return fmt.Errorf("failed to query broken links: %w", err)
		}

		if format == "json" {
			data := map[string]interface{}{
				"backend": "kuzu",
				"links":   links,
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Broken wikilinks (kuzu): %d\n\n", len(links))
			for _, link := range links {
				fmt.Printf("  %s → %s [%s]\n", link.FromPath, link.ToPath, link.Type)
			}
		}

		if len(links) == 0 {
			fmt.Println("No broken links found.")
		}

		return nil
	},
}
