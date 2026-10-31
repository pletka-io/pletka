//go:build integration

package gitmaterializer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestUnknownDraftAuditPreservesHeadIndexAndWorktree(t *testing.T) {
	f := setupMatScopedFixture(t)
	f.field("AUDIT_FIELD")
	if err := f.mat.InitProject(f.ctx, f.projectID); err != nil {
		t.Fatal(err)
	}
	git := newGitRunner(f.workDir(), nil)
	beforeHead, err := git.RevParseHead(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.workDir(), "operator-note.txt")
	if err := os.WriteFile(path, []byte("staged version"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := git.Add(f.ctx, "operator-note.txt"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("unstaged version"), 0o600); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(f.workDir(), ".git", "index")
	beforeIndex, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	cs := f.changeSet("must not commit", entry("field", "AUDIT_FIELD", "update"), entry("example", "unhandled", "create"))
	for range 2 { // Retries remain pending; they cannot absorb staged user files.
		if err := f.mat.processChangeSet(f.ctx, cs); err == nil || !strings.Contains(err.Error(), "unsupported draft audit kind") {
			t.Fatalf("unexpected result: %v", err)
		}
		row, err := f.queries.WeaveGetChangeSet(f.ctx, cs.ID)
		if err != nil {
			t.Fatal(err)
		}
		if row.ProcessedAt.Valid || row.GitCommitSha != nil || row.MaterializedOutcome == nil || *row.MaterializedOutcome != "failed" {
			t.Fatalf("unsupported change set was consumed: %+v", row)
		}
	}
	afterHead, err := git.RevParseHead(f.ctx)
	if err != nil || afterHead != beforeHead {
		t.Fatalf("HEAD changed: %s %v", afterHead, err)
	}
	afterIndex, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(beforeIndex, afterIndex); diff != "" {
		t.Fatalf("index changed: %s", diff)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "unstaged version" {
		t.Fatalf("worktree changed: %s %v", data, err)
	}
}
