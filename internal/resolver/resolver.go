package resolver

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Resolver maneja la resolución de wikilinks [[...]] a rutas de archivo
type Resolver struct {
	byBasename map[string][]string // "srv-x" → ["02_Servidores/srv-x.md", ...]
	byFullpath map[string]string   // "Planes/nota" → "Planes/nota.md"
}

// BuildResolver construye las tablas de resolución a partir de todas las rutas
func BuildResolver(allPaths []string) *Resolver {
	r := &Resolver{
		byBasename: make(map[string][]string),
		byFullpath: make(map[string]string),
	}

	for _, path := range allPaths {
		// Normalizar: quitar .md si trae
		normalized := strings.TrimSuffix(path, ".md")

		// Agregar a byFullpath (sin .md)
		r.byFullpath[normalized] = path

		// Extraer basename (nombre sin directorio)
		basename := filepath.Base(normalized)
		r.byBasename[basename] = append(r.byBasename[basename], path)
	}

	// Ordenar los duplicados por path (alfabético) para determinismo
	for _, paths := range r.byBasename {
		sort.Strings(paths)
	}

	return r
}

// Resolve intenta resolver un wikilink crudo a una ruta de archivo
// Retorna (path resuelto, ok)
func (r *Resolver) Resolve(rawTarget string) (string, bool) {
	// Normalizar: quitar .md si trae, quitar anclas #seccion
	target := strings.TrimSuffix(rawTarget, ".md")
	if idx := strings.Index(target, "#"); idx >= 0 {
		target = target[:idx]
	}
	target = strings.TrimSpace(target)

	if target == "" {
		return "", false
	}

	// Si contiene /, intentar byFullpath
	if strings.Contains(target, "/") {
		if path, ok := r.byFullpath[target]; ok {
			return path, true
		}
		// Si no encontró, no retroceder a basename
		return "", false
	}

	// Sin /, intentar byBasename
	if paths, ok := r.byBasename[target]; ok && len(paths) > 0 {
		// Si hay colisión, tomar la primera (alfabética) y loguear warning
		if len(paths) > 1 {
			fmt.Fprintf(os.Stderr, "warning: wikilink collision '%s' → multiple paths found, using first: %s\n", rawTarget, paths[0])
		}
		return paths[0], true
	}

	// Sin match
	return "", false
}
