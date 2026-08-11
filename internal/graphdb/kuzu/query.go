// query.go: métodos de alto nivel para ejecutar Cypher en Kuzu.
//
// Este archivo agrega dos operaciones al wrapper Kuzu:
//   - Execute(query): para queries de escritura (CREATE, MERGE, DELETE, SET).
//     Retorna el conteo de filas afectadas.
//   - Query(query): para queries de lectura (MATCH ... RETURN ...).
//     Itera los resultados via callback hasta que no haya más filas.
//
// Diseñado para reemplazar gradualmente los métodos que usan AGE en
// internal/graphdb/queries.go (lecturas) y cypher.go (escrituras).
//
// Limitaciones actuales:
//   - No soporta query parameters ($name). Las queries se interpolan como
//     strings (alineado con cómo cypher.go ya trabaja en AGE).
//   - No soporta transacciones explícitas. El loader existente usa
//     conn.BeginTx() de pgx; cuando adaptemos el loader a Kuzu, este archivo
//     deberá extender para soportar tx (siguiente fase).

package kuzu

import (
	"fmt"

	kuzudb "github.com/kuzudb/go-kuzu"
)

// RowCallback se llama por cada fila retornada por Query().
// Retorna false para detener la iteración temprano.
type RowCallback func(row map[string]any) bool

// Execute ejecuta una query Cypher de escritura.
// Retorna el número de filas afectadas.
//
// Uso típico: CREATE, MERGE, DELETE, SET, REMOVE.
func (c *Conn) Execute(query string) (int64, error) {
	if c == nil || c.conn == nil {
		return 0, fmt.Errorf("kuzu.Execute: conexión nula")
	}

	stmt, err := c.conn.Prepare(query)
	if err != nil {
		return 0, fmt.Errorf("prepare %q: %w", query, err)
	}
	defer stmt.Close()

	result, err := c.conn.Execute(stmt, nil)
	if err != nil {
		return 0, fmt.Errorf("execute %q: %w", query, err)
	}
	defer result.Close()

	return int64(result.GetNumberOfRows()), nil
}

// Query ejecuta una query Cypher de lectura e itera los resultados.
//
// cb se llama por cada fila con un map columna→valor. Si cb retorna false,
// la iteración se detiene.
//
// Uso típico: MATCH ... RETURN ...
func (c *Conn) Query(query string, cb RowCallback) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("kuzu.Query: conexión nula")
	}

	stmt, err := c.conn.Prepare(query)
	if err != nil {
		return fmt.Errorf("prepare %q: %w", query, err)
	}
	defer stmt.Close()

	result, err := c.conn.Execute(stmt, nil)
	if err != nil {
		return fmt.Errorf("execute %q: %w", query, err)
	}
	defer result.Close()

	columns := result.GetColumnNames()
	numCols := result.GetNumberOfColumns()

	for result.HasNext() {
		tup, err := result.Next()
		if err != nil {
			return fmt.Errorf("next row: %w", err)
		}

		row := make(map[string]any, numCols)
		for i := uint64(0); i < numCols; i++ {
			v, err := tup.GetValue(i)
			if err != nil {
				tup.Close()
				return fmt.Errorf("get value[%d]: %w", i, err)
			}
			// columns[i] es seguro: GetNumberOfColumns == len(GetColumnNames)
			row[columns[i]] = v
		}
		tup.Close()

		if !cb(row) {
			break
		}
	}

	return nil
}

// QueryRows ejecuta una query y retorna todas las filas como slice de maps.
// Convenience helper para tests y casos donde quieres materializar todo.
//
// Para grafos grandes, preferí Query() con callback para no cargar todo en RAM.
func (c *Conn) QueryRows(query string) ([]map[string]any, error) {
	var rows []map[string]any
	err := c.Query(query, func(row map[string]any) bool {
		// Copia el map para que cada fila sea independiente
		cp := make(map[string]any, len(row))
		for k, v := range row {
			cp[k] = v
		}
		rows = append(rows, cp)
		return true
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// QueryResult alias para mantener compatibilidad con código que espera
// el tipo de go-kuzu (re-exportamos para callers que quieran control fino).
type QueryResult = kuzudb.QueryResult
