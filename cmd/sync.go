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
	syncvault "github.com/freddytaborda/vault-graph/internal/syncvault"
)

var (
	full       bool
	sinceMtime bool
	prune      bool
)

var syncCmd = &cobra.Command{
	Use:   "sync [--full|--since-mtime] [--prune]",
	Short: "Carga o sincroniza el vault en el grafo",
	Long: `Carga el vault en el grafo (AGE o Kuzu).

Por defecto, usa --full (re-sincroniza todos los archivos).

Flags:
  --full         Sincroniza todos los archivos (default: true)
  --since-mtime  Solo sincroniza archivos modificados
  --prune        Borra del grafo los nodos cuyos paths ya no existen en el
                 vault (purga huérfanos). Útil para eliminar referencias a
                 archivos que fueron movidos o borrados del vault.

Backend:
  - Si KUZU_PATH está configurado, escribe a Kuzu (embebido).
  - Si no, escribe a AGE (PostgreSQL, legacy).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Cargar config
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Si KUZU_PATH está configurado, redirigir al sync-kuzu.
		if cfg.KuzuPath != "" {
			fmt.Printf("Backend: Kuzu (KUZU_PATH=%s)\n", cfg.KuzuPath)
			return runSyncKuzu(cfg, prune)
		}

		// Backend AGE (legacy).
		fmt.Printf("Backend: AGE (DATABASE_URL)\n")
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

// runSyncKuzu ejecuta el sync contra Kuzu usando internal/syncvault.
// Es un wrapper que invoca la función exportada syncvault.Sync.
func runSyncKuzu(cfg *config.Config, prune bool) error {
	stats, err := syncvault.Sync(cfg.VaultPath, cfg.KuzuPath, false)
	if err != nil {
		return fmt.Errorf("sync-kuzu: %w", err)
	}
	fmt.Printf("Found %d markdown files\n", stats.NodeCount)
	fmt.Printf("Kuzu:    %d nodos en %s\n", stats.NodeCount, stats.NodeDuration)
	fmt.Printf("Kuzu:    %d aristas detectadas\n", stats.EdgeCount)
	fmt.Printf("Kuzu:    %d/%d aristas en %s\n", stats.EdgesWritten, stats.EdgeCount, stats.EdgeDuration)
	fmt.Printf("\n=== Sync Summary (kuzu) ===\n")
	fmt.Printf("Nodes written: %d\n", stats.NodeCount)
	fmt.Printf("Edges detected: %d\n", stats.EdgeCount)
	fmt.Printf("Edges written: %d\n", stats.EdgesWritten)
	if prune {
		// Prune: borrar nodos cuyo path ya no existe en el vault.
		pruned, err := syncvault.Prune(cfg.KuzuPath, stats.AllPaths)
		if err != nil {
			return fmt.Errorf("prune Kuzu: %w", err)
		}
		fmt.Printf("Pruned orphan nodes: %d\n", pruned)
	}
	return nil
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
