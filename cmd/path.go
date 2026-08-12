package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
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

// shortestPathAGE consulta AGE con shortestPath.
func shortestPathAGE(dbURL, from, to string) (PathResult, error) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		return PathResult{}, fmt.Errorf("connect AGE: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `LOAD 'age'`); err != nil {
		return PathResult{}, fmt.Errorf("LOAD age: %w", err)
	}
	if _, err := conn.Exec(ctx, `SET search_path = ag_catalog, public`); err != nil {
		return PathResult{}, fmt.Errorf("SET search_path: %w", err)
	}

	rows, err := conn.Query(ctx, fmt.Sprintf(`
		SELECT * FROM cypher('vault', $$
		  MATCH p = shortestPath((a {path: '%s'})-[*..15]-(b {path: '%s'}))
		  RETURN length(p) AS hops, nodes(p) AS ns
		$$) AS (hops agtype, ns agtype)
	`, escapeCypherString(from), escapeCypherString(to)))
	if err != nil {
		return PathResult{}, fmt.Errorf("query AGE: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return PathResult{Found: false}, nil
	}
	var hops int
	var nsRaw string
	if err := rows.Scan(&hops, &nsRaw); err != nil {
		return PathResult{}, fmt.Errorf("scan: %w", err)
	}
	// nodes(p) en AGE es complejo de parsear (json). Por simplicidad
	// retornamos solo el conteo de hops y los paths como string.
	// Para análisis detallado, usar 'query' directamente.
	nodes := []PathNode{
		{Type: "?", Path: fmt.Sprintf("path from %s to %s (%d hops, see query for details)", from, to, hops)},
	}
	return PathResult{Found: true, Hops: hops, Nodes: nodes}, nil
}

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

		var path PathResult
		if cfg.KuzuPath != "" {
			path, err = shortestPathKuzu(cfg.KuzuPath, from, to)
		} else {
			path, err = shortestPathAGE(cfg.DatabaseURL, from, to)
		}
		if err != nil {
			return fmt.Errorf("failed to query shortest path: %w", err)
		}

		backend := backendName(cfg)
		if format == "json" {
			data := map[string]interface{}{
				"backend": backend,
				"path":    path,
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
		} else {
			if path.Found {
				fmt.Printf("Shortest path from %s to %s (%s, %d hops):\n\n", from, to, backend, path.Hops)
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
