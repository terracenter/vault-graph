package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb/factory"
)


// orphansAGE eliminado en cleanup final. El CLI es 100% Kuzu.

var orphansCmd = &cobra.Command{
	Use:   "orphans",
	Short: "Lista notas sin conexiones (sin ENLAZA entrante ni saliente)",
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

		orphans, err := store.OrphanNodes(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to query orphan nodes: %w", err)
		}

		if format == "json" {
			data := map[string]interface{}{
				"backend": cfg.Backend,
				"orphans": orphans,
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Orphan notes (kuzu, sin ENLAZA): %d\n\n", len(orphans))
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
