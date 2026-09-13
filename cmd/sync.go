package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb"
	"github.com/freddytaborda/vault-graph/internal/graphdb/factory"
	"github.com/spf13/cobra"
)

var (
	full       bool
	sinceMtime bool
	prune      bool
)

var syncCmd = &cobra.Command{
	Use:   "sync [--full|--since-mtime] [--prune]",
	Short: "Carga o sincroniza el vault en el grafo",
	Long: `Carga el vault en el grafo (soporta kuzu y age).

Por defecto, usa --full (re-sincroniza todos los archivos).

Flags:
  --full         Sincroniza todos los archivos (default: true)
  --since-mtime  Solo sincroniza archivos modificados
  --prune        Borra del grafo los nodos cuyos paths ya no existen en el
                 vault (purga huérfanos). Útil para eliminar referencias a
                 archivos que fueron movidos o borrados del vault.

Requiere KUZU_PATH o DATABASE_URL según backend en el .env.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		store, err := factory.NewStore(cmd.Context(), cfg)
		if err != nil {
			return fmt.Errorf("failed to create store: %w", err)
		}
		defer store.Close()

		fmt.Printf("Backend: %s\n", cfg.Backend)
		return runSync(cmd.Context(), cfg, store, prune)
	},
}

func runSync(ctx context.Context, cfg *config.Config, store graphdb.Store, doPrune bool) error {
	paths, err := collectMarkdownFiles(cfg.VaultPath)
	if err != nil {
		return fmt.Errorf("collect: %w", err)
	}
	fmt.Printf("Found %d markdown files\n", len(paths))

	// Crear nodos.
	startNodes := time.Now()
	for _, p := range paths {
		if err := store.MergeNode(ctx, p); err != nil {
			return fmt.Errorf("create node %s: %w", p, err)
		}
	}
	fmt.Printf("Sync:    %d nodos en %s\n", len(paths), time.Since(startNodes))

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
	fmt.Printf("Sync:    %d aristas detectadas\n", len(edgeSet))

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
			if err := store.MergeNode(ctx, to); err != nil {
				return fmt.Errorf("create dest node %s: %w", to, err)
			}
			known[to] = true
		}
		if err := store.MergeEdge(ctx, from, to); err != nil {
			return fmt.Errorf("create edge %s->%s: %w", from, to, err)
		}
		written++
	}
	fmt.Printf("Sync:    %d/%d aristas en %s\n", written, len(edgeSet), time.Since(startEdges))

	fmt.Printf("\n=== Sync Summary (%s) ===\n", cfg.Backend)
	fmt.Printf("Nodes written: %d\n", len(paths))
	fmt.Printf("Edges detected: %d\n", len(edgeSet))
	fmt.Printf("Edges written: %d\n", written)

	if doPrune {
		pruned, err := pruneOrphans(ctx, store, paths)
		if err != nil {
			return fmt.Errorf("prune: %w", err)
		}
		fmt.Printf("Pruned orphan nodes: %d\n", pruned)
	}
	return nil
}

func pruneOrphans(ctx context.Context, store graphdb.Store, keepPaths []string) (int, error) {
	keep := make(map[string]bool, len(keepPaths))
	for _, p := range keepPaths {
		keep[p] = true
	}

	var orphans []string
	allPaths, err := store.ListPaths(ctx, graphdb.PathFilter{})
	if err != nil {
		return 0, fmt.Errorf("list nodes: %w", err)
	}
	for _, p := range allPaths {
		if !keep[p] {
			orphans = append(orphans, p)
		}
	}

	pruned := 0
	for _, p := range orphans {
		if _, err := store.Query(ctx, fmt.Sprintf("MATCH (n:File {path: '%s'}) DETACH DELETE n", escapeCypher(p))); err != nil {
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
func resolveWikilink(vaultPath, fromPath, link string) string {
	clean := strings.TrimSuffix(link, ".md")
	candidates := []string{
		clean + ".md",
		filepath.Join(filepath.Dir(fromPath), clean) + ".md",
		filepath.Join(filepath.Dir(fromPath), clean, "index.md"),
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
	rootCmd.AddCommand(syncCmd)
}
