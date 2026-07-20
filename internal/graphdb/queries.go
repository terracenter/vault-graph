package graphdb

import (
	"context"
	"fmt"
)

// BrokenLink representa un wikilink no resuelto
type BrokenLink struct {
	FromPath string `json:"from_path"`
	ToPath   string `json:"to_path"`
	Type     string `json:"type"`
}

// QueryBrokenLinks retorna todas las aristas con resuelto=false
func (c *Conn) QueryBrokenLinks(ctx context.Context) ([]BrokenLink, error) {
	query := `
	SELECT * FROM cypher('vault', $$
	  MATCH (a)-[r]->(b)
	  WHERE r.resuelto = false
	  RETURN a.path AS from_path, b.path AS to_path, type(r) AS rel_type
	  ORDER BY from_path
	$$) AS (from_path agtype, to_path agtype, rel_type agtype);
	`

	rows, err := c.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query broken links failed: %w", err)
	}
	defer rows.Close()

	var links []BrokenLink
	for rows.Next() {
		var fromPath, toPath, relType string
		if err := rows.Scan(&fromPath, &toPath, &relType); err != nil {
			return nil, fmt.Errorf("scan broken link failed: %w", err)
		}

		// Remover comillas JSON si existen
		from := trimJSON(fromPath)
		to := trimJSON(toPath)
		typ := trimJSON(relType)

		if from != "" && to != "" {
			links = append(links, BrokenLink{
				FromPath: from,
				ToPath:   to,
				Type:     typ,
			})
		}
	}

	return links, rows.Err()
}

// trimJSON remueve comillas de un string JSON
func trimJSON(s string) string {
	if len(s) > 0 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// OrphanNode representa un nodo sin conexiones
type OrphanNode struct {
	Path   string `json:"path"`
	Titulo string `json:"titulo"`
	Type   string `json:"type"`
}

// QueryOrphanNodes retorna nodos sin ninguna arista entrante ni saliente (cualquier tipo)
func (c *Conn) QueryOrphanNodes(ctx context.Context) ([]OrphanNode, error) {
	query := `
	SELECT * FROM cypher('vault', $$
	  MATCH (n)
	  WHERE NOT EXISTS {
	    MATCH (n)-[]-()
	  } AND NOT EXISTS {
	    MATCH ()-[]-(n)
	  }
	  RETURN n.path AS path, n.titulo AS titulo, labels(n)[0] AS node_type
	  ORDER BY path
	$$) AS (path agtype, titulo agtype, node_type agtype);
	`

	rows, err := c.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query orphan nodes failed: %w", err)
	}
	defer rows.Close()

	var orphans []OrphanNode
	for rows.Next() {
		var path, nodeType string
		var titulo *string // nullable
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
			orphans = append(orphans, OrphanNode{
				Path:   p,
				Titulo: t,
				Type:   nt,
			})
		}
	}

	return orphans, rows.Err()
}

// NodeStats contiene estadísticas del grafo
type NodeStats struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// QueryNodeStats retorna conteo de nodos por tipo
func (c *Conn) QueryNodeStats(ctx context.Context) ([]NodeStats, error) {
	query := `
	SELECT * FROM cypher('vault', $$
	  MATCH (n)
	  RETURN labels(n)[0] AS node_type, count(*) AS cnt
	  ORDER BY node_type
	$$) AS (node_type agtype, cnt agtype);
	`

	rows, err := c.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query node stats failed: %w", err)
	}
	defer rows.Close()

	var stats []NodeStats
	for rows.Next() {
		var nodeType, cnt string
		if err := rows.Scan(&nodeType, &cnt); err != nil {
			return nil, fmt.Errorf("scan node stats failed: %w", err)
		}

		nt := trimJSON(nodeType)
		var count int
		fmt.Sscanf(cnt, "%d", &count)

		if nt != "" {
			stats = append(stats, NodeStats{
				Type:  nt,
				Count: count,
			})
		}
	}

	return stats, rows.Err()
}

