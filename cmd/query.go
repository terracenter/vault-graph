package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query \"<cypher crudo>\"",
	Short: "Ejecuta una query Cypher read-only",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implementar en Fase 3
		fmt.Printf("query: placeholder (Fase 3)\n")
		return nil
	},
}
