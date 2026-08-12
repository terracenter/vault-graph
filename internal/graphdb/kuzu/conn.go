// Package kuzu provee un wrapper mínimo sobre github.com/kuzudb/go-kuzu
// para que vault-graph pueda usar Kuzu como almacén de grafos embebido,
// en lugar de Apache AGE sobre PostgreSQL.
//
// Kuzu es una base de datos de grafos embebida (no requiere servicio aparte),
// implementa Cypher, y persiste en un único archivo en disco. Esto resuelve
// dos limitaciones de AGE en este host:
//   1. Sin servicio extra: Kuzu vive dentro del binario vault-graph.
//   2. Backup nativo: un archivo único que se copia con `cp`.
//
// Schema inicial (alineado con el modelo de vault-graph):
//   - Nodos con label File, propiedades path (PK) + idx (INT64 opcional).
//   - Aristas con label ENLAZA (FROM File TO File).
//
// Este wrapper es solo Fase 1: abre/cierra DB y ejecuta queries crudos.
// Las fases siguientes adaptan queries.go y loader.go para usar este paquete
// en lugar del paquete graphdb existente (que habla con AGE).
package kuzu

import (
	"errors"
	"fmt"
	"os"

	kuzudb "github.com/kuzudb/go-kuzu"
)

// Conn envuelve una conexión a una base de datos Kuzu.
type Conn struct {
	db     *kuzudb.Database
	conn   *kuzudb.Connection
	dbPath string
}

// Open abre (o crea) una base de datos Kuzu en dbPath.
//
// Si el archivo de DB existe, lo abre. Si no, lo crea y aplica el schema
// mínimo para vault-graph (nodo File con PK en path, arista ENLAZA).
func Open(dbPath string) (*Conn, error) {
	if dbPath == "" {
		return nil, errors.New("kuzu: dbPath vacío")
	}

	db, err := kuzudb.OpenDatabase(dbPath, kuzudb.DefaultSystemConfig())
	if err != nil {
		return nil, fmt.Errorf("kuzu.OpenDatabase(%q): %w", dbPath, err)
	}

	conn, err := kuzudb.OpenConnection(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("kuzu.OpenConnection: %w", err)
	}

	c := &Conn{db: db, conn: conn, dbPath: dbPath}

	// Si la DB es nueva, aplicar schema. Si ya existe, esto fallará con
	// "already exists" — ese error lo ignoramos (idempotente).
	if err := c.applySchema(); err != nil {
		// Distinguir entre "ya existe" (OK) y error real
		if !isAlreadyExistsError(err) {
			_ = c.Close()
			return nil, fmt.Errorf("kuzu.applySchema: %w", err)
		}
	}

	return c, nil
}

// applySchema crea las tablas mínimas si no existen.
// Idempotente: si ya existen, retorna un error "already exists" que
// Open ignora.
//
// El schema se migra progresivamente. Si una DB existente fue creada
// con un schema anterior (sin la property "summary", por ejemplo),
// applySchema intenta agregar la property con ALTER. Si ALTER falla
// porque ya existe, se ignora.
func (c *Conn) applySchema() error {
	for _, q := range []string{
		"CREATE NODE TABLE File(path STRING, idx INT64, summary STRING, PRIMARY KEY(path))",
		"CREATE REL TABLE ENLAZA(FROM File TO File)",
	} {
		stmt, err := c.conn.Prepare(q)
		if err != nil {
			return fmt.Errorf("prepare %q: %w", q, err)
		}
		_, err = c.conn.Execute(stmt, nil)
		stmt.Close()
		if err != nil && !isAlreadyExistsError(err) {
			return err
		}
	}

	// Migración de DBs existentes: agregar properties que el schema
	// anterior no tenía. ALTER TABLE ... ADD PROPERTY es idempotente
	// (segunda ejecución falla con "already exists", que ignoramos).
	for _, q := range []string{
		"ALTER TABLE File ADD summary STRING",
	} {
		stmt, err := c.conn.Prepare(q)
		if err != nil {
			return fmt.Errorf("prepare %q: %w", q, err)
		}
		_, err = c.conn.Execute(stmt, nil)
		stmt.Close()
		if err != nil && !isAlreadyExistsError(err) {
			return fmt.Errorf("alter: %w", err)
		}
	}

	return nil
}

// isAlreadyExistsError detecta el error "already exists" de Kuzu para
// hacerlo idempotente. Captura múltiples variantes del mensaje:
//   - "already exists" (CREATE TABLE)
//   - "already has property" (ALTER TABLE ADD)
func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "already exists") || contains(msg, "already has property")
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Close libera los recursos de la conexión.
// Las funciones Close() de go-kuzu no retornan error (usan finalizer).
func (c *Conn) Close() error {
	if c == nil {
		return nil
	}
	if c.conn != nil {
		c.conn.Close()
	}
	if c.db != nil {
		c.db.Close()
	}
	return nil
}

// Path devuelve la ruta del archivo de DB.
func (c *Conn) Path() string {
	return c.dbPath
}

// FileExists devuelve true si ya existe un archivo DB en dbPath.
// Usado por callers para decidir entre crear y abrir.
func FileExists(dbPath string) bool {
	info, err := os.Stat(dbPath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
