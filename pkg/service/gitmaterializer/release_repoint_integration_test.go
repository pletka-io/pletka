//go:build integration

package gitmaterializer_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestReleaseRepointRecoveryAndCAS(t *testing.T) {
	skipIfNoGit(t)
	pool := testdb.EmptyPool(t)
	ctx := zmatAuthCtx(seedZMAT(t, pool))
	for _, sql := range []string{
		`INSERT INTO weave_projects(id,owner_id) VALUES ('ZMAT2','ZMAT_OWNER')`,
		`INSERT INTO weave_models(id,project_id) VALUES ('ZMAT2-model','ZMAT2')`,
		`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('ZMAT','7.0.1','ZMAT_OWNER'),('ZMAT2','7.0.1','ZMAT_OWNER')`,
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	dir := filepath.Join(root, "ZMAT")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	gitOut(t, dir, "init", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(dir, "draft.txt"), []byte("unchanged draft\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, dir, "add", "draft.txt")
	gitOut(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.org", "commit", "-m", "old snapshot")
	old := gitOut(t, dir, "rev-parse", "HEAD")
	gitOut(t, dir, "update-ref", "refs/heads/releases", old)
	gitOut(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.org", "tag", "-a", "v7.0.1", "-m", "old tag")
	oldTag := gitOut(t, dir, "rev-parse", "refs/tags/v7.0.1")
	m := gitmaterializer.NewMaterializer(pool, root, nil)
	svc := newReleaseService(pool)
	plan, err := svc.PlanRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	refs := make([]gitmaterializer.ReleaseRefState, 0, len(plan.Projects))
	for _, p := range plan.Projects {
		r, err := m.InspectReleaseRefs(ctx, p.ProjectID, p.Version)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, r)
	}
	check := func(ctx context.Context, id string, resume bool) error {
		for _, r := range refs {
			if err := m.CheckReleaseRepoint(ctx, id, r, resume); err != nil {
				return err
			}
		}
		return nil
	}
	var prepared []gitmaterializer.PreparedRelease
	fail := true
	publish := func(ctx context.Context, op release.RebuildOperation) (map[string]string, error) {
		prepared = nil
		for _, r := range refs {
			p, err := m.PrepareReleaseRepoint(ctx, op.ID, r)
			if err != nil {
				return nil, err
			}
			prepared = append(prepared, p)
		}
		commits := map[string]string{}
		for i, p := range prepared {
			if err := m.PublishReleaseRepoint(ctx, op.ID, p); err != nil {
				return nil, err
			}
			commits[p.Expected.ProjectID] = p.Commit
			if fail && i == 0 {
				return nil, errors.New("process died after first repository")
			}
		}
		return commits, nil
	}
	request := release.RebuildRequest{Plan: plan}
	op, err := svc.RunRebuild(ctx, request, check, publish)
	if err == nil || len(prepared) != 2 {
		t.Fatalf("expected interruption after prepare: %v", err)
	}
	firstCommit := prepared[0].Commit
	prefix := "refs/pletka-rebuild/" + op.ID + "/"
	if got := gitOut(t, dir, "rev-parse", prefix+"previous-tag"); got != oldTag {
		t.Fatal("old annotated tag not retained")
	}
	if got := gitOut(t, dir, "rev-parse", prefix+"previous-branch"); got != old {
		t.Fatal("old branch not retained")
	}
	if got := gitOut(t, dir, "rev-parse", "HEAD"); got != old {
		t.Fatal("default branch moved")
	}
	if got := gitOut(t, dir, "status", "--porcelain"); got != "" {
		t.Fatalf("working tree changed: %s", got)
	}
	// Unexpected movement cannot be accepted as recovery. Restore only our test
	// edit; the production command never performs this repair automatically.
	gitOut(t, dir, "update-ref", "refs/tags/v7.0.1", old)
	if err := m.PublishReleaseRepoint(ctx, op.ID, prepared[0]); err == nil {
		t.Fatal("unreviewed tag movement overwritten")
	}
	gitOut(t, dir, "update-ref", "refs/tags/v7.0.1", prepared[0].Tag)
	if _, err := pool.Exec(ctx, `UPDATE weave_fields SET ui_name='{"en":"Later draft"}' WHERE project_id='ZMAT'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE weave_releases SET title='Later catalog title' WHERE project_id IN ('ZMAT','ZMAT2')`); err != nil {
		t.Fatal(err)
	}
	fail = false
	finished, err := svc.RunRebuild(ctx, request, check, publish)
	if err != nil {
		t.Fatal(err)
	}
	if !finished.Complete || prepared[0].Commit != firstCommit {
		t.Fatal("resume rebuilt a different commit")
	}
	for _, p := range prepared {
		repo := filepath.Join(root, p.Expected.ProjectID)
		if gitOut(t, repo, "rev-parse", "refs/heads/releases") != p.Commit || gitOut(t, repo, "rev-parse", "refs/tags/v7.0.1^{}") != p.Commit {
			t.Fatal("tag/branch do not point to prepared commit")
		}
	}
}
