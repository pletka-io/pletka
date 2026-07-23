package gitmaterializer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// WriteTreeFrom stages the full content of srcDir into a scratch index (via a
// temp GIT_INDEX_FILE + GIT_WORK_TREE=srcDir) and writes it as a tree object.
// It never touches the repo's default-branch work tree or main index.
func (g *gitRunner) WriteTreeFrom(ctx context.Context, srcDir string) (string, error) {
	idxPath, cleanup, err := newTempIndex()
	if err != nil {
		return "", err
	}
	defer cleanup()

	env := []string{
		"GIT_INDEX_FILE=" + idxPath,
		"GIT_WORK_TREE=" + srcDir,
	}
	if err := g.run(ctx, env, "add", "-A"); err != nil {
		return "", fmt.Errorf("git add -A (write-tree-from): %w", err)
	}
	tree, err := g.runOut(ctx, env, "write-tree")
	if err != nil {
		return "", fmt.Errorf("git write-tree: %w", err)
	}
	return tree, nil
}

// CommitTree creates a commit object pointing at treeSHA, with parentSHA as
// its parent ("" for a root commit). It does not update any ref.
func (g *gitRunner) CommitTree(ctx context.Context, treeSHA, parentSHA, message, authorName, authorEmail string) (string, error) {
	env := []string{
		"GIT_AUTHOR_NAME=" + authorName,
		"GIT_AUTHOR_EMAIL=" + authorEmail,
		"GIT_COMMITTER_NAME=" + authorName,
		"GIT_COMMITTER_EMAIL=" + authorEmail,
	}
	args := []string{"commit-tree", treeSHA}
	if parentSHA != "" {
		args = append(args, "-p", parentSHA)
	}
	args = append(args, "-m", message)

	sha, err := g.runOut(ctx, env, args...)
	if err != nil {
		return "", fmt.Errorf("git commit-tree: %w", err)
	}
	return sha, nil
}

// UpdateRef points ref at sha, creating it if it doesn't exist yet.
func (g *gitRunner) UpdateRef(ctx context.Context, ref, sha string) error {
	if err := g.run(ctx, nil, "update-ref", ref, sha); err != nil {
		return fmt.Errorf("git update-ref: %w", err)
	}
	return nil
}

// TagAnnotated creates an annotated tag object named name at sha. git refuses
// to overwrite an existing tag, so re-tagging an existing name with a
// different sha returns an error.
func (g *gitRunner) TagAnnotated(ctx context.Context, name, message, sha, taggerName, taggerEmail string) error {
	env := []string{
		"GIT_COMMITTER_NAME=" + taggerName,
		"GIT_COMMITTER_EMAIL=" + taggerEmail,
	}
	if err := g.run(ctx, env, "tag", "-a", name, "-m", message, sha); err != nil {
		return fmt.Errorf("git tag -a: %w", err)
	}
	return nil
}

// RefSHA resolves ref (a full ref name, tag name, or other rev expression) to
// its object sha. exists is false (with a nil error) when ref does not
// resolve to anything.
func (g *gitRunner) RefSHA(ctx context.Context, ref string) (string, bool, error) {
	return g.revParseVerify(ctx, ref)
}

// TreeOfRev resolves rev to the sha of the tree it points at (rev^{tree}).
// exists is false (with a nil error) when rev does not resolve.
func (g *gitRunner) TreeOfRev(ctx context.Context, rev string) (string, bool, error) {
	return g.revParseVerify(ctx, rev+"^{tree}")
}

// revParseVerify runs `git rev-parse --verify --quiet <rev>`, distinguishing
// "rev does not exist" (git exit code 1) from a real error.
func (g *gitRunner) revParseVerify(ctx context.Context, rev string) (string, bool, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--quiet", rev)
	cmd.Dir = g.workDir
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return strings.TrimSpace(out.String()), true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return "", false, nil
	}
	return "", false, fmt.Errorf("git rev-parse --verify %s: %w (stderr: %s)", rev, err, stderr.String())
}

// ShowFile returns the content of relPath as it exists at rev. exists is
// false (with a nil error) when relPath is not present at rev.
func (g *gitRunner) ShowFile(ctx context.Context, rev, relPath string) ([]byte, bool, error) {
	cmd := exec.CommandContext(ctx, "git", "show", rev+":"+relPath)
	cmd.Dir = g.workDir
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return out.Bytes(), true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 128 {
		msg := stderr.String()
		if strings.Contains(msg, "does not exist") || strings.Contains(msg, "exists on disk, but not in") {
			return nil, false, nil
		}
	}
	return nil, false, fmt.Errorf("git show %s:%s: %w (stderr: %s)", rev, relPath, err, stderr.String())
}

// WriteTreeReplacingFile builds a new tree identical to baseRev's tree except
// relPath is replaced with content, via a scratch index (read-tree + a single
// updated blob + write-tree). It never touches the repo's default-branch work
// tree or main index.
func (g *gitRunner) WriteTreeReplacingFile(ctx context.Context, baseRev, relPath string, content []byte) (string, error) {
	idxPath, cleanup, err := newTempIndex()
	if err != nil {
		return "", err
	}
	defer cleanup()

	env := []string{"GIT_INDEX_FILE=" + idxPath}

	if err := g.run(ctx, env, "read-tree", baseRev); err != nil {
		return "", fmt.Errorf("git read-tree: %w", err)
	}

	blobSHA, err := g.runOutStdin(ctx, env, content, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", fmt.Errorf("git hash-object: %w", err)
	}

	cacheInfo := fmt.Sprintf("100644,%s,%s", blobSHA, relPath)
	if err := g.run(ctx, env, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
		return "", fmt.Errorf("git update-index: %w", err)
	}

	tree, err := g.runOut(ctx, env, "write-tree")
	if err != nil {
		return "", fmt.Errorf("git write-tree: %w", err)
	}
	return tree, nil
}

// runOutStdin behaves like runOut but feeds stdin to the command.
func (g *gitRunner) runOutStdin(ctx context.Context, extraEnv []string, stdin []byte, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.workDir
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	cmd.Stdin = bytes.NewReader(stdin)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w (stderr: %s)",
			strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(out.String()), nil
}

// newTempIndex reserves a unique path for a scratch GIT_INDEX_FILE used in a
// temp-index git plumbing sequence, so it never disturbs the repo's real
// index. os.CreateTemp is used only to get a collision-free name; the file
// itself is removed immediately so git creates a fresh index there on first
// write (an empty pre-existing file is an invalid index and git refuses it).
// The returned cleanup func removes the index git creates; callers must
// defer it.
func newTempIndex() (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "pletka-idx-")
	if err != nil {
		return "", nil, fmt.Errorf("create temp index: %w", err)
	}
	idxPath := f.Name()
	if closeErr := f.Close(); closeErr != nil {
		os.Remove(idxPath)
		return "", nil, fmt.Errorf("close temp index: %w", closeErr)
	}
	if err := os.Remove(idxPath); err != nil {
		return "", nil, fmt.Errorf("remove temp index placeholder: %w", err)
	}
	return idxPath, func() { os.Remove(idxPath) }, nil
}
