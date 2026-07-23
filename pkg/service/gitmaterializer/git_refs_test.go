package gitmaterializer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteTreeFromAndCommitTree(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	ctx := context.Background()

	g := newGitRunner(dir, nil)
	if err := g.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sub", "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	treeSHA, err := g.WriteTreeFrom(ctx, srcDir)
	if err != nil {
		t.Fatalf("WriteTreeFrom: %v", err)
	}
	if len(treeSHA) != 40 {
		t.Fatalf("expected 40-char tree sha, got %q", treeSHA)
	}

	commitSHA, err := g.CommitTree(ctx, treeSHA, "", "initial release commit", "pletka-system", "system@pletka.local")
	if err != nil {
		t.Fatalf("CommitTree: %v", err)
	}
	if len(commitSHA) != 40 {
		t.Fatalf("expected 40-char commit sha, got %q", commitSHA)
	}

	if err := g.UpdateRef(ctx, "refs/heads/releases", commitSHA); err != nil {
		t.Fatalf("UpdateRef: %v", err)
	}

	sha, exists, err := g.RefSHA(ctx, "refs/heads/releases")
	if err != nil {
		t.Fatalf("RefSHA: %v", err)
	}
	if !exists {
		t.Fatal("expected refs/heads/releases to exist")
	}
	if sha != commitSHA {
		t.Errorf("RefSHA = %q, want %q", sha, commitSHA)
	}

	// Determinism: same content -> same tree sha.
	treeSHA2, err := g.WriteTreeFrom(ctx, srcDir)
	if err != nil {
		t.Fatalf("WriteTreeFrom (2nd): %v", err)
	}
	if treeSHA2 != treeSHA {
		t.Errorf("expected deterministic tree sha, got %q then %q", treeSHA, treeSHA2)
	}

	// The default-branch work tree and repo index must be untouched.
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("expected clean git status in workDir, got:\n%s", out)
	}
}

func TestCommitTreeWithParentAndTag(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	ctx := context.Background()

	g := newGitRunner(dir, nil)
	if err := g.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tree1, err := g.WriteTreeFrom(ctx, srcDir)
	if err != nil {
		t.Fatalf("WriteTreeFrom: %v", err)
	}
	first, err := g.CommitTree(ctx, tree1, "", "first", "pletka-system", "system@pletka.local")
	if err != nil {
		t.Fatalf("CommitTree first: %v", err)
	}

	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tree2, err := g.WriteTreeFrom(ctx, srcDir)
	if err != nil {
		t.Fatalf("WriteTreeFrom (2nd): %v", err)
	}
	second, err := g.CommitTree(ctx, tree2, first, "second", "pletka-system", "system@pletka.local")
	if err != nil {
		t.Fatalf("CommitTree second: %v", err)
	}

	if err := g.TagAnnotated(ctx, "v1.0.0", "Title", second, "pletka-system", "system@pletka.local"); err != nil {
		t.Fatalf("TagAnnotated: %v", err)
	}

	treeOfTag, exists, err := g.TreeOfRev(ctx, "v1.0.0")
	if err != nil {
		t.Fatalf("TreeOfRev: %v", err)
	}
	if !exists {
		t.Fatal("expected v1.0.0 to exist")
	}
	if treeOfTag != tree2 {
		t.Errorf("TreeOfRev(v1.0.0) = %q, want %q", treeOfTag, tree2)
	}

	sha, exists, err := g.RefSHA(ctx, "refs/tags/v1.0.0")
	if err != nil {
		t.Fatalf("RefSHA: %v", err)
	}
	if !exists {
		t.Fatal("expected refs/tags/v1.0.0 to exist")
	}
	if sha == "" {
		t.Error("expected non-empty tag object sha")
	}

	// Re-tagging with a different sha must be refused by git.
	if err := g.TagAnnotated(ctx, "v1.0.0", "Title again", first, "pletka-system", "system@pletka.local"); err == nil {
		t.Error("expected error re-tagging existing tag with a different sha")
	}
}

func TestShowFileAndWriteTreeReplacingFile(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	ctx := context.Background()

	g := newGitRunner(dir, nil)
	if err := g.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	srcDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcDir, "releases"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "releases", "index.yaml"), []byte("v: 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "other.txt"), []byte("unchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tree, err := g.WriteTreeFrom(ctx, srcDir)
	if err != nil {
		t.Fatalf("WriteTreeFrom: %v", err)
	}
	commit, err := g.CommitTree(ctx, tree, "", "initial", "pletka-system", "system@pletka.local")
	if err != nil {
		t.Fatalf("CommitTree: %v", err)
	}

	content, exists, err := g.ShowFile(ctx, commit, "releases/index.yaml")
	if err != nil {
		t.Fatalf("ShowFile: %v", err)
	}
	if !exists {
		t.Fatal("expected releases/index.yaml to exist")
	}
	if string(content) != "v: 1" {
		t.Errorf("ShowFile content = %q, want %q", content, "v: 1")
	}

	_, exists, err = g.ShowFile(ctx, commit, "missing.yaml")
	if err != nil {
		t.Fatalf("ShowFile (missing) unexpected error: %v", err)
	}
	if exists {
		t.Fatal("expected missing.yaml to not exist")
	}

	newTree, err := g.WriteTreeReplacingFile(ctx, commit, "releases/index.yaml", []byte("v: 2"))
	if err != nil {
		t.Fatalf("WriteTreeReplacingFile: %v", err)
	}
	newCommit, err := g.CommitTree(ctx, newTree, commit, "bump version", "pletka-system", "system@pletka.local")
	if err != nil {
		t.Fatalf("CommitTree (new): %v", err)
	}

	content, exists, err = g.ShowFile(ctx, newCommit, "releases/index.yaml")
	if err != nil {
		t.Fatalf("ShowFile (new): %v", err)
	}
	if !exists {
		t.Fatal("expected releases/index.yaml to exist in new commit")
	}
	if string(content) != "v: 2" {
		t.Errorf("ShowFile content = %q, want %q", content, "v: 2")
	}

	otherContent, exists, err := g.ShowFile(ctx, newCommit, "other.txt")
	if err != nil {
		t.Fatalf("ShowFile (other.txt): %v", err)
	}
	if !exists {
		t.Fatal("expected other.txt to still exist")
	}
	if string(otherContent) != "unchanged\n" {
		t.Errorf("other.txt content = %q, want unchanged", otherContent)
	}
}

func TestRefSHAMissing(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	ctx := context.Background()

	g := newGitRunner(dir, nil)
	if err := g.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	sha, exists, err := g.RefSHA(ctx, "refs/heads/releases")
	if err != nil {
		t.Fatalf("RefSHA: %v", err)
	}
	if exists {
		t.Fatal("expected refs/heads/releases to not exist on a fresh repo")
	}
	if sha != "" {
		t.Errorf("expected empty sha, got %q", sha)
	}
}
