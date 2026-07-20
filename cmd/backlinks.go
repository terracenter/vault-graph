package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var backlinksCmd = &cobra.Command{
	Use:   "backlinks <path>",
	Short: "Obtiene los wikilinks entrantes a una nota",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implementar en Fase 3
		fmt.Printf("backlinks: placeholder (Fase 3)\n")
		return nil
	},
}
