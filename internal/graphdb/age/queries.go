package age

import (
	"context"
	"fmt"

	"github.com/freddytaborda/vault-graph/internal/graphdb"
)

func trimJSON(s string) string {
	if len(s) > 0 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func (s *Store) BrokenLinks(ctx context.Context) ([]graphdb.BrokenLink, error) {
	query := `
	SELECT * FROM cypher('vault', $$
	  MATCH (a)-[r]->(b)
	  WHERE r.resuelto = false
	  RETURN a.path AS from_path, b.path AS to_path, type(r) AS rel_type
	  ORDER BY from_path
	$$) AS (from_path agtype, to_path agtype, rel_type agtype);
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query broken links failed: %w", err)
	}
	defer rows.Close()

	var links []graphdb.BrokenLink
	for rows.Next() {
		var fromPath, toPath, relType string
		if err := rows.Scan(&fromPath, &toPath, &relType); err != nil {
			return nil, fmt.Errorf("scan broken link failed: %w", err)
		}
		from := trimJSON(fromPath)
		to := trimJSON(toPath)
		typ := trimJSON(relType)
		if from != "" && to != "" {
			links = append(links, graphdb.BrokenLink{
				FromPath: from,
				ToPath:   to,
				Type:     typ,
			})
		}
	}
	return links, rows.Err()
}

func (s *Store) OrphanNodes(ctx context.Context) ([]graphdb.OrphanNode, error) {
	query := `
	SELECT * FROM cypher('vault', $$
	  MATCH (n)
	  WHERE NOT EXISTS { MATCH (n)-[]-() } AND NOT EXISTS { MATCH ()-[]-(n) }
	  RETURN n.path AS path, n.titulo AS titulo, labels(n)[0] AS node_type
	  ORDER BY path
	$$) AS (path agtype, titulo agtype, node_type agtype);
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query orphan nodes failed: %w", err)
	}
	defer rows.Close()

	var orphans []graphdb.OrphanNode
	for rows.Next() {
		var path, nodeType string
		var titulo *string
		if err := rows.Scan(&path, &titulo, &nodeType); err != nil {
			return nil, fmt.Errorf("scan orphan node failed: %w", err)
		}
		p := trimJSON(path)
		nt := trimJSON(nodeType)
		t := ""
		if titulo != nil {
			t = trimJSON(*titulo)
		}
		if p != "" {
			orphans = append(orphans, graphdb.OrphanNode{Path: p, Titulo: t, Type: nt})
		}
	}
	return orphans, rows.Err()
}

func (s *Store) Stats(ctx context.Context) ([]graphdb.NodeStat, []graphdb.ClientServerRelation, error) {
	queryStats := `
	SELECT * FROM cypher('vault', $$
	  MATCH (n)
	  RETURN labels(n)[0] AS node_type, count(*) AS cnt
	  ORDER BY node_type
	$$) AS (node_type agtype, cnt agtype);
	`
	rows, err := s.pool.Query(ctx, queryStats)
	if err != nil {
		return nil, nil, fmt.Errorf("query node stats failed: %w", err)
	}
	var stats []graphdb.NodeStat
	for rows.Next() {
		var nodeType, cnt string
		if err := rows.Scan(&nodeType, &cnt); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("scan node stats failed: %w", err)
		}
		nt := trimJSON(nodeType)
		var count int
		fmt.Sscanf(cnt, "%d", &count)
		if nt != "" {
			stats = append(stats, graphdb.NodeStat{Type: nt, Count: count})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	queryRel := `
	SELECT * FROM cypher('vault', $$
	  MATCH (c:Cliente)-[r]-(s:Servidor)
	  RETURN c.path AS cliente, s.path AS servidor, type(r) AS rel_type
	  LIMIT 5
	$$) AS (cliente agtype, servidor agtype, rel_type agtype);
	`
	rows, err = s.pool.Query(ctx, queryRel)
	if err != nil {
		return nil, nil, fmt.Errorf("query client-server relations failed: %w", err)
	}
	defer rows.Close()
	var relations []graphdb.ClientServerRelation
	for rows.Next() {
		var cliente, servidor, relType string
		if err := rows.Scan(&cliente, &servidor, &relType); err != nil {
			return nil, nil, fmt.Errorf("scan relation failed: %w", err)
		}
		c := trimJSON(cliente)
		srv := trimJSON(servidor)
		rt := trimJSON(relType)
		if c != "" && srv != "" {
			relations = append(relations, graphdb.ClientServerRelation{Cliente: c, Servidor: srv, Type: rt})
		}
	}
	return stats, relations, rows.Err()
}

func (s *Store) Neighbors(ctx context.Context, path string, hops int) ([]graphdb.Neighbor, error) {
	var query string
	if hops == 1 {
		query = fmt.Sprintf(`
		SELECT * FROM cypher('vault', $$
		  MATCH (start {path: '%s'})-[r]-(neighbor)
		  WHERE start IS NOT NULL AND neighbor IS NOT NULL
		  RETURN DISTINCT neighbor.path AS path, labels(neighbor)[0] AS node_type, type(r) AS rel_type
		  ORDER BY path
		$$) AS (path agtype, node_type agtype, rel_type agtype);
		`, escapeString(path))
	} else {
		query = fmt.Sprintf(`
		SELECT * FROM cypher('vault', $$
		  MATCH (start {path: '%s'})-[*1..%d]-(neighbor)
		  WHERE start IS NOT NULL AND neighbor IS NOT NULL
		  RETURN DISTINCT neighbor.path AS path, labels(neighbor)[0] AS node_type
		  ORDER BY path
		$$) AS (path agtype, node_type agtype);
		`, escapeString(path), hops)
	}
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query neighbors failed: %w", err)
	}
	defer rows.Close()

	var neighbors []graphdb.Neighbor
	for rows.Next() {
		if hops == 1 {
			var p, nt, rt string
			if err := rows.Scan(&p, &nt, &rt); err != nil {
				return nil, fmt.Errorf("scan neighbor failed: %w", err)
			}
			if p = trimJSON(p); p != "" {
				neighbors = append(neighbors, graphdb.Neighbor{Path: p, Type: trimJSON(nt), Rel: trimJSON(rt)})
			}
		} else {
			var p, nt string
			if err := rows.Scan(&p, &nt); err != nil {
				return nil, fmt.Errorf("scan neighbor failed: %w", err)
			}
			if p = trimJSON(p); p != "" {
				neighbors = append(neighbors, graphdb.Neighbor{Path: p, Type: trimJSON(nt), Rel: "multi-hop"})
			}
		}
	}
	return neighbors, rows.Err()
}

