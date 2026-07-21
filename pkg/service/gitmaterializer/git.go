package gitmaterializer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// emptyTreeSHA is git's well-known hash of the empty tree, used to diff the
// index on a repo that has no commits yet.
const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// gitRunner wraps the git CLI via os/exec for a single working tree.
type gitRunner struct {
	workDir string
	logger  *slog.Logger
}

func newGitRunner(workDir string, logger *slog.Logger) *gitRunner {
	if logger == nil {
		logger = slog.Default()
	}
	return &gitRunner{workDir: workDir, logger: logger}
}

// Available returns nil if the git binary is on PATH and usable.
func (g *gitRunner) Available(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git not available: %w", err)
	}
	return nil
}

func (g *gitRunner) Init(ctx context.Context) error {
	if err := os.MkdirAll(g.workDir, 0o755); err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}
	return g.run(ctx, nil, "init")
}

func (g *gitRunner) Add(ctx context.Context, path string) error {
	return g.run(ctx, nil, "add", "--", path)
}

func (g *gitRunner) AddAll(ctx context.Context) error {
	return g.run(ctx, nil, "add", "-A")
}

func (g *gitRunner) Commit(ctx context.Context, message, authorName, authorEmail string) (string, error) {
	// Idempotent: a re-materialization that produced a byte-identical tree has
	// nothing staged. Return the existing HEAD rather than failing on an empty
	// commit, so re-running init-git (or the poller) on an unchanged project is
	// a safe no-op instead of "git commit: exit status 1".
	if !g.HasStagedChanges(ctx) {
		return g.RevParseHead(ctx)
	}
	env := []string{
		"GIT_AUTHOR_NAME=" + authorName,
		"GIT_AUTHOR_EMAIL=" + authorEmail,
		"GIT_COMMITTER_NAME=" + authorName,
		"GIT_COMMITTER_EMAIL=" + authorEmail,
	}
	if err := g.run(ctx, env, "commit", "-m", message, "--allow-empty-message"); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}
	return g.RevParseHead(ctx)
}

// HasStagedChanges reports whether the index differs from HEAD (or from the
// empty tree when the repo has no commits yet). Used to skip an empty commit
// when a re-save produced byte-identical files that git sees as no change.
func (g *gitRunner) HasStagedChanges(ctx context.Context) bool {
	args := []string{"diff", "--cached", "--quiet"}
	if _, err := g.RevParseHead(ctx); err != nil {
		// No HEAD yet: diff the index against the empty tree so the first
		// staged files register as changes.
		args = append(args, emptyTreeSHA)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.workDir
	err := cmd.Run()
	if err == nil {
		return false // exit 0: index clean
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return true // exit 1: staged changes present
	}
	// Any other failure: assume changes and let the commit attempt surface it.
	return true
}

// StagedFileCount returns how many files are staged relative to HEAD (or the
// empty tree on a repo with no commits). This is the commit's diff size — the
// "size of this change" as opposed to the whole-tree write cost.
func (g *gitRunner) StagedFileCount(ctx context.Context) int {
	args := []string{"diff", "--cached", "--name-only"}
	if _, err := g.RevParseHead(ctx); err != nil {
		args = append(args, emptyTreeSHA)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.workDir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0
	}
	trimmed := strings.TrimSpace(out.String())
	if trimmed == "" {
		return 0
	}
	return strings.Count(trimmed, "\n") + 1
}

func (g *gitRunner) RevParseHead(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = g.workDir
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse: %w (stderr: %s)", err, stderr.String())
	}
	return strings.TrimSpace(out.String()), nil
}

func (g *gitRunner) run(ctx context.Context, extraEnv []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.workDir
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w (stderr: %s)",
			strings.Join(args, " "), err, stderr.String())
	}
	return nil
}
