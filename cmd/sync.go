package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
)

var (
	full       bool
	sinceMtime bool
	prune      bool
)

var syncCmd = &cobra.Command{
	Use:   "sync [--full|--since-mtime] [--prune]",
	Short: "Carga o sincroniza el vault en el grafo Kuzu",
	Long: `Carga el vault en el grafo Kuzu (backend único desde 2026-08-12).

Por defecto, usa --full (re-sincroniza todos los archivos).

Flags:
  --full         Sincroniza todos los archivos (default: true)
  --since-mtime  Solo sincroniza archivos modificados
  --prune        Borra del grafo los nodos cuyos paths ya no existen en el
                 vault (purga huérfanos). Útil para eliminar referencias a
                 archivos que fueron movidos o borrados del vault.

Requiere KUZU_PATH en el .env o como variable de entorno.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if cfg.KuzuPath == "" {
			return fmt.Errorf("sync requiere KUZU_PATH (Kuzu backend); AGE no soportado desde 2026-08-12")
		}

		fmt.Printf("Backend: Kuzu (KUZU_PATH=%s)\n", cfg.KuzuPath)
		return runSyncKuzu(cfg, prune)
	},
}

func runSyncKuzu(cfg *config.Config, doPrune bool) error {
	paths, err := collectMarkdownFiles(cfg.VaultPath)
	if err != nil {
		return fmt.Errorf("collect: %w", err)
	}
	fmt.Printf("Found %d markdown files\n", len(paths))

	// Borrar DB previa.
	if err := os.Remove(cfg.KuzuPath); err != nil && !os.IsNotExist(err) {
		fmt.Printf("warning: remove %s: %v\n", cfg.KuzuPath, err)
	}
	if err := os.Remove(cfg.KuzuPath + ".wal"); err != nil && !os.IsNotExist(err) {
		fmt.Printf("warning: remove %s.wal: %v\n", cfg.KuzuPath, err)
	}

	conn, err := kuzu.Open(cfg.KuzuPath)
	if err != nil {
		return fmt.Errorf("open Kuzu: %w", err)
	}
	defer conn.Close()

	// Crear nodos.
	startNodes := time.Now()
	for _, p := range paths {
		q := fmt.Sprintf("CREATE (n:File {path: '%s'})", escapeCypher(p))
		if _, err := conn.Execute(q); err != nil {
			return fmt.Errorf("create node %s: %w", p, err)
		}
	}
	fmt.Printf("Kuzu:    %d nodos en %s\n", len(paths), time.Since(startNodes))

	// Detectar aristas.
	edgeSet := map[string]struct{}{}
	for _, from := range paths {
		wikilinks, err := extractWikilinks(filepath.Join(cfg.VaultPath, from))
		if err != nil {
			fmt.Printf("warning: extract %s: %v\n", from, err)
			continue
		}
		for _, link := range wikilinks {
			resolved := resolveWikilink(cfg.VaultPath, from, link)
			if resolved == "" {
				continue
			}
			edgeSet[fmt.Sprintf("%s\t%s", from, resolved)] = struct{}{}
		}
	}
	fmt.Printf("Kuzu:    %d aristas detectadas\n", len(edgeSet))

	// Set de paths conocidos, para crear destinos no-.md si hace falta.
	known := make(map[string]bool, len(paths))
	for _, p := range paths {
		known[p] = true
	}

	// Escribir aristas (MERGE para idempotencia; crea destino si falta).
	startEdges := time.Now()
	written := 0
	for edge := range edgeSet {
		parts := strings.SplitN(edge, "\t", 2)
		from, to := parts[0], parts[1]
		if !known[to] {
			mq := fmt.Sprintf("MERGE (n:File {path: '%s'})", escapeCypher(to))
			if _, err := conn.Execute(mq); err != nil {
				return fmt.Errorf("create dest node %s: %w", to, err)
			}
			known[to] = true
		}
		q := fmt.Sprintf(
			"MATCH (a:File {path: '%s'}), (b:File {path: '%s'}) MERGE (a)-[:ENLAZA]->(b)",
			escapeCypher(from), escapeCypher(to))
		if _, err := conn.Execute(q); err != nil {
			return fmt.Errorf("create edge %s->%s: %w", from, to, err)
		}
		written++
	}
	fmt.Printf("Kuzu:    %d/%d aristas en %s\n", written, len(edgeSet), time.Since(startEdges))

	fmt.Printf("\n=== Sync Summary (kuzu) ===\n")
	fmt.Printf("Nodes written: %d\n", len(paths))
	fmt.Printf("Edges detected: %d\n", len(edgeSet))
	fmt.Printf("Edges written: %d\n", written)

	if doPrune {
		pruned, err := pruneOrphans(conn, paths)
		if err != nil {
			return fmt.Errorf("prune: %w", err)
		}
		fmt.Printf("Pruned orphan nodes: %d\n", pruned)
	}
	return nil
}

func pruneOrphans(conn *kuzu.Conn, keepPaths []string) (int, error) {
	keep := make(map[string]bool, len(keepPaths))
	for _, p := range keepPaths {
		keep[p] = true
	}

	var orphans []string
	err := conn.Query("MATCH (n:File) RETURN n.path AS p",
		func(row map[string]any) bool {
			p, _ := row["p"].(string)
			if !keep[p] {
				orphans = append(orphans, p)
			}
			return true
		})
	if err != nil {
		return 0, fmt.Errorf("list nodes: %w", err)
	}

	pruned := 0
	for _, p := range orphans {
		q := fmt.Sprintf("MATCH (n:File {path: '%s'}) DETACH DELETE n", escapeCypher(p))
		if _, err := conn.Execute(q); err != nil {
			return pruned, fmt.Errorf("delete node %s: %w", p, err)
		}
		pruned++
	}
	return pruned, nil
}

func collectMarkdownFiles(vaultPath string) ([]string, error) {
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
		relPath = strings.ReplaceAll(relPath, "\\", "/")
		files = append(files, relPath)
		return nil
	})
	return files, err
}

var wikiLinkRegex = regexp.MustCompile(`\[\[([^\]|#]+)(?:#[^\]|]*)?(?:\|[^\]]*)?\]\]`)

func extractWikilinks(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	matches := wikiLinkRegex.FindAllStringSubmatch(string(data), -1)
	var links []string
	for _, m := range matches {
		link := strings.TrimSpace(m[1])
		if link != "" {
			links = append(links, link)
		}
	}
	return links, nil
}

// resolveWikilink resuelve un wikilink a un path RELATIVO dentro del vault.
//
// Política estricta (post-2026-08-12, Kuzu-only):
//   - El destino DEBE ser un archivo con extensión .md.
//   - Se prueban 3 candidatos: `<link>.md`, `<dir(from)>/<link>.md`,
//     `<dir(from)>/<link>/index.md`.
//   - El primer candidato que exista en disco Y sea un archivo regular
//     (no directorio) Y termine en ".md" gana.
//   - Si ninguno cumple, retorna "" → el wikilink queda roto en el
//     grafo (link saliente sin destino), igual que un wikilink a un
//     .md inexistente.
//
// IMPORTANTE: NO se resuelven links a archivos no-.md (.html, .drawio,
// imágenes, etc.) ni a directorios. Eso evita que el sync cree nodos
// fantasma en el grafo. Si el vault quiere enlazar a recursos no-.md,
// debe hacerlo con markdown normal, no wikilinks.
func resolveWikilink(vaultPath, fromPath, link string) string {
	candidates := []string{
		link + ".md",
		filepath.Join(filepath.Dir(fromPath), link) + ".md",
		filepath.Join(filepath.Dir(fromPath), link, "index.md"),
	}
	for _, c := range candidates {
		full := filepath.Join(vaultPath, c)
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		if !strings.HasSuffix(c, ".md") {
			continue
		}
		return strings.ReplaceAll(c, "\\", "/")
	}
	return ""
}

func escapeCypher(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

func init() {
	syncCmd.Flags().BoolVar(&full, "full", true, "sincronizar desde cero (default)")
	syncCmd.Flags().BoolVar(&sinceMtime, "since-mtime", false, "sincronizar solo archivos modificados")
	syncCmd.Flags().BoolVar(&prune, "prune", false, "eliminar nodos cuyos paths ya no existen en el vault (purga huérfanos)")
}