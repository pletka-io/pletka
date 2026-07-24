//go:build integration

package gitmaterializer_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

// zmatProjectID is the synthetic project these tests materialize releases for.
// It is deliberately distinct from ZREL (pkg/weave/release outbox tests) so the
// two files never collide in the shared per-package clone, and every version it
// cuts stays in the 7.x range so the namespace-bindings-archive cleanup (scoped
// by version_number LIKE '7.%') never touches another test's rows.
const zmatProjectID = "ZMAT"

// idxFile is the subset of releases/index.yaml the assertions need.
type idxFile struct {
	SchemaVersion int `yaml:"schema_version"`
	Releases      []struct {
		Version         string `yaml:"version"`
		Tag             string `yaml:"tag"`
		ArchivedAt      string `yaml:"archived_at"`
		ArchivedMessage string `yaml:"archived_message"`
	} `yaml:"releases"`
}

// seedZMAT creates a synthetic project mirroring seedZREL in the release
// package: one owner actor, project, category, field, model, base override.
// Registers a cleanup that removes both the live rows and the archive rows
// Create's snapshot step writes (global namespace bindings archived under the
// release version are scoped by version_number LIKE '7.%').
func seedZMAT(t *testing.T, pool *pgxpool.Pool) context.Context {
	t.Helper()
	ctx := context.Background()

	purge := func() {
		bg := context.Background()
		for _, stmt := range []string{
			`DELETE FROM weave_change_set WHERE project_id='ZMAT'`,
			`DELETE FROM weave_releases WHERE project_id='ZMAT'`,
			`DELETE FROM weave_field_overrides WHERE entity_id LIKE 'ZMAT%' OR field_id IN (SELECT id FROM weave_fields WHERE project_id='ZMAT')`,
			`DELETE FROM weave_fields WHERE project_id='ZMAT'`,
			`DELETE FROM weave_models WHERE project_id='ZMAT'`,
			`DELETE FROM weave_categories WHERE project_id='ZMAT'`,
			`DELETE FROM weave_memberships WHERE scope_type='project' AND scope_id='ZMAT'`,
			`DELETE FROM weave_projects WHERE id='ZMAT'`,
			`DELETE FROM weave_actors WHERE id='ZMAT_OWNER'`,
			`DELETE FROM weave_field_overrides_archive WHERE project_id='ZMAT'`,
			`DELETE FROM weave_fields_archive WHERE project_id='ZMAT'`,
			`DELETE FROM weave_models_archive WHERE project_id='ZMAT'`,
			`DELETE FROM weave_categories_archive WHERE project_id='ZMAT'`,
			`DELETE FROM weave_projects_archive WHERE id='ZMAT'`,
			`DELETE FROM weave_namespace_bindings_archive WHERE version_number LIKE '7.%'`,
		} {
			_, _ = pool.Exec(bg, stmt)
		}
	}
	purge()
	t.Cleanup(purge)

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed %q: %v", sql, err)
		}
	}

	uiName, _ := json.Marshal(map[string]string{"en": "Materializer Release Probe"})
	mustExec(`INSERT INTO weave_actors (id, type, display_name, slug, visibility, created_at, updated_at)
	          VALUES ('ZMAT_OWNER','organization','Mat Release Probe Org','zmat-owner','private',NOW(),NOW())`)
	mustExec(`INSERT INTO weave_projects (id, ui_name, description, status, owner_id, visibility, created_at, updated_at)
	          VALUES ($1,$2,$2,'draft','ZMAT_OWNER','private',NOW(),NOW())`, zmatProjectID, uiName)
	mustExec(`INSERT INTO weave_categories (id, semantic_id, project_id, ui_name, status, created_at, updated_at)
	          VALUES ('ZMAT.CAT.1','ZMAT.CAT.1',$1,$2,'draft',NOW(),NOW())`, zmatProjectID, uiName)
	mustExec(`INSERT INTO weave_fields (id, semantic_id, system_name, project_id, ui_name, status, path_elements, created_at, updated_at)
	          VALUES ('ZMATF1','ZMATF.1','mat_release_probe',$1,$2,'draft','[]',NOW(),NOW())`, zmatProjectID, uiName)
	mustExec(`INSERT INTO weave_models (id, project_id, ui_name, status, created_at, updated_at)
	          VALUES ('ZMATM.1',$1,$2,'draft',NOW(),NOW())`, zmatProjectID, uiName)
	mustExec(`INSERT INTO weave_field_overrides (field_id, entity_type, entity_id, project_id, created_at, updated_at)
	          VALUES ('ZMATF1','model','ZMATM.1',$1,NOW(),NOW())`, zmatProjectID)

	return ctx
}

