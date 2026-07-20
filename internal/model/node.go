package model

import "time"

type NodeType string

const (
	TypeNota     NodeType = "Nota"
	TypeCliente  NodeType = "Cliente"
	TypeServidor NodeType = "Servidor"
	TypeManual   NodeType = "Manual"
	TypePlan     NodeType = "Plan"
	TypeDiario   NodeType = "Diario"
	TypeWiki     NodeType = "Wiki"
	TypeTag      NodeType = "Tag"
	TypeHost     NodeType = "Host"
)

// Node representa un nodo en el grafo vault
type Node struct {
	Path    string         // clave lógica, relativa a la raíz del vault
	Type    NodeType       // label AGE
	Titulo  string
	Carpeta string
	MTime   time.Time
	Created string // frontmatter "created", string YYYY-MM-DD si existe
	Extra   map[string]any // props específicas (ip, os, role, client, active_servers...)
}

// Edge representa una arista en el grafo
type Edge struct {
	FromPath string
	ToPath   string // o ToTag/ToHost cuando el destino no es un archivo
	Type     string // ENLAZA | ETIQUETA | PERTENECE_A | USA_SERVIDOR | MENCIONA_HOST
	Resuelto bool   // false → wikilink roto, alimenta `broken`
}

// ClassifyByFolder determina el NodeType de una ruta basado en su prefijo
func ClassifyByFolder(path string) NodeType {
	// Evaluar en orden: el más específico primero
	if hasPrefix(path, "Planes/LLM-Wiki/wiki/") {
		return TypeWiki
	}
	if hasPrefix(path, "01_Clientes/") {
		return TypeCliente
	}
	if hasPrefix(path, "02_Servidores/") {
		return TypeServidor
	}
	if hasPrefix(path, "03_Manuales_Borradores/") {
		return TypeManual
	}
	if hasPrefix(path, "Planes/") {
		return TypePlan
	}
	if hasPrefix(path, "05_Diario/") {
		return TypeDiario
	}
	return TypeNota
}

func hasPrefix(path, prefix string) bool {
	l := len(prefix)
	return len(path) >= l && path[:l] == prefix
}
