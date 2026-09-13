package cmd

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb"
	"github.com/freddytaborda/vault-graph/internal/graphdb/factory"
	"github.com/freddytaborda/vault-graph/internal/ollama"
)

var (
	sampleSize int
	nodeType   string
)

// typeToPrefix mapea un nombre de tipo a prefijo de carpeta del vault.
// Mantiene paridad con las labels de AGE (Manual/Plan/Wiki/etc.) usando
// la convención de nombres de carpetas existente en el vault.
func typeToPrefix(t string) string {
	switch strings.ToLower(t) {
	case "plan", "planes":
		return "Planes/"
	case "manual", "manuales":
		return "03_Manuales_Borradores/"
	case "wiki":
		return "04_Wiki/"
	case "diario", "diarios":
		return "05_Diario/"
	case "nota", "notas":
		return "06_Desarrollo/"
	case "cliente", "clientes":
		return "01_Clientes/"
	case "servidor", "servidores":
		return "02_Servidores/"
	}
	return ""
}


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

		store, err := factory.NewStore(cmd.Context(), cfg)
		if err != nil {
			return fmt.Errorf("failed to create store: %w", err)
		}
		defer store.Close()

		// Verificar que Ollama URL está configurado
		if cfg.OllamaURL == "" {
			cfg.OllamaURL = "http://100.76.175.78:11434" // Fallback al minipc
		}

		// Crear cliente Ollama y verificar conectividad
		fmt.Printf("Verificando conectividad a Ollama en %s...\n", cfg.OllamaURL)
		ollamaClient := ollama.NewClient(cfg.OllamaURL, "qwen2.5-coder:7b-instruct-q4_K_M", 3)
		ctx := context.Background()
		if err := ollamaClient.CheckHealth(ctx); err != nil {
			return fmt.Errorf("failed to connect to Ollama: %w", err)
		}
		fmt.Println("✓ Conectado a Ollama")

		// Obtener lista de paths según flags.
		var nodePaths []string
		if sampleSize > 0 {
			allPaths, err := store.ListPaths(cmd.Context(), graphdb.PathFilter{})
			if err != nil {
				return fmt.Errorf("list paths failed: %w", err)
			}
			if sampleSize >= len(allPaths) {
				nodePaths = allPaths
			} else {
				idx := rand.Perm(len(allPaths))
				for i := 0; i < sampleSize; i++ {
					nodePaths = append(nodePaths, allPaths[idx[i]])
				}
			}
		} else if nodeType != "" {
			allPaths, err := store.ListPaths(cmd.Context(), graphdb.PathFilter{})
			if err != nil {
				return fmt.Errorf("list paths failed: %w", err)
			}
			prefix := typeToPrefix(nodeType)
			if prefix == "" {
				return fmt.Errorf("tipo '%s' no reconocido", nodeType)
			}
			for _, p := range allPaths {
				if strings.HasPrefix(p, prefix) {
					nodePaths = append(nodePaths, p)
				}
			}
		} else {
			allPaths, err := store.ListPaths(cmd.Context(), graphdb.PathFilter{})
			if err != nil {
				return fmt.Errorf("list paths failed: %w", err)
			}
			nodePaths = allPaths
		}
		if err != nil {
			return fmt.Errorf("failed to query paths: %w", err)
		}
		if len(nodePaths) == 0 {
			fmt.Println("No nodes found matching criteria")
			return nil
		}
		fmt.Printf("Procesando %d nodos...\n\n", len(nodePaths))

		// Preparar items para batch enrich.
		items := make([]struct{ Path, Content string }, len(nodePaths))
		for i, p := range nodePaths {
			filePath := filepath.Join(cfg.VaultPath, p)
			content, err := os.ReadFile(filePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠ Error reading %s: %v\n", p, err)
				items[i] = struct{ Path, Content string }{Path: p, Content: ""}
				continue
			}
			items[i] = struct{ Path, Content string }{Path: p, Content: string(content)}
		}

		// Ejecutar batch en paralelo.
		results := ollamaClient.EnrichBatch(ctx, items)

		// Procesar resultados y guardar en Kuzu.
		successCount := 0
		skipCount := 0
		failCount := 0

		for _, result := range results {
			if !result.Success {
				skipCount++
				fmt.Fprintf(os.Stderr, "⊘ Skipped %s: %v\n", result.Path, result.Error)
				continue
			}

			if err := store.UpdateSummary(cmd.Context(), result.Path, result.Summary); err != nil {
				failCount++
				fmt.Fprintf(os.Stderr, "✗ Failed to save %s: %v\n", result.Path, err)
				continue
			}

			successCount++
			summary := strings.TrimSpace(result.Summary)
			if len(summary) > 80 {
				summary = summary[:77] + "..."
			}
			fmt.Printf("✓ %s\n  → %s\n", result.Path, summary)
		}

		fmt.Printf("\n---\n")
		fmt.Printf("Resultados: %d exitosos, %d skipped, %d fallidos\n",
			successCount, skipCount, failCount)

		return nil
	},
}

func init() {
	enrichCmd.Flags().IntVar(&sampleSize, "sample", 0, "Número de nodos al azar a enriquecer")
	enrichCmd.Flags().StringVar(&nodeType, "type", "", "Tipo de nodo a enriquecer (ej. Cliente, Servidor, Plan, Manual, Wiki, Diario, Nota)")
}
