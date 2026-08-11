// cmd/migrate-age-to-kuzu: herramienta one-shot para migrar el grafo de
// Apache AGE (en PostgreSQL) a Kuzu embebido.
//
// USO:
//   # 1. Conteo / dry-run (no escribe nada)
//   go run ./cmd/migrate-age-to-kuzu \
//     --age-dburl "$DATABASE_URL" \
//     --kuzu-path /home/freddy/Workspace/vault.kuzu \
//     --dry-run
//
//   # 2. Piloto (5 nodos) a archivo temporal
//   go run ./cmd/migrate-age-to-kuzu \
//     --age-dburl "$DATABASE_URL" \
//     --kuzu-path /tmp/pilot.kuzu \
//     --dry-run=false --limit 5
//
//   # 3. Migración completa (todos los nodos con label + ENLAZA)
//   go run ./cmd/migrate-age-to-kuzu \
//     --age-dburl "$DATABASE_URL" \
//     --kuzu-path /home/freddy/Workspace/vault.kuzu \
//     --dry-run=false
//
// QUÉ MIGRA:
//   - Nodos con label válida (Manual, Plan, Wiki, Diario, Nota, Servidor, Cliente).
//   - Aristas ENLAZA entre esos nodos.
//   - NO migra carcasas label-less (residuo de wikilinks rotos en AGE).
//   - NO migra aristas ETIQUETA, MENCIONA_HOST, PERTENECE_A — son
//     residuo histórico no mantenible (ver reference
//     "etiqueta-tags-silently-broken.md" en vault-graph-workflow skill).
//
// ESQUEMA EN KUZU:
//   - Todos los nodos migran con label "File" unificada (no multi-label como AGE).
//     Las labels originales se pierden en este nivel; la información de tipo queda
//     disponible via Kuzu Explorer o queries adicionales si se necesita.
//   - Property "path" se preserva exactamente.
//   - Aristas migran como ENLAZA (mismo nombre).
//
// POR QUÉ EXISTE:
//   - vault-graph históricamente usó AGE. Kuzu es embebido, sin servicio aparte,
//     sin bug de pg_dump, sin RAM de PostgreSQL — ideal para hermes-contabo (7.8 GiB).

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
)

