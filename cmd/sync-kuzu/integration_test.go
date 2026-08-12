// Test de integración para sync-kuzu.
//
// Crea un mini-vault sintético en un directorio temporal, ejecuta
// syncVault() contra él, y verifica:
//   1. Todos los archivos .md se migran como nodos.
//   2. Los wikilinks a archivos .md se migran como aristas ENLAZA.
//   3. Los wikilinks a archivos no-.md (.html, .drawio) también se migran
//      (fix del bug de Fase 4 — el script crea los nodos destino).
//   4. Los archivos inexistentes (carcasas) NO generan aristas
//      (resolve retorna "").
//   5. Re-sync es idempotente: correr 2 veces no duplica.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncKuzuIntegration(t *testing.T) {
	// 1. Crear mini-vault sintético.
	// c.md apunta a d.html. archivo.drawio existe pero NO es destino
	// de ningún wikilink (queda como nodo huérfano de un sync anterior).
	tmpDir := t.TempDir()
	files := map[string]string{
		"a.md":            "Enlace a [[b]] y a [[c]].\n",
		"b.md":            "Enlace a [[c]].\n",
		"c.md":            "Enlace a [[d.html]] (no-md).\n",
		"d.html":          "<html>body</html>\n",
		"e.md":            "Enlace a [[inexistente.md]] (carcasa, no se migra).\n",
		"subdir/f.md":     "Anidado.\n",
		"archivo.drawio":  "<diagram>...</diagram>\n",
	}
	for rel, content := range files {
		full := filepath.Join(tmpDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	// 2. Correr sync contra Kuzu temporal.
	kuzuPath := filepath.Join(tmpDir, "test.kuzu")
	if err := syncVault(tmpDir, kuzuPath, false); err != nil {
		t.Fatalf("primer sync: %v", err)
	}

	// 3. Verificar contenido en Kuzu.
	_, kzCount, kzEdges, err := openAndCount(kuzuPath)
	if err != nil {
		t.Fatalf("open Kuzu: %v", err)
	}

	// Esperado: 5 archivos .md (a, b, c, e, subdir/f) como nodos.
	// + 1 destino no-.md (d.html, destino de arista c->d.html).
	// archivo.drawio existe pero NO es destino de arista, no se crea.
	// Total: 6 nodos.
	if kzCount != 6 {
		t.Errorf("nodos: got %d, esperado 6 (5 .md + 1 destino no-.md con arista)", kzCount)
	}

	// Esperado: 4 aristas (a->b, a->c, b->c, c->d.html).
	// e->inexistente no se migra (resolve retorna "").
	if kzEdges != 4 {
		t.Errorf("aristas: got %d, esperado 4", kzEdges)
	}

	// 4. Verificar que la arista a d.html existe (test del fix de Fase 4).
	htmlCount := 0
	conn, _, _, err := openAndCount(kuzuPath)
	if err != nil {
		t.Fatalf("open Kuzu 2: %v", err)
	}
	defer conn.Close()
	err = conn.Query(
		"MATCH (a:File {path: 'c.md'})-[r]->(b:File) WHERE b.path ENDS WITH '.html' RETURN b.path",
		func(row map[string]any) bool {
			htmlCount++
			return true
		})
	if err != nil {
		t.Fatalf("query html: %v", err)
	}
	if htmlCount != 1 {
		t.Errorf("arista a .html: got %d, esperado 1", htmlCount)
	}

	// 5. Re-sync es idempotente: no debe duplicar.
	if err := syncVault(tmpDir, kuzuPath, false); err != nil {
		t.Fatalf("segundo sync: %v", err)
	}
	_, kzCount2, kzEdges2, err := openAndCount(kuzuPath)
	if err != nil {
		t.Fatalf("open Kuzu 2: %v", err)
	}
	if kzCount2 != kzCount {
		t.Errorf("idempotencia nodos: primer=%d, segundo=%d (deberían ser iguales)",
			kzCount, kzCount2)
	}
	if kzEdges2 != kzEdges {
		t.Errorf("idempotencia aristas: primer=%d, segundo=%d (deberían ser iguales)",
			kzEdges, kzEdges2)
	}
}
