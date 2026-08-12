package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
)

// OrphanNode es la forma uniforme de un huérfano.
type OrphanNode struct {
	Type   string `json:"type"`
	Path   string `json:"path"`
	Titulo string `json:"titulo"`
}

// orphansKuzu retorna los nodos sin aristas ENLAZA entrantes ni salientes.
// En Kuzu todos los nodos tienen label "File" (sin distinción Manual/Plan/etc).
func orphansKuzu(kuzuPath string) ([]OrphanNode, error) {
	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return nil, fmt.Errorf("open Kuzu: %w", err)
	}
	defer conn.Close()

	var results []OrphanNode
	err = conn.Query(`
		MATCH (n:File)
		WHERE NOT EXISTS { MATCH (n)-[]->() }
		  AND NOT EXISTS { MATCH ()-[]->(n) }
		RETURN n.path AS p`,
		func(row map[string]any) bool {
			p, _ := row["p"].(string)
			results = append(results, OrphanNode{Type: "File", Path: p, Titulo: ""})
			return true
		})
	if err != nil {
		return nil, fmt.Errorf("query Kuzu: %w", err)
	}
	return results, nil
}

// orphansAGE eliminado en cleanup final. El CLI es 100% Kuzu.

var orphansCmd = &cobra.Command{
	Use:   "orphans",
	Short: "Lista notas sin conexiones (sin ENLAZA entrante ni saliente)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if cfg.KuzuPath == "" {
			return fmt.Errorf("orphans requiere KUZU_PATH (Kuzu backend); AGE no soportado desde 2026-08-12")
		}

		orphans, err := orphansKuzu(cfg.KuzuPath)
		if err != nil {
			return fmt.Errorf("failed to query orphan nodes: %w", err)
		}

		if format == "json" {
			data := map[string]interface{}{
				"backend": "kuzu",
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
