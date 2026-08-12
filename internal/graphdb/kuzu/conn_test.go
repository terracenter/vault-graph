package kuzu

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper: crea un directorio temporal para el test y devuelve la ruta
// del archivo DB + función de limpieza.
func setupDB(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.kuzu")
	cleanup := func() {
		_ = os.Remove(dbPath)
		_ = os.Remove(dbPath + ".wal")
	}
	return dbPath, cleanup
}

// TestOpen_creaNuevaDB verifica que Open() crea la DB y aplica el schema.
func TestOpen_creaNuevaDB(t *testing.T) {
	dbPath, cleanup := setupDB(t)
	defer cleanup()

	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer conn.Close()

	if !FileExists(dbPath) {
		t.Errorf("FileExists(%q) = false, esperado true tras Open", dbPath)
	}
}

// TestOpen_idempotente verifica que llamar Open() dos veces sobre el
// mismo path no falla (la segunda llamada detecta "already exists"
// en CREATE TABLE y lo ignora).
func TestOpen_idempotente(t *testing.T) {
	dbPath, cleanup := setupDB(t)
	defer cleanup()

	conn1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("primer Open() error = %v", err)
	}
	conn1.Close()

	conn2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("segundo Open() (idempotente) error = %v", err)
	}
	defer conn2.Close()
}

// TestOpen_pathVacio verifica que Open("") falla con error claro.
func TestOpen_pathVacio(t *testing.T) {
	_, err := Open("")
	if err == nil {
		t.Fatal("Open(\"\") debería retornar error, retornó nil")
	}
	if !strings.Contains(err.Error(), "dbPath") {
		t.Errorf("error debería mencionar dbPath, obtuvo: %v", err)
	}
}

// TestFileExists_verifica el helper.
func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nope.kuzu")
	if FileExists(dbPath) {
		t.Error("FileExists de archivo inexistente debería ser false")
	}

	// Crear archivo
	if err := os.WriteFile(dbPath, []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	if !FileExists(dbPath) {
		t.Error("FileExists de archivo existente debería ser true")
	}
}

// TestConn_Path verifica el getter.
func TestConn_Path(t *testing.T) {
	dbPath, cleanup := setupDB(t)
	defer cleanup()

	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer conn.Close()

	if conn.Path() != dbPath {
		t.Errorf("Path() = %q, esperado %q", conn.Path(), dbPath)
	}
}

// TestConn_Close_nilSafe verifica que Close() no falla con Conn nil.
func TestConn_Close_nilSafe(t *testing.T) {
	var c *Conn
	if err := c.Close(); err != nil {
		t.Errorf("Close() en Conn nil debería retornar nil, obtuvo: %v", err)
	}
}

// TestIsAlreadyExistsError verifica el detector de errores.
func TestIsAlreadyExistsError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"no contiene", errors.New("otro error"), false},
		{"contiene already exists", errors.New("Catalog exception: File already exists in catalog."), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAlreadyExistsError(tt.err); got != tt.want {
				t.Errorf("isAlreadyExistsError(%v) = %v, esperado %v", tt.err, got, tt.want)
			}
		})
	}
}
