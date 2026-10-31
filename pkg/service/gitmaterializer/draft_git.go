package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/projectbundle"
)

// CapturedDraftRef is the automation-owned content ref. Publishing it never
// stages, checks out, or modifies a user's HEAD, index or working files.
const CapturedDraftRef = "refs/heads/pletka-draft"

func (g *gitRunner) checkDraftRef(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "-q", CapturedDraftRef)
	cmd.Dir = g.workDir
	target, err := cmd.CombinedOutput()
	if err == nil {
		return fmt.Errorf("captured draft ref must not be symbolic: %s", target)
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		return fmt.Errorf("inspect captured draft ref: %w (%s)", err, target)
	}
	worktrees, err := g.runOut(ctx, nil, "worktree", "list", "--porcelain")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(worktrees, "\n") {
		if line == "branch "+CapturedDraftRef {
			return fmt.Errorf("captured draft ref is checked out; switch branches before publishing")
		}
	}
	return nil
}

// Write exact bytes without clean filters, attributes, autocrlf, user index, or
// worktree state. One hash-object process handles all files; a scratch index
// records the explicit complete inventory, including deletions.
func (g *gitRunner) writeDraftTree(ctx context.Context, files projectbundle.Files) (string, error) {
	dir, err := os.MkdirTemp("", "pletka-draft-tree-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := writeCompleteFiles(dir, files); err != nil {
		return "", err
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	var input strings.Builder
	for _, path := range paths {
		input.WriteString(filepath.Join(dir, filepath.FromSlash(path)) + "\n")
	}
	hashes, err := g.runOutStdin(ctx, nil, []byte(input.String()), "hash-object", "-w", "--no-filters", "--stdin-paths")
	if err != nil {
		return "", err
	}
	oids := strings.Fields(hashes)
	if len(oids) != len(paths) {
		return "", fmt.Errorf("draft object inventory differs")
	}
	idx, cleanup, err := newTempIndex()
	if err != nil {
		return "", err
	}
	defer cleanup()
	env := []string{"GIT_INDEX_FILE=" + idx}
	if err := g.run(ctx, env, "read-tree", "--empty"); err != nil {
		return "", err
	}
	var entries strings.Builder
	for i, path := range paths {
		fmt.Fprintf(&entries, "100644 %s\t%s%c", oids[i], path, 0)
	}
	if _, err := g.runOutStdin(ctx, env, []byte(entries.String()), "update-index", "-z", "--index-info"); err != nil {
		return "", err
	}
	return g.runOut(ctx, env, "write-tree")
}

func (g *gitRunner) commitDraftTree(ctx context.Context, tree, parent, message, name, email string, at time.Time) (string, error) {
	date := fmt.Sprintf("%d +0000", at.Unix())
	env := []string{"GIT_AUTHOR_NAME=" + name, "GIT_AUTHOR_EMAIL=" + email, "GIT_COMMITTER_NAME=" + name, "GIT_COMMITTER_EMAIL=" + email, "GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date}
	args := []string{"commit-tree", tree}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	return g.runOutStdin(ctx, env, []byte(message+"\n"), args...)
}

func (g *gitRunner) publishDraft(ctx context.Context, expected, sha string) error {
	if err := g.checkDraftRef(ctx); err != nil {
		return err
	}
	current, _, err := g.RefSHA(ctx, CapturedDraftRef)
	if err != nil {
		return err
	}
	if current == sha {
		return nil
	}
	if current != expected {
		return fmt.Errorf("captured draft ref changed: expected %s, found %s", expected, current)
	}
	if expected == "" {
		expected = strings.Repeat("0", len(sha))
	}
	return g.run(ctx, nil, "update-ref", "--no-deref", CapturedDraftRef, sha, expected)
}
