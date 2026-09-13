package kuzu

import (
	"context"
	"path/filepath"
	"testing"
)

// helper para store_test.go
func setupStore(t *testing.T) (*Store, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.kuzu")
	vaultPath := filepath.Join(dir, "vault")
	store, err := NewStore(dbPath, vaultPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store, func() {
		store.Close()
	}
}

func TestStore_MergeNodeEdge(t *testing.T) {
	s, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()

	err := s.MergeNode(ctx, "a.md")
	if err != nil {
		t.Fatalf("MergeNode a.md: %v", err)
	}
	err = s.MergeNode(ctx, "b.md")
	if err != nil {
		t.Fatalf("MergeNode b.md: %v", err)
	}
	err = s.MergeEdge(ctx, "a.md", "b.md")
	if err != nil {
		t.Fatalf("MergeEdge a->b: %v", err)
	}

	paths, err := s.ListPaths(ctx, struct{}{})
	if err != nil {
		t.Fatalf("ListPaths: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("got %d paths, expected 2", len(paths))
	}
}