func main() {
	ageDBURL := flag.String("age-dburl", "", "DATABASE_URL de AGE (requerido)")
	kuzuPath := flag.String("kuzu-path", "", "ruta al archivo Kuzu destino (requerido)")
	dryRun := flag.Bool("dry-run", true, "solo contar, no escribir (default TRUE por seguridad)")
	limit := flag.Int("limit", 0, "limitar nodos a migrar (0=todos; útil para piloto)")
	flag.Parse()

	if *ageDBURL == "" || *kuzuPath == "" {
		fmt.Fprintf(os.Stderr, "uso: --age-dburl <url> --kuzu-path <ruta> [--dry-run=false]\n")
		os.Exit(1)
	}

	fmt.Printf("=== migración AGE -> Kuzu ===\n")
	fmt.Printf("AGE:    %s\n", maskURL(*ageDBURL))
	fmt.Printf("Kuzu:   %s\n", *kuzuPath)
	fmt.Printf("DryRun: %v\n", *dryRun)

	ctx := context.Background()

	// 1. Conectar a AGE y contar
	age, err := pgx.Connect(ctx, *ageDBURL)
	if err != nil {
		log.Fatalf("connect AGE: %v", err)
	}
	defer age.Close(ctx)

	// AGE requiere LOAD 'age' + SET search_path antes de cypher().
	// pgx no soporta meta-commands (LOAD es psql, no SQL estándar),
	// así que los ejecutamos como Exec separado.
	if _, err := age.Exec(ctx, `LOAD 'age'`); err != nil {
		log.Fatalf("LOAD age: %v", err)
	}
	if _, err := age.Exec(ctx, `SET search_path = ag_catalog, public`); err != nil {
		log.Fatalf("SET search_path: %v", err)
	}

	var ageNodeCount, ageEdgeCount int
	err = age.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM cypher('vault', $$ MATCH (n) RETURN n $$) AS (n agtype)),
		  (SELECT count(*) FROM cypher('vault', $$ MATCH ()-[r]->() RETURN r $$) AS (r agtype))
	`).Scan(&ageNodeCount, &ageEdgeCount)
	if err != nil {
		log.Fatalf("count AGE: %v", err)
	}
	fmt.Printf("AGE:    %d nodos, %d aristas totales\n", ageNodeCount, ageEdgeCount)

	// 2. Conteo de nodos con label válida
	rows, err := age.Query(ctx, `
		SELECT * FROM cypher('vault', $$
		  MATCH (n) WHERE labels(n)[0] <> '' RETURN n.path, labels(n)[0] AS l
		$$) AS (path agtype, l agtype)
	`)
	if err != nil {
		log.Fatalf("query AGE labeled: %v", err)
	}
	defer rows.Close()

	type labeledNode struct {
		path  string
		label string
	}
	var labeled []labeledNode
	for rows.Next() {
		var p, l string
		if err := rows.Scan(&p, &l); err != nil {
			log.Fatalf("scan: %v", err)
		}
		labeled = append(labeled, labeledNode{path: p, label: l})
	}
	fmt.Printf("AGE:    %d nodos con label válida\n", len(labeled))

	// Aplicar --limit si se especificó
	if *limit > 0 && len(labeled) > *limit {
		labeled = labeled[:*limit]
		fmt.Printf("AGE:    limitado a %d nodos (modo piloto)\n", *limit)
	}

	// 3. Distribución de labels
	dist := map[string]int{}
	for _, n := range labeled {
		dist[n.label]++
	}
	fmt.Printf("AGE:    distribución labels:\n")
	for l, c := range dist {
		fmt.Printf("         %s: %d\n", l, c)
	}

	// 4. (El conteo y listado de aristas ENLAZA se hace más abajo,
	//      dentro del bloque de escritura, después de cargar nodos.)

	if *dryRun {
		fmt.Printf("\n=== DRY RUN — no se escribió nada ===\n")
		fmt.Printf("Para escribir a Kuzu, usar: --dry-run=false\n")
		return
	}

	// 5. Conectar a Kuzu y escribir
	if err := os.Remove(*kuzuPath); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: remove %s: %v", *kuzuPath, err)
	}
	if err := os.Remove(*kuzuPath + ".wal"); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: remove %s.wal: %v", *kuzuPath, err)
	}

	kz, err := kuzu.Open(*kuzuPath)
	if err != nil {
		log.Fatalf("open Kuzu: %v", err)
	}
	defer kz.Close()

	fmt.Printf("Kuzu:   abierto OK en %s\n", *kuzuPath)

	// 6. Migrar nodos
	fmt.Printf("Kuzu:   migrando %d nodos...\n", len(labeled))
	nodesOK := 0
	nodesFail := 0
	for _, n := range labeled {
		// RETURN count(*) para reportar filas (contrato Kuzu).
		q := fmt.Sprintf("CREATE (n:File {path: '%s'}) RETURN count(*)",
			escapeCypherString(n.path))
		if _, err := kz.Execute(q); err != nil {
			log.Printf("error migrando nodo %s: %v", n.path, err)
			nodesFail++
			continue
		}
		nodesOK++
	}
	fmt.Printf("Kuzu:   %d/%d nodos migrados (%d fallos)\n",
		nodesOK, len(labeled), nodesFail)
	if nodesFail > 0 {
		log.Printf("WARNING: %d nodos fallaron, continuando", nodesFail)
	}

	// 7. Leer aristas ENLAZA desde AGE y migrar a Kuzu
	// Solo aristas cuyos endpoints están en el set de nodos migrados.
	pathSet := make(map[string]bool, len(labeled))
	for _, n := range labeled {
		pathSet[n.path] = true
	}

	var eRows pgx.Rows
	eRows, err = age.Query(ctx, `
		SELECT * FROM cypher('vault', $$
		  MATCH (a)-[:ENLAZA]->(b)
		  RETURN a.path AS from, b.path AS to
		$$) AS (from_path agtype, to_path agtype)
	`)
	if err != nil {
		log.Fatalf("query AGE ENLAZA edges: %v", err)
	}
	defer eRows.Close()

	type edge struct{ from, to string }
	var edges []edge
	for eRows.Next() {
		var from, to string
		if err := eRows.Scan(&from, &to); err != nil {
			log.Fatalf("scan edge: %v", err)
		}
		// Solo agregar si ambos endpoints están en el set migrado.
		if pathSet[from] && pathSet[to] {
			edges = append(edges, edge{from: from, to: to})
		}
	}
	fmt.Printf("Kuzu:   %d aristas ENLAZA a migrar (filtradas por nodos existentes)\n",
		len(edges))

	// 8. Migrar aristas
	edgesOK := 0
	edgesFail := 0
	for _, e := range edges {
		// MATCH (a),(b) CREATE (a)-[:ENLAZA]->(b) — patrón Cypher estándar.
		q := fmt.Sprintf(
			"MATCH (a:File {path: '%s'}), (b:File {path: '%s'}) CREATE (a)-[:ENLAZA]->(b)",
			escapeCypherString(e.from), escapeCypherString(e.to))
		if _, err := kz.Execute(q); err != nil {
			log.Printf("error migrando arista %s -> %s: %v", e.from, e.to, err)
			edgesFail++
			continue
		}
		edgesOK++
	}
	fmt.Printf("Kuzu:   %d/%d aristas migradas (%d fallos)\n",
		edgesOK, len(edges), edgesFail)

	// 9. Verificación final
	fmt.Printf("\n=== verificación final en Kuzu ===\n")
	kzCount := 0
	err = kz.Query("MATCH (n:File) RETURN count(*)",
		func(row map[string]any) bool {
			// Kuzu nombra la columna "COUNT_STAR()" para count(*).
			for _, v := range row {
				if c, ok := v.(int64); ok {
					kzCount = int(c)
				}
			}
			return true
		})
	if err != nil {
		log.Printf("verify count nodos: %v", err)
	} else {
		fmt.Printf("Kuzu:   %d nodos File\n", kzCount)
	}

	kzEdges := 0
	err = kz.Query("MATCH ()-[r:ENLAZA]->() RETURN count(*)",
		func(row map[string]any) bool {
			for _, v := range row {
				if c, ok := v.(int64); ok {
					kzEdges = int(c)
				}
			}
			return true
		})
	if err != nil {
		log.Printf("verify count aristas: %v", err)
	} else {
		fmt.Printf("Kuzu:   %d aristas ENLAZA\n", kzEdges)
	}

	fmt.Printf("\n=== resumen ===\n")
	fmt.Printf("AGE  origen: %d nodos con label, %d aristas ENLAZA\n",
		len(labeled), len(edges))
	fmt.Printf("Kuzu destino: %d nodos, %d aristas\n", kzCount, kzEdges)
	if kzCount == len(labeled) && kzEdges == len(edges) {
		fmt.Printf("✅ migración exitosa\n")
	} else {
		fmt.Printf("⚠️  conteos no coinciden — revisar logs\n")
	}
}

func escapeCypherString(s string) string {
	// Escape básico — alinea con cypher.go de vault-graph.
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

func maskURL(u string) string {
	// Oculta password en el log.
	// Formato: postgresql://user:pass@host:port/db
	atIdx := -1
	colonIdx := -1
	for i := 0; i < len(u); i++ {
		if u[i] == '@' && atIdx == -1 {
			atIdx = i
		}
		if u[i] == ':' && colonIdx == -1 {
			colonIdx = i
		}
	}
	if colonIdx > -1 && colonIdx < atIdx {
		return u[:colonIdx+1] + "***" + u[atIdx:]
	}
	return u
}
