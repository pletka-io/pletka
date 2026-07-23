//go:build integration

package pathaudit_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/pathaudit"
)

// completeTestProjectID is the throwaway project seeded by
// seedProjectWithCompletePath. Chosen to be distinct from any real project
// system_name in the shared test database.
const completeTestProjectID = "ZZTEST"

// seedProjectWithCompletePath inserts a throwaway project ("ZZTEST") and one
// field whose path_elements ends in a stray {type:"complete"} element — the
// pre-migration path-editor "complete" marker, before it moved to a boolean
// flag. Registers cleanup to remove the seeded rows when the test finishes;
// weave_path_elements rows cascade automatically via
// weave_path_elements_field_id_fkey (ON DELETE CASCADE).
func seedProjectWithCompletePath(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	const (
		actorID = "ZZTEST_ACTOR"
		fieldID = "ZZTESTF.1"
	)

	_, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, type, display_name, system_name, slug, email)
		VALUES ($1,'human','ZZTest Actor','zztest_actor','zztest-actor','zztest@test.local')
		ON CONFLICT (id) DO NOTHING`, actorID)
	if err != nil {
		t.Fatalf("seed actor: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_projects (id, system_name, owner_id)
		VALUES ($1,$1,$2)
		ON CONFLICT (id) DO NOTHING`, completeTestProjectID, actorID)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}

	const pathElements = `[
		{"type":"class","uri":"skos:Concept","prefix":"skos","local_name":"Concept","position":0},
		{"type":"complete","uri":"","prefix":"","local_name":"complete","position":1}
	]`
	_, err = pool.Exec(ctx, `INSERT INTO weave_fields (id, project_id, semantic_id, path_elements)
		VALUES ($1,$2,$1,$3::jsonb)
		ON CONFLICT (id) DO NOTHING`, fieldID, completeTestProjectID, pathElements)
	if err != nil {
		t.Fatalf("seed field: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id=$1`, completeTestProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id=$1`, completeTestProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id=$1`, actorID)
	})
}

// TestAuditSkipsCompleteMarker inserts a field whose path ends in a stray
// {type:"complete"} element and asserts the audit does not flag it, the same
// way it already ignores type='literal'.
func TestAuditSkipsCompleteMarker(t *testing.T) {
	// Runs against this package's per-package fixture clone (TestMain ->
	// testdb.Setup). The test seeds its own throwaway project/actor/field, so
	// it needs nothing from the hydrated fixture set beyond a working clone.
	pool := testdb.Pool(t)
	ctx := context.Background()

	seedProjectWithCompletePath(t, pool)

	errs, _, err := pathaudit.NewService(pool).Audit(ctx, completeTestProjectID)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	for _, e := range errs {
		if e.Qname == ":complete" {
			t.Fatalf("audit flagged the complete marker: %+v", e)
		}
	}
}
