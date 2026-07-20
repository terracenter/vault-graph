package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb"
)

var pathCmd = &cobra.Command{
	Use:   "path <path-a> <path-b>",
	Short: "Encuentra el camino más corto entre dos notas",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		from := args[0]
		to := args[1]

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

		path, err := conn.QueryShortestPath(ctx, from, to)
		if err != nil {
			return fmt.Errorf("failed to query shortest path: %w", err)
		}

		// Formatear salida
		if format == "json" {
			b, _ := json.MarshalIndent(path, "", "  ")
			fmt.Println(string(b))
		} else {
			if path.Found {
				fmt.Printf("Shortest path from %s to %s: %d hops\n\n", from, to, path.Hops)
				for i, node := range path.Nodes {
					fmt.Printf("  %d. [%s] %s\n", i, node.Type, node.Path)
				}
			} else {
				fmt.Printf("No path found between %s and %s.\n", from, to)
			}
		}

		return nil
	},
}
