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

// MigrationStats contiene los conteos de una migración.
type MigrationStats struct {
	AgeTotalNodes     int
	AgeTotalEdges     int
	AgeLabeledNodes   int
	AgeEnlaEdges      int
	KuzuWrittenNodes  int
	KuzuFailedNodes   int
	KuzuWrittenEdges  int
	KuzuFailedEdges   int
	KuzuVerifiedNodes int
	KuzuVerifiedEdges int
}

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

	stats, err := migrate(*ageDBURL, *kuzuPath, *limit, *dryRun)
	if err != nil {
		log.Fatalf("migrate: %v", err)
	}

	fmt.Printf("\n=== resumen ===\n")
	fmt.Printf("AGE  origen: %d nodos con label, %d aristas ENLAZA\n",
		stats.AgeLabeledNodes, stats.AgeEnlaEdges)
	fmt.Printf("Kuzu destino: %d nodos, %d aristas\n",
		stats.KuzuVerifiedNodes, stats.KuzuVerifiedEdges)
	if stats.KuzuVerifiedNodes == stats.AgeLabeledNodes &&
		stats.KuzuVerifiedEdges == stats.AgeEnlaEdges {
		fmt.Printf("✅ migración exitosa\n")
	} else {
		fmt.Printf("⚠️  conteos no coinciden — revisar logs\n")
	}
}

