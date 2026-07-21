//go:build integration

package model

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestListModelsStatusFilter verifies that domain.WithFilter("status", ...)
// is applied in SQL (filter-before-paging) rather than in memory: List
// returns only matching rows, and the returned count reflects the filtered
// set, not the unfiltered total.
//
// Seeds three throwaway models (2 draft, 1 published) under a dedicated
// project id so the assertion is non-vacuous regardless of what real data
// happens to be in the dev DB (GLB is all-draft there, which would make
// this vacuous; the AFS project has mixed draft/published data today but
// is real project data this test shouldn't depend on or mutate).
func TestListModelsStatusFilter(t *testing.T) {
	pool := usageTestPool(t)
	ctx := context.Background()

	const projID = "TSTSF"
	ids := []string{"TSTSFM.1", "TSTSFM.2", "TSTSFM.3"}
	statuses := []string{"draft", "draft", "published"}
	names := []string{"tstsf_model_1", "tstsf_model_2", "tstsf_model_3"}

	for i, id := range ids {
		_, err := pool.Exec(ctx, `INSERT INTO weave_models (id, project_id, status, system_name)
			VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`, id, projID, statuses[i], names[i])
		if err != nil {
			t.Fatalf("seed model %s: %v", id, err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models WHERE project_id = $1`, projID)
	})

	store := NewPostgresStore(pool)

	all, totalAll, err := store.List(ctx, domain.WithProjectID(projID), domain.WithLimit(1000))
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if totalAll != 3 || len(all) != 3 {
		t.Fatalf("unfiltered seed check: totalAll=%d len(all)=%d, want 3/3", totalAll, len(all))
	}

	drafts, totalDrafts, err := store.List(ctx, domain.WithProjectID(projID), domain.WithLimit(1000), domain.WithFilter("status", "draft"))
	if err != nil {
		t.Fatalf("list drafts: %v", err)
	}
	if totalDrafts != 2 {
		t.Fatalf("totalDrafts = %d, want 2", totalDrafts)
	}
	if len(drafts) != 2 {
		t.Fatalf("len(drafts) = %d, want 2", len(drafts))
	}
	for _, m := range drafts {
		if string(m.Status) != "draft" {
			t.Fatalf("non-draft row %s in filtered result (status=%s)", m.SemanticID, m.Status)
		}
	}

	published, totalPublished, err := store.List(ctx, domain.WithProjectID(projID), domain.WithLimit(1000), domain.WithFilter("status", "published"))
	if err != nil {
		t.Fatalf("list published: %v", err)
	}
	if totalPublished != 1 || len(published) != 1 {
		t.Fatalf("totalPublished=%d len(published)=%d, want 1/1", totalPublished, len(published))
	}
	if string(published[0].Status) != "published" {
		t.Fatalf("non-published row %s in filtered result (status=%s)", published[0].SemanticID, published[0].Status)
	}

	if totalDrafts > totalAll {
		t.Fatalf("count mismatch: drafts=%d all=%d", totalDrafts, totalAll)
	}
	if len(all) == len(drafts) && totalAll != totalDrafts {
		t.Fatal("filter had no effect but counts differ")
	}
}
