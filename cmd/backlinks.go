package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
)

// BacklinkResult es la forma uniforme de un backlink.
type BacklinkResult struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Rel  string `json:"rel"`
}

// backlinksKuzu retorna los nodos que apuntan al path dado.
// Kuzu no soporta 'INCOMING' como en Cypher moderno; usamos dirección inversa.
func backlinksKuzu(kuzuPath, path string) ([]BacklinkResult, error) {
	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return nil, fmt.Errorf("open Kuzu: %w", err)
	}
	defer conn.Close()

	var results []BacklinkResult
	err = conn.Query(
		"MATCH (a:File)-[r:ENLAZA]->(b:File {path: '"+escapeCypherString(path)+"'}) RETURN a.path AS from_path, label(r) AS rel_type",
		func(row map[string]any) bool {
			from, _ := row["from_path"].(string)
			rel, _ := row["rel_type"].(string)
			results = append(results, BacklinkResult{Type: "File", Path: from, Rel: rel})
			return true
		})
	if err != nil {
		return nil, fmt.Errorf("query Kuzu: %w", err)
	}
	return results, nil
}

// backlinksAGE eliminado en cleanup final (commit actual). El CLI es 100% Kuzu.

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

		if cfg.KuzuPath == "" {
			return fmt.Errorf("backlinks requiere KUZU_PATH (Kuzu backend); AGE no soportado desde 2026-08-12")
		}

		backlinks, err := backlinksKuzu(cfg.KuzuPath, path)
		if err != nil {
			return fmt.Errorf("failed to query backlinks: %w", err)
		}

		if format == "json" {
			output := map[string]interface{}{
				"backend":   "kuzu",
				"path":      path,
				"backlinks": backlinks,
			}
			b, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Backlinks to %s (kuzu): %d found\n\n", path, len(backlinks))
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
