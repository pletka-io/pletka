//go:build integration

package override_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/override"
)

func TestCollectionPlacements_CRUD(t *testing.T) {
	pool := testdb.Pool(t)
	store := override.NewPostgresStore(pool)
	ctx := context.Background()

	const modelID = "TSTPLCM.1"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_collection_placements WHERE model_id = $1`, modelID)
	})

	max := 3
	p := domain.CollectionPlacement{
		ProjectID:    "TSTPLC",
		ModelID:      modelID,
		CategoryID:   "TSTPLC.CAT.1",
		CollectionID: "TSTPLCC.1",
		IsRequired:   true,
		MinOccurs:    1,
		MaxOccurs:    &max,
	}

	if err := store.UpsertPlacement(ctx, &p); err != nil {
		t.Fatalf("upsert new: %v", err)
	}
	if p.ID == 0 {
		t.Fatalf("upsert new: expected generated id, got 0")
	}

	list, err := listLive(ctx, t, store, modelID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list: want 1 placement, got %d", len(list))
	}
	if diff := cmp.Diff(p, list[0], cmpopts.IgnoreFields(domain.CollectionPlacement{}, "ID")); diff != "" {
		t.Errorf("placement mismatch (-want +got):\n%s", diff)
	}

	// Upsert existing updates constraints, keeps the same row.
	p.IsRequired = false
	p.MinOccurs = 0
	p.MaxOccurs = nil
	p.IsHidden = true
	if err := store.UpsertPlacement(ctx, &p); err != nil {
		t.Fatalf("upsert existing: %v", err)
	}
	list, err = listLive(ctx, t, store, modelID)
	if err != nil {
		t.Fatalf("list after update: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list after update: want 1 placement, got %d", len(list))
	}
	if list[0].ID != p.ID {
		t.Errorf("update minted a new row: id %d -> %d", p.ID, list[0].ID)
	}
	if !list[0].IsHidden || list[0].IsRequired || list[0].MaxOccurs != nil {
		t.Errorf("update not applied: %+v", list[0])
	}

	if err := store.DeletePlacement(ctx, modelID, p.CategoryID, p.CollectionID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, err = listLive(ctx, t, store, modelID)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("list after delete: want 0, got %d", len(list))
	}
}

func TestCollectionPlacements_ListUnknownModelEmpty(t *testing.T) {
	pool := testdb.Pool(t)
	store := override.NewPostgresStore(pool)

	list, err := listLive(context.Background(), t, store, "TSTPLCM.NOPE")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("want empty list, got %d", len(list))
	}
}

// listLive reads placements through the Draft reader, the only way a Store
// now exposes them.
func listLive(ctx context.Context, t *testing.T, store override.Store, modelID string) ([]domain.CollectionPlacement, error) {
	t.Helper()
	r, err := store.At(auth.Draft())
	if err != nil {
		t.Fatalf("At(Draft): %v", err)
	}
	return r.ListPlacements(ctx, "", modelID)
}
