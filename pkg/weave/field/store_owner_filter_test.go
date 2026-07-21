//go:build integration

package field

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestListFieldsOwnerFilter is the live-DB gate for the owner_id filter on
// WeaveListFields/WeaveCountFields: domain.WithFilter("owner_id", modelID)
// narrows List to exactly the fields placed on that model (rows and count
// agree), and an empty owner value is unfiltered.
//
// The test self-seeds its own scenario under a synthetic project id
// (OWFILT) rather than depending on ambient dev-DB data — this package runs
// TestMain's testdb.Setup, so it always executes against an isolated
// fixture clone that never contains real customer projects.
func TestListFieldsOwnerFilter(t *testing.T) {
	pool := batchUsageRefsTestPool(t)
	ctx := context.Background()

	const (
		actorID   = "OWFILT_ACTOR"
		projectID = "OWFILT"
		modelID   = "OWFILTM.1"
		// Three fields in the project; only two are placed on modelID via
		// weave_field_overrides, so the owner filter is non-vacuous.
		placedFieldID1 = "OWFILTF.1"
		placedFieldID2 = "OWFILTF.2"
		unplacedField  = "OWFILTF.3"
	)

	_, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, type, display_name, system_name, slug, email)
		VALUES ($1,'human','Owner Filter Test','owfilt_test','owfilt-test','owfilt@test.local') ON CONFLICT (id) DO NOTHING`, actorID)
	if err != nil {
		t.Fatalf("seed actor: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, projectID, actorID)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_fields (id, project_id) VALUES ($1,$4), ($2,$4), ($3,$4)
		ON CONFLICT (id) DO NOTHING`, placedFieldID1, placedFieldID2, unplacedField, projectID)
	if err != nil {
		t.Fatalf("seed fields: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_models (id, project_id, system_name) VALUES ($1,$2,'owfilt_model')
		ON CONFLICT (id) DO NOTHING`, modelID, projectID)
	if err != nil {
		t.Fatalf("seed model: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id)
		VALUES ($1,$3,'model',$4), ($2,$3,'model',$4)`, placedFieldID1, placedFieldID2, projectID, modelID)
	if err != nil {
		t.Fatalf("seed overrides: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id=$1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id=$1`, actorID)
	})

	store := NewPostgresStore(pool)

	const wantPlaced = 2
	const wantTotal = 3

	filtered, totalFiltered, err := store.List(ctx,
		domain.WithProjectID(projectID),
		domain.WithLimit(1000),
		domain.WithFilter("owner_id", modelID),
	)
	if err != nil {
		t.Fatalf("list owner-filtered: %v", err)
	}
	if totalFiltered != wantPlaced {
		t.Fatalf("totalFiltered = %d, want %d", totalFiltered, wantPlaced)
	}
	if int64(len(filtered)) != wantPlaced {
		t.Fatalf("len(filtered) = %d, want %d", len(filtered), wantPlaced)
	}

	// Rows and count must describe the same set: every returned field
	// actually carries an override row for this owner.
	for _, f := range filtered {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM weave_field_overrides
				WHERE field_id = $1 AND entity_type IN ('model', 'collection') AND entity_id = $2
			)
		`, f.ID, modelID).Scan(&exists); err != nil {
			t.Fatalf("verify placement for field %s: %v", f.ID, err)
		}
		if !exists {
			t.Errorf("field %s (%s) returned by owner filter but has no override row for %s", f.ID, f.SemanticID, modelID)
		}
	}

	// Non-vacuous: the filtered set must be strictly smaller than the
	// project's unfiltered field count.
	_, totalAll, err := store.List(ctx, domain.WithProjectID(projectID), domain.WithLimit(1))
	if err != nil {
		t.Fatalf("list unfiltered: %v", err)
	}
	if totalAll != wantTotal {
		t.Fatalf("totalAll = %d, want %d", totalAll, wantTotal)
	}
	if totalFiltered >= totalAll {
		t.Fatalf("owner filter had no effect: totalFiltered=%d totalAll=%d", totalFiltered, totalAll)
	}

	// Empty owner = unfiltered: matches the project-wide total.
	_, totalEmpty, err := store.List(ctx, domain.WithProjectID(projectID), domain.WithLimit(1), domain.WithFilter("owner_id", ""))
	if err != nil {
		t.Fatalf("list empty owner: %v", err)
	}
	if totalEmpty != totalAll {
		t.Fatalf("totalEmpty = %d, want %d (unfiltered)", totalEmpty, totalAll)
	}
}