// ClientServerRelation representa una relación Cliente-Servidor
type ClientServerRelation struct {
	Cliente  string `json:"cliente"`
	Servidor string `json:"servidor"`
	Type     string `json:"type"`
}

// QueryClientServerRelations retorna muestra de relaciones Cliente↔Servidor
func (c *Conn) QueryClientServerRelations(ctx context.Context, limit int) ([]ClientServerRelation, error) {
	query := fmt.Sprintf(`
	SELECT * FROM cypher('vault', $$
	  MATCH (c:Cliente)-[r]-(s:Servidor)
	  RETURN c.path AS cliente, s.path AS servidor, type(r) AS rel_type
	  LIMIT %d
	$$) AS (cliente agtype, servidor agtype, rel_type agtype);
	`, limit)

	rows, err := c.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query client-server relations failed: %w", err)
	}
	defer rows.Close()

	var relations []ClientServerRelation
	for rows.Next() {
		var cliente, servidor, relType string
		if err := rows.Scan(&cliente, &servidor, &relType); err != nil {
			return nil, fmt.Errorf("scan relation failed: %w", err)
		}

		c := trimJSON(cliente)
		s := trimJSON(servidor)
		rt := trimJSON(relType)

		if c != "" && s != "" {
			relations = append(relations, ClientServerRelation{
				Cliente:  c,
				Servidor: s,
				Type:     rt,
			})
		}
	}

	return relations, rows.Err()
}

// Neighbor representa un nodo vecino
type Neighbor struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Rel  string `json:"rel"`
}

// QueryNeighbors retorna vecinos hasta N hops de un nodo dado
func (c *Conn) QueryNeighbors(ctx context.Context, path string, hops int) ([]Neighbor, error) {
	// Para hops=1, usar relación directa. Para hops>1, usar path variable sin type()
	var query string
	if hops == 1 {
		query = fmt.Sprintf(`
		SELECT * FROM cypher('vault', $$
		  MATCH (start {path: '%s'})-[r]-(neighbor)
		  WHERE start IS NOT NULL AND neighbor IS NOT NULL
		  RETURN DISTINCT neighbor.path AS path, labels(neighbor)[0] AS node_type, type(r) AS rel_type
		  ORDER BY path
		$$) AS (path agtype, node_type agtype, rel_type agtype);
		`, path)
	} else {
		query = fmt.Sprintf(`
		SELECT * FROM cypher('vault', $$
		  MATCH (start {path: '%s'})-[*1..%d]-(neighbor)
		  WHERE start IS NOT NULL AND neighbor IS NOT NULL
		  RETURN DISTINCT neighbor.path AS path, labels(neighbor)[0] AS node_type
		  ORDER BY path
		$$) AS (path agtype, node_type agtype);
		`, path, hops)
	}

	rows, err := c.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query neighbors failed: %w", err)
	}
	defer rows.Close()

	var neighbors []Neighbor
	for rows.Next() {
		if hops == 1 {
			var p, nt, rt string
			if err := rows.Scan(&p, &nt, &rt); err != nil {
				return nil, fmt.Errorf("scan neighbor failed: %w", err)
			}
			path := trimJSON(p)
			nodeType := trimJSON(nt)
			relType := trimJSON(rt)
			if path != "" && nodeType != "" {
				neighbors = append(neighbors, Neighbor{
					Path: path,
					Type: nodeType,
					Rel:  relType,
				})
			}
		} else {
			var p, nt string
			if err := rows.Scan(&p, &nt); err != nil {
				return nil, fmt.Errorf("scan neighbor failed: %w", err)
			}
			path := trimJSON(p)
			nodeType := trimJSON(nt)
			if path != "" && nodeType != "" {
				neighbors = append(neighbors, Neighbor{
					Path: path,
					Type: nodeType,
					Rel:  "multi-hop",
				})
			}
		}
	}

	return neighbors, rows.Err()
}

