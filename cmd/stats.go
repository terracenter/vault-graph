package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb/factory"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Muestra estadísticas del grafo",
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

		stats, relations, err := store.Stats(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to query stats: %w", err)
		}

		if format == "json" {
			data := map[string]interface{}{
				"backend":       cfg.Backend,
				"node_stats":    stats,
				"client_server": relations,
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("=== Node Statistics (%s) ===\n", cfg.Backend)
			totalNodes := 0
			for _, stat := range stats {
				fmt.Printf("  %-12s: %4d\n", stat.Type, stat.Count)
				totalNodes += stat.Count
			}
			fmt.Printf("  %-12s: %4d\n", "TOTAL", totalNodes)

			fmt.Println("\n=== Sample Cliente<->Servidor Relations ===")
			if len(relations) == 0 {
				fmt.Println("  (no relations found)")
			} else {
				for _, rel := range relations {
					fmt.Printf("  %s --[%s]--> %s\n", rel.Cliente, rel.Type, rel.Servidor)
				}
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
