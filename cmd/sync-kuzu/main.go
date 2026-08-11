// cmd/sync-kuzu: sincroniza el vault a una base de datos Kuzu embebida.
//
// Diferencias con cmd/sync (AGE):
//   - No usa transacciones pgx — Kuzu es single-writer, embebido.
//   - Schema fijo: NODE TABLE File + REL TABLE ENLAZA (idéntico al wrapper).
//   - Solo migra nodos con label File y aristas ENLAZA.
//   - Reescribe el archivo Kuzu desde cero en cada sync (idempotente).
//
// USO:
//   # Build
//   go build -o vg-kuzu-sync ./cmd/sync-kuzu
//
//   # Run con vault default (lee VAULT_PATH de env o usa ~/Workspace/Obsidian)
//   vg-kuzu-sync
//
//   # Run con vault específico y DB específica
//   vg-kuzu-sync --vault /path/to/vault --kuzu /path/to/vault.kuzu
//
//   # Dry-run (cuenta, no escribe)
//   vg-kuzu-sync --dry-run

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
)

func main() {
	vaultPath := flag.String("vault", "", "ruta al vault (default $VAULT_PATH o ~/Workspace/Obsidian)")
	kuzuPath := flag.String("kuzu", "", "ruta al archivo Kuzu destino (default /home/freddy/Workspace/.agents/kuzu/vault.kuzu)")
	dryRun := flag.Bool("dry-run", false, "solo contar, no escribir (default false)")
	flag.Parse()

	if *vaultPath == "" {
		*vaultPath = os.Getenv("VAULT_PATH")
	}
	if *vaultPath == "" {
		home, _ := os.UserHomeDir()
		*vaultPath = filepath.Join(home, "Workspace", "Obsidian")
	}
	if *kuzuPath == "" {
		*kuzuPath = "/home/freddy/Workspace/.agents/kuzu/vault.kuzu"
	}

	fmt.Printf("=== sync vault → Kuzu ===\n")
	fmt.Printf("Vault:   %s\n", *vaultPath)
	fmt.Printf("Kuzu:    %s\n", *kuzuPath)
	fmt.Printf("DryRun:  %v\n", *dryRun)

	if err := syncVault(*vaultPath, *kuzuPath, *dryRun); err != nil {
		log.Fatalf("sync: %v", err)
	}
}

// syncVault ejecuta el sync completo del vault a Kuzu.
// Es la lógica pura del sync, separada de main() para permitir testing.
// Retorna error fatal; main() lo reporta y termina.
func syncVault(vaultPath, kuzuPath string, dryRun bool) error {
	if _, err := os.Stat(vaultPath); err != nil {
		return fmt.Errorf("vault no accesible: %w", err)
	}

	allPaths, err := collectMarkdownFiles(vaultPath)
	if err != nil {
		return fmt.Errorf("collect markdown: %w", err)
	}
	fmt.Printf("Vault:   %d archivos .md\n", len(allPaths))

	if dryRun {
		fmt.Printf("\n=== DRY RUN — no se escribió nada ===\n")
		fmt.Printf("Para escribir, usar: --dry-run=false\n")
		return nil
	}

	// Borrar DB previa (rewrite limpio)
	if err := os.Remove(kuzuPath); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: remove %s: %v", kuzuPath, err)
	}
	if err := os.Remove(kuzuPath + ".wal"); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: remove %s.wal: %v", kuzuPath, err)
	}

	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return fmt.Errorf("open Kuzu: %w", err)
	}
	defer conn.Close()

	fmt.Printf("Kuzu:    abierto OK\n")

	// Escribir nodos
	fmt.Printf("Kuzu:    escribiendo %d nodos...\n", len(allPaths))
	inicio := time.Now()
	for _, path := range allPaths {
		q := fmt.Sprintf("CREATE (n:File {path: '%s'}) RETURN count(*)",
			escapeCypherString(path))
		if _, err := conn.Execute(q); err != nil {
			log.Printf("error CREATE %s: %v", path, err)
		}
	}
	fmt.Printf("Kuzu:    %d nodos en %v\n", len(allPaths), time.Since(inicio))

	// Aristas ENLAZA: escanear wikilinks [[...]] en cada .md.
	// MVP: solo ENLAZA (las más importantes). Tags/hosts se omiten.
	fmt.Printf("Kuzu:    escaneando wikilinks en %d archivos...\n", len(allPaths))
	aristas := []arista{}
	for _, path := range allPaths {
		full := filepath.Join(vaultPath, path)
		content, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		for _, link := range extractWikilinks(string(content)) {
			resolved := resolve(vaultPath, path, link)
			if resolved != "" {
				aristas = append(aristas, arista{from: path, to: resolved})
			}
		}
	}
	fmt.Printf("Kuzu:    %d aristas ENLAZA detectadas\n", len(aristas))

	// Escribir aristas. Antes de cada CREATE, asegurar que el nodo destino
	// existe en Kuzu. Sin esto, las aristas a archivos no-.md (ej. .drawio,
	// .html) fallan silenciosamente porque el MATCH (b:File {path: ...})
	// no encuentra el nodo. Usamos MERGE (idempotente) en lugar de CREATE
	// porque los nodos pueden ya existir (creados en la pasada de nodos .md
	// o en iteraciones previas del loop).
	fmt.Printf("Kuzu:    escribiendo aristas...\n")
	inicio = time.Now()
	edgeOK := 0
	edgeFail := 0
	for _, e := range aristas {
		// MERGE del destino: crea si no existe, no-op si existe.
		if _, err := conn.Execute(fmt.Sprintf(
			"MERGE (n:File {path: '%s'})",
			escapeCypherString(e.to))); err != nil {
			log.Printf("error MERGE destino %s: %v", e.to, err)
			edgeFail++
			continue
		}
		// MERGE de la arista: crea si no existe, no-op si existe.
		// Evita duplicados al re-sync.
		q := fmt.Sprintf(
			"MATCH (a:File {path: '%s'}), (b:File {path: '%s'}) MERGE (a)-[:ENLAZA]->(b)",
			escapeCypherString(e.from), escapeCypherString(e.to))
		if _, err := conn.Execute(q); err != nil {
			log.Printf("error MERGE arista %s -> %s: %v", e.from, e.to, err)
			edgeFail++
			continue
		}
		edgeOK++
	}
	fmt.Printf("Kuzu:    %d/%d aristas en %v (%d fallos)\n",
		edgeOK, len(aristas), time.Since(inicio), edgeFail)

	// Verificación final
	fmt.Printf("\n=== verificación final ===\n")
	kzCount, kzEdges, err := countAll(conn)
	if err != nil {
		return fmt.Errorf("verify final: %w", err)
	}
	fmt.Printf("Kuzu:    %d nodos, %d aristas\n", kzCount, kzEdges)
	return nil
}

