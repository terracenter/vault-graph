package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var enrichCmd = &cobra.Command{
	Use:   "enrich [--sample N|--type X]",
	Short: "Enriquece nodos con resúmenes LLM (Ollama)",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implementar en Fase 4
		fmt.Printf("enrich: placeholder (Fase 4)\n")
		return nil
	},
}
