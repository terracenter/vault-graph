package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
)

// NodeStat es la forma uniforme de stats de nodos (compatible con Kuzu y AGE).
type NodeStat struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// ClientServerRelation es la forma uniforme de relaciones (compatible).
type ClientServerRelation struct {
	Cliente  string `json:"cliente"`
	Type     string `json:"type"`
	Servidor string `json:"servidor"`
}

// statsKuzu retorna los conteos por label (en Kuzu solo hay "File") y
// las relaciones de muestra (top 5).
func statsKuzu(kuzuPath string) ([]NodeStat, []ClientServerRelation, error) {
	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open Kuzu: %w", err)
	}
	defer conn.Close()

	// Conteo de nodos por label. En Kuzu solo tenemos label "File".
	var stats []NodeStat
	count := 0
	err = conn.Query("MATCH (n:File) RETURN count(*)",
		func(row map[string]any) bool {
			for _, v := range row {
				if c, ok := v.(int64); ok {
					count = int(c)
				}
			}
			return true
		})
	if err != nil {
		return nil, nil, fmt.Errorf("count nodos: %w", err)
	}
	stats = append(stats, NodeStat{Type: "File", Count: count})

	// Relaciones de muestra. En Kuzu, todas las aristas son ENLAZA,
	// así que devolvemos las primeras 5. Usamos AS para aliasar
	// columnas y que el map tenga keys estables.
	var relations []ClientServerRelation
	err = conn.Query(
		"MATCH (a:File)-[r:ENLAZA]->(b:File) RETURN a.path AS from_path, label(r) AS rel_type, b.path AS to_path LIMIT 5",
		func(row map[string]any) bool {
			a, _ := row["from_path"].(string)
			t, _ := row["rel_type"].(string)
			b, _ := row["to_path"].(string)
			relations = append(relations, ClientServerRelation{Cliente: a, Type: t, Servidor: b})
			return true
		})
	if err != nil {
		return nil, nil, fmt.Errorf("query relations: %w", err)
	}

	return stats, relations, nil
}

// statsAGE eliminado en cleanup final (commit actual). El CLI es 100% Kuzu.
//
// (La función se mantuvo como referencia histórica hasta confirmar que
// el backend Kuzu cubre todos los casos de uso.)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Muestra estadísticas del grafo",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if cfg.KuzuPath == "" {
			return fmt.Errorf("stats requiere KUZU_PATH (Kuzu backend); AGE no soportado desde 2026-08-12")
		}

		stats, relations, err := statsKuzu(cfg.KuzuPath)
		if err != nil {
			return fmt.Errorf("failed to query stats: %w", err)
		}

		if format == "json" {
			data := map[string]interface{}{
				"backend":       "kuzu",
				"node_stats":    stats,
				"client_server": relations,
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Println("=== Node Statistics (kuzu) ===")
			totalNodes := 0
			for _, stat := range stats {
				fmt.Printf("  %-12s: %4d\n", stat.Type, stat.Count)
				totalNodes += stat.Count
			}
			fmt.Printf("  %-12s: %4d\n", "TOTAL", totalNodes)

			fmt.Println("\n=== Sample Cliente↔Servidor Relations ===")
			if len(relations) == 0 {
				fmt.Println("  (no relations found)")
			} else {
				for _, rel := range relations {
					fmt.Printf("  %s --[%s]--> %s\n", rel.Cliente, rel.Type, rel.Servidor)
				}
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
