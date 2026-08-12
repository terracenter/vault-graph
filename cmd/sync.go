package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
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
		// Cargar config
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

func init() {
	syncCmd.Flags().BoolVar(&full, "full", true, "sincronizar desde cero (default)")
	syncCmd.Flags().BoolVar(&sinceMtime, "sinceMtime", false, "sincronizar solo archivos modificados")
	syncCmd.Flags().BoolVar(&prune, "prune", false, "eliminar nodos cuyos paths ya no existen en el vault (purga huérfanos)")
}
