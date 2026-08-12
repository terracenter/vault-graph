// Test de integración para sync-kuzu (wrapper CLI sobre internal/syncvault).
//
// Crea un mini-vault sintético en un directorio temporal, ejecuta
// syncvault.Sync() contra él, y verifica:
//   1. Todos los archivos .md se migran como nodos.
//   2. Aristas ENLAZA entre archivos se crean correctamente.
//   3. Archivos no-.md destino también reciben nodo (fix Fase 4).
//   4. Sync es idempotente.

package main

import (
	"os"
	"path/filepath"
	"testing"

	kuzu "github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
	syncvault "github.com/freddytaborda/vault-graph/internal/syncvault"
)

func TestSyncKuzuIntegration(t *testing.T) {
	// 1. Crear mini-vault sintético.
	// c.md apunta a d.html explícitamente (el sync-kuzu solo resuelve
	// cuando el wikilink matchea el archivo real con extensión correcta).
	// archivo.drawio existe pero NO es destino de ningún wikilink
	// (queda como nodo huérfano de un sync anterior, no se prueba aquí).
	tmpDir := t.TempDir()
	files := map[string]string{
		"a.md":           "Enlace a [[b]] y a [[c]].\n",
		"b.md":           "Enlace a [[c]].\n",
		"c.md":           "Enlace a [[d.html]].\n",
		"d.html":         "<html>dummy</html>",
		"e.md":           "Sin enlaces.\n",
		"subdir/f.md":    "Sin enlaces.\n",
	}
	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		if err := writeFile(path, content); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	kuzuPath := filepath.Join(tmpDir, "vault.kuzu")

	// 2. Primera sync.
	stats, err := syncvault.Sync(tmpDir, kuzuPath, false)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	// 3. Verificaciones.
	// 5 archivos .md: a, b, c, e, subdir/f.
	// d.html y archivo.drawio son archivos no-.md que NO se cuentan como
	// archivos a migrar, pero el sync puede crear nodos para ellos si son
	// destino de wikilinks.
	if stats.NodeCount != 5 {
		t.Errorf("archivos detectados: got %d, esperado 5", stats.NodeCount)
	}
	// 3 aristas ENLAZA: a→b, a→c, b→c, c→d → 4 aristas detectadas
	// (el sync crea nodo para d.html al no existir).
	if stats.EdgesWritten != 4 {
		t.Errorf("aristas escritas: got %d, esperado 4", stats.EdgesWritten)
	}

	// 4. El nodo d.html debe existir aunque no sea .md.
	count, _, err := countKuzuAll(t, kuzuPath)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	// 5 .md + 1 nodo destino (d.html) = 6
	if count != 6 {
		t.Errorf("nodos en Kuzu: got %d, esperado 6 (5 .md + d.html)", count)
	}

	// 5. Re-sync es idempotente: no debe duplicar.
	_, err = syncvault.Sync(tmpDir, kuzuPath, false)
	if err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	count2, _, err := countKuzuAll(t, kuzuPath)
	if err != nil {
		t.Fatalf("count post re-sync: %v", err)
	}
	if count2 != 6 {
		t.Errorf("re-sync duplicó: got %d, esperado 6", count2)
	}
}

// writeFile helper para crear directorios padre y escribir el archivo.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// countKuzuAll retorna (nodos File, aristas ENLAZA, error) abriendo
// un archivo Kuzu y contando.
func countKuzuAll(t *testing.T, kuzuPath string) (int, int, error) {
	conn, err := kuzu.Open(kuzuPath)
	if err != nil {
		return 0, 0, err
	}
	defer conn.Close()

	var nodes, edges int
	err = conn.Query("MATCH (n:File) RETURN count(*)",
		func(row map[string]any) bool {
			for _, v := range row {
				if c, ok := v.(int64); ok {
					nodes = int(c)
				}
			}
			return true
		})
	if err != nil {
		return 0, 0, err
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
		return 0, 0, err
	}
	return nodes, edges, nil
}