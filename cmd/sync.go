package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/internal/graphdb"
	"github.com/freddytaborda/vault-graph/config"
)

var (
	full       bool
	sinceMtime bool
	prune      bool
)

var syncCmd = &cobra.Command{
	Use:   "sync [--full|--since-mtime] [--prune]",
	Short: "Carga o sincroniza el vault en el grafo AGE",
	Long: `Carga el vault en el grafo AGE.

Por defecto, usa --full (re-sincroniza todos los archivos).

Flags:
  --full         Sincroniza todos los archivos (default: true)
  --since-mtime  Solo sincroniza archivos modificados
  --prune        Borra del grafo los nodos cuyos paths ya no existen en el
                 vault (purga huérfanos). Útil para eliminar referencias a
                 archivos que fueron movidos o borrados del vault.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Cargar config
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Conectar a PostgreSQL
		ctx := context.Background()
		conn, err := graphdb.NewConn(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
		defer conn.Close()

		// Listar todos los archivos .md del vault (excluyendo directorios específicos)
		allPaths, err := collectMarkdownFiles(cfg.VaultPath)
		if err != nil {
			return fmt.Errorf("failed to collect markdown files: %w", err)
		}

		fmt.Printf("Found %d markdown files\n", len(allPaths))

		// Crear loader y cargar
		loader := graphdb.NewLoader(conn, cfg.VaultPath)
		stats, err := loader.LoadFull(ctx, allPaths)
		if err != nil {
			return fmt.Errorf("failed to load vault: %w", err)
		}

		// Prune opcional: borrar nodos cuyos paths ya no existen en el vault
		if prune {
			pruned, err := conn.PruneOrphanNodes(ctx, allPaths)
			if err != nil {
				return fmt.Errorf("failed to prune orphan nodes: %w", err)
			}
			fmt.Printf("Pruned orphan nodes: %d\n", pruned)
		}

		// Imprimir resumen
		fmt.Printf("\n=== Sync Summary ===\n")
		fmt.Printf("Nodes created/updated: (batch operation)\n")
		fmt.Printf("Edges created: (batch operation)\n")
		fmt.Printf("Unresolved wikilinks: %d\n", stats.UnresolvedWikilinks)
		fmt.Printf("Basename collisions detected: %d\n", stats.CollisionsDetected)

		return nil
	},
}

// collectMarkdownFiles recorre el vault y retorna todas las rutas .md
// Excluye: .git/, .obsidian/, Planes/LLM-Wiki/graphrag/
func collectMarkdownFiles(vaultPath string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(vaultPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Excluir directorios
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".obsidian" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			// Excluir graphrag
			if strings.HasSuffix(path, "Planes/LLM-Wiki/graphrag") {
				return filepath.SkipDir
			}
			return nil
		}

		// Solo .md
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		// Convertir a ruta relativa
		relPath, err := filepath.Rel(vaultPath, path)
		if err != nil {
			return err
		}

		// Normalizar separadores a /
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		files = append(files, relPath)
		return nil
	})

	return files, err
}

func init() {
	syncCmd.Flags().BoolVar(&full, "full", true, "sincronizar desde cero (default)")
	syncCmd.Flags().BoolVar(&sinceMtime, "since-mtime", false, "sincronizar solo archivos modificados")
	syncCmd.Flags().BoolVar(&prune, "prune", false, "eliminar nodos cuyos paths ya no existen en el vault (purga huérfanos)")
}
