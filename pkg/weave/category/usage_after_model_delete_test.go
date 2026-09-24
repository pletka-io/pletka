//go:build integration

package category

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/model"
)

// TestUsageZeroAfterOwningModelDeleted proves the curator-facing consequence
// of the placement leak: before the fix, deleting a model left its
// weave_field_overrides row behind, so a category the model placed a field
// into kept counting that placement forever, in_use stayed true, and the
// category could never be deleted. With the fix, model.Store.Delete removes
// the model's own placement rows in the same transaction as the model row,
// so the category's usage count returns to zero and the delete preflight
// (IsInUse) unblocks.
func TestUsageZeroAfterOwningModelDeleted(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const (
		actorID   = "TSTDELCAT_ACTOR"
		projectID = "TSTDELCAT"
		fieldID   = "TSTDELCATF.1"
		modelID   = "TSTDELCATM.1"
		catID     = "TSTDELCAT_CAT1"
	)

	mustExec(t, pool, ctx, `INSERT INTO weave_actors (id, type, display_name, system_name, slug, email)
		VALUES ($1,'human','Del Cat Test','del_cat_test','del-cat-test','delcat@test.local')
		ON CONFLICT (id) DO NOTHING`, actorID)
	mustExec(t, pool, ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, projectID, actorID)
	mustExec(t, pool, ctx, `INSERT INTO weave_categories (id, project_id, system_name) VALUES ($1,$2,'del_cat_test')
		ON CONFLICT (id) DO NOTHING`, catID, projectID)
	mustExec(t, pool, ctx, `INSERT INTO weave_fields (id, project_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, fieldID, projectID)
	mustExec(t, pool, ctx, `INSERT INTO weave_models (id, project_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, modelID, projectID)

	var overrideID int64
	if err := pool.QueryRow(ctx, `INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id, category_id)
		VALUES ($1,$2,'model',$3,$4) RETURNING id`, fieldID, projectID, modelID, catID).Scan(&overrideID); err != nil {
		t.Fatalf("seed override: %v", err)
	}

	t.Cleanup(func() {
		mustExecCleanup(pool, ctx, `DELETE FROM weave_field_overrides WHERE id = $1`, overrideID)
		mustExecCleanup(pool, ctx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
		mustExecCleanup(pool, ctx, `DELETE FROM weave_categories WHERE project_id = $1`, projectID)
		mustExecCleanup(pool, ctx, `DELETE FROM weave_fields WHERE id = $1`, fieldID)
		mustExecCleanup(pool, ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		mustExecCleanup(pool, ctx, `DELETE FROM weave_actors WHERE id = $1`, actorID)
	})

	catStore := NewPostgresStore(pool)
	modelStore := model.NewPostgresStore(pool)

	// Before delete: the category is in use and its usage count reflects
	// the model's placement.
	before, err := catStore.IsInUse(ctx, projectID, catID, "")
	if err != nil {
		t.Fatalf("IsInUse (before): %v", err)
	}
	if !before {
		t.Fatalf("IsInUse (before) = false, want true (model still owns a placement in this category)")
	}
	beforeCount := modelFieldCountFor(t, catStore, ctx, projectID, catID)
	if beforeCount != 1 {
		t.Fatalf("model_field_count (before) = %d, want 1", beforeCount)
	}

	// Delete the model — this is the fixed code path under test.
	if err := modelStore.Delete(ctx, modelID); err != nil {
		t.Fatalf("model Delete: %v", err)
	}

	// After delete: usage count is back to zero and the category is no
	// longer in use.
	after, err := catStore.IsInUse(ctx, projectID, catID, "")
	if err != nil {
		t.Fatalf("IsInUse (after): %v", err)
	}
	if after {
		t.Fatalf("IsInUse (after) = true, want false (the model's placement row should be gone) — category usage count never reaches zero and the category can never be deleted")
	}
	afterCount := modelFieldCountFor(t, catStore, ctx, projectID, catID)
	if afterCount != 0 {
		t.Fatalf("model_field_count (after) = %d, want 0", afterCount)
	}

	// The delete preflight now unblocks: the category can be deleted.
	if err := catStore.Delete(ctx, projectID, catID); err != nil {
		t.Fatalf("category Delete: %v (category should now be deletable — usage count is zero)", err)
	}

	var remaining int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM weave_categories WHERE id = $1`, catID).Scan(&remaining); err != nil {
		t.Fatalf("count category rows: %v", err)
	}
	if remaining != 0 {
		t.Errorf("category row survived delete: got %d, want 0", remaining)
	}
}

func modelFieldCountFor(t *testing.T, store Store, ctx context.Context, projectID, catID string) int64 {
	t.Helper()
	rows, err := store.ListWithCounts(ctx, projectID)
	if err != nil {
		t.Fatalf("ListWithCounts: %v", err)
	}
	for _, r := range rows {
		if r.ID == catID {
			return r.ModelFieldCount
		}
	}
	t.Fatalf("category %s not found in ListWithCounts result", catID)
	return -1
}

func mustExec(t *testing.T, pool *pgxpool.Pool, ctx context.Context, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("seed exec %q: %v", sql, err)
	}
}

func mustExecCleanup(pool *pgxpool.Pool, ctx context.Context, sql string, args ...any) {
	_, _ = pool.Exec(ctx, sql, args...)
}
