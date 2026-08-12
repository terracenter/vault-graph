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
	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
	"github.com/freddytaborda/vault-graph/internal/ollama"
)

var (
	sampleSize int
	nodeType   string
)

// enrichPathsKuzu retorna los paths de nodos a enriquecer según flags:
// - --sample N: N paths al azar.
// - --type T: filter por tipo. Como Kuzu perdió granularidad de label
//   (todo es File), este flag busca en el nombre de la carpeta raíz
//   del path (e.g. "Plan" matchea paths bajo Planes/*, "Cliente"
//   matchea paths bajo 01_Clientes/*).
func enrichPathsKuzu(kuzuPath, nodeType string, sampleSize int) ([]string, error) {
	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return nil, fmt.Errorf("open Kuzu: %w", err)
	}
	defer conn.Close()

	if sampleSize > 0 {
		// Kuzu no tiene rand() — traemos todos los paths y elegimos N al azar en Go.
		var allPaths []string
		err = conn.Query("MATCH (n:File) RETURN n.path AS p",
			func(row map[string]any) bool {
				p, _ := row["p"].(string)
				if p != "" {
					allPaths = append(allPaths, p)
				}
				return true
			})
		if err != nil {
			return nil, fmt.Errorf("sample query: %w", err)
		}
		// Selección aleatoria uniforme sin reemplazo.
		if sampleSize >= len(allPaths) {
			return allPaths, nil
		}
		// Fisher-Yates parcial: los primeros N elementos del shuffle.
		idx := rand.Perm(len(allPaths))
		paths := make([]string, sampleSize)
		for i := 0; i < sampleSize; i++ {
			paths[i] = allPaths[idx[i]]
		}
		return paths, nil
	}

	// Filtro por "tipo" — convención: matchear la primera carpeta del path.
	// "Plan" → empieza con "Planes/", "Cliente" → "01_Clientes/", etc.
	prefix := typeToPrefix(nodeType)
	if prefix == "" {
		return nil, fmt.Errorf("tipo '%s' no reconocido (soportados: Plan, Manual, Wiki, Diario, Nota, Cliente, Servidor)", nodeType)
	}
	var paths []string
	err = conn.Query(
		fmt.Sprintf("MATCH (n:File) WHERE n.path STARTS WITH '%s' RETURN n.path AS p", escapeCypherString(prefix)),
		func(row map[string]any) bool {
			p, _ := row["p"].(string)
			if p != "" {
				paths = append(paths, p)
			}
			return true
		})
	if err != nil {
		return nil, fmt.Errorf("type query: %w", err)
	}
	return paths, nil
}

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

// enrichUpdateSummaryKuzu persiste el resumen LLM al nodo.
// Kuzu requiere MERGE/SET con property existente en schema — summary
// se setea vía SET n.summary = ... usando MERGE para crear el campo
// si no existe.
func enrichUpdateSummaryKuzu(kuzuPath, nodePath, summary string) error {
	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return fmt.Errorf("open Kuzu: %w", err)
	}
	defer conn.Close()

	q := fmt.Sprintf(
		"MATCH (n:File {path: '%s'}) SET n.summary = '%s'",
		escapeCypherString(nodePath), escapeCypherString(summary))
	_, err = conn.Execute(q)
	if err != nil {
		return fmt.Errorf("update summary: %w", err)
	}
	return nil
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

		if cfg.KuzuPath == "" {
			return fmt.Errorf("enrich requiere KUZU_PATH (Kuzu backend); AGE no soportado")
		}

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
		nodePaths, err := enrichPathsKuzu(cfg.KuzuPath, nodeType, sampleSize)
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

			if err := enrichUpdateSummaryKuzu(cfg.KuzuPath, result.Path, result.Summary); err != nil {
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
	rootCmd.AddCommand(enrichCmd)
}