func (s *Store) Backlinks(ctx context.Context, path string, hops int) ([]graphdb.Neighbor, error) {
	query := fmt.Sprintf(`
	SELECT * FROM cypher('vault', $$
	  MATCH (source)-[r]->(target {path: '%s'})
	  WHERE source IS NOT NULL AND target IS NOT NULL
	  RETURN DISTINCT source.path AS path, labels(source)[0] AS node_type, type(r) AS rel_type
	  ORDER BY path
	$$) AS (path agtype, node_type agtype, rel_type agtype);
	`, escapeString(path))
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query backlinks failed: %w", err)
	}
	defer rows.Close()

	var backlinks []graphdb.Neighbor
	for rows.Next() {
		var p, nt, rt string
		if err := rows.Scan(&p, &nt, &rt); err != nil {
			return nil, fmt.Errorf("scan backlink failed: %w", err)
		}
		if p = trimJSON(p); p != "" {
			backlinks = append(backlinks, graphdb.Neighbor{Path: p, Type: trimJSON(nt), Rel: trimJSON(rt)})
		}
	}
	return backlinks, rows.Err()
}

func (s *Store) ShortestPath(ctx context.Context, from, to string) (graphdb.PathResult, error) {
	result := graphdb.PathResult{Nodes: []graphdb.PathNode{}, Found: false}
	query := fmt.Sprintf(`
	SELECT * FROM cypher('vault', $$
	  MATCH p = (start {path: '%s'})-[*1..3]-(target {path: '%s'})
	  RETURN length(p) AS hops_count
	  ORDER BY hops_count
	  LIMIT 1
	$$) AS (hops_count agtype);
	`, escapeString(from), escapeString(to))
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return result, fmt.Errorf("query shortest path failed: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var hopsStr string
		if err := rows.Scan(&hopsStr); err != nil {
			return result, fmt.Errorf("scan shortest path failed: %w", err)
		}
		result.Found = true
		if hopsStr != "" {
			fmt.Sscanf(trimJSON(hopsStr), "%d", &result.Hops)
		}
		result.Nodes = []graphdb.PathNode{{Path: from, Type: "start"}, {Path: to, Type: "end"}}
	}
	return result, rows.Err()
}

func (s *Store) Query(ctx context.Context, cypher string) ([]map[string]any, error) {
	query := fmt.Sprintf(`SELECT * FROM cypher('vault', $$ %s $$) AS (result agtype);`, cypher)
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("raw query failed: %w", err)
	}
	defer rows.Close()
	var results []map[string]any
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return nil, fmt.Errorf("scan raw result failed: %w", err)
		}
		results = append(results, map[string]any{"value": result})
	}
	return results, rows.Err()
}
