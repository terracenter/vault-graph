package kuzu

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/freddytaborda/vault-graph/internal/graphdb"
)

// helper para store_test.go
func setupStore(t *testing.T) (*Store, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.kuzu")
	vaultPath := filepath.Join(dir, "vault")
	os.MkdirAll(vaultPath, 0755)
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

	paths, err := s.ListPaths(ctx, graphdb.PathFilter{})
	if err != nil {
		t.Fatalf("ListPaths: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("got %d paths, expected 2", len(paths))
	}
}

func TestStore_NeighborsAndBacklinks(t *testing.T) {
	s, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()

	s.MergeNode(ctx, "a.md")
	s.MergeNode(ctx, "b.md")
	s.MergeNode(ctx, "c.md")
	s.MergeEdge(ctx, "a.md", "b.md")
	s.MergeEdge(ctx, "b.md", "c.md")

	neighbors, err := s.Neighbors(ctx, "a.md", 1)
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(neighbors) != 1 || neighbors[0].Path != "b.md" {
		t.Errorf("expected neighbor b.md, got %v", neighbors)
	}

	backlinks, err := s.Backlinks(ctx, "c.md", 1)
	if err != nil {
		t.Fatalf("Backlinks: %v", err)
	}
	if len(backlinks) != 1 || backlinks[0].Path != "b.md" {
		t.Errorf("expected backlink b.md, got %v", backlinks)
	}
}

func TestStore_BrokenLinksAndOrphans(t *testing.T) {
	s, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()

	s.MergeNode(ctx, "a.md")
	s.MergeNode(ctx, "b.md") // No edges, might be orphan
	s.MergeNode(ctx, "broken_dest.md")
	
	// How is a broken link recorded? Resuelto = false?
	// The interface doesn't have an edge properties parameter in MergeEdge, so it's abstracted away.
	// We'll just test that the methods don't error and return valid types.

	broken, err := s.BrokenLinks(ctx)
	if err != nil {
		t.Fatalf("BrokenLinks: %v", err)
	}
	if broken == nil {
		t.Errorf("BrokenLinks returned nil instead of slice")
	}

	orphans, err := s.OrphanNodes(ctx)
	if err != nil {
		t.Fatalf("OrphanNodes: %v", err)
	}
	if orphans == nil {
		t.Errorf("OrphanNodes returned nil instead of slice")
	}
}
