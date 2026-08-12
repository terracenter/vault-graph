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

// orphansAGE retorna los huérfanos desde AGE.
func orphansAGE(dbURL string) ([]OrphanNode, error) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("connect AGE: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `LOAD 'age'`); err != nil {
		return nil, fmt.Errorf("LOAD age: %w", err)
	}
	if _, err := conn.Exec(ctx, `SET search_path = ag_catalog, public`); err != nil {
		return nil, fmt.Errorf("SET search_path: %w", err)
	}

	rows, err := conn.Query(ctx, `
		SELECT * FROM cypher('vault', $$
		  MATCH (n)
		  WHERE NOT EXISTS { MATCH (n)-[]->() }
		    AND NOT EXISTS { MATCH ()-[]->(n) }
		  RETURN labels(n)[0] AS t, n.path AS p, n.titulo AS tit
		$$) AS (t agtype, p agtype, tit agtype)
	`)
	if err != nil {
		return nil, fmt.Errorf("query AGE: %w", err)
	}
	defer rows.Close()

	var results []OrphanNode
	for rows.Next() {
		var t, p, tit string
		if err := rows.Scan(&t, &p, &tit); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		results = append(results, OrphanNode{Type: t, Path: p, Titulo: tit})
	}
	return results, rows.Err()
}

var orphansCmd = &cobra.Command{
	Use:   "orphans",
	Short: "Lista notas sin conexiones (sin ENLAZA entrante ni saliente)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		var orphans []OrphanNode
		if cfg.KuzuPath != "" {
			orphans, err = orphansKuzu(cfg.KuzuPath)
		} else {
			orphans, err = orphansAGE(cfg.DatabaseURL)
		}
		if err != nil {
			return fmt.Errorf("failed to query orphan nodes: %w", err)
		}

		backend := backendName(cfg)
		if format == "json" {
			data := map[string]interface{}{
				"backend": backend,
				"orphans": orphans,
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Orphan notes (%s, sin ENLAZA): %d\n\n", backend, len(orphans))
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
