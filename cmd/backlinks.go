package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb/factory"
	"github.com/spf13/cobra"
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

		store, err := factory.NewStore(cmd.Context(), cfg)
		if err != nil {
			return fmt.Errorf("failed to create store: %w", err)
		}
		defer store.Close()

		backlinks, err := store.Backlinks(cmd.Context(), path, 1)
		if err != nil {
			return fmt.Errorf("failed to query backlinks: %w", err)
		}

		if format == "json" {
			output := map[string]interface{}{
				"backend":   cfg.Backend,
				"path":      path,
				"backlinks": backlinks,
			}
			b, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Backlinks to %s (%s): %d found\n\n", path, cfg.Backend, len(backlinks))
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
