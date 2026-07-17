package gitmaterializer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestHasStagedChanges is the idempotency mechanism behind "an identical
// re-save produces no new commit": re-staging byte-identical content leaves
// the index clean, so processChangeSet skips the commit.
func TestHasStagedChanges(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	g := newGitRunner(dir, nil)
	if err := g.Available(ctx); err != nil {
		t.Skip("git not available")
	}
	if err := g.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}

	path := filepath.Join(dir, "a.yaml")
	write := func(s string) {
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// Fresh repo, nothing staged.
	if g.HasStagedChanges(ctx) {
		t.Fatal("empty repo: want no staged changes")
	}

	// New file staged (no HEAD yet — exercises the empty-tree path).
	write("x")
	if err := g.Add(ctx, "a.yaml"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !g.HasStagedChanges(ctx) {
		t.Fatal("new staged file: want staged changes")
	}
	if got := g.StagedFileCount(ctx); got != 1 {
		t.Fatalf("StagedFileCount new file (no HEAD): got %d want 1", got)
	}

	if _, err := g.Commit(ctx, "init", "t", "t@example.com"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if g.HasStagedChanges(ctx) {
		t.Fatal("after commit: want clean index")
	}
	if got := g.StagedFileCount(ctx); got != 0 {
		t.Fatalf("StagedFileCount after commit: got %d want 0", got)
	}

	// Re-save identical bytes: index stays clean → idempotent, no commit.
	write("x")
	if err := g.Add(ctx, "a.yaml"); err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if g.HasStagedChanges(ctx) {
		t.Fatal("identical re-save: want no staged changes (idempotent)")
	}

	// Changed bytes: real diff, should commit.
	write("y")
	if err := g.Add(ctx, "a.yaml"); err != nil {
		t.Fatalf("add changed: %v", err)
	}
	if !g.HasStagedChanges(ctx) {
		t.Fatal("changed bytes: want staged changes")
	}
}
