package graphdb

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/freddytaborda/vault-graph/internal/model"
)

// escapeString escapa caracteres especiales en strings para Cypher
func escapeString(s string) string {
	// Escapar backslashes primero, luego comillas
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

// MergeNode inserta o actualiza un nodo via MERGE
func (c *Conn) MergeNode(ctx context.Context, tx pgx.Tx, node *model.Node) error {
	// Construir Cypher con valores literales (escapados)
	cypher := fmt.Sprintf(`
	  MERGE (n:%s {path: '%s'})
	  SET n.titulo = '%s', n.carpeta = '%s', n.mtime = %d, n.tipo = '%s'
	  RETURN n
	`, string(node.Type), escapeString(node.Path), escapeString(node.Titulo),
		escapeString(node.Carpeta), node.MTime.Unix(), string(node.Type))

	query := fmt.Sprintf(`SELECT * FROM cypher('vault', $$%s$$) AS (n agtype);`, cypher)

	_, err := tx.Exec(ctx, query)
	return err
}

// MergeEdge inserta o actualiza una arista via MERGE
func (c *Conn) MergeEdge(ctx context.Context, tx pgx.Tx, edge *model.Edge) error {
	// Construir Cypher con valores literales (escapados)
	cypher := fmt.Sprintf(`
	  MERGE (a {path: '%s'})
	  MERGE (b {path: '%s'})
	  MERGE (a)-[r:%s]->(b)
	  SET r.resuelto = %v
	  RETURN r
	`, escapeString(edge.FromPath), escapeString(edge.ToPath), edge.Type,
		edge.Resuelto)

	query := fmt.Sprintf(`SELECT * FROM cypher('vault', $$%s$$) AS (r agtype);`, cypher)

	_, err := tx.Exec(ctx, query)
	return err
}

// BeginTx comienza una transacción
func (c *Conn) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return c.pool.Begin(ctx)
}