// migrate ejecuta el flujo completo de migración AGE → Kuzu.
// Lógica pura separada de main() para permitir testing.
//
// Retorna MigrationStats con los conteos. Si dryRun=true, no escribe
// nada y stats.KuzuWritten* quedan en 0.
func migrate(ageDBURL, kuzuPath string, limit int, dryRun bool) (MigrationStats, error) {
	var stats MigrationStats
	ctx := context.Background()

	// 1. Conectar a AGE.
	age, err := pgx.Connect(ctx, ageDBURL)
	if err != nil {
		return stats, fmt.Errorf("connect AGE: %w", err)
	}
	defer age.Close(ctx)

	// 2. Setup AGE (LOAD + search_path).
	if _, err := age.Exec(ctx, `LOAD 'age'`); err != nil {
		return stats, fmt.Errorf("LOAD age: %w", err)
	}
	if _, err := age.Exec(ctx, `SET search_path = ag_catalog, public`); err != nil {
		return stats, fmt.Errorf("SET search_path: %w", err)
	}

	// 3. Conteo total.
	err = age.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM cypher('vault', $$ MATCH (n) RETURN n $$) AS (n agtype)),
		  (SELECT count(*) FROM cypher('vault', $$ MATCH ()-[r]->() RETURN r $$) AS (r agtype))
	`).Scan(&stats.AgeTotalNodes, &stats.AgeTotalEdges)
	if err != nil {
		return stats, fmt.Errorf("count AGE: %w", err)
	}
	fmt.Printf("AGE:    %d nodos, %d aristas totales\n", stats.AgeTotalNodes, stats.AgeTotalEdges)

	// 4. Nodos con label válida.
	rows, err := age.Query(ctx, `
		SELECT * FROM cypher('vault', $$
		  MATCH (n) WHERE labels(n)[0] <> '' RETURN n.path, labels(n)[0] AS l
		$$) AS (path agtype, l agtype)
	`)
	if err != nil {
		return stats, fmt.Errorf("query AGE labeled: %w", err)
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
			return stats, fmt.Errorf("scan: %w", err)
		}
		labeled = append(labeled, labeledNode{path: p, label: l})
	}
	stats.AgeLabeledNodes = len(labeled)
	fmt.Printf("AGE:    %d nodos con label válida\n", len(labeled))

	if limit > 0 && len(labeled) > limit {
		labeled = labeled[:limit]
		fmt.Printf("AGE:    limitado a %d nodos (modo piloto)\n", limit)
	}

	// 5. Distribución de labels.
	dist := map[string]int{}
	for _, n := range labeled {
		dist[n.label]++
	}
	fmt.Printf("AGE:    distribución labels:\n")
	for l, c := range dist {
		fmt.Printf("         %s: %d\n", l, c)
	}

	// 6. Dry-run: solo contar, no escribir.
	if dryRun {
		fmt.Printf("\n=== DRY RUN — no se escribió nada ===\n")
		fmt.Printf("Para escribir, usar: --dry-run=false\n")
		return stats, nil
	}

	// 7. Borrar DB previa.
	if err := os.Remove(kuzuPath); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: remove %s: %v", kuzuPath, err)
	}
	if err := os.Remove(kuzuPath + ".wal"); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: remove %s.wal: %v", kuzuPath, err)
	}

	// 8. Abrir Kuzu.
	kz, err := kuzu.Open(kuzuPath)
	if err != nil {
		return stats, fmt.Errorf("open Kuzu: %w", err)
	}
	defer kz.Close()
	fmt.Printf("Kuzu:   abierto OK en %s\n", kuzuPath)

	// 9. Migrar nodos.
	fmt.Printf("Kuzu:   migrando %d nodos...\n", len(labeled))
	for _, n := range labeled {
		q := fmt.Sprintf("CREATE (n:File {path: '%s'}) RETURN count(*)",
			escapeCypherString(n.path))
		if _, err := kz.Execute(q); err != nil {
			log.Printf("error migrando nodo %s: %v", n.path, err)
			stats.KuzuFailedNodes++
			continue
		}
		stats.KuzuWrittenNodes++
	}
	fmt.Printf("Kuzu:   %d/%d nodos migrados (%d fallos)\n",
		stats.KuzuWrittenNodes, len(labeled), stats.KuzuFailedNodes)
	if stats.KuzuFailedNodes > 0 {
		log.Printf("WARNING: %d nodos fallaron, continuando", stats.KuzuFailedNodes)
	}

	// 10. Aristas: path set + lectura + escritura.
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
		return stats, fmt.Errorf("query AGE ENLAZA edges: %w", err)
	}
	defer eRows.Close()

	type edge struct{ from, to string }
	var edges []edge
	for eRows.Next() {
		var from, to string
		if err := eRows.Scan(&from, &to); err != nil {
			return stats, fmt.Errorf("scan edge: %w", err)
		}
		if pathSet[from] && pathSet[to] {
			edges = append(edges, edge{from: from, to: to})
		}
	}
	stats.AgeEnlaEdges = len(edges)
	fmt.Printf("Kuzu:   %d aristas ENLAZA a migrar (filtradas por nodos existentes)\n",
		len(edges))

	// 11. Escribir aristas.
	fmt.Printf("Kuzu:   escribiendo aristas...\n")
	for _, e := range edges {
		q := fmt.Sprintf(
			"MATCH (a:File {path: '%s'}), (b:File {path: '%s'}) CREATE (a)-[:ENLAZA]->(b)",
			escapeCypherString(e.from), escapeCypherString(e.to))
		if _, err := kz.Execute(q); err != nil {
			log.Printf("error migrando arista %s -> %s: %v", e.from, e.to, err)
			stats.KuzuFailedEdges++
			continue
		}
		stats.KuzuWrittenEdges++
	}
	fmt.Printf("Kuzu:   %d/%d aristas migradas (%d fallos)\n",
		stats.KuzuWrittenEdges, len(edges), stats.KuzuFailedEdges)

	// 12. Verificación final.
	fmt.Printf("\n=== verificación final en Kuzu ===\n")
	stats.KuzuVerifiedNodes, stats.KuzuVerifiedEdges, err = countKuzuAll(kz)
	if err != nil {
		return stats, fmt.Errorf("verify final: %w", err)
	}
	fmt.Printf("Kuzu:   %d nodos File, %d aristas ENLAZA\n",
		stats.KuzuVerifiedNodes, stats.KuzuVerifiedEdges)

	return stats, nil
}

// countKuzuAll retorna (nodos File, aristas ENLAZA, error).
// Helper para tests y para verificación final de migrate().
func countKuzuAll(conn *kuzu.Conn) (int, int, error) {
	var nodes, edges int
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
		return 0, 0, fmt.Errorf("count nodos: %w", err)
	}
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
		return 0, 0, fmt.Errorf("count aristas: %w", err)
	}
	return nodes, edges, nil
}

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

func maskURL(u string) string {
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
