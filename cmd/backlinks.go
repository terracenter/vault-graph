package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb"
)

var backlinksCmd = &cobra.Command{
	Use:   "backlinks <path>",
	Short: "Obtiene los wikilinks entrantes a una nota",
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

		backlinks, err := conn.QueryBacklinks(ctx, path)
		if err != nil {
			return fmt.Errorf("failed to query backlinks: %w", err)
		}

		// Formatear salida
		if format == "json" {
			output := map[string]interface{}{
				"path":      path,
				"backlinks": backlinks,
			}
			b, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Backlinks to %s: %d found\n\n", path, len(backlinks))
			for _, b := range backlinks {
				fmt.Printf("  [%s] %s [%s]\n", b.Type, b.Path, b.Rel)
			}
		}

		if len(backlinks) == 0 {
			fmt.Printf("No backlinks found to %s.\n", path)
		}

		return nil
	},
}
