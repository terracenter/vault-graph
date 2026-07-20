package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	format  string // json|table (default table)
)

// rootCmd representa el comando base
var rootCmd = &cobra.Command{
	Use:   "vault-graph",
	Short: "ETL + CLI para el grafo del vault en PostgreSQL 18 + Apache AGE",
	Long:  "Carga nodos y aristas del vault Obsidian en un grafo AGE consultable.",
}

// Execute ejecuta el comando root
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "ruta al archivo de configuración .env")
	rootCmd.PersistentFlags().StringVar(&format, "format", "table", "formato de salida: json|table")

	// Agregar subcomandos
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(neighborsCmd)
	rootCmd.AddCommand(backlinksCmd)
	rootCmd.AddCommand(pathCmd)
	rootCmd.AddCommand(orphansCmd)
	rootCmd.AddCommand(brokenCmd)
	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(enrichCmd)
}
