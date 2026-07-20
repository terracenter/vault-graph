package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb"
)

var orphansCmd = &cobra.Command{
	Use:   "orphans",
	Short: "Lista notas sin conexiones (sin ENLAZA entrante ni saliente)",
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

		orphans, err := conn.QueryOrphanNodes(ctx)
		if err != nil {
			return fmt.Errorf("failed to query orphan nodes: %w", err)
		}

		// Formatear salida
		if format == "json" {
			b, _ := json.MarshalIndent(orphans, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Orphan notes (sin ENLAZA): %d\n\n", len(orphans))
			for _, orphan := range orphans {
				if orphan.Titulo != "" {
					fmt.Printf("  [%s] %s — %s\n", orphan.Type, orphan.Path, orphan.Titulo)
				} else {
					fmt.Printf("  [%s] %s\n", orphan.Type, orphan.Path)
				}
			}
		}

		if len(orphans) == 0 {
			fmt.Println("No orphan notes found.")
		}

		return nil
	},
}
