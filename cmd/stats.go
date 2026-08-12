package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
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

// statsAGE consulta AGE directamente con Cypher (vía pgx, sin el wrapper
// graphdb para evitar dependencia circular).
func statsAGE(dbURL string) ([]NodeStat, []ClientServerRelation, error) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect AGE: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `LOAD 'age'`); err != nil {
		return nil, nil, fmt.Errorf("LOAD age: %w", err)
	}
	if _, err := conn.Exec(ctx, `SET search_path = ag_catalog, public`); err != nil {
		return nil, nil, fmt.Errorf("SET search_path: %w", err)
	}

	// Conteo de nodos por label. Usamos el mismo patrón que en graphdb/queries.go.
	var stats []NodeStat
	rows, err := conn.Query(ctx, `
		SELECT * FROM cypher('vault', $$
		  MATCH (n)
		  RETURN labels(n)[0] AS l
		$$) AS (l agtype)
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("query labels: %w", err)
	}
	defer rows.Close()
	dist := map[string]int{}
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, nil, fmt.Errorf("scan: %w", err)
		}
		dist[l]++
	}
	for t, c := range dist {
		stats = append(stats, NodeStat{Type: t, Count: c})
	}

	// Relaciones Cliente-Servidor.
	var relations []ClientServerRelation
	rows, err = conn.Query(ctx, `
		SELECT * FROM cypher('vault', $$
		  MATCH (c)-[r]->(s)
		  RETURN c.path, type(r), s.path
		  LIMIT 5
		$$) AS (c_path agtype, rel_type agtype, s_path agtype)
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("query relations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c, t, s string
		if err := rows.Scan(&c, &t, &s); err != nil {
			return nil, nil, fmt.Errorf("scan rel: %w", err)
		}
		relations = append(relations, ClientServerRelation{Cliente: c, Type: t, Servidor: s})
	}

	return stats, relations, nil
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Muestra estadísticas del grafo",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Decidir backend: Kuzu si está configurado, sino AGE.
		var stats []NodeStat
		var relations []ClientServerRelation
		if cfg.KuzuPath != "" {
			stats, relations, err = statsKuzu(cfg.KuzuPath)
		} else {
			stats, relations, err = statsAGE(cfg.DatabaseURL)
		}
		if err != nil {
			return fmt.Errorf("failed to query stats: %w", err)
		}

		backend := backendName(cfg)
		if format == "json" {
			data := map[string]interface{}{
				"backend":          backend,
				"node_stats":       stats,
				"client_server":    relations,
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("=== Node Statistics (%s) ===\n", backend)
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
