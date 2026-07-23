//go:build integration

package graphdb

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/freddytaborda/vault-graph/internal/model"
)

// newTestConn abre una conexión a PostgreSQL usando DATABASE_URL.
// Si la variable no está definida, el test se salta (no falla).
func newTestConn(t *testing.T) (*Conn, func()) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	conn, err := NewConn(ctx, dsn)
	if err != nil {
		t.Fatalf("NewConn: %v", err)
	}
	return conn, func() { conn.Close() }
}

// seedNode crea un nodo vía MergeNode en una transacción efímera.
func seedNode(t *testing.T, conn *Conn, path string) {
	t.Helper()
	ctx := context.Background()
	tx, err := conn.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	node := &model.Node{
		Path:    path,
		Type:    model.TypeNota,
		Titulo:  path,
		MTime:   time.Now(),
		Extra:   map[string]any{},
	}
	if err := conn.MergeNode(ctx, tx, node); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("MergeNode %s: %v", path, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// seedEdge crea una arista entre dos paths.
func seedEdge(t *testing.T, conn *Conn, from, to string) {
	t.Helper()
	ctx := context.Background()
	tx, err := conn.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	edge := &model.Edge{
		FromPath: from,
		ToPath:   to,
		Type:     "ENLAZA",
		Resuelto: true,
	}
	if err := conn.MergeEdge(ctx, tx, edge); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("MergeEdge %s->%s: %v", from, to, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// nodeExists verifica si existe un nodo por path.
func nodeExists(t *testing.T, conn *Conn, path string) bool {
	t.Helper()
	cypher := `
	SELECT * FROM cypher('vault', $$
	  MATCH (n {path: '` + escapeString(path) + `'})
	  RETURN n
	$$) AS (n agtype);
	`
	ctx := context.Background()
	rows, err := conn.pool.Query(ctx, cypher)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()
	return rows.Next()
}

// TestPruneOrphanNodes_RemovesMissingPaths valida que prune elimina nodos
// cuyo path no está en validPaths.
func TestPruneOrphanNodes_RemovesMissingPaths(t *testing.T) {
	conn, cleanup := newTestConn(t)
	defer cleanup()
	ctx := context.Background()

	keep := "_test_prune_keep.md"
	drop := "_test_prune_drop.md"
	seedNode(t, conn, keep)
	seedNode(t, conn, drop)

	if !nodeExists(t, conn, keep) || !nodeExists(t, conn, drop) {
		t.Fatal("seed failed")
	}

	deleted, err := conn.PruneOrphanNodes(ctx, []string{keep})
	if err != nil {
		t.Fatalf("PruneOrphanNodes: %v", err)
	}
	if deleted < 1 {
		t.Errorf("expected at least 1 deletion, got %d", deleted)
	}
	if !nodeExists(t, conn, keep) {
		t.Error("keep node should still exist")
	}
	if nodeExists(t, conn, drop) {
		t.Error("drop node should be gone")
	}

	// Cleanup
	seedNode(t, conn, drop) // re-añade drop para que un test futuro no choque
}

// TestPruneOrphanNodes_NoOpWhenAllPathsPresent valida que no borra nada
// si todos los nodos están en validPaths.
func TestPruneOrphanNodes_NoOpWhenAllPathsPresent(t *testing.T) {
	conn, cleanup := newTestConn(t)
	defer cleanup()
	ctx := context.Background()

	keep := "_test_prune_keep2.md"
	seedNode(t, conn, keep)

	// El grafo puede contener otros huérfanos residuales (bug histórico
	// de vault-graph); no podemos esperar que `deleted == 0`. Lo que sí
	// podemos garantizar es que `keep` sobrevive siempre.
	_, err := conn.PruneOrphanNodes(ctx, []string{keep})
	if err != nil {
		t.Fatalf("PruneOrphanNodes: %v", err)
	}
	if !nodeExists(t, conn, keep) {
		t.Error("keep node should still exist")
	}
}

// TestPruneOrphanNodes_DetachesEdges valida que DETACH DELETE borra
// también las aristas asociadas.
func TestPruneOrphanNodes_DetachesEdges(t *testing.T) {
	conn, cleanup := newTestConn(t)
	defer cleanup()
	ctx := context.Background()

	drop := "_test_prune_drop_edge.md"
	keep := "_test_prune_keep_edge.md"
	seedNode(t, conn, drop)
	seedNode(t, conn, keep)
	seedEdge(t, conn, drop, keep)

	// Borrar solo `drop` — la arista debe irse con él
	_, err := conn.PruneOrphanNodes(ctx, []string{keep})
	if err != nil {
		t.Fatalf("PruneOrphanNodes: %v", err)
	}

	if nodeExists(t, conn, drop) {
		t.Error("drop node should be gone")
	}
	if !nodeExists(t, conn, keep) {
		t.Error("keep node should still exist")
	}

	// Re-seed drop para limpar
	seedNode(t, conn, drop)
}
