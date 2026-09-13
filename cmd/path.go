package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb/factory"
	"github.com/spf13/cobra"
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

		store, err := factory.NewStore(cmd.Context(), cfg)
		if err != nil {
			return fmt.Errorf("failed to create store: %w", err)
		}
		defer store.Close()

		path, err := store.ShortestPath(cmd.Context(), from, to)
		if err != nil {
			return fmt.Errorf("failed to query shortest path: %w", err)
		}

		if format == "json" {
			data := map[string]interface{}{
				"backend": cfg.Backend,
				"path":    path,
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
		} else {
			if path.Found {
				fmt.Printf("Shortest path from %s to %s (%s, %d hops):\n\n", from, to, cfg.Backend, path.Hops)
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
