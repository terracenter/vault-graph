package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb"
)

var brokenCmd = &cobra.Command{
	Use:   "broken",
	Short: "Lista wikilinks no resueltos",
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

		links, err := conn.QueryBrokenLinks(ctx)
		if err != nil {
			return fmt.Errorf("failed to query broken links: %w", err)
		}

		// Formatear salida
		if format == "json" {
			b, _ := json.MarshalIndent(links, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Broken wikilinks: %d\n\n", len(links))
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
