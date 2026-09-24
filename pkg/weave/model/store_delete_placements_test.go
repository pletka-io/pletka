//go:build integration

package model

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestDelete_RemovesOwnPlacementsAndRefs proves that deleting a model
// removes the placement rows the model owns (weave_field_overrides with
// entity_type='model', entity_id=<model>) and that those rows' refs
// (weave_override_refs) go with them, in the same transaction as the model
// delete. weave_field_overrides carries no FK on entity_id, so a bare
// `DELETE FROM weave_models` used to leave these rows behind — permanently
// inflating any category they were tagged with.
//
// It also proves deleting one model's placements does not touch a
// different model's placements (a sibling override+ref pair survives).
func TestDelete_RemovesOwnPlacementsAndRefs(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	seedActAndAssertPlacementDelete(t, pool, ctx)
}

func seedActAndAssertPlacementDelete(t *testing.T, pool *pgxpool.Pool, ctx context.Context) {
	t.Helper()
	const (
		actorID   = "TSTDELP_ACTOR"
		projectID = "TSTDELP"
		fieldID   = "TSTDELPF.1"
		modelID   = "TSTDELPM.1" // deleted by the test
		otherID   = "TSTDELPM.2" // sibling model, must survive untouched
	)

	mustExec(t, pool, ctx, `INSERT INTO weave_actors (id, type, display_name, system_name, slug, email)
		VALUES ($1,'human','Del Placements Test','del_placements_test','del-placements-test','delp@test.local')
		ON CONFLICT (id) DO NOTHING`, actorID)
	mustExec(t, pool, ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, projectID, actorID)
	mustExec(t, pool, ctx, `INSERT INTO weave_fields (id, project_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, fieldID, projectID)
	mustExec(t, pool, ctx, `INSERT INTO weave_models (id, project_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, modelID, projectID)
	mustExec(t, pool, ctx, `INSERT INTO weave_models (id, project_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, otherID, projectID)

	var overrideID, otherOverrideID int64
	if err := pool.QueryRow(ctx, `INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id)
		VALUES ($1,$2,'model',$3) RETURNING id`, fieldID, projectID, modelID).Scan(&overrideID); err != nil {
		t.Fatalf("seed override: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id)
		VALUES ($1,$2,'model',$3) RETURNING id`, fieldID, projectID, otherID).Scan(&otherOverrideID); err != nil {
		t.Fatalf("seed other override: %v", err)
	}
	mustExec(t, pool, ctx, `INSERT INTO weave_override_refs (override_id, ref_type, target_id, semantic_id, position)
		VALUES ($1,'resource_model','TSTDELP_TARGET','TSTDELP_TARGET',0)`, overrideID)
	mustExec(t, pool, ctx, `INSERT INTO weave_override_refs (override_id, ref_type, target_id, semantic_id, position)
		VALUES ($1,'resource_model','TSTDELP_TARGET','TSTDELP_TARGET',0)`, otherOverrideID)

	t.Cleanup(func() {
		mustExecCleanup(pool, ctx, `DELETE FROM weave_override_refs WHERE override_id IN ($1,$2)`, overrideID, otherOverrideID)
		mustExecCleanup(pool, ctx, `DELETE FROM weave_field_overrides WHERE id IN ($1,$2)`, overrideID, otherOverrideID)
		mustExecCleanup(pool, ctx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
		mustExecCleanup(pool, ctx, `DELETE FROM weave_fields WHERE id = $1`, fieldID)
		mustExecCleanup(pool, ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		mustExecCleanup(pool, ctx, `DELETE FROM weave_actors WHERE id = $1`, actorID)
	})

	store := NewPostgresStore(pool)
	if err := store.Delete(ctx, modelID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var modelRows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM weave_models WHERE id = $1`, modelID).Scan(&modelRows); err != nil {
		t.Fatalf("count model rows: %v", err)
	}
	if modelRows != 0 {
		t.Errorf("model row survived delete: got %d, want 0", modelRows)
	}

	var overrideRows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM weave_field_overrides WHERE id = $1`, overrideID).Scan(&overrideRows); err != nil {
		t.Fatalf("count override rows: %v", err)
	}
	if overrideRows != 0 {
		t.Errorf("placement row survived model delete: got %d, want 0 (this is the leak the fix closes)", overrideRows)
	}

	var refRows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM weave_override_refs WHERE override_id = $1`, overrideID).Scan(&refRows); err != nil {
		t.Fatalf("count ref rows: %v", err)
	}
	if refRows != 0 {
		t.Errorf("override ref survived model delete: got %d, want 0 (should cascade from weave_field_overrides)", refRows)
	}

	var otherOverrideRows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM weave_field_overrides WHERE id = $1`, otherOverrideID).Scan(&otherOverrideRows); err != nil {
		t.Fatalf("count other override rows: %v", err)
	}
	if otherOverrideRows != 1 {
		t.Errorf("sibling model's placement was touched: got %d rows, want 1", otherOverrideRows)
	}

	var otherRefRows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM weave_override_refs WHERE override_id = $1`, otherOverrideID).Scan(&otherRefRows); err != nil {
		t.Fatalf("count other ref rows: %v", err)
	}
	if otherRefRows != 1 {
		t.Errorf("sibling model's override ref was touched: got %d rows, want 1", otherRefRows)
	}

	var otherModelRows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM weave_models WHERE id = $1`, otherID).Scan(&otherModelRows); err != nil {
		t.Fatalf("count sibling model rows: %v", err)
	}
	if otherModelRows != 1 {
		t.Errorf("sibling model row was touched: got %d, want 1", otherModelRows)
	}
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
