package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Muestra estadísticas del grafo",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		ctx := context.Background()
		conn, err := graphdb.NewConn(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
		defer conn.Close()

		// Obtener stats
		stats, err := conn.QueryNodeStats(ctx)
		if err != nil {
			return fmt.Errorf("failed to query node stats: %w", err)
		}

		// Obtener relaciones Cliente-Servidor
		relations, err := conn.QueryClientServerRelations(ctx, 5)
		if err != nil {
			return fmt.Errorf("failed to query client-server relations: %w", err)
		}

		if format == "json" {
			data := map[string]interface{}{
				"node_stats":       stats,
				"client_server":    relations,
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Println("=== Node Statistics ===")
			totalNodes := 0
			for _, stat := range stats {
				fmt.Printf("  %-12s: %4d\n", stat.Type, stat.Count)
				totalNodes += stat.Count
			}
			fmt.Printf("  %-12s: %4d\n", "TOTAL", totalNodes)

			fmt.Println("\n=== Sample Cliente↔Servidor Relations ===")
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
