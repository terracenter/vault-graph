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

// backlinksAGE consulta AGE para wikilinks entrantes.
func backlinksAGE(dbURL, path string) ([]BacklinkResult, error) {
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

	rows, err := conn.Query(ctx, fmt.Sprintf(`
		SELECT * FROM cypher('vault', $$
		  MATCH (a)-[r]->(b {path: '%s'})
		  RETURN labels(a)[0] AS from_type, type(r) AS rel_type, a.path AS from_path
		$$) AS (from_type agtype, rel_type agtype, from_path agtype)
	`, escapeCypherString(path)))
	if err != nil {
		return nil, fmt.Errorf("query AGE: %w", err)
	}
	defer rows.Close()

	var results []BacklinkResult
	for rows.Next() {
		var t, rel, p string
		if err := rows.Scan(&t, &rel, &p); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		results = append(results, BacklinkResult{Type: t, Path: p, Rel: rel})
	}
	return results, rows.Err()
}

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

		var backlinks []BacklinkResult
		if cfg.KuzuPath != "" {
			backlinks, err = backlinksKuzu(cfg.KuzuPath, path)
		} else {
			backlinks, err = backlinksAGE(cfg.DatabaseURL, path)
		}
		if err != nil {
			return fmt.Errorf("failed to query backlinks: %w", err)
		}

		backend := backendName(cfg)
		if format == "json" {
			output := map[string]interface{}{
				"backend":   backend,
				"path":      path,
				"backlinks": backlinks,
			}
			b, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Backlinks to %s (%s): %d found\n\n", path, backend, len(backlinks))
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
