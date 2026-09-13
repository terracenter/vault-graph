package kuzu

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/freddytaborda/vault-graph/internal/graphdb"
)

type Store struct {
	conn      *Conn
	vaultPath string
}

func NewStore(dbPath, vaultPath string) (*Store, error) {
	conn, err := Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open Kuzu: %w", err)
	}
	return &Store{conn: conn, vaultPath: vaultPath}, nil
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

func (s *Store) Neighbors(ctx context.Context, path string, nhops int) ([]graphdb.Neighbor, error) {
	visited := make(map[string]bool)
	visited[path] = true
	current := []string{path}
	var results []graphdb.Neighbor

	for hop := 0; hop < nhops; hop++ {
		next := []string{}
		for _, p := range current {
			escaped := escapeCypherString(p)
			err := s.conn.Query(
				"MATCH (a:File {path: '"+escaped+"'})-[r]->(b:File) RETURN a.path AS from_path, label(r) AS rel_type, b.path AS to_path",
				func(row map[string]any) bool {
					rel, _ := row["rel_type"].(string)
					to, _ := row["to_path"].(string)
					if !visited[to] {
						results = append(results, graphdb.Neighbor{Type: "File", Path: to, Rel: rel})
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

func (s *Store) Backlinks(ctx context.Context, path string, hops int) ([]graphdb.Neighbor, error) {
	var results []graphdb.Neighbor
	err := s.conn.Query(
		"MATCH (a:File)-[r:ENLAZA]->(b:File {path: '"+escapeCypherString(path)+"'}) RETURN a.path AS from_path, label(r) AS rel_type",
		func(row map[string]any) bool {
			from, _ := row["from_path"].(string)
			rel, _ := row["rel_type"].(string)
			results = append(results, graphdb.Neighbor{Type: "File", Path: from, Rel: rel})
			return true
		})
	if err != nil {
		return nil, fmt.Errorf("query Kuzu: %w", err)
	}
	return results, nil
}

func (s *Store) ShortestPath(ctx context.Context, from, to string) (graphdb.PathResult, error) {
	visited := make(map[string]bool)
	visited[from] = true
	parent := make(map[string]string)
	queue := []string{from}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == to {
			path := []string{}
			n := to
			for n != "" {
				path = append([]string{n}, path...)
				p, ok := parent[n]
				if !ok {
					break
				}
				n = p
			}
			nodes := make([]graphdb.PathNode, len(path))
			for i, p := range path {
				nodes[i] = graphdb.PathNode{Type: "File", Path: p}
			}
			return graphdb.PathResult{Found: true, Hops: len(path) - 1, Nodes: nodes}, nil
		}

		escaped := escapeCypherString(current)
		err := s.conn.Query(
			"MATCH (a:File {path: '"+escaped+"'})-[r]->(b:File) RETURN b.path AS to_path",
			func(row map[string]any) bool {
				next, _ := row["to_path"].(string)
				if !visited[next] {
					visited[next] = true
					parent[next] = current
					queue = append(queue, next)
				}
				return true
			})
		if err != nil {
			return graphdb.PathResult{}, fmt.Errorf("query BFS: %w", err)
		}
	}
	return graphdb.PathResult{Found: false}, nil
}

func (s *Store) OrphanNodes(ctx context.Context) ([]graphdb.OrphanNode, error) {
	var results []graphdb.OrphanNode
	err := s.conn.Query(`
		MATCH (n:File)
		WHERE NOT EXISTS { MATCH (n)-[]->() }
		  AND NOT EXISTS { MATCH ()-[]->(n) }
		RETURN n.path AS p`,
		func(row map[string]any) bool {
			p, _ := row["p"].(string)
			results = append(results, graphdb.OrphanNode{Type: "File", Path: p, Titulo: ""})
			return true
		})
	if err != nil {
		return nil, fmt.Errorf("query Kuzu: %w", err)
	}
	return results, nil
}

func (s *Store) Stats(ctx context.Context) ([]graphdb.NodeStat, []graphdb.ClientServerRelation, error) {
	var stats []graphdb.NodeStat
	count := 0
	err := s.conn.Query("MATCH (n:File) RETURN count(*)",
		func(row map[string]any) bool {
			for _, v := range row {
				if c, ok := v.(int64); ok {
					count = int(c)
				}
			}
			return true
		})
	if err != nil {
		return nil, nil, fmt.Errorf("count nodos: %w", err)
	}
	stats = append(stats, graphdb.NodeStat{Type: "File", Count: count})

	var relations []graphdb.ClientServerRelation
	err = s.conn.Query(
		"MATCH (a:File)-[r:ENLAZA]->(b:File) RETURN a.path AS from_path, label(r) AS rel_type, b.path AS to_path LIMIT 5",
		func(row map[string]any) bool {
			a, _ := row["from_path"].(string)
			t, _ := row["rel_type"].(string)
			b, _ := row["to_path"].(string)
			relations = append(relations, graphdb.ClientServerRelation{Cliente: a, Type: t, Servidor: b})
			return true
		})
	if err != nil {
		return nil, nil, fmt.Errorf("query relations: %w", err)
	}
	return stats, relations, nil
}

func (s *Store) Query(ctx context.Context, cypher string) ([]map[string]any, error) {
	return s.conn.QueryRows(cypher)
}

// Helper methods for BrokenLinks
func walkMarkdownFiles(vaultPath string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(vaultPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".obsidian" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if strings.HasSuffix(path, "Planes/LLM-Wiki/graphrag") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		relPath, err := filepath.Rel(vaultPath, path)
		if err != nil {
			return err
		}
		files = append(files, strings.ReplaceAll(relPath, "\\", "/"))
		return nil
	})
	return files, err
}

var wikiLinkRegex = regexp.MustCompile(`\[\[([^\]|#]+)(?:#[^\]|]*)?(?:\|[^\]]*)?\]\]`)

func extractWikilinks(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	matches := wikiLinkRegex.FindAllStringSubmatch(string(data), -1)
	var links []string
	for _, m := range matches {
		link := strings.TrimSpace(m[1])
		if link != "" {
			links = append(links, link)
		}
	}
	return links, nil
}

func resolveWikilink(vaultPath, fromPath, link string) string {
	clean := strings.TrimSuffix(link, ".md")
	candidates := []string{
		clean + ".md",
		filepath.Join(filepath.Dir(fromPath), clean) + ".md",
		filepath.Join(filepath.Dir(fromPath), clean, "index.md"),
	}
	for _, c := range candidates {
		full := filepath.Join(vaultPath, c)
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		if !strings.HasSuffix(c, ".md") {
			continue
		}
		return strings.ReplaceAll(c, "\\", "/")
	}
	return ""
}

func (s *Store) BrokenLinks(ctx context.Context) ([]graphdb.BrokenLink, error) {
	if s.vaultPath == "" {
		return nil, fmt.Errorf("BrokenLinks requiere vaultPath")
	}

	paths, err := walkMarkdownFiles(s.vaultPath)
	if err != nil {
		return nil, fmt.Errorf("walk vault: %w", err)
	}
	known := make(map[string]bool, len(paths))
	for _, p := range paths {
		known[p] = true
	}

	var broken []graphdb.BrokenLink
	for _, from := range paths {
		wikis, err := extractWikilinks(filepath.Join(s.vaultPath, from))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: extract %s: %v\n", from, err)
			continue
		}
		for _, link := range wikis {
			resolved := resolveWikilink(s.vaultPath, from, link)
			if resolved == "" && !strings.HasSuffix(strings.ToLower(link), ".md") {
				resolved = resolveWikilink(s.vaultPath, from, link+".md")
			}
			if resolved == "" {
				target := strings.TrimSuffix(link, ".md") + ".md"
				broken = append(broken, graphdb.BrokenLink{
					FromPath: from,
					ToPath:   target,
					Type:     "wikilink-unresolvable",
				})
				continue
			}
			if !known[resolved] {
				broken = append(broken, graphdb.BrokenLink{
					FromPath: from,
					ToPath:   resolved,
					Type:     "wikilink-target-missing",
				})
			}
		}
	}

	if broken == nil {
		broken = []graphdb.BrokenLink{}
	}
	return broken, nil
}