// countAll retorna (conn-ready, nodos, aristas, error).
// Útil para tests: devuelve los conteos sin imprimir.
func countAll(conn *kuzu.Conn) (int, int, error) {
	var nodes, edges int
	err := conn.Query("MATCH (n:File) RETURN count(*)",
		func(row map[string]any) bool {
			for _, v := range row {
				if c, ok := v.(int64); ok {
					nodes = int(c)
				}
			}
			return true
		})
	if err != nil {
		return 0, 0, fmt.Errorf("count nodos: %w", err)
	}
	err = conn.Query("MATCH ()-[r:ENLAZA]->() RETURN count(*)",
		func(row map[string]any) bool {
			for _, v := range row {
				if c, ok := v.(int64); ok {
					edges = int(c)
				}
			}
			return true
		})
	if err != nil {
		return 0, 0, fmt.Errorf("count aristas: %w", err)
	}
	return nodes, edges, nil
}

// openAndCount abre una DB Kuzu y retorna (conn, nodos, aristas, error).
// Helper para tests de integración.
func openAndCount(kuzuPath string) (*kuzu.Conn, int, int, error) {
	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("open: %w", err)
	}
	nodes, edges, err := countAll(conn)
	if err != nil {
		conn.Close()
		return nil, 0, 0, err
	}
	return conn, nodes, edges, nil
}

type arista struct{ from, to string }

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
		rel, err := filepath.Rel(vaultPath, path)
		if err != nil {
			return err
		}
		rel = strings.ReplaceAll(rel, "\\", "/")
		files = append(files, rel)
		return nil
	})
	return files, err
}

// extractWikilinks extrae targets [[X]] del body.
// MVP: regex simple (sin alias, sin escape).
func extractWikilinks(body string) []string {
	var out []string
	i := 0
	for i < len(body)-3 {
		if body[i] == '[' && body[i+1] == '[' {
			// buscar cierre ]]
			j := i + 2
			for j < len(body)-1 {
				if body[j] == ']' && body[j+1] == ']' {
					inner := body[i+2 : j]
					// tomar target antes de | si hay alias
					target := inner
					if pipe := strings.Index(inner, "|"); pipe > 0 {
						target = inner[:pipe]
					}
					target = strings.TrimSpace(target)
					if target != "" {
						out = append(out, target)
					}
					i = j + 2
					goto next
				}
				j++
			}
		}
		i++
	next:
	}
	return out
}

// resolve mapea un wikilink target a un path real del vault.
// MVP: solo targets que ya son paths válidos (con o sin .md).
func resolve(vaultPath, fromPath, target string) string {
	// Caso 1: target ya es un path completo del vault (visto en migración AGE).
	if fileExists(vaultPath, target) {
		return target
	}
	if fileExists(vaultPath, target+".md") {
		return target + ".md"
	}
	// Caso 2: target es basename, buscar recursivamente.
	if !strings.Contains(target, "/") {
		base := filepath.Base(target)
		_ = base
		// MVP: omitir — solo resuelve paths exactos.
		// TODO Fase 4.1: basename resolution iterando allPaths.
	}
	return ""
}

func fileExists(vaultPath, rel string) bool {
	full := filepath.Join(vaultPath, rel)
	info, err := os.Stat(full)
	if err != nil {
		return false
	}
	return !info.IsDir()
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
