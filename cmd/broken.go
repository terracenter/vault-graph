package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb/factory"
	"github.com/spf13/cobra"
)

var brokenCmd = &cobra.Command{
	Use:   "broken",
	Short: "Lista wikilinks no resueltos",
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

		links, err := store.BrokenLinks(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to query broken links: %w", err)
		}

		if format == "json" {
			data := map[string]interface{}{
				"backend": cfg.Backend,
				"links":   links,
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Broken wikilinks (%s): %d\n\n", cfg.Backend, len(links))
			for _, link := range links {
				fmt.Printf("  %s → %s [%s]\n", link.FromPath, link.ToPath, link.Type)
			}
		}

		if len(links) == 0 {
			fmt.Println("No broken links found.")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(brokenCmd)
}
