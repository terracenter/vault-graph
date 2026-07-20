package graphdb

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/freddytaborda/vault-graph/internal/model"
)

// MergeNode inserta o actualiza un nodo via MERGE
func (c *Conn) MergeNode(ctx context.Context, tx pgx.Tx, node *model.Node) error {
	query := `
	SELECT * FROM cypher('vault', $$
	  MERGE (n:` + string(node.Type) + ` {path: $path})
	  SET n.titulo = $titulo, n.carpeta = $carpeta, n.mtime = $mtime, n.tipo = $tipo
	  RETURN n
	$$, $1) AS (n agtype);
	`

	_, err := tx.Exec(ctx, query,
		node.Path,
		node.Titulo,
		node.Carpeta,
		node.MTime.Unix(),
		string(node.Type),
	)
	return err
}

// MergeEdge inserta o actualiza una arista via MERGE
func (c *Conn) MergeEdge(ctx context.Context, tx pgx.Tx, edge *model.Edge) error {
	// Cypher query parametrizado: MERGE ambos extremos + la relación
	query := `
	SELECT * FROM cypher('vault', $$
	  MERGE (a {path: $from})
	  MERGE (b {path: $to})
	  MERGE (a)-[r:` + edge.Type + `]->(b)
	  SET r.resuelto = $resuelto
	  RETURN r
	$$, $1) AS (r agtype);
	`

	_, err := tx.Exec(ctx, query,
		edge.FromPath,
		edge.ToPath,
		edge.Resuelto,
	)
	return err
}

// BeginTx comienza una transacción
func (c *Conn) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return c.pool.Begin(ctx)
}
