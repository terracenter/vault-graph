package kuzu

import (
	"path/filepath"
	"testing"
)

// helper para tests: abre DB fresca con schema aplicado.
func openFresh(t *testing.T) (*Conn, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.kuzu")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return conn, func() {
		conn.Close()
	}
}

// TestExecute_crearNodos valida Execute con CREATE.
// NOTA Kuzu: CREATE sin RETURN reporta GetNumberOfRows()=0 (contrato de la API).
// Para reportar filas afectadas, usar "CREATE ... RETURN n" o "RETURN count(*)"
// (ver docs: https://kuzudb.github.io/docs/cypher/difference, sección FINISH).
func TestExecute_crearNodos(t *testing.T) {
	c, cleanup := openFresh(t)
	defer cleanup()

	// Kuzu retorna 0 filas para CREATE sin RETURN (no es bug, es contrato).
	n, err := c.Execute("CREATE (n:File {path: 'a.md'})")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if n != 0 {
		t.Errorf("CREATE sin RETURN: filas afectadas = %d, esperado 0 (contrato Kuzu)", n)
	}

	// Con RETURN count(*) SÍ retorna el conteo correcto.
	n2, err := c.Execute("CREATE (n:File {path: 'b.md'}) RETURN count(*)")
	if err != nil {
		t.Fatalf("Execute con RETURN: %v", err)
	}
	if n2 != 1 {
		t.Errorf("CREATE RETURN count(*): filas afectadas = %d, esperado 1", n2)
	}

	// Segundo CREATE del mismo path: debe fallar (PK duplicada).
	_, err = c.Execute("CREATE (n:File {path: 'b.md'}) RETURN n")
	if err == nil {
		t.Error("segundo CREATE con PK duplicada debería fallar")
	}
}

// TestQuery_MatchNodos valida Query con MATCH y callback.
func TestQuery_MatchNodos(t *testing.T) {
	c, cleanup := openFresh(t)
	defer cleanup()

	for _, p := range []string{"a.md", "b.md", "c.md"} {
		if _, err := c.Execute("CREATE (n:File {path: '" + p + "'})"); err != nil {
			t.Fatalf("setup Execute: %v", err)
		}
	}

	var paths []string
	err := c.Query("MATCH (n:File) RETURN n.path AS path", func(row map[string]any) bool {
		if p, ok := row["path"].(string); ok {
			paths = append(paths, p)
		}
		return true
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(paths) != 3 {
		t.Errorf("got %d rows, expected 3: %v", len(paths), paths)
	}
}

// TestQuery_MatchAristas valida Query con MATCH-aristas, como usa vault-graph.
func TestQuery_MatchAristas(t *testing.T) {
	c, cleanup := openFresh(t)
	defer cleanup()

	// Setup
	for _, p := range []string{"a.md", "b.md", "c.md"} {
		if _, err := c.Execute("CREATE (n:File {path: '" + p + "'})"); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	for _, pair := range []struct{ from, to string }{
		{"a.md", "b.md"},
		{"b.md", "c.md"},
	} {
		q := "MATCH (a:File {path: '" + pair.from + "'}), (b:File {path: '" + pair.to + "'}) CREATE (a)-[:ENLAZA]->(b)"
		if _, err := c.Execute(q); err != nil {
			t.Fatalf("setup arista: %v", err)
		}
	}

	// Query aristas
	var edges []string
	err := c.Query("MATCH (a:File)-[r]->(b:File) RETURN a.path, label(r), b.path",
		func(row map[string]any) bool {
			a, _ := row["a.path"].(string)
			rel, _ := row["label(r)"].(string)
			b, _ := row["b.path"].(string)
			edges = append(edges, a+" -["+rel+"]-> "+b)
			return true
		})
	if err != nil {
		t.Fatalf("Query aristas: %v", err)
	}
	if len(edges) != 2 {
		t.Errorf("got %d aristas, expected 2: %v", len(edges), edges)
	}
}

// TestQuery_filtroLabels valida el filtro de detección de label-less.
// FORMA CORRECTA en Kuzu: label(n) = '' (singular, NO labels(n)[0] = '').
// Cita oficial: https://kuzudb.github.io/docs/cypher/difference (sección WHERE):
//   "MATCH (n) WHERE label(n) = 'Person' RETURN n"
// labels(n)[0] = '' NO funciona en Kuzu porque labels(n) retorna string, no lista.
func TestQuery_filtroLabels(t *testing.T) {
	c, cleanup := openFresh(t)
	defer cleanup()

	// Crear un nodo File (tiene label)
	if _, err := c.Execute("CREATE (n:File {path: 'a.md'}) RETURN n"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// label(n) = '' debería NO matchear (todos tienen label File)
	count := 0
	err := c.Query("MATCH (n) WHERE label(n) = '' RETURN n.path",
		func(row map[string]any) bool {
			count++
			return true
		})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if count != 0 {
		t.Errorf("label(n) = '': got %d nodos label-less, esperado 0", count)
	}

	// label(n) = 'File' SÍ matchea (control positivo)
	countPositive := 0
	err = c.Query("MATCH (n) WHERE label(n) = 'File' RETURN n.path",
		func(row map[string]any) bool {
			countPositive++
			return true
		})
	if err != nil {
		t.Fatalf("Query positivo: %v", err)
	}
	if countPositive != 1 {
		t.Errorf("label(n) = 'File': got %d nodos, esperado 1", countPositive)
	}
}

// TestQueryRows_materializa verifica el helper de materialización.
func TestQueryRows_materializa(t *testing.T) {
	c, cleanup := openFresh(t)
	defer cleanup()

	for _, p := range []string{"x.md", "y.md"} {
		if _, err := c.Execute("CREATE (n:File {path: '" + p + "'})"); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	rows, err := c.QueryRows("MATCH (n:File) RETURN n.path AS p, n.idx AS i")
	if err != nil {
		t.Fatalf("QueryRows: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("got %d rows, expected 2", len(rows))
	}
	// Cada row debe ser independiente (test de copia)
	if len(rows) >= 2 {
		rows[0]["p"] = "MUTATED"
		if rows[1]["p"] == "MUTATED" {
			t.Error("filas comparten memoria; QueryRows no está copiando")
		}
	}
}

// TestExecute_count valida que Execute reporta filas afectadas.
func TestExecute_count(t *testing.T) {
	c, cleanup := openFresh(t)
	defer cleanup()

	// SET afecta 0 filas si no hay match previo
	n, err := c.Execute("MATCH (n:File {path: 'nope.md'}) SET n.idx = 1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if n != 0 {
		t.Errorf("SET sin match debería afectar 0 filas, got %d", n)
	}
}

// TestExecute_queryMalformada retorna error descriptivo.
func TestExecute_queryMalformada(t *testing.T) {
	c, cleanup := openFresh(t)
	defer cleanup()

	_, err := c.Execute("THIS IS NOT CYPHER")
	if err == nil {
		t.Error("query malformada debería retornar error")
	}
}

// TestQuery_callbackEarlyStop valida que el callback puede detener iteración.
func TestQuery_callbackEarlyStop(t *testing.T) {
	c, cleanup := openFresh(t)
	defer cleanup()

	for _, p := range []string{"a.md", "b.md", "c.md"} {
		if _, err := c.Execute("CREATE (n:File {path: '" + p + "'})"); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	count := 0
	err := c.Query("MATCH (n:File) RETURN n.path", func(row map[string]any) bool {
		count++
		return count < 2 // detener después de la segunda fila
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if count != 2 {
		t.Errorf("callback se llamó %d veces, esperaba exactamente 2", count)
	}
}
