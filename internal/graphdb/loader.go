package graphdb

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/freddytaborda/vault-graph/internal/model"
	"github.com/freddytaborda/vault-graph/internal/parser"
	"github.com/freddytaborda/vault-graph/internal/resolver"
)

// LoaderStats contiene estadísticas de la carga
type LoaderStats struct {
	NodesCreated        int
	NodesUpdated        int
	EdgesCreated        int
	UnresolvedWikilinks int
	CollisionsDetected  int
}

// Loader orquesta la carga de archivos .md en el grafo
type Loader struct {
	conn       *Conn
	vaultPath  string
	batchSize  int // número de archivos por transacción (default 50)
	resolver   *resolver.Resolver
	stats      LoaderStats
}

// NewLoader crea un nuevo Loader
func NewLoader(conn *Conn, vaultPath string) *Loader {
	return &Loader{
		conn:      conn,
		vaultPath: vaultPath,
		batchSize: 50,
	}
}

// LoadFull carga todos los archivos .md del vault
func (l *Loader) LoadFull(ctx context.Context, allPaths []string) (LoaderStats, error) {
	// Construir resolver
	l.resolver = resolver.BuildResolver(allPaths)

	// Primera pasada: crear nodos
	if err := l.mergeAllNodes(ctx, allPaths); err != nil {
		return l.stats, err
	}

	// Segunda pasada: crear aristas
	if err := l.mergeAllEdges(ctx, allPaths); err != nil {
		return l.stats, err
	}

	return l.stats, nil
}

// mergeAllNodes crea/actualiza todos los nodos
func (l *Loader) mergeAllNodes(ctx context.Context, allPaths []string) error {
	for i := 0; i < len(allPaths); i += l.batchSize {
		end := i + l.batchSize
		if end > len(allPaths) {
			end = len(allPaths)
		}
		batch := allPaths[i:end]

		tx, err := l.conn.BeginTx(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		for _, path := range batch {
			content, err := os.ReadFile(l.vaultPath + "/" + path)
			if err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("failed to read %s: %w", path, err)
			}

			node := l.parseNode(path, string(content))
			if err := l.conn.MergeNode(ctx, tx, node); err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("failed to merge node %s: %w", path, err)
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	return nil
}

// mergeAllEdges crea/actualiza todas las aristas
func (l *Loader) mergeAllEdges(ctx context.Context, allPaths []string) error {
	for i := 0; i < len(allPaths); i += l.batchSize {
		end := i + l.batchSize
		if end > len(allPaths) {
			end = len(allPaths)
		}
		batch := allPaths[i:end]

		tx, err := l.conn.BeginTx(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		for _, path := range batch {
			content, err := os.ReadFile(l.vaultPath + "/" + path)
			if err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("failed to read %s: %w", path, err)
			}

			edges := l.parseEdges(path, string(content))
			for _, edge := range edges {
				if err := l.conn.MergeEdge(ctx, tx, &edge); err != nil {
					tx.Rollback(ctx)
					return fmt.Errorf("failed to merge edge %s->%s: %w", edge.FromPath, edge.ToPath, err)
				}
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	return nil
}

// parseNode extrae un Node de un archivo .md
func (l *Loader) parseNode(path string, content string) *model.Node {
	fm, body := parser.ParseFrontmatter(content)

	// Extraer título
	titulo := parser.GetString(fm, "title")
	if titulo == "" {
		// Fallback: usar el filename
		titulo = path
	}

	// Extraer carpeta
	carpeta := ""
	if idx := len(path) - 1; idx > 0 {
		for i := idx; i >= 0; i-- {
			if path[i] == '/' {
				carpeta = path[:i]
				break
			}
		}
	}

	// Clasificar tipo por carpeta
	nodeType := model.ClassifyByFolder(path)

	// Extraer created date
	created := parser.GetString(fm, "created")

	// Extra props según el tipo
	extra := make(map[string]any)
	if nodeType == model.TypeServidor {
		extra["ip"] = parser.GetString(fm, "ip")
		extra["os"] = parser.GetString(fm, "os")
		extra["role"] = parser.GetString(fm, "role")
		extra["client"] = parser.GetString(fm, "client")
	} else if nodeType == model.TypeCliente {
		extra["segment"] = parser.GetString(fm, "segment")
		extra["active_servers"] = parser.GetStringSlice(fm, "active_servers")
	}

	// Tags e inline
	tags := parser.ExtractTags(fm, body)
	extra["tags"] = tags

	// Hosts mencionados
	hosts := parser.ExtractHosts(body)
	extra["hosts"] = hosts

	return &model.Node{
		Path:    path,
		Type:    nodeType,
		Titulo:  titulo,
		Carpeta: carpeta,
		MTime:   time.Now(),
		Created: created,
		Extra:   extra,
	}
}

// parseEdges extrae todas las aristas de un archivo .md
func (l *Loader) parseEdges(path string, content string) []model.Edge {
	fm, body := parser.ParseFrontmatter(content)
	edges := make([]model.Edge, 0)

	// Wikilinks [[...]]
	wikilinks := parser.ExtractWikilinks(body)
	for _, link := range wikilinks {
		resolved, ok := l.resolver.Resolve(link.Target)
		if !ok {
			l.stats.UnresolvedWikilinks++
		}
		edges = append(edges, model.Edge{
			FromPath: path,
			ToPath:   resolved,
			Type:     "ENLAZA",
			Resuelto: ok,
		})
	}

	// Tags → :Tag nodes
	tags := parser.ExtractTags(fm, body)
	for _, tag := range tags {
		edges = append(edges, model.Edge{
			FromPath: path,
			ToPath:   tag, // Tag node sin .md
			Type:     "ETIQUETA",
			Resuelto: true,
		})
	}

	// Relaciones específicas
	nodeType := model.ClassifyByFolder(path)

	// Servidor → Cliente (PERTENECE_A)
	if nodeType == model.TypeServidor {
		client := parser.GetString(fm, "client")
		if client != "" {
			resolved, ok := l.resolver.Resolve(client)
			if !ok {
				l.stats.UnresolvedWikilinks++
			}
			edges = append(edges, model.Edge{
				FromPath: path,
				ToPath:   resolved,
				Type:     "PERTENECE_A",
				Resuelto: ok,
			})
		}
	}

	// Cliente → Servidor (USA_SERVIDOR)
	if nodeType == model.TypeCliente {
		servers := parser.GetStringSlice(fm, "active_servers")
		for _, srv := range servers {
			resolved, ok := l.resolver.Resolve(srv)
			if !ok {
				l.stats.UnresolvedWikilinks++
			}
			edges = append(edges, model.Edge{
				FromPath: path,
				ToPath:   resolved,
				Type:     "USA_SERVIDOR",
				Resuelto: ok,
			})
		}
	}

	// Hosts mencionados
	hosts := parser.ExtractHosts(body)
	for _, host := range hosts {
		edges = append(edges, model.Edge{
			FromPath: path,
			ToPath:   host,
			Type:     "MENCIONA_HOST",
			Resuelto: true,
		})
	}

	return edges
}
