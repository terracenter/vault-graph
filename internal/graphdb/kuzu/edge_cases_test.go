// Edge cases documentados durante la migración AGE → Kuzu.
//
// Estos tests cubren comportamiento específico de Apache Kuzu v0.11.3
// que es distinto a Apache AGE o Neo4j. La documentación oficial está en:
//
//   ~/Workspace/Obsidian/06_Desarrollo/kuzu-notas-oficiales-2026-08-11.md
//
// Lo que cubre este archivo:
//   1. label(n) vs labels(n)[0] (la trampa de Fase 4).
//   2. Columna de count(*) se llama COUNT_STAR() (no "count(*)").
//   3. Schema explícito: PK duplicada falla con error claro.
//   4. Re-apertura de DB preserva schema (idempotencia del applySchema).
//   5. CREATE sin RETURN reporta 0 filas (contrato de la API).
//   6. MERGE es idempotente (re-correr no falla ni duplica).

package kuzu

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestEdgeCase_label_vs_labels valida que label(n) es la forma correcta
// de filtrar por label en Kuzu. labels(n)[0] es incorrecto (siempre
// matchea porque labels(n) retorna string, no lista).
//
// Cita oficial: https://kuzudb.github.io/docs/cypher/difference (sección WHERE).
func TestEdgeCase_label_vs_labels(t *testing.T) {
	c, cleanup := openFresh(t)
	defer cleanup()

	if _, err := c.Execute("CREATE (n:File {path: 'a.md'}) RETURN n"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// label(n) = '' NO matchea (correcto: n tiene label File).
	countSingular := 0
	err := c.Query("MATCH (n) WHERE label(n) = '' RETURN n.path",
		func(row map[string]any) bool { countSingular++; return true })
	if err != nil {
		t.Fatalf("label query: %v", err)
	}
	if countSingular != 0 {
		t.Errorf("label(n) = '': got %d, esperado 0", countSingular)
	}

	// label(n) = 'File' SÍ matchea (control positivo).
	countFile := 0
	err = c.Query("MATCH (n) WHERE label(n) = 'File' RETURN n.path",
		func(row map[string]any) bool { countFile++; return true })
	if err != nil {
		t.Fatalf("label File query: %v", err)
	}
	if countFile != 1 {
		t.Errorf("label(n) = 'File': got %d, esperado 1", countFile)
	}

	// labels(n)[0] = '' es el patrón de AGE que NO funciona en Kuzu.
	// Kuzu trata labels(n) como string, no lista, y labels(n)[0]
	// retorna string vacío — matcheando TODO nodo que tenga label.
	// Este test documenta el bug conocido (Regla 8 del reference)
	// para que cualquier refactor futuro no lo rompa accidentalmente.
	countLabelsArr := 0
	err = c.Query("MATCH (n) WHERE labels(n)[0] = '' RETURN n.path",
		func(row map[string]any) bool { countLabelsArr++; return true })
	if err != nil {
		t.Fatalf("labels[n][0] query: %v", err)
	}
	// Comportamiento real: labels(n)[0] retorna "" para nodo con label,
	// así que el WHERE matchea siempre. Esperamos 1, no 0.
	if countLabelsArr != 1 {
		t.Errorf("labels(n)[0] = '': got %d, esperado 1 (comportamiento real de Kuzu; NO usar este patrón, usar label(n))", countLabelsArr)
	}
}

// TestEdgeCase_countStarColumnName valida que la columna de count(*)
// se llama COUNT_STAR() en el map de row, no "count(*)".
//
// Hallazgo empírico (Fase 2, sesión 2026-08-11): Kuzu nombra la
// columna COUNT_STAR() automáticamente. El wrapper debe iterar el map
// buscando el int64, no por nombre de columna.
func TestEdgeCase_countStarColumnName(t *testing.T) {
	c, cleanup := openFresh(t)
	defer cleanup()

	if _, err := c.Execute("CREATE (n:File {path: 'a.md'}) RETURN n"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	rows, err := c.QueryRows("MATCH (n:File) RETURN count(*) AS total")
	if err != nil {
		t.Fatalf("count query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d, esperado 1", len(rows))
	}
	// Verificar que la columna existe y es int64.
	row := rows[0]
	var found bool
	for _, v := range row {
		if c, ok := v.(int64); ok {
			if c != 1 {
				t.Errorf("count: got %d, esperado 1", c)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("count(*) no retornó int64 en ninguna columna, row: %#v", row)
	}
}

// TestEdgeCase_PKDuplicadaFalla valida que intentar crear un nodo con
// path que ya existe falla con error claro (no panic, no write silencioso).
func TestEdgeCase_PKDuplicadaFalla(t *testing.T) {
	c, cleanup := openFresh(t)
	defer cleanup()

	if _, err := c.Execute("CREATE (n:File {path: 'a.md'}) RETURN n"); err != nil {
		t.Fatalf("primer create: %v", err)
	}

	_, err := c.Execute("CREATE (n:File {path: 'a.md'}) RETURN n")
	if err == nil {
		t.Fatal("segundo create con PK duplicada debería fallar")
	}
	if !strings.Contains(err.Error(), "primary key") &&
		!strings.Contains(err.Error(), "uniqueness") {
		t.Errorf("error de PK duplicada no es claro: %v", err)
	}
}

// TestEdgeCase_ReAbrirPreservaSchema valida que cerrar y reabrir
// la DB preserva los datos (applySchema es idempotente).
func TestEdgeCase_ReAbrirPreservaSchema(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.kuzu")

	// Primera apertura: crear DB y agregar dato.
	c1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("primer open: %v", err)
	}
	if _, err := c1.Execute("CREATE (n:File {path: 'persistido.md'}) RETURN n"); err != nil {
		t.Fatalf("create: %v", err)
	}
	c1.Close()

	// Segunda apertura: debe detectar schema existente y no fallar.
	c2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("segundo open: %v", err)
	}
	defer c2.Close()

	// Verificar que el dato persiste.
	rows, err := c2.QueryRows("MATCH (n:File {path: 'persistido.md'}) RETURN n.path")
	if err != nil {
		t.Fatalf("query post-reopen: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("datos perdidos tras reopen: %d rows", len(rows))
	}
}

// TestEdgeCase_CreateSinRetunRetornaCero valida el contrato de la API:
// CREATE sin RETURN reporta GetNumberOfRows() = 0 (no bug, contrato).
// El wrapper Execute retorna este conteo al caller.
func TestEdgeCase_CreateSinRetunRetornaCero(t *testing.T) {
	c, cleanup := openFresh(t)
	defer cleanup()

	n, err := c.Execute("CREATE (n:File {path: 'a.md'})")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if n != 0 {
		t.Errorf("CREATE sin RETURN: got %d, esperado 0 (contrato Kuzu)", n)
	}

	// Con RETURN count(*) SÍ retorna 1.
	n, err = c.Execute("CREATE (n:File {path: 'b.md'}) RETURN count(*)")
	if err != nil {
		t.Fatalf("execute con RETURN: %v", err)
	}
	if n != 1 {
		t.Errorf("CREATE RETURN count(*): got %d, esperado 1", n)
	}
}

// TestEdgeCase_MergeEsIdempotente valida que re-correr el mismo MERGE
// no falla ni duplica. Esto es lo que permite re-sync sin errores.
func TestEdgeCase_MergeEsIdempotente(t *testing.T) {
	c, cleanup := openFresh(t)
	defer cleanup()

	// MERGE 3 veces el mismo nodo.
	for i := 0; i < 3; i++ {
		_, err := c.Execute("MERGE (n:File {path: 'a.md'})")
		if err != nil {
			t.Fatalf("merge iter %d: %v", i, err)
		}
	}

	// Verificar que hay UN solo nodo.
	rows, err := c.QueryRows("MATCH (n:File {path: 'a.md'}) RETURN n.path")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("MERGE no idempotente: got %d nodos, esperado 1", len(rows))
	}
}
