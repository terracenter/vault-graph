package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb"
	"github.com/freddytaborda/vault-graph/internal/ollama"
)

var (
	sampleSize int
	nodeType   string
)

var enrichCmd = &cobra.Command{
	Use:   "enrich [--sample N | --type TYPE]",
	Short: "Enriquece nodos con resúmenes LLM (Ollama)",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Validar flags
		if sampleSize == 0 && nodeType == "" {
			return fmt.Errorf("debe especificar --sample N o --type TYPE")
		}
		if sampleSize > 0 && nodeType != "" {
			return fmt.Errorf("no puede usar --sample y --type simultáneamente")
		}

		// Cargar config
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Conectar a PostgreSQL
		ctx := context.Background()
		conn, err := graphdb.NewConn(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
		defer conn.Close()

		// Verificar que Ollama URL está configurado
		if cfg.OllamaURL == "" {
			cfg.OllamaURL = "http://100.76.175.78:11434" // Fallback al minipc
		}

		// Crear cliente Ollama y verificar conectividad
		fmt.Printf("Verificando conectividad a Ollama en %s...\n", cfg.OllamaURL)
		ollamaClient := ollama.NewClient(cfg.OllamaURL, "qwen2.5-coder:7b-instruct-q4_K_M", 3)
		if err := ollamaClient.CheckHealth(ctx); err != nil {
			return fmt.Errorf("failed to connect to Ollama: %w", err)
		}
		fmt.Println("✓ Conectado a Ollama")

		// Obtener lista de nodos según criterios
		var nodePaths []graphdb.NodePath
		if sampleSize > 0 {
			fmt.Printf("Obteniendo muestra de %d nodos al azar...\n", sampleSize)
			nodePaths, err = conn.QueryNodesSample(ctx, sampleSize)
		} else {
			fmt.Printf("Obteniendo nodos de tipo %s...\n", nodeType)
			nodePaths, err = conn.QueryNodesByType(ctx, nodeType)
		}

		if err != nil {
			return fmt.Errorf("failed to query nodes: %w", err)
		}

		if len(nodePaths) == 0 {
			fmt.Println("No nodes found matching criteria")
			return nil
		}

		fmt.Printf("Procesando %d nodos...\n\n", len(nodePaths))

		// Preparar items para batch enrich
		items := make([]struct{ Path, Content string }, len(nodePaths))
		for i, np := range nodePaths {
			filePath := filepath.Join(cfg.VaultPath, np.Path)
			content, err := os.ReadFile(filePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠ Error reading %s: %v\n", np.Path, err)
				items[i] = struct{ Path, Content string }{Path: np.Path, Content: ""}
				continue
			}
			items[i] = struct{ Path, Content string }{Path: np.Path, Content: string(content)}
		}

		// Ejecutar batch en paralelo
		results := ollamaClient.EnrichBatch(ctx, items)

		// Procesar resultados y guardar en DB
		successCount := 0
		skipCount := 0
		failCount := 0

		for _, result := range results {
			if !result.Success {
				skipCount++
				fmt.Fprintf(os.Stderr, "⊘ Skipped %s: %v\n", result.Path, result.Error)
				continue
			}

			// Guardar resumen en DB
			if err := conn.UpdateNodeSummary(ctx, result.Path, result.Summary); err != nil {
				failCount++
				fmt.Fprintf(os.Stderr, "✗ Failed to save %s: %v\n", result.Path, err)
				continue
			}

			successCount++
			// Imprimir resumen (truncado a 80 caracteres)
			summary := strings.TrimSpace(result.Summary)
			if len(summary) > 80 {
				summary = summary[:77] + "..."
			}
			fmt.Printf("✓ %s\n  → %s\n", result.Path, summary)
		}

		// Resumen final
		fmt.Printf("\n---\n")
		fmt.Printf("Resultados: %d exitosos, %d skipped, %d fallidos\n",
			successCount, skipCount, failCount)

		return nil
	},
}

func init() {
	enrichCmd.Flags().IntVar(&sampleSize, "sample", 0, "Número de nodos al azar a enriquecer")
	enrichCmd.Flags().StringVar(&nodeType, "type", "", "Tipo de nodo a enriquecer (ej. Cliente, Servidor)")
	rootCmd.AddCommand(enrichCmd)
}
