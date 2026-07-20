package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pathCmd = &cobra.Command{
	Use:   "path <path-a> <path-b>",
	Short: "Encuentra el camino más corto entre dos notas",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implementar en Fase 3
		fmt.Printf("path: placeholder (Fase 3)\n")
		return nil
	},
}
