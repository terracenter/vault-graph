// cmd/query-kuzu: ejecuta queries Cypher contra una DB Kuzu embebida.
//
// Comando hermano de vg query, pero apunta al archivo Kuzu (no AGE).
// Útil para validar que la migración o el sync produjo los datos esperados.
//
// USO:
//   # Conteo
//   vg-kuzu-query --kuzu /path/to/vault.kuzu --query "MATCH (n) RETURN count(*)"
//
//   # Listar paths
//   vg-kuzu-query --kuzu /path/to/vault.kuzu --query "MATCH (n:File) RETURN n.path LIMIT 10"
//
//   # Sin query (default: stats)
//   vg-kuzu-query --kuzu /path/to/vault.kuzu

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
)

func main() {
	kuzuPath := flag.String("kuzu", "", "ruta al archivo Kuzu (requerido)")
	query := flag.String("query", "", "query Cypher a ejecutar (vacío = stats)")
	flag.Parse()

	if *kuzuPath == "" {
		fmt.Fprintf(os.Stderr, "uso: --kuzu <ruta> [--query <cypher>]\n")
		os.Exit(1)
	}

	conn, err := kuzu.Open(*kuzuPath)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer conn.Close()

	if *query == "" {
		printStats(conn)
		return
	}

	fmt.Printf("Query: %s\n", *query)
	rowCount := 0
	err = conn.Query(*query, func(row map[string]any) bool {
		rowCount++
		fmt.Printf("  Row %d: %v\n", rowCount, row)
		return true
	})
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	fmt.Printf("Total: %d rows\n", rowCount)
}

func printStats(conn *kuzu.Conn) {
	nodes := 0
	err := conn.Query("MATCH (n:File) RETURN count(*)",
		func(row map[string]any) bool {
			for _, v := range row {
				if c, ok := v.(int64); ok {
					nodes = int(c)
				}
			}
			return true
		})
	if err != nil {
		log.Fatalf("count nodos: %v", err)
	}

	edges := 0
	err = conn.Query("MATCH ()-[r:ENLAZA]->() RETURN count(*)",
		func(row map[string]any) bool {
			for _, v := range row {
				if c, ok := v.(int64); ok {
					edges = int(c)
				}
			}
			return true
		})
	if err != nil {
		log.Fatalf("count aristas: %v", err)
	}

	fmt.Printf("=== Kuzu stats ===\n")
	fmt.Printf("Conexión: abierta\n")
	fmt.Printf("Nodos File: %d\n", nodes)
	fmt.Printf("Aristas ENLAZA: %d\n", edges)
}
