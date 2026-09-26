//go:build integration

package field

import (
	"context"
	"testing"
)

// TestDeleteFieldRemovesItsOverrideRows is the live-DB gate for the leak that
// field.Service.Delete's comment used to deny: it claimed "Base override is
// removed by FK cascade on weave_fields", but weave_field_overrides carries no
// foreign key to weave_fields at all — its only FK is to weave_import_staging.
// So every field delete left its base override row behind, carrying a
// category_id that WeaveListCategoriesWithCounts still counts and that keeps a
// category permanently "in use" with no drill-down able to explain the number.
//
// The field's own in-use guard cannot catch this: UsageReport carries only
// ModelCount and CollectionCount, so InUse() never sees a base row and the
// delete proceeds.
//
// Seeds its own scenario under a synthetic project id rather than leaning on
// ambient dev-DB data — this package's TestMain runs testdb.Setup, so it
// always executes against an isolated fixture clone.
func TestDeleteFieldRemovesItsOverrideRows(t *testing.T) {
	pool := batchUsageRefsTestPool(t)
	ctx := context.Background()

	const (
		actorID    = "TSTDOV_ACTOR"
		projectID  = "TSTDOV"
		categoryID = "TSTDOV.CAT.1"
		// The field under test, plus a second field whose base override must
		// survive — a delete scoped too widely would take both.
		deletedFieldID   = "TSTDOVF.1"
		survivingFieldID = "TSTDOVF.2"
	)

	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, type, display_name, system_name, slug, email)
		VALUES ($1,'human','Delete Override Test','tstdov_test','tstdov-test','tstdov@test.local')
		ON CONFLICT (id) DO NOTHING`, actorID); err != nil {
		t.Fatalf("seed actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, projectID, actorID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_categories (id, project_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, categoryID, projectID); err != nil {
		t.Fatalf("seed category: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_fields (id, project_id) VALUES ($1,$3), ($2,$3)
		ON CONFLICT (id) DO NOTHING`, deletedFieldID, survivingFieldID, projectID); err != nil {
		t.Fatalf("seed fields: %v", err)
	}
	// The base override rows: entity_type '' is what makes a row the base
	// placement, and category_id is what makes the leak visible in a
	// category's in-use count.
	if _, err := pool.Exec(ctx, `INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id, category_id)
		VALUES ($1,$3,'','',$4), ($2,$3,'','',$4)`,
		deletedFieldID, survivingFieldID, projectID, categoryID); err != nil {
		t.Fatalf("seed base overrides: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_categories WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, actorID)
	})

	countOverrides := func(fieldID string) int {
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM weave_field_overrides WHERE field_id = $1`, fieldID).Scan(&n); err != nil {
			t.Fatalf("count overrides for %s: %v", fieldID, err)
		}
		return n
	}

	if got := countOverrides(deletedFieldID); got != 1 {
		t.Fatalf("precondition: overrides for the field about to be deleted = %d, want 1", got)
	}

	store := NewPostgresStore(pool)
	if err := store.Delete(ctx, deletedFieldID); err != nil {
		t.Fatalf("delete field: %v", err)
	}

	if got := countOverrides(deletedFieldID); got != 0 {
		t.Errorf("deleted field left %d override row(s) behind, want 0 — the row is orphaned and still counts against its category", got)
	}
	if got := countOverrides(survivingFieldID); got != 1 {
		t.Errorf("overrides for the untouched field = %d, want 1 — the delete reached beyond its own field", got)
	}

	var fieldRows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM weave_fields WHERE id = $1`, deletedFieldID).Scan(&fieldRows); err != nil {
		t.Fatalf("count field rows: %v", err)
	}
	if fieldRows != 0 {
		t.Errorf("field row count after delete = %d, want 0", fieldRows)
	}
}
