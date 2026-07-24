package graphdb

import (
	"context"
	"fmt"
	"strings"
)

// PruneOrphanNodes elimina del grafo cualquier nodo (que represente un
// archivo .md) cuyo path no esté en validPaths. Usa DETACH DELETE para borrar
// también las aristas asociadas. Devuelve la cantidad de nodos eliminados.
//
// Importante: solo se borran nodos cuyo path termina en ".md" para preservar
// nodos virtuales como Tag y Host (cuyo path no es un archivo del vault).
//
// Estrategia: primero recolecta los paths de los nodos candidatos con un
// SELECT puro, luego ejecuta un DELETE con esos paths en una cláusula IN.
// Esto evita ambigüedades de `count(n)` post-DELETE en AGE.
//
// Convención: si validPaths está vacío, no hace nada (fail-safe).
func (c *Conn) PruneOrphanNodes(ctx context.Context, validPaths []string) (int, error) {
	if len(validPaths) == 0 {
		return 0, nil
	}

	// Construir lista IN con strings escapados (patrón del proyecto: fmt.Sprintf)
	parts := make([]string, len(validPaths))
	for i, p := range validPaths {
		parts[i] = fmt.Sprintf("'%s'", escapeString(p))
	}
	pathsList := strings.Join(parts, ", ")

	// Paso 1: recolectar los paths de los nodos huérfanos (sin tocar nada)
	selectCypher := fmt.Sprintf(`
	  MATCH (n)
	  WHERE n.path ENDS WITH '.md' AND NOT n.path IN [%s]
	  RETURN n.path AS p
	`, pathsList)
	selectQuery := fmt.Sprintf(`SELECT * FROM cypher('vault', $$%s$$) AS (p agtype);`, selectCypher)

	rows, err := c.pool.Query(ctx, selectQuery)
	if err != nil {
		return 0, fmt.Errorf("prune orphan nodes (select) failed: %w", err)
	}
	defer rows.Close()

	var orphanPaths []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return 0, fmt.Errorf("scan prune row: %w", err)
		}
		path := trimJSON(raw)
		if path != "" {
			orphanPaths = append(orphanPaths, path)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("rows err: %w", err)
	}
	rows.Close()

	// Si no hay huérfanos, no ejecutar DELETE
	if len(orphanPaths) == 0 {
		return 0, nil
	}

	// Paso 2: ejecutar DETACH DELETE con los paths recolectados
	deleteParts := make([]string, len(orphanPaths))
	for i, p := range orphanPaths {
		deleteParts[i] = fmt.Sprintf("'%s'", escapeString(p))
	}
	orphanList := strings.Join(deleteParts, ", ")

	deleteCypher := fmt.Sprintf(`
	  MATCH (n)
	  WHERE n.path IN [%s]
	  DETACH DELETE n
	`, orphanList)
	deleteQuery := fmt.Sprintf(`SELECT * FROM cypher('vault', $$%s$$) AS (result agtype);`, deleteCypher)

	if _, err := c.pool.Exec(ctx, deleteQuery); err != nil {
		return 0, fmt.Errorf("prune orphan nodes (delete) failed: %w", err)
	}

	return len(orphanPaths), nil
}