// Backlink representa un enlace entrante
type Backlink struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Rel  string `json:"rel"`
}

// QueryBacklinks retorna nodos que enlazan HACIA el path dado
func (c *Conn) QueryBacklinks(ctx context.Context, path string) ([]Backlink, error) {
	query := fmt.Sprintf(`
	SELECT * FROM cypher('vault', $$
	  MATCH (source)-[r]->(target {path: '%s'})
	  WHERE source IS NOT NULL AND target IS NOT NULL
	  RETURN DISTINCT source.path AS path, labels(source)[0] AS node_type, type(r) AS rel_type
	  ORDER BY path
	$$) AS (path agtype, node_type agtype, rel_type agtype);
	`, path)

	rows, err := c.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query backlinks failed: %w", err)
	}
	defer rows.Close()

	var backlinks []Backlink
	for rows.Next() {
		var path, nodeType, relType string
		if err := rows.Scan(&path, &nodeType, &relType); err != nil {
			return nil, fmt.Errorf("scan backlink failed: %w", err)
		}

		p := trimJSON(path)
		nt := trimJSON(nodeType)
		rt := trimJSON(relType)

		if p != "" && nt != "" {
			backlinks = append(backlinks, Backlink{
				Path: p,
				Type: nt,
				Rel:  rt,
			})
		}
	}

	return backlinks, rows.Err()
}

// PathNode representa un nodo en el camino más corto
type PathNode struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

// ShortestPath representa el resultado de una búsqueda de camino más corto
type ShortestPath struct {
	From  string     `json:"from"`
	To    string     `json:"to"`
	Hops  int        `json:"hops"`
	Nodes []PathNode `json:"nodes"`
	Found bool       `json:"found"`
}

// QueryShortestPath encuentra el camino más corto entre dos nodos (limitado a 3 hops)
func (c *Conn) QueryShortestPath(ctx context.Context, from, to string) (ShortestPath, error) {
	result := ShortestPath{
		From:  from,
		To:    to,
		Nodes: []PathNode{},
		Found: false,
	}

	// Query limitada a 3 hops máximo para evitar búsquedas exponenciales
	query := fmt.Sprintf(`
	SELECT * FROM cypher('vault', $$
	  MATCH p = (start {path: '%s'})-[*1..3]-(target {path: '%s'})
	  RETURN length(p) AS hops_count
	  ORDER BY hops_count
	  LIMIT 1
	$$) AS (hops_count agtype);
	`, from, to)

	rows, err := c.pool.Query(ctx, query)
	if err != nil {
		return result, fmt.Errorf("query shortest path failed: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		var hopsStr string
		if err := rows.Scan(&hopsStr); err != nil {
			return result, fmt.Errorf("scan shortest path failed: %w", err)
		}

		// Parse hops count
		result.Found = true
		if hopsStr != "" {
			hopsStr = trimJSON(hopsStr)
			fmt.Sscanf(hopsStr, "%d", &result.Hops)
		}

		// Populate nodes array con from y to como extremos
		result.Nodes = []PathNode{
			{Path: from, Type: "start"},
			{Path: to, Type: "end"},
		}
	}

	return result, rows.Err()
}

// QueryRaw ejecuta una query Cypher raw y retorna los resultados como mapas genéricos
func (c *Conn) QueryRaw(ctx context.Context, cypher string) ([]map[string]interface{}, error) {
	// Envolver el Cypher en la función cypher() de AGE
	query := fmt.Sprintf(`
	SELECT * FROM cypher('vault', $$
	  %s
	$$) AS (result agtype);
	`, cypher)

	rows, err := c.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("raw query failed: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return nil, fmt.Errorf("scan raw result failed: %w", err)
		}

		// Retornar el resultado como string (AGE devuelve agtype)
		m := map[string]interface{}{"value": result}
		results = append(results, m)
	}

	return results, rows.Err()
}
