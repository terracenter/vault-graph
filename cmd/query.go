package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
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

// queryAGE eliminado en cleanup final. El CLI es 100% Kuzu.

var queryCmd = &cobra.Command{
	Use:   "query \"<cypher crudo>\"",
	Short: "Ejecuta una query Cypher read-only contra Kuzu",
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

		if cfg.KuzuPath == "" {
			return fmt.Errorf("query requiere KUZU_PATH (Kuzu backend); AGE no soportado desde 2026-08-12")
		}

		if _, err := os.Stat(cfg.KuzuPath); err != nil {
			return fmt.Errorf("Kuzu file not accessible: %w", err)
		}

		results, err := queryKuzu(cfg.KuzuPath, cypher)
		if err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}

		if format == "json" {
			output := map[string]interface{}{
				"query":   cypher,
				"results": results,
				"backend": "kuzu",
			}
			b, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Query results (kuzu): %d rows\n\n", len(results))
			for i, row := range results {
				fmt.Printf("  Row %d: %v\n", i+1, row)
			}
		}

		return nil
	},
}
