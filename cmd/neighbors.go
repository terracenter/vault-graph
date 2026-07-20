package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var neighborsCmd = &cobra.Command{
	Use:   "neighbors <path> [--hops N]",
	Short: "Obtiene los nodos vecinos de una nota",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implementar en Fase 3
		fmt.Printf("neighbors: placeholder (Fase 3)\n")
		return nil
	},
}
