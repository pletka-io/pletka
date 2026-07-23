//go:build integration

package project_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/project"
)

// TestDeleteProject_GuardsAndCascade exercises the guarded project delete:
// external dependents block it (children, adoption receipts, cross-project
// value refs), and an unblocked delete removes every owned row without
// touching neighbors. Runs against the per-package clone, so the real
// fixture projects double as both the neighbor set and the guard cases.
func TestDeleteProject_GuardsAndCascade(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := project.NewPostgresStore(pool)
	svc := project.NewService(store, nil, nil, nil)

	// --- Guard cases against the REAL fixtures (read-only) ---

	// LA has children (ING and AME's vendored parents inherit from it) and
	// adoption receipts elsewhere: must be blocked.
	blockers, err := store.DeleteBlockers(ctx, testdb.FixtureParent)
	if err != nil {
		t.Fatalf("DeleteBlockers(%s): %v", testdb.FixtureParent, err)
	}
	if !blockers.Blocked() || blockers.Children == 0 {
		t.Fatalf("expected %s to be blocked via children, got %+v", testdb.FixtureParent, blockers)
	}
	if _, err := svc.DeleteProject(ctx, testdb.FixtureParent); err == nil {
		t.Fatalf("DeleteProject(%s) unexpectedly allowed", testdb.FixtureParent)
	}

	// --- Cascade case on a synthetic project ---

	const pid = "ZDEL"
	uiName, _ := json.Marshal(map[string]string{"en": "Delete Probe"})
	cleanup := func() {
		bg := context.Background()
		for _, stmt := range []string{
			`DELETE FROM weave_field_overrides WHERE entity_id LIKE 'ZDEL%' OR field_id IN (SELECT id FROM weave_fields WHERE project_id='ZDEL')`,
			`DELETE FROM weave_fields WHERE project_id='ZDEL'`,
			`DELETE FROM weave_models WHERE project_id='ZDEL'`,
			`DELETE FROM weave_categories WHERE project_id='ZDEL'`,
			`DELETE FROM weave_memberships WHERE scope_type='project' AND scope_id='ZDEL'`,
			`DELETE FROM weave_projects WHERE id='ZDEL'`,
			`DELETE FROM weave_actors WHERE id='ZDEL_OWNER'`,
		} {
			_, _ = pool.Exec(bg, stmt)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed %q: %v", sql, err)
		}
	}
	mustExec(`INSERT INTO weave_actors (id, type, display_name, slug, visibility, created_at, updated_at)
	          VALUES ('ZDEL_OWNER','organization','Delete Probe Org','zdel-owner','private',NOW(),NOW())`)
	mustExec(`INSERT INTO weave_projects (id, ui_name, description, status, owner_id, visibility, created_at, updated_at)
	          VALUES ($1,$2,$2,'draft','ZDEL_OWNER','private',NOW(),NOW())`, pid, uiName)
	mustExec(`INSERT INTO weave_categories (id, semantic_id, project_id, ui_name, status, created_at, updated_at)
	          VALUES ('ZDEL.CAT.1','ZDEL.CAT.1',$1,$2,'draft',NOW(),NOW())`, pid, uiName)
	mustExec(`INSERT INTO weave_fields (id, semantic_id, system_name, project_id, ui_name, status, path_elements, created_at, updated_at)
	          VALUES ('ZDELF1','ZDELF.1','delete_probe',$1,$2,'draft','[]',NOW(),NOW())`, pid, uiName)
	mustExec(`INSERT INTO weave_models (id, project_id, ui_name, status, created_at, updated_at)
	          VALUES ('ZDELM.1',$1,$2,'draft',NOW(),NOW())`, pid, uiName)
	mustExec(`INSERT INTO weave_field_overrides (field_id, entity_type, entity_id, project_id, created_at, updated_at)
	          VALUES ('ZDELF1','model','ZDELM.1',$1,NOW(),NOW())`, pid)
	mustExec(`INSERT INTO weave_memberships (actor_id, scope_type, scope_id, role, created_at)
	          VALUES ('ZDEL_OWNER','project',$1,'owner',NOW())`, pid)

	blockers, err = store.DeleteBlockers(ctx, pid)
	if err != nil {
		t.Fatalf("DeleteBlockers(%s): %v", pid, err)
	}
	if blockers.Blocked() {
		t.Fatalf("synthetic project unexpectedly blocked: %+v", blockers)
	}

	before := countProjects(ctx, t, pool)
	stats, err := svc.DeleteProject(ctx, pid)
	if err != nil {
		t.Fatalf("DeleteProject(%s): %v", pid, err)
	}
	if !stats.ProjectDeleted || stats.Fields != 1 || stats.Models != 1 || stats.Categories != 1 || stats.Overrides != 1 {
		t.Fatalf("unexpected delete stats: %+v", stats)
	}

	for _, check := range []struct {
		name, sql string
	}{
		{"project row", `SELECT count(*) FROM weave_projects WHERE id='ZDEL'`},
		{"fields", `SELECT count(*) FROM weave_fields WHERE project_id='ZDEL'`},
		{"models", `SELECT count(*) FROM weave_models WHERE project_id='ZDEL'`},
		{"categories", `SELECT count(*) FROM weave_categories WHERE project_id='ZDEL'`},
		{"overrides", `SELECT count(*) FROM weave_field_overrides WHERE entity_id='ZDELM.1'`},
		{"memberships", `SELECT count(*) FROM weave_memberships WHERE scope_type='project' AND scope_id='ZDEL'`},
	} {
		var n int
		if err := pool.QueryRow(ctx, check.sql).Scan(&n); err != nil {
			t.Fatalf("verify %s: %v", check.name, err)
		}
		if n != 0 {
			t.Errorf("%s not fully deleted: %d rows remain", check.name, n)
		}
	}

	// Neighbors untouched: exactly one project gone.
	if after := countProjects(ctx, t, pool); after != before-1 {
		t.Fatalf("project count %d -> %d; expected exactly one deletion", before, after)
	}
}

func countProjects(ctx context.Context, t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM weave_projects`).Scan(&n); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	return n
}
