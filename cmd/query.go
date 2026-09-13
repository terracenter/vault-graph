package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb/factory"
	"github.com/spf13/cobra"
)

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

		store, err := factory.NewStore(cmd.Context(), cfg)
		if err != nil {
			return fmt.Errorf("failed to create store: %w", err)
		}
		defer store.Close()

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
				"backend": cfg.Backend,
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
