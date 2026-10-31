//go:build integration

package gitmaterializer_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func copyTagTree(t *testing.T, repo, rev string) string {
	t.Helper()
	dir := t.TempDir()
	for _, path := range strings.Split(gitOut(t, repo, "ls-tree", "-r", "--name-only", rev), "\n") {
		data, err := exec.CommandContext(t.Context(), "git", "-C", repo, "show", rev+":"+path).Output()
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCompleteReleaseTagRestoresOriginalHistory(t *testing.T) {
	skipIfNoGit(t)
	pool := testdb.EmptyPool(t)
	ctx := seedZMAT(t, pool)
	if _, err := newReleaseService(pool).Create(zmatAuthCtx(ctx), zmatProjectID, release.CreateInput{Version: "7.31.0", Title: "Original title"}); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	mat := gitmaterializer.NewMaterializer(pool, base, testLogger())
	if _, err := mat.ProcessPending(ctx, 100); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(base, zmatProjectID)
	const tag = "v7.31.0"
	before := gitOut(t, repo, "rev-parse", tag)
	dir := copyTagTree(t, repo, tag)
	bundle, err := gitmaterializer.LoadCompleteProjectBundle(dir, projectbundle.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := projectbundle.DecodeCatalog(bundle.Project.Snapshot(), bundle.Catalog, projectbundle.DefaultLimits().MaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	records, err := catalog.SelectedReleases(bundle.Project.Snapshot())
	if err != nil || len(records) != 1 || records[0].CreatedByID != "ZMAT_OWNER" || records[0].Title != "Original title" {
		t.Fatalf("original release header lost: %+v %v", records, err)
	}
	target := testdb.EmptyPool(t)
	plan, err := gitmaterializer.NewMaterializer(target, t.TempDir(), testLogger()).HydrateProjectSnapshot(ctx, dir)
	if err != nil || plan.ImportResult == nil || !plan.ImportResult.Committed {
		t.Fatalf("complete tag restoration failed: %v", err)
	}
	assembler, err := projectread.New(projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	factory := readsession.NewFactory(target).WithSourceAuthorization(func(context.Context, auth.Resource) error { return nil })
	if err := factory.Read(ctx, auth.Release("7.31.0"), zmatProjectID, func(s *readsession.Session) error {
		value, err := assembler.Project(ctx, s)
		if err != nil {
			return err
		}
		got, err := projectbundle.Encode(value)
		if err != nil {
			return err
		}
		want, err := projectbundle.Encode(bundle.Project.Snapshot())
		if err != nil {
			return err
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatal(diff)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Verification must use the frozen overlay even after mutable catalog edits.
	descriptorExec(t, pool, `UPDATE weave_releases SET title='Later title',description='Later description' WHERE project_id=$1 AND version='7.31.0'`, zmatProjectID)
	descriptorExec(t, pool, `INSERT INTO weave_change_set(project_id,actor_name,actor_email,commit_message,closed_at,kind,release_version) VALUES($1,'system','system@local','retry',now(),'release','7.31.0')`, zmatProjectID)
	if _, err := mat.ProcessPending(ctx, 100); err != nil {
		t.Fatal(err)
	}
	outcome, message, _ := changeSetOutcome(t, pool, "7.31.0")
	if outcome != "noop" || gitOut(t, repo, "rev-parse", tag) != before {
		t.Fatalf("catalog edit changed frozen release: %s %s", outcome, message)
	}
}

func TestReleaseCatalogSharesSessionAndLifetime(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := seedZMAT(t, pool)
	if _, err := newReleaseService(pool).Create(zmatAuthCtx(ctx), zmatProjectID, release.CreateInput{Version: "7.32.0", Title: "Before"}); err != nil {
		t.Fatal(err)
	}
	var escaped *readsession.Session
	if err := readsession.NewFactory(pool).Read(ctx, auth.Release("7.32.0"), zmatProjectID, func(s *readsession.Session) error {
		escaped = s
		descriptorExec(t, pool, `UPDATE weave_releases SET title='After' WHERE project_id=$1`, zmatProjectID)
		record, err := s.ReleaseRecord(ctx)
		if err != nil {
			return err
		}
		if record.Title != "Before" || record.Version != "7.32.0" {
			t.Fatal("release header escaped its bound snapshot")
		}
		rows, err := s.Catalog(ctx)
		if err == nil && (len(rows) != 1 || rows[0].Title != "Before") {
			t.Fatal("catalog used a different snapshot")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := escaped.Catalog(ctx); err == nil {
		t.Fatal("closed catalog session remained usable")
	}
	descriptorExec(t, pool, `INSERT INTO weave_releases(project_id,version,created_by_id,created_at,archived_at) VALUES($1,'ancient','ZMAT_OWNER','0001-01-01 00:00:00+00 BC','infinity')`, zmatProjectID)
	if err := readsession.NewFactory(pool).Read(ctx, auth.Draft(), zmatProjectID, func(s *readsession.Session) error {
		if _, err := s.ReleaseRecord(ctx); err == nil {
			t.Fatal("draft acquired an immutable release header")
		}
		rows, err := s.Catalog(ctx)
		if err != nil {
			return err
		}
		if len(rows) != 2 || !strings.HasSuffix(string(rows[1].CreatedAt), " BC") || rows[1].ArchivedAt == nil || *rows[1].ArchivedAt != "infinity" {
			t.Fatal("catalog timestamp text lost era or infinity")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestStoredLegacyTagUsesLegacyVerifier(t *testing.T) {
	skipIfNoGit(t)
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "unknown"}[unknown], func(t *testing.T) {
			pool := testdb.EmptyPool(t)
			ctx := seedZMAT(t, pool)
			if _, err := newReleaseService(pool).Create(zmatAuthCtx(ctx), zmatProjectID, release.CreateInput{Version: "7.33.0"}); err != nil {
				t.Fatal(err)
			}
			base := t.TempDir()
			repo := filepath.Join(base, zmatProjectID)
			mat := gitmaterializer.NewMaterializer(pool, base, testLogger())
			if err := mat.BuildLegacyReleaseTree(ctx, repo, zmatProjectID, "7.33.0"); err != nil {
				t.Fatal(err)
			}
			if unknown {
				if err := os.WriteFile(filepath.Join(repo, projectbundle.ManifestPath), []byte("schema_version: 999\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			gitOut(t, repo, "init")
			gitOut(t, repo, "add", "--all")
			gitOut(t, repo, "-c", "user.name=test", "-c", "user.email=test@local", "commit", "-m", "historical format")
			gitOut(t, repo, "-c", "user.name=test", "-c", "user.email=test@local", "tag", "-a", "v7.33.0", "-m", "historical tag")
			before := gitOut(t, repo, "rev-parse", "v7.33.0")
			if !unknown {
				// A prepared object from the old writer must resume unchanged
				// after upgrade, including its original catalog overlay.
				operation := strings.Repeat("a", 64)
				prefix := "refs/pletka-rebuild/" + operation + "/"
				original := gitOut(t, repo, "rev-parse", "HEAD")
				gitOut(t, repo, "update-ref", prefix+"prepared-commit", original)
				gitOut(t, repo, "update-ref", prefix+"prepared-tag", before)
				expected, err := mat.InspectReleaseRefs(ctx, zmatProjectID, "7.33.0")
				if err != nil {
					t.Fatal(err)
				}
				descriptorExec(t, pool, `UPDATE weave_releases SET title='Changed after preparation' WHERE project_id=$1`, zmatProjectID)
				prepared, err := mat.PrepareReleaseRepoint(ctx, operation, expected)
				if err != nil || prepared.Commit != original || prepared.Tag != before {
					t.Fatalf("old prepared release was reinterpreted: %+v %v", prepared, err)
				}
			}
			if _, err := mat.ProcessPending(ctx, 100); err != nil {
				t.Fatal(err)
			}
			outcome, message, _ := changeSetOutcome(t, pool, "7.33.0")
			if (!unknown && outcome != "noop") || (unknown && (outcome != "failed" || !strings.Contains(message, "unsupported stored project format"))) {
				t.Fatalf("wrong stored-format dispatch: %s %s", outcome, message)
			}
			if gitOut(t, repo, "rev-parse", "v7.33.0") != before {
				t.Fatal("verification rewrote a historical tag")
			}
		})
	}
}
