package graphdb

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Conn mantiene la conexión a PostgreSQL con AGE
type Conn struct {
	pool *pgxpool.Pool
}

// NewConn crea una nueva conexión a PostgreSQL
func NewConn(ctx context.Context, databaseURL string) (*Conn, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	// Ping para verificar conectividad
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	conn := &Conn{pool: pool}

	// Cargar extensión AGE y establecer search_path
	if err := conn.setupAGE(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to setup AGE: %w", err)
	}

	return conn, nil
}

// setupAGE carga la extensión AGE y establece el search_path
func (c *Conn) setupAGE(ctx context.Context) error {
	// LOAD 'age'
	if _, err := c.pool.Exec(ctx, "LOAD 'age'"); err != nil {
		return fmt.Errorf("failed to load age: %w", err)
	}

	// SET search_path
	if _, err := c.pool.Exec(ctx, "SET search_path = ag_catalog, \"$user\", public"); err != nil {
		return fmt.Errorf("failed to set search_path: %w", err)
	}

	return nil
}

// Close cierra la conexión
func (c *Conn) Close() {
	if c.pool != nil {
		c.pool.Close()
	}
}

// Pool retorna el pool de conexiones (para acceso directo si es necesario)
func (c *Conn) Pool() *pgxpool.Pool {
	return c.pool
}
