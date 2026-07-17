package model

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func usageTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:pw123@localhost:5433/zellij_weave_multipath?sslmode=disable"
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

func TestUsageGlobal_CountsCrossProjectRefs(t *testing.T) {
	pool := usageTestPool(t)
	ctx := context.Background()
	const (
		actorID = "TST_UG_ACTOR"
		projB   = "TSTUGB"    // referencing project
		modelID = "TSTUGAM.1" // model owned by a different (owner) project
		fieldID = "TSTUGBF.1" // field in project B
	)

	// Seed: actor → project B → field in B → base override in B → override-ref targeting the model.
	_, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, type, display_name, system_name, slug, email)
		VALUES ($1,'human','UG Test','ug_test','ug-test','ug@test.local') ON CONFLICT (id) DO NOTHING`, actorID)
	if err != nil {
		t.Fatalf("seed actor: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, projB, actorID)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_fields (id, project_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, fieldID, projB)
	if err != nil {
		t.Fatalf("seed field: %v", err)
	}
	var overrideID int64
	err = pool.QueryRow(ctx, `INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id)
		VALUES ($1,$2,'','') RETURNING id`, fieldID, projB).Scan(&overrideID)
	if err != nil {
		t.Fatalf("seed override: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_override_refs (override_id, ref_type, target_id, semantic_id, position)
		VALUES ($1,'resource_model',$2,$2,0)`, overrideID, modelID)
	if err != nil {
		t.Fatalf("seed override ref: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_override_refs WHERE override_id=$1`, overrideID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides WHERE id=$1`, overrideID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE id=$1`, fieldID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id=$1`, projB)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id=$1`, actorID)
	})

	store := NewPostgresStore(pool)

	global, err := store.UsageGlobal(ctx, modelID)
	if err != nil {
		t.Fatalf("UsageGlobal: %v", err)
	}
	if global.FieldCount != 1 {
		t.Errorf("UsageGlobal FieldCount = %d, want 1", global.FieldCount)
	}
	if len(global.ProjectIDs) != 1 || global.ProjectIDs[0] != projB {
		t.Errorf("UsageGlobal ProjectIDs = %v, want [%s]", global.ProjectIDs, projB)
	}

	// Project-scoped Usage from the model's OWNER project sees nothing (the bug).
	scoped, err := store.Usage(ctx, "TSTUGA", modelID)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if scoped.FieldCount != 0 {
		t.Errorf("owner-project Usage FieldCount = %d, want 0 (cross-project ref is invisible to it)", scoped.FieldCount)
	}
}
