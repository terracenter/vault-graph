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

// brokenAGE consulta los wikilinks no resueltos.
func brokenAGE(dbURL string) ([]BrokenLink, error) {
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
		  MATCH (a)-[r]->(b)
		  WHERE b.path IS NULL
		  RETURN a.path, b.path, type(r)
		$$) AS (from_path agtype, to_path agtype, rel_type agtype)
	`)
	if err != nil {
		return nil, fmt.Errorf("query AGE: %w", err)
	}
	defer rows.Close()

	var results []BrokenLink
	for rows.Next() {
		var f, t, ty string
		if err := rows.Scan(&f, &t, &ty); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		results = append(results, BrokenLink{FromPath: f, ToPath: t, Type: ty})
	}
	return results, rows.Err()
}

var brokenCmd = &cobra.Command{
	Use:   "broken",
	Short: "Lista wikilinks no resueltos",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		var links []BrokenLink
		if cfg.KuzuPath != "" {
			links, err = brokenKuzu(cfg.KuzuPath)
		} else {
			links, err = brokenAGE(cfg.DatabaseURL)
		}
		if err != nil {
			return fmt.Errorf("failed to query broken links: %w", err)
		}

		backend := backendName(cfg)
		if format == "json" {
			data := map[string]interface{}{
				"backend": backend,
				"links":   links,
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Broken wikilinks (%s): %d\n\n", backend, len(links))
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
