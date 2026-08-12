package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
)

// PathNode es un nodo en el camino más corto.
type PathNode struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// PathResult contiene el resultado de QueryShortestPath.
type PathResult struct {
	Found bool       `json:"found"`
	Hops  int        `json:"hops"`
	Nodes []PathNode `json:"nodes"`
}

// shortestPathKuzu implementa BFS iterativo en Kuzu (no soporta
// SHORESTEST de openCypher de forma eficiente para paths variables).
func shortestPathKuzu(kuzuPath, from, to string) (PathResult, error) {
	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return PathResult{}, fmt.Errorf("open Kuzu: %w", err)
	}
	defer conn.Close()

	visited := make(map[string]bool)
	visited[from] = true
	parent := make(map[string]string) // child -> parent
	queue := []string{from}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == to {
			// reconstruir camino
			path := []string{}
			n := to
			for n != "" {
				path = append([]string{n}, path...)
				p, ok := parent[n]
				if !ok {
					break
				}
				n = p
			}
			nodes := make([]PathNode, len(path))
			for i, p := range path {
				nodes[i] = PathNode{Type: "File", Path: p}
			}
			return PathResult{Found: true, Hops: len(path) - 1, Nodes: nodes}, nil
		}

		escaped := escapeCypherString(current)
		err = conn.Query(
			"MATCH (a:File {path: '"+escaped+"'})-[r]->(b:File) RETURN b.path AS to_path",
			func(row map[string]any) bool {
				next, _ := row["to_path"].(string)
				if !visited[next] {
					visited[next] = true
					parent[next] = current
					queue = append(queue, next)
				}
				return true
			})
		if err != nil {
			return PathResult{}, fmt.Errorf("query BFS: %w", err)
		}
	}
	return PathResult{Found: false}, nil
}

// shortestPathAGE eliminado en cleanup final. El CLI es 100% Kuzu.

var pathCmd = &cobra.Command{
	Use:   "path <path-a> <path-b>",
	Short: "Encuentra el camino más corto entre dos notas",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		from := args[0]
		to := args[1]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if cfg.KuzuPath == "" {
			return fmt.Errorf("path requiere KUZU_PATH (Kuzu backend); AGE no soportado desde 2026-08-12")
		}

		path, err := shortestPathKuzu(cfg.KuzuPath, from, to)
		if err != nil {
			return fmt.Errorf("failed to query shortest path: %w", err)
		}

		if format == "json" {
			data := map[string]interface{}{
				"backend": "kuzu",
				"path":    path,
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
		} else {
			if path.Found {
				fmt.Printf("Shortest path from %s to %s (kuzu, %d hops):\n\n", from, to, path.Hops)
				for i, node := range path.Nodes {
					fmt.Printf("  %d. [%s] %s\n", i, node.Type, node.Path)
				}
			} else {
				fmt.Printf("No path found between %s and %s.\n", from, to)
			}
		}

		return nil
	},
}
