package graphdb

import "context"

type Neighbor struct {
	Type string `json:"type"` // label del nodo origen
	Path string `json:"path"` // path del nodo vecino
	Rel  string `json:"rel"`  // label de la arista
}

type BrokenLink struct {
	FromPath string `json:"from_path"`
	ToPath   string `json:"to_path"`
	Type     string `json:"type"`
}

type OrphanNode struct {
	Type   string `json:"type"`
	Path   string `json:"path"`
	Titulo string `json:"titulo"`
}

type NodeStat struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type ClientServerRelation struct {
	Cliente  string `json:"cliente"`
	Type     string `json:"type"`
	Servidor string `json:"servidor"`
}

type PathNode struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

type PathResult struct {
	Found bool       `json:"found"`
	Hops  int        `json:"hops"`
	Nodes []PathNode `json:"nodes"`
}

// PathFilter no se encontró en el código existente, se define vacío por ahora.
type PathFilter struct {
}

type Store interface {
	Neighbors(ctx context.Context, path string, hops int) ([]Neighbor, error)
	Backlinks(ctx context.Context, path string, hops int) ([]Neighbor, error)
	ShortestPath(ctx context.Context, from, to string) (PathResult, error)
	BrokenLinks(ctx context.Context) ([]BrokenLink, error)
	OrphanNodes(ctx context.Context) ([]OrphanNode, error)
	Stats(ctx context.Context) ([]NodeStat, []ClientServerRelation, error)
	Query(ctx context.Context, cypher string) ([]map[string]any, error)
	MergeNode(ctx context.Context, path string) error
	MergeEdge(ctx context.Context, from, to string) error
	ListPaths(ctx context.Context, filter PathFilter) ([]string, error)
	UpdateSummary(ctx context.Context, path, summary string) error
	Close() error
}
