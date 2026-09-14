package age

import (
	"context"
	"fmt"
	"strings"

	"github.com/freddytaborda/vault-graph/internal/graphdb"
)

func escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

func (s *Store) MergeNode(ctx context.Context, path string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	cypher := fmt.Sprintf(`MERGE (n:File {path: '%s'}) RETURN n`, escapeString(path))
	query := fmt.Sprintf(`SELECT * FROM cypher('vault', $$%s$$) AS (n agtype);`, cypher)
	_, err = tx.Exec(ctx, query)
	if err == nil {
		tx.Commit(ctx)
	}
	return err
}

func (s *Store) MergeEdge(ctx context.Context, from, to string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	cypher := fmt.Sprintf(`MERGE (a {path: '%s'}) MERGE (b {path: '%s'}) MERGE (a)-[r:ENLAZA]->(b) RETURN r`, escapeString(from), escapeString(to))
	query := fmt.Sprintf(`SELECT * FROM cypher('vault', $$%s$$) AS (r agtype);`, cypher)
	_, err = tx.Exec(ctx, query)
	if err == nil {
		tx.Commit(ctx)
	}
	return err
}

func (s *Store) UpdateSummary(ctx context.Context, path, summary string) error {
	cypher := fmt.Sprintf(`
	  MATCH (n:File {path: '%s'})
	  SET n.resumen_llm = '%s'
	  RETURN n
	`, escapeString(path), escapeString(summary))
	query := fmt.Sprintf(`SELECT * FROM cypher('vault', $$%s$$) AS (n agtype);`, cypher)
	_, err := s.pool.Exec(ctx, query)
	return err
}

func (s *Store) ListPaths(ctx context.Context, filter graphdb.PathFilter) ([]string, error) {
	query := `
	SELECT * FROM cypher('vault', $$
	  MATCH (n:File)
	  RETURN n.path AS path
	$$) AS (path agtype);
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list paths failed: %w", err)
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		if len(path) > 0 && path[0] == '"' && path[len(path)-1] == '"' {
			path = path[1 : len(path)-1]
		}
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, rows.Err()
}
