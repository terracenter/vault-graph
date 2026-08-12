package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
	"github.com/jackc/pgx/v5"
)

// queryKuzu ejecuta una query read-only contra Kuzu y retorna las filas.
func queryKuzu(kuzuPath, cypher string) ([]map[string]any, error) {
	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return nil, fmt.Errorf("open Kuzu: %w", err)
	}
	defer conn.Close()

	var rows []map[string]any
	err = conn.Query(cypher, func(row map[string]any) bool {
		cp := make(map[string]any, len(row))
		for k, v := range row {
			cp[k] = v
		}
		rows = append(rows, cp)
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("query Kuzu: %w", err)
	}
	return rows, nil
}

// queryAGE ejecuta una query read-only contra AGE y retorna las filas.
func queryAGE(dbURL, cypher string) ([]map[string]any, error) {
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

	rows, err := conn.Query(ctx, "SELECT * FROM cypher('vault', $$ "+cypher+" $$) AS (result agtype)")
	if err != nil {
		return nil, fmt.Errorf("query AGE: %w", err)
	}
	defer rows.Close()

	// cypher() retorna una sola columna "result" de tipo agtype (json-ish).
	// Parseamos y devolvemos como map[string]any para uniformidad con Kuzu.
	var results []map[string]any
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		// Devolver como un map con la key "result" para mantener shape uniforme.
		results = append(results, map[string]any{"result": raw})
	}
	return results, rows.Err()
}

var queryCmd = &cobra.Command{
	Use:   "query \"<cypher crudo>\"",
	Short: "Ejecuta una query Cypher read-only",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cypher := args[0]

		// Validar que NO contenga palabras clave de mutación
		mutationKeywords := []string{"MERGE", "CREATE", "DELETE", "SET", "REMOVE"}
		cypherUpper := strings.ToUpper(cypher)
		for _, kw := range mutationKeywords {
			if strings.Contains(cypherUpper, kw) {
				return fmt.Errorf("query contains forbidden keyword '%s' — only read-only queries allowed", kw)
			}
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Decidir backend: Kuzu si está configurado, sino AGE.
		var results []map[string]any
		if cfg.KuzuPath != "" {
			if _, err := os.Stat(cfg.KuzuPath); err != nil {
				return fmt.Errorf("Kuzu file not accessible: %w", err)
			}
			results, err = queryKuzu(cfg.KuzuPath, cypher)
		} else {
			results, err = queryAGE(cfg.DatabaseURL, cypher)
		}
		if err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}

		// Formatear salida
		if format == "json" {
			output := map[string]interface{}{
				"query":   cypher,
				"results": results,
				"backend": backendName(cfg),
			}
			b, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(b))
		} else {
			backend := backendName(cfg)
			fmt.Printf("Query results (%s): %d rows\n\n", backend, len(results))
			for i, row := range results {
				fmt.Printf("  Row %d: %v\n", i+1, row)
			}
		}

		return nil
	},
}

// backendName retorna el nombre del backend activo para diagnóstico.
func backendName(cfg *config.Config) string {
	if cfg.KuzuPath != "" {
		return "kuzu"
	}
	return "age"
}
