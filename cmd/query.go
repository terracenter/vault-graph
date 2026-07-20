package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb"
)

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

		ctx := context.Background()
		conn, err := graphdb.NewConn(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
		defer conn.Close()

		// Ejecutar query raw
		results, err := conn.QueryRaw(ctx, cypher)
		if err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}

		// Formatear salida
		if format == "json" {
			output := map[string]interface{}{
				"query":   cypher,
				"results": results,
			}
			b, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Query results: %d rows\n\n", len(results))
			for i, row := range results {
				fmt.Printf("  Row %d: %v\n", i+1, row)
			}
		}

		return nil
	},
}