func zmatAuthCtx(ctx context.Context) context.Context {
	authCtx := weaveauth.WithSnapshot(ctx, &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	return weaveauth.WithPrincipal(authCtx, &weaveauth.Principal{ActorID: "ZMAT_OWNER"})
}

func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
}

func newReleaseService(pool *pgxpool.Pool) *release.Service {
	return release.NewService(release.NewPostgresStore(pool), pool, nil)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// gitOut runs a git command in dir and returns trimmed stdout, failing the test
// on error.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}

// revParseQuiet resolves rev in dir, returning ("", false) when it does not
// resolve.
func revParseQuiet(t *testing.T, dir, rev string) (string, bool) {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--verify", "--quiet", rev).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func decodeIndexAt(t *testing.T, dir, rev string) idxFile {
	t.Helper()
	raw := gitOut(t, dir, "show", rev+":releases/index.yaml")
	var idx idxFile
	if err := yaml.Unmarshal([]byte(raw), &idx); err != nil {
		t.Fatalf("decode index at %s: %v", rev, err)
	}
	return idx
}

func changeSetOutcome(t *testing.T, pool *pgxpool.Pool, version string) (outcome, errMsg, sha string) {
	t.Helper()
	var o, e, s *string
	if err := pool.QueryRow(context.Background(), `
		SELECT materialized_outcome, materialized_error, git_commit_sha
		FROM weave_change_set
		WHERE project_id='ZMAT' AND release_version=$1
		ORDER BY id DESC LIMIT 1
	`, version).Scan(&o, &e, &s); err != nil {
		t.Fatalf("read change set outcome for %s: %v", version, err)
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	return deref(o), deref(e), deref(s)
}

func TestReleaseWritePath(t *testing.T) {
	skipIfNoGit(t)
	pool := testPool(t)
	ctx := seedZMAT(t, pool)
	authCtx := zmatAuthCtx(ctx)

	baseDir := t.TempDir()
	mat := gitmaterializer.NewMaterializer(pool, baseDir, testLogger())

	svc := newReleaseService(pool)
	if _, err := svc.Create(authCtx, zmatProjectID, release.CreateInput{Version: "7.0.0", Title: "First cut"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := mat.ProcessPending(ctx, 100); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}

	repo := filepath.Join(baseDir, zmatProjectID)

	// releases branch exists with exactly 1 commit.
	if _, ok := revParseQuiet(t, repo, "refs/heads/releases"); !ok {
		t.Fatalf("refs/heads/releases not created")
	}
	if got := gitOut(t, repo, "rev-list", "--count", "refs/heads/releases"); got != "1" {
		t.Fatalf("expected 1 commit on releases, got %s", got)
	}

	// tag v7.0.0 exists and points at the releases tip commit.
	tagSHA, ok := revParseQuiet(t, repo, "v7.0.0")
	if !ok {
		t.Fatalf("tag v7.0.0 not created")
	}
	branchSHA, _ := revParseQuiet(t, repo, "refs/heads/releases")
	tagCommit := gitOut(t, repo, "rev-list", "-n", "1", "v7.0.0")
	if tagCommit != branchSHA {
		t.Fatalf("tag v7.0.0 (%s -> commit %s) does not point at releases tip %s", tagSHA, tagCommit, branchSHA)
	}

	// commit tree contains releases/index.yaml with exactly 1 entry.
	idx := decodeIndexAt(t, repo, "refs/heads/releases")
	if len(idx.Releases) != 1 {
		t.Fatalf("expected 1 index entry, got %d: %+v", len(idx.Releases), idx.Releases)
	}
	e := idx.Releases[0]
	if e.Version != "7.0.0" || e.Tag != "v7.0.0" || e.ArchivedAt != "" {
		t.Fatalf("unexpected index entry: %+v", e)
	}

	// snapshot landed at root (spot-check project.yaml).
	if _, ok := revParseQuiet(t, repo, "refs/heads/releases:project.yaml"); !ok {
		t.Fatalf("expected project.yaml in release tree")
	}

	// change set marked processed with committed outcome.
	outcome, _, sha := changeSetOutcome(t, pool, "7.0.0")
	if outcome != "committed" {
		t.Fatalf("expected outcome 'committed', got %q", outcome)
	}
	if sha != branchSHA {
		t.Fatalf("expected git_commit_sha %s, got %s", branchSHA, sha)
	}

	// draft branch untouched: no default-branch commit exists.
	if _, ok := revParseQuiet(t, repo, "HEAD"); ok {
		t.Fatalf("expected no default-branch (HEAD) commit after release-only materialization")
	}
}

func TestReleaseIdempotentRerunAndDeterminism(t *testing.T) {
	skipIfNoGit(t)
	pool := testPool(t)
	ctx := seedZMAT(t, pool)
	authCtx := zmatAuthCtx(ctx)

	baseDir := t.TempDir()
	mat := gitmaterializer.NewMaterializer(pool, baseDir, testLogger())

	svc := newReleaseService(pool)
	if _, err := svc.Create(authCtx, zmatProjectID, release.CreateInput{Version: "7.1.0"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := mat.ProcessPending(ctx, 100); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}

	repo := filepath.Join(baseDir, zmatProjectID)
	tagBefore, ok := revParseQuiet(t, repo, "v7.1.0")
	if !ok {
		t.Fatalf("tag v7.1.0 not created")
	}

	// Manually enqueue a duplicate closed release change set.
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_change_set (project_id, actor_name, actor_email, commit_message, closed_at, kind, release_version)
		VALUES ('ZMAT','pletka-system','system@pletka.local','Release v7.1.0', now(), 'release', '7.1.0')
	`); err != nil {
		t.Fatalf("insert duplicate change set: %v", err)
	}
	if _, err := mat.ProcessPending(ctx, 100); err != nil {
		t.Fatalf("ProcessPending (rerun): %v", err)
	}

	// Idempotent: outcome noop, tag unchanged, still 1 commit.
	outcome, _, _ := changeSetOutcome(t, pool, "7.1.0")
	if outcome != "noop" {
		t.Fatalf("expected duplicate rerun outcome 'noop', got %q", outcome)
	}
	tagAfter, _ := revParseQuiet(t, repo, "v7.1.0")
	if tagAfter != tagBefore {
		t.Fatalf("tag v7.1.0 changed: %s -> %s", tagBefore, tagAfter)
	}
	if got := gitOut(t, repo, "rev-list", "--count", "refs/heads/releases"); got != "1" {
		t.Fatalf("expected still 1 commit on releases, got %s", got)
	}

	// Determinism: build the release tree twice, diff the file sets byte-for-byte.
	a := t.TempDir()
	b := t.TempDir()
	if err := mat.BuildReleaseTree(ctx, a, zmatProjectID, "7.1.0"); err != nil {
		t.Fatalf("BuildReleaseTree(a): %v", err)
	}
	if err := mat.BuildReleaseTree(ctx, b, zmatProjectID, "7.1.0"); err != nil {
		t.Fatalf("BuildReleaseTree(b): %v", err)
	}
	if diff := cmp.Diff(collectTree(t, a), collectTree(t, b)); diff != "" {
		t.Fatalf("release tree not deterministic (-a +b):\n%s", diff)
	}
}

// collectTree returns a map of relative path -> file content for every file
// under root, so two builds can be compared byte-for-byte.
func collectTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("collect tree %s: %v", root, err)
	}
	return out
}

func TestReleaseTagMismatchFailsLoudly(t *testing.T) {
	skipIfNoGit(t)
	pool := testPool(t)
	ctx := seedZMAT(t, pool)
	authCtx := zmatAuthCtx(ctx)

	baseDir := t.TempDir()
	mat := gitmaterializer.NewMaterializer(pool, baseDir, testLogger())

	svc := newReleaseService(pool)
	if _, err := svc.Create(authCtx, zmatProjectID, release.CreateInput{Version: "7.2.0"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := mat.ProcessPending(ctx, 100); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}

	repo := filepath.Join(baseDir, zmatProjectID)

	// Corrupt: retag v7.2.0 at a dummy commit whose tree differs from the
	// archive-derived snapshot (empty tree).
	emptyTree := gitOut(t, repo, "hash-object", "-t", "tree", "/dev/null")
	// -c identity flags: CI runners have no global git identity and both
	// commit-tree and annotated tags refuse to run without one.
	dummy, err := exec.Command("git", "-C", repo,
		"-c", "user.name=pletka-test", "-c", "user.email=test@pletka.local",
		"commit-tree", emptyTree, "-m", "dummy").Output()
	if err != nil {
		t.Fatalf("commit-tree dummy: %v", err)
	}
	dummySHA := strings.TrimSpace(string(dummy))
	if _, err := exec.Command("git", "-C", repo, "tag", "-d", "v7.2.0").Output(); err != nil {
		t.Fatalf("tag -d: %v", err)
	}
	if err := exec.Command("git", "-C", repo,
		"-c", "user.name=pletka-test", "-c", "user.email=test@pletka.local",
		"tag", "-a", "v7.2.0", "-m", "corrupt", dummySHA).Run(); err != nil {
		t.Fatalf("retag: %v", err)
	}
	tagBefore, _ := revParseQuiet(t, repo, "v7.2.0")

	// Enqueue a duplicate release change set.
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_change_set (project_id, actor_name, actor_email, commit_message, closed_at, kind, release_version)
		VALUES ('ZMAT','pletka-system','system@pletka.local','Release v7.2.0', now(), 'release', '7.2.0')
	`); err != nil {
		t.Fatalf("insert duplicate change set: %v", err)
	}
	if _, err := mat.ProcessPending(ctx, 100); err != nil {
		t.Fatalf("ProcessPending returned error, want nil (mismatch is a processed outcome): %v", err)
	}

	outcome, errMsg, _ := changeSetOutcome(t, pool, "7.2.0")
	if outcome != "release_tag_mismatch" {
		t.Fatalf("expected outcome 'release_tag_mismatch', got %q", outcome)
	}
	if !strings.Contains(errMsg, emptyTree) || !strings.Contains(errMsg, "!=") {
		t.Fatalf("expected materialized_error to mention both trees, got %q", errMsg)
	}

	// Tag NOT modified.
	tagAfter, _ := revParseQuiet(t, repo, "v7.2.0")
	if tagAfter != tagBefore {
		t.Fatalf("tag v7.2.0 was modified: %s -> %s", tagBefore, tagAfter)
	}
}

func TestReleaseArchivedFlow(t *testing.T) {
	skipIfNoGit(t)
	pool := testPool(t)
	ctx := seedZMAT(t, pool)
	authCtx := zmatAuthCtx(ctx)

	baseDir := t.TempDir()
	mat := gitmaterializer.NewMaterializer(pool, baseDir, testLogger())

	svc := newReleaseService(pool)
	if _, err := svc.Create(authCtx, zmatProjectID, release.CreateInput{Version: "7.3.0"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := mat.ProcessPending(ctx, 100); err != nil {
		t.Fatalf("ProcessPending (create): %v", err)
	}

	repo := filepath.Join(baseDir, zmatProjectID)
	tagBefore, ok := revParseQuiet(t, repo, "v7.3.0")
	if !ok {
		t.Fatalf("tag v7.3.0 not created")
	}

	if _, err := svc.Archive(authCtx, zmatProjectID, "7.3.0", release.ArchiveInput{Message: "superseded"}); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if _, err := mat.ProcessPending(ctx, 100); err != nil {
		t.Fatalf("ProcessPending (archive): %v", err)
	}

	// releases branch now has 2 commits.
	if got := gitOut(t, repo, "rev-list", "--count", "refs/heads/releases"); got != "2" {
		t.Fatalf("expected 2 commits on releases after archive, got %s", got)
	}

	// tip's index entry carries archived_at + archived_message.
	tip := decodeIndexAt(t, repo, "refs/heads/releases")
	if len(tip.Releases) != 1 {
		t.Fatalf("expected 1 index entry at tip, got %d", len(tip.Releases))
	}
	if tip.Releases[0].ArchivedAt == "" || tip.Releases[0].ArchivedMessage != "superseded" {
		t.Fatalf("expected archived_at + message at tip, got %+v", tip.Releases[0])
	}

	// tag v7.3.0 SHA unchanged.
	tagAfter, _ := revParseQuiet(t, repo, "v7.3.0")
	if tagAfter != tagBefore {
		t.Fatalf("tag v7.3.0 changed by archive: %s -> %s", tagBefore, tagAfter)
	}

	// tag's own commit still shows the pre-archive index (no archived_at).
	frozen := decodeIndexAt(t, repo, "v7.3.0")
	if len(frozen.Releases) != 1 {
		t.Fatalf("expected 1 index entry in frozen tag tree, got %d", len(frozen.Releases))
	}
	if frozen.Releases[0].ArchivedAt != "" {
		t.Fatalf("expected frozen tag index to have no archived_at, got %+v", frozen.Releases[0])
	}
}

func TestTwoWorkersNoCorruption(t *testing.T) {
	skipIfNoGit(t)
	pool := testPool(t)
	ctx := seedZMAT(t, pool)
	authCtx := zmatAuthCtx(ctx)

	baseDir := t.TempDir()

	svc := newReleaseService(pool)
	if _, err := svc.Create(authCtx, zmatProjectID, release.CreateInput{Version: "7.4.0"}); err != nil {
		t.Fatalf("Create 7.4.0: %v", err)
	}
	if _, err := svc.Create(authCtx, zmatProjectID, release.CreateInput{Version: "7.4.1"}); err != nil {
		t.Fatalf("Create 7.4.1: %v", err)
	}

	// Two Materializer instances on the SAME baseDir, draining concurrently.
	matA := gitmaterializer.NewMaterializer(pool, baseDir, testLogger())
	matB := gitmaterializer.NewMaterializer(pool, baseDir, testLogger())

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, m := range []*gitmaterializer.Materializer{matA, matB} {
		wg.Add(1)
		go func(mm *gitmaterializer.Materializer) {
			defer wg.Done()
			if _, err := mm.ProcessPending(ctx, 100); err != nil {
				errs <- err
			}
		}(m)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent ProcessPending: %v", err)
	}

	repo := filepath.Join(baseDir, zmatProjectID)

	// Both tags exist.
	if _, ok := revParseQuiet(t, repo, "v7.4.0"); !ok {
		t.Fatalf("tag v7.4.0 missing")
	}
	if _, ok := revParseQuiet(t, repo, "v7.4.1"); !ok {
		t.Fatalf("tag v7.4.1 missing")
	}

	// Exactly 2 commits on releases.
	if got := gitOut(t, repo, "rev-list", "--count", "refs/heads/releases"); got != "2" {
		t.Fatalf("expected 2 commits on releases, got %s", got)
	}

	// git fsck clean.
	if out, err := exec.Command("git", "-C", repo, "fsck", "--full").CombinedOutput(); err != nil {
		t.Fatalf("git fsck failed: %v\n%s", err, out)
	}
}
