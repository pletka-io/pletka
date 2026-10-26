package gitmaterializer

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A detached maintenance process can write objects after Commit returns and
// after the materializer releases its project lock. Trace the subprocess boundary
// rather than relying on a timing-sensitive TempDir cleanup failure.
func TestGitRunnerCommitDoesNotSpawnMaintenance(t *testing.T) {
	skipIfNoGit(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	tracePath := filepath.Join(t.TempDir(), "git-trace.json")
	t.Setenv("GIT_TRACE2_EVENT", tracePath)
	ctx := t.Context()
	g := newGitRunner(t.TempDir(), nil)
	if err := g.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := g.run(ctx, nil, "config", "maintenance.auto", "true"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(g.workDir, "content"), []byte("snapshot"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := g.AddAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Commit(ctx, "snapshot", "Test", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	trace, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(trace)), "\n") {
		var event struct {
			Event string   `json:"event"`
			Argv  []string `json:"argv"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Event == "child_start" {
			for _, arg := range event.Argv {
				if arg == "maintenance" || arg == "gc" {
					t.Fatalf("commit spawned background-capable maintenance: %v", event.Argv)
				}
			}
		}
	}
}

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
