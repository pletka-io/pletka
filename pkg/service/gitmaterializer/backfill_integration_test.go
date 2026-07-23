//go:build integration

package gitmaterializer_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

// zbkfProjectID is the synthetic project the backfill test materializes
// releases for. Distinct from ZMAT (release_materialize_integration_test.go)
// and ZREL (pkg/weave/release outbox tests) so the shared per-package clone
// never collides; every version it cuts stays in the 6.x range.
const zbkfProjectID = "ZBKF"

// seedZBKF creates a synthetic project mirroring seedZMAT: one owner actor,
// project, category, field, model, base override. Registers a cleanup that
// removes both the live rows and the archive rows Create's snapshot step
// writes (global namespace bindings archived under this test's release
// versions are scoped by version_number LIKE '6.%').
func seedZBKF(t *testing.T, pool *pgxpool.Pool) context.Context {
	t.Helper()
	ctx := context.Background()

	purge := func() {
		bg := context.Background()
		for _, stmt := range []string{
			`DELETE FROM weave_change_set WHERE project_id='ZBKF'`,
			`DELETE FROM weave_releases WHERE project_id='ZBKF'`,
			`DELETE FROM weave_field_overrides WHERE entity_id LIKE 'ZBKF%' OR field_id IN (SELECT id FROM weave_fields WHERE project_id='ZBKF')`,
			`DELETE FROM weave_fields WHERE project_id='ZBKF'`,
			`DELETE FROM weave_models WHERE project_id='ZBKF'`,
			`DELETE FROM weave_categories WHERE project_id='ZBKF'`,
			`DELETE FROM weave_memberships WHERE scope_type='project' AND scope_id='ZBKF'`,
			`DELETE FROM weave_projects WHERE id='ZBKF'`,
			`DELETE FROM weave_actors WHERE id='ZBKF_OWNER'`,
			`DELETE FROM weave_field_overrides_archive WHERE project_id='ZBKF'`,
			`DELETE FROM weave_fields_archive WHERE project_id='ZBKF'`,
			`DELETE FROM weave_models_archive WHERE project_id='ZBKF'`,
			`DELETE FROM weave_categories_archive WHERE project_id='ZBKF'`,
			`DELETE FROM weave_projects_archive WHERE id='ZBKF'`,
			`DELETE FROM weave_namespace_bindings_archive WHERE version_number LIKE '6.%'`,
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

	uiName, _ := json.Marshal(map[string]string{"en": "Backfill Probe"})
	mustExec(`INSERT INTO weave_actors (id, type, display_name, slug, visibility, created_at, updated_at)
	          VALUES ('ZBKF_OWNER','organization','Backfill Probe Org','zbkf-owner','private',NOW(),NOW())`)
	mustExec(`INSERT INTO weave_projects (id, ui_name, description, status, owner_id, visibility, created_at, updated_at)
	          VALUES ($1,$2,$2,'draft','ZBKF_OWNER','private',NOW(),NOW())`, zbkfProjectID, uiName)
	mustExec(`INSERT INTO weave_categories (id, semantic_id, project_id, ui_name, status, created_at, updated_at)
	          VALUES ('ZBKF.CAT.1','ZBKF.CAT.1',$1,$2,'draft',NOW(),NOW())`, zbkfProjectID, uiName)
	mustExec(`INSERT INTO weave_fields (id, semantic_id, system_name, project_id, ui_name, status, path_elements, created_at, updated_at)
	          VALUES ('ZBKFF1','ZBKFF.1','backfill_probe',$1,$2,'draft','[]',NOW(),NOW())`, zbkfProjectID, uiName)
	mustExec(`INSERT INTO weave_models (id, project_id, ui_name, status, created_at, updated_at)
	          VALUES ('ZBKFM.1',$1,$2,'draft',NOW(),NOW())`, zbkfProjectID, uiName)
	mustExec(`INSERT INTO weave_field_overrides (field_id, entity_type, entity_id, project_id, created_at, updated_at)
	          VALUES ('ZBKFF1','model','ZBKFM.1',$1,NOW(),NOW())`, zbkfProjectID)

	return ctx
}

func zbkfAuthCtx(ctx context.Context) context.Context {
	authCtx := weaveauth.WithSnapshot(ctx, &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	return weaveauth.WithPrincipal(authCtx, &weaveauth.Principal{ActorID: "ZBKF_OWNER"})
}

// unprocessedReleaseChangeSets counts unprocessed kind='release' change sets
// for projectID.
func unprocessedReleaseChangeSets(t *testing.T, pool *pgxpool.Pool, projectID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM weave_change_set
		WHERE project_id = $1 AND kind = 'release' AND processed_at IS NULL
	`, projectID).Scan(&count); err != nil {
		t.Fatalf("count unprocessed release change sets: %v", err)
	}
	return count
}

// TestEnqueueMissingReleases simulates the fleet's pre-stage-1 releases: rows
// in weave_releases with no outbox trail at all (created before the 'release'
// change-set kind existed). EnqueueMissingReleases must backfill exactly one
// change set per missing tag, ProcessPending must materialize both tags, and a
// second EnqueueMissingReleases run must not re-enqueue once the tags exist.
func TestEnqueueMissingReleases(t *testing.T) {
	skipIfNoGit(t)
	pool := testPool(t)
	ctx := seedZBKF(t, pool)
	authCtx := zbkfAuthCtx(ctx)

	baseDir := t.TempDir()
	mat := gitmaterializer.NewMaterializer(pool, baseDir, testLogger())

	svc := newReleaseService(pool)
	if _, err := svc.Create(authCtx, zbkfProjectID, release.CreateInput{Version: "6.0.0"}); err != nil {
		t.Fatalf("Create 6.0.0: %v", err)
	}
	if _, err := svc.Create(authCtx, zbkfProjectID, release.CreateInput{Version: "6.1.0"}); err != nil {
		t.Fatalf("Create 6.1.0: %v", err)
	}

	// Simulate pre-stage-1 history: the releases exist, but delete their
	// auto-enqueued change sets so weave_releases has no outbox trail at all.
	if _, err := pool.Exec(ctx, `DELETE FROM weave_change_set WHERE project_id = $1 AND kind = 'release'`, zbkfProjectID); err != nil {
		t.Fatalf("delete auto-enqueued change sets: %v", err)
	}
	if got := unprocessedReleaseChangeSets(t, pool, zbkfProjectID); got != 0 {
		t.Fatalf("expected 0 release change sets after simulated pre-stage-1 delete, got %d", got)
	}

	if _, _, err := mat.EnqueueMissingReleases(ctx); err != nil {
		t.Fatalf("EnqueueMissingReleases: %v", err)
	}
	if got := unprocessedReleaseChangeSets(t, pool, zbkfProjectID); got != 2 {
		t.Fatalf("expected 2 unprocessed release change sets for %s, got %d", zbkfProjectID, got)
	}

	if _, err := mat.ProcessPending(ctx, 100); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}

	repo := filepath.Join(baseDir, zbkfProjectID)
	if _, ok := revParseQuiet(t, repo, "v6.0.0"); !ok {
		t.Fatalf("tag v6.0.0 not created")
	}
	if _, ok := revParseQuiet(t, repo, "v6.1.0"); !ok {
		t.Fatalf("tag v6.1.0 not created")
	}

	// Re-running mid-drain (or after) must not double-enqueue: both tags now
	// exist, so nothing new should appear for ZBKF.
	if _, _, err := mat.EnqueueMissingReleases(ctx); err != nil {
		t.Fatalf("EnqueueMissingReleases (rerun): %v", err)
	}
	if got := unprocessedReleaseChangeSets(t, pool, zbkfProjectID); got != 0 {
		t.Fatalf("expected 0 unprocessed release change sets for %s after rerun, got %d", zbkfProjectID, got)
	}
}
