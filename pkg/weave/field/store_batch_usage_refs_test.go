//go:build integration

package field

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func batchUsageRefsTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:pw123@localhost:5433/pletka_weave?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skip("database not available:", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Skip("database not reachable:", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// TestBatchUsageRefs seeds a throwaway project with a field placed in one
// model and one collection (via weave_field_overrides), and an orphan field
// with no placements. Asserts the placed field resolves both refs, the
// orphan is absent from the map, and empty input yields an empty map with
// no error.
func TestBatchUsageRefs(t *testing.T) {
	pool := batchUsageRefsTestPool(t)
	ctx := context.Background()
	const (
		actorID       = "TST_BUR_ACTOR"
		projectID     = "TSTBUR"
		placedFieldID = "TSTBURF.1"
		orphanFieldID = "TSTBURF.2"
		modelID       = "TSTBURM.1"
		collectionID  = "TSTBURC.1"
	)

	_, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, type, display_name, system_name, slug, email)
		VALUES ($1,'human','BUR Test','bur_test','bur-test','bur@test.local') ON CONFLICT (id) DO NOTHING`, actorID)
	if err != nil {
		t.Fatalf("seed actor: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, projectID, actorID)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_fields (id, project_id) VALUES ($1,$2), ($3,$2)
		ON CONFLICT (id) DO NOTHING`, placedFieldID, projectID, orphanFieldID)
	if err != nil {
		t.Fatalf("seed fields: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_models (id, project_id, system_name) VALUES ($1,$2,'bur_model')
		ON CONFLICT (id) DO NOTHING`, modelID, projectID)
	if err != nil {
		t.Fatalf("seed model: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_collections (id, project_id, system_name) VALUES ($1,$2,'bur_collection')
		ON CONFLICT (id) DO NOTHING`, collectionID, projectID)
	if err != nil {
		t.Fatalf("seed collection: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id)
		VALUES ($1,$2,'model',$3), ($1,$2,'collection',$4)`, placedFieldID, projectID, modelID, collectionID)
	if err != nil {
		t.Fatalf("seed overrides: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_collections WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id=$1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id=$1`, actorID)
	})

	store := NewPostgresStore(pool)

	// Empty input -> empty map, no error.
	empty, err := store.BatchUsageRefs(ctx, nil)
	if err != nil {
		t.Fatalf("BatchUsageRefs(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("BatchUsageRefs(nil) = %#v, want empty map", empty)
	}

	got, err := store.BatchUsageRefs(ctx, []string{placedFieldID, orphanFieldID})
	if err != nil {
		t.Fatalf("BatchUsageRefs: %v", err)
	}

	if _, ok := got[orphanFieldID]; ok {
		t.Errorf("orphan field %q present in result, want absent", orphanFieldID)
	}

	placed, ok := got[placedFieldID]
	if !ok {
		t.Fatalf("placed field %q missing from result", placedFieldID)
	}
	if len(placed.Models) != 1 || placed.Models[0].ID != modelID || placed.Models[0].SemanticID != modelID {
		t.Errorf("placed.Models = %#v, want one ref with ID/SemanticID %q", placed.Models, modelID)
	}
	if len(placed.Models) == 1 && placed.Models[0].SystemName != "bur_model" {
		t.Errorf("placed.Models[0].SystemName = %q, want %q", placed.Models[0].SystemName, "bur_model")
	}
	if len(placed.Collections) != 1 || placed.Collections[0].ID != collectionID || placed.Collections[0].SemanticID != collectionID {
		t.Errorf("placed.Collections = %#v, want one ref with ID/SemanticID %q", placed.Collections, collectionID)
	}
	if len(placed.Collections) == 1 && placed.Collections[0].SystemName != "bur_collection" {
		t.Errorf("placed.Collections[0].SystemName = %q, want %q", placed.Collections[0].SystemName, "bur_collection")
	}
}
