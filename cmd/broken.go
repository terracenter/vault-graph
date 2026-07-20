package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var brokenCmd = &cobra.Command{
	Use:   "broken",
	Short: "Lista wikilinks no resueltos",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implementar en Fase 3
		fmt.Printf("broken: placeholder (Fase 3)\n")
		return nil
	},
}
