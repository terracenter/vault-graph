package age

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store mantiene la conexión a PostgreSQL con AGE
type Store struct {
	pool *pgxpool.Pool
}

// NewStore crea un nuevo conexión a PostgreSQL
func NewStore(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	// Ping para verificar conectividad
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	store := &Store{pool: pool}

	// Cargar extensión AGE y establecer search_path
	if err := store.setupAGE(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to setup AGE: %w", err)
	}

	return store, nil
}

// setupAGE carga la extensión AGE y establece el search_path
func (c *Store) setupAGE(ctx context.Context) error {
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
func (c *Store) Close() error {
	if c.pool != nil {
		c.pool.Close()
	}
	return nil
}

// Pool retorna el pool de conexiones (para acceso directo si es necesario)
func (c *Store) Pool() *pgxpool.Pool {
	return c.pool
}
