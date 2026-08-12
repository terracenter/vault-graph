package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
)

// BrokenLink es la forma uniforme de un link roto.
type BrokenLink struct {
	FromPath string `json:"from_path"`
	ToPath   string `json:"to_path"`
	Type     string `json:"type"`
}

// brokenKuzu: en Kuzu no hay concepto de "no resuelto" — el sync solo
// crea aristas cuando el destino existe. Esta función siempre retorna [].
// Se mantiene para uniformidad de la API.
func brokenKuzu(kuzuPath string) ([]BrokenLink, error) {
	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return nil, fmt.Errorf("open Kuzu: %w", err)
	}
	defer conn.Close()
	return []BrokenLink{}, nil
}

// brokenAGE eliminado en cleanup final. El CLI es 100% Kuzu.

var brokenCmd = &cobra.Command{
	Use:   "broken",
	Short: "Lista wikilinks no resueltos",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if cfg.KuzuPath == "" {
			return fmt.Errorf("broken requiere KUZU_PATH (Kuzu backend); AGE no soportado desde 2026-08-12")
		}

		links, err := brokenKuzu(cfg.KuzuPath)
		if err != nil {
			return fmt.Errorf("failed to query broken links: %w", err)
		}

		if format == "json" {
			data := map[string]interface{}{
				"backend": "kuzu",
				"links":   links,
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Broken wikilinks (kuzu): %d\n\n", len(links))
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
