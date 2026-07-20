package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb"
)

var hops int

var neighborsCmd = &cobra.Command{
	Use:   "neighbors <path> [--hops N]",
	Short: "Obtiene los nodos vecinos de una nota",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

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

		neighbors, err := conn.QueryNeighbors(ctx, path, hops)
		if err != nil {
			return fmt.Errorf("failed to query neighbors: %w", err)
		}

		// Formatear salida
		if format == "json" {
			output := map[string]interface{}{
				"path":      path,
				"hops":      hops,
				"neighbors": neighbors,
			}
			b, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Neighbors of %s (hops=%d): %d found\n\n", path, hops, len(neighbors))
			for _, n := range neighbors {
				fmt.Printf("  [%s] %s [%s]\n", n.Type, n.Path, n.Rel)
			}
		}

		if len(neighbors) == 0 {
			fmt.Printf("No neighbors found for %s.\n", path)
		}

		return nil
	},
}

func init() {
	neighborsCmd.Flags().IntVar(&hops, "hops", 1, "número de hops (default 1)")
}
