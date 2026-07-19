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
// HERM.9 is a known model in the dev DB (project HER) with a large,
// real field-placement set — chosen deliberately over a synthetic seed so
// the assertion exercises the actual weave_field_overrides EXISTS clause
// end to end, the same way TestService_DescribeTerm_Smoke exercises a real
// ontology term. The expected count is computed from the same table
// directly rather than hardcoded, so the test doesn't rot if placements
// change.
func TestListFieldsOwnerFilter(t *testing.T) {
	pool := batchUsageRefsTestPool(t)
	ctx := context.Background()

	const projectID = "HER"
	const ownerModelID = "HERM.9"

	var wantCount int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM weave_fields wf
		WHERE wf.project_id = $1
		  AND EXISTS (
		      SELECT 1 FROM weave_field_overrides fo
		      WHERE fo.field_id = wf.id
		        AND fo.entity_type IN ('model', 'collection')
		        AND fo.entity_id = $2
		  )
	`, projectID, ownerModelID).Scan(&wantCount); err != nil {
		t.Fatalf("compute expected owner-filtered count: %v", err)
	}
	if wantCount == 0 {
		t.Skipf("model %s in project %s has no field placements in this dev DB — test would be vacuous, skipping", ownerModelID, projectID)
	}

	store := NewPostgresStore(pool)

	filtered, totalFiltered, err := store.List(ctx,
		domain.WithProjectID(projectID),
		domain.WithLimit(1000),
		domain.WithFilter("owner_id", ownerModelID),
	)
	if err != nil {
		t.Fatalf("list owner-filtered: %v", err)
	}
	if totalFiltered != wantCount {
		t.Fatalf("totalFiltered = %d, want %d", totalFiltered, wantCount)
	}
	if int64(len(filtered)) != wantCount {
		t.Fatalf("len(filtered) = %d, want %d", len(filtered), wantCount)
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
		`, f.ID, ownerModelID).Scan(&exists); err != nil {
			t.Fatalf("verify placement for field %s: %v", f.ID, err)
		}
		if !exists {
			t.Errorf("field %s (%s) returned by owner filter but has no override row for %s", f.ID, f.SemanticID, ownerModelID)
		}
	}

	// Non-vacuous: the filtered set must be strictly smaller than the
	// project's unfiltered field count.
	_, totalAll, err := store.List(ctx, domain.WithProjectID(projectID), domain.WithLimit(1))
	if err != nil {
		t.Fatalf("list unfiltered: %v", err)
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
