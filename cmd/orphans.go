package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var orphansCmd = &cobra.Command{
	Use:   "orphans",
	Short: "Lista notas sin conexiones (sin ENLAZA entrante ni saliente)",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implementar en Fase 3
		fmt.Printf("orphans: placeholder (Fase 3)\n")
		return nil
	},
}
