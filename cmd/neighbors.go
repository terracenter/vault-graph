package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/freddytaborda/vault-graph/config"
	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
)

var hops int

// escapeCypherString escapa backslash y comilla simple para uso en
// strings Cypher. Helper local para cmd/neighbors.go.
func escapeCypherString(s string) string {
	out := ""
	for _, c := range s {
		switch c {
		case '\\':
			out += "\\\\"
		case '\'':
			out += "\\'"
		default:
			out += string(c)
		}
	}
	return out
}

// Neighbor es la forma uniforme de un vecino (compatible con Kuzu y AGE).
type Neighbor struct {
	Type string `json:"type"` // label del nodo origen
	Path string `json:"path"` // path del nodo vecino
	Rel  string `json:"rel"`  // label de la arista
}

// neighborsKuzu retorna los nodos vecinos de un path hasta N hops.
// Como Kuzu embebido no soporta depths variables en una sola query,
// iteramos hops veces con un conjunto acumulado de paths visitados.
func neighborsKuzu(kuzuPath, path string, nhops int) ([]Neighbor, error) {
	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return nil, fmt.Errorf("open Kuzu: %w", err)
	}
	defer conn.Close()

	visited := make(map[string]bool)
	visited[path] = true
	current := []string{path}
	var results []Neighbor

	for hop := 0; hop < nhops; hop++ {
		next := []string{}
		for _, p := range current {
			escaped := escapeCypherString(p)
			err := conn.Query(
				"MATCH (a:File {path: '"+escaped+"'})-[r]->(b:File) RETURN a.path AS from_path, label(r) AS rel_type, b.path AS to_path",
				func(row map[string]any) bool {
					rel, _ := row["rel_type"].(string)
					to, _ := row["to_path"].(string)
					if !visited[to] {
						results = append(results, Neighbor{Type: "File", Path: to, Rel: rel})
						visited[to] = true
						next = append(next, to)
					}
					return true
				})
			if err != nil {
				return nil, fmt.Errorf("query hop %d: %w", hop, err)
			}
		}
		if len(next) == 0 {
			break
		}
		current = next
	}
	return results, nil
}

// neighborsAGE queda como referencia histórica (legacy, no usado en runtime).
// Se mantiene el código en este archivo pero la rama que lo invoca fue
// eliminada del CLI. Para 2026-08-12+ el CLI es 100% Kuzu.
//
// Keeping the function here as dead code would require maintaining unused
// imports (pgx) and bloat the binary. Lo eliminamos por completo.
//
// (Código AGE removido en commit 7d956f4 → limpieza final en commit actual)

var neighborsCmd = &cobra.Command{
	Use:   "neighbors <path> [--hops N]",
	Short: "Obtiene los nodos vecinos de una nota",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if cfg.KuzuPath == "" {
			return fmt.Errorf("neighbors requiere KUZU_PATH (Kuzu backend); AGE no soportado desde 2026-08-12")
		}

		neighbors, err := neighborsKuzu(cfg.KuzuPath, path, hops)
		if err != nil {
			return fmt.Errorf("failed to query neighbors: %w", err)
		}

		if format == "json" {
			output := map[string]interface{}{
				"backend":   "kuzu",
				"path":      path,
				"hops":      hops,
				"neighbors": neighbors,
			}
			b, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Neighbors of %s (hops=%d, kuzu): %d found\n\n", path, hops, len(neighbors))
			for _, n := range neighbors {
				fmt.Printf("  [%s] %s [%s]\n", n.Type, n.Path, n.Rel)
			}
		}

		if len(neighbors) == 0 {
			fmt.Printf("No neighbors found for %s.\n", path)
		}

		return nil
	},
}

func init() {
	neighborsCmd.Flags().IntVar(&hops, "hops", 1, "número de hops (default 1)")
}
