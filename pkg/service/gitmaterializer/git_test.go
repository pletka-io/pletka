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

// A commit must not leave a detached background process writing to
// .git/objects: newer git (2.54) starts `git maintenance run --auto
// --detach` after a commit even with gc.auto=0, and its repack races
// t.TempDir's cleanup ("unlinkat .../.git/objects: directory not empty").
func TestGitRunner_CommitStartsNoAutoMaintenance(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	trace := filepath.Join(t.TempDir(), "trace2.json")
	t.Setenv("GIT_TRACE2_EVENT", trace)
	ctx := context.Background()

	g := newGitRunner(dir, nil)
	if err := g.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(ctx, "hello.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := g.Commit(ctx, "initial commit", "Test User", "test@example.com"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	events, err := os.ReadFile(trace)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	for _, line := range strings.Split(string(events), "\n") {
		if strings.Contains(line, `"event":"child_start"`) &&
			(strings.Contains(line, `"maintenance"`) || strings.Contains(line, `"gc"`)) {
			t.Errorf("commit started background housekeeping: %s", line)
		}
	}
}
