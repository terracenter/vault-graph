// Package syncvault sincroniza un vault de Obsidian contra una DB Kuzu.
//
// Esta lógica fue extraída de cmd/sync-kuzu/main.go para permitir su
// reuso desde cmd/sync.go (que decide backend AGE o Kuzu según KUZU_PATH).
package syncvault

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
)

// Stats contiene el resultado de una sincronización.
type Stats struct {
	NodeCount    int
	NodeDuration time.Duration
	EdgeCount    int
	EdgesWritten int
	EdgeDuration time.Duration
	AllPaths     []string // paths usados para prune posterior
}

// Sync ejecuta el sync completo del vault contra el archivo Kuzu.
// dryRun=true solo recolecta paths sin escribir nada.
//
// Devuelve Stats con conteos y durations.
func Sync(vaultPath, kuzuPath string, dryRun bool) (Stats, error) {
	var stats Stats

	// 1. Recolectar archivos .md.
	paths, err := collectMarkdownFiles(vaultPath)
	if err != nil {
		return stats, fmt.Errorf("collect: %w", err)
	}
	stats.AllPaths = paths
	stats.NodeCount = len(paths)

	if dryRun {
		return stats, nil
	}

	// 2. Borrar DB previa.
	if err := os.Remove(kuzuPath); err != nil && !os.IsNotExist(err) {
		// No fatal, solo warning.
		fmt.Printf("warning: remove %s: %v\n", kuzuPath, err)
	}
	if err := os.Remove(kuzuPath + ".wal"); err != nil && !os.IsNotExist(err) {
		fmt.Printf("warning: remove %s.wal: %v\n", kuzuPath, err)
	}

	// 3. Abrir Kuzu.
	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return stats, fmt.Errorf("open Kuzu: %w", err)
	}
	defer conn.Close()

	// 4. Crear nodos.
	startNodes := time.Now()
	for _, p := range paths {
		q := fmt.Sprintf("CREATE (n:File {path: '%s'})", escapeCypher(p))
		if _, err := conn.Execute(q); err != nil {
			return stats, fmt.Errorf("create node %s: %w", p, err)
		}
	}
	stats.NodeDuration = time.Since(startNodes)

	// 5. Detectar wikilinks y crear aristas.
	startEdges := time.Now()
	edgeSet := map[string]struct{}{} // dedupe
	for _, from := range paths {
		wikilinks, err := extractWikilinks(filepath.Join(vaultPath, from))
		if err != nil {
			fmt.Printf("warning: extract %s: %v\n", from, err)
			continue
		}
		for _, link := range wikilinks {
			resolved := resolveWikilink(vaultPath, from, link)
			if resolved == "" {
				continue
			}
			edgeSet[fmt.Sprintf("%s\t%s", from, resolved)] = struct{}{}
		}
	}
	stats.EdgeCount = len(edgeSet)

	// 6. Crear aristas. Si el destino no existe (archivo no-.md),
	//    creamos el nodo destino también — fix de Fase 4.
	existingNodes := make(map[string]bool, len(paths))
	for _, p := range paths {
		existingNodes[p] = true
	}

	for edge := range edgeSet {
		parts := strings.SplitN(edge, "\t", 2)
		from, to := parts[0], parts[1]

		// Si destino no existe como nodo, crearlo.
		if !existingNodes[to] {
			q := fmt.Sprintf("MERGE (n:File {path: '%s'})", escapeCypher(to))
			if _, err := conn.Execute(q); err != nil {
				return stats, fmt.Errorf("create dest node %s: %w", to, err)
			}
			existingNodes[to] = true
		}

		q := fmt.Sprintf(
			"MATCH (a:File {path: '%s'}), (b:File {path: '%s'}) MERGE (a)-[:ENLAZA]->(b)",
			escapeCypher(from), escapeCypher(to))
		if _, err := conn.Execute(q); err != nil {
			return stats, fmt.Errorf("create edge %s->%s: %w", from, to, err)
		}
		stats.EdgesWritten++
	}
	stats.EdgeDuration = time.Since(startEdges)

	return stats, nil
}

// Prune elimina nodos cuyo path NO está en `keepPaths`.
// Devuelve la cantidad de nodos eliminados.
func Prune(kuzuPath string, keepPaths []string) (int, error) {
	keep := make(map[string]bool, len(keepPaths))
	for _, p := range keepPaths {
		keep[p] = true
	}

	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return 0, fmt.Errorf("open Kuzu: %w", err)
	}
	defer conn.Close()

	// Listar todos los nodos.
	var orphans []string
	err = conn.Query("MATCH (n:File) RETURN n.path AS p",
		func(row map[string]any) bool {
			p, _ := row["p"].(string)
			if !keep[p] {
				orphans = append(orphans, p)
			}
			return true
		})
	if err != nil {
		return 0, fmt.Errorf("list nodes: %w", err)
	}

	// Borrar uno por uno (Kuzu no soporta DELETE WHERE en una sola query).
	pruned := 0
	for _, p := range orphans {
		q := fmt.Sprintf("MATCH (n:File {path: '%s'}) DETACH DELETE n", escapeCypher(p))
		if _, err := conn.Execute(q); err != nil {
			return pruned, fmt.Errorf("delete node %s: %w", p, err)
		}
		pruned++
	}
	return pruned, nil
}

// collectMarkdownFiles recorre el vault y retorna todas las rutas .md
// relativas, excluyendo .git/, .obsidian/ y Planes/LLM-Wiki/graphrag/.
func collectMarkdownFiles(vaultPath string) ([]string, error) {
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
		relPath = strings.ReplaceAll(relPath, "\\", "/")
		files = append(files, relPath)
		return nil
	})
	return files, err
}

// wikiLinkRegex captura [[link]] o [[link|alias]].
var wikiLinkRegex = regexp.MustCompile(`\[\[([^\]|#]+)(?:#[^\]|]*)?(?:\|[^\]]*)?\]\]`)

// extractWikilinks lee un archivo y retorna sus wikilinks (sin alias ni heading).
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

// resolveWikilink intenta resolver un wikilink a un path relativo al vault.
// Retorna "" si no se puede resolver.
func resolveWikilink(vaultPath, fromPath, link string) string {
	// Estrategia: probar variantes del link como archivo .md.
	candidates := []string{
		link + ".md",
		filepath.Join(filepath.Dir(fromPath), link) + ".md",
		filepath.Join(filepath.Dir(fromPath), link, "index.md"),
	}
	for _, c := range candidates {
		full := filepath.Join(vaultPath, c)
		if _, err := os.Stat(full); err == nil {
			return strings.ReplaceAll(c, "\\", "/")
		}
	}
	// Si el link apunta a un archivo no-.md que existe, también lo aceptamos.
	// Ej: .drawio, .html.
	candidatesNonMd := []string{
		link,
		filepath.Join(filepath.Dir(fromPath), link),
	}
	for _, c := range candidatesNonMd {
		full := filepath.Join(vaultPath, c)
		if _, err := os.Stat(full); err == nil {
			return strings.ReplaceAll(c, "\\", "/")
		}
	}
	return ""
}

// escapeCypher escapa backslash y comilla simple.
func escapeCypher(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}