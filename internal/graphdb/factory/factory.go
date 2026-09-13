package factory

import (
	"context"
	"fmt"

	"github.com/freddytaborda/vault-graph/config"
	"github.com/freddytaborda/vault-graph/internal/graphdb/age"
	"github.com/freddytaborda/vault-graph/internal/graphdb/kuzu"
)

func NewStore(ctx context.Context, cfg *config.Config) (graphdb.Store, error) {
	switch cfg.Backend {
	case "kuzu":
		return kuzu.NewStore(cfg.KuzuPath, cfg.VaultPath)
	case "age":
		return age.NewStore(ctx, cfg.DatabaseURL)
	default:
		return nil, fmt.Errorf("graphdb: backend desconocido %q", cfg.Backend)
	}
}
