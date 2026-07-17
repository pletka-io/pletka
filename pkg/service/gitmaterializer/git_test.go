package gitmaterializer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
}

func TestGitRunner_InitAddCommit(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	ctx := context.Background()

	g := newGitRunner(dir, nil)

	if err := g.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf(".git not created: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(ctx, "hello.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	sha, err := g.Commit(ctx, "initial commit", "Test User", "test@example.com")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("expected 40-char SHA, got %q", sha)
	}

	out, err := exec.Command("git", "-C", dir, "log", "--format=%H %an %ae %s").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(string(out), "Test User") {
		t.Errorf("expected Test User in git log, got:\n%s", out)
	}
}
