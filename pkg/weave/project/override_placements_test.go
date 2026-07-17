package project

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func intp(v int) *int { return &v }

func TestReconcilePlacements(t *testing.T) {
	max3 := 3

	t.Run("posted placement upserts with normalized category", func(t *testing.T) {
		cats := []overrideEditorCategory{{
			CategoryID: "", // uncategorized, denormalized form
			Items: []overrideEditorItem{{
				Widget: "collection-group",
				ID:     "LAC.1",
				Placement: &domain.CollectionPlacement{
					IsRequired: true, MinOccurs: 1, MaxOccurs: &max3,
				},
			}},
		}}
		ups, dels, err := reconcilePlacements(nil, cats, "LA", "LAM.1")
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if len(dels) != 0 {
			t.Fatalf("want 0 deletes, got %d", len(dels))
		}
		if len(ups) != 1 {
			t.Fatalf("want 1 upsert, got %d", len(ups))
		}
		u := ups[0]
		if u.ProjectID != "LA" || u.ModelID != "LAM.1" || u.CategoryID != "" ||
			u.CollectionID != "LAC.1" || !u.IsRequired || u.MinOccurs != 1 ||
			u.MaxOccurs == nil || *u.MaxOccurs != 3 {
			t.Errorf("upsert wrong: %+v", u)
		}
	})

	t.Run("existing placement for removed group is deleted", func(t *testing.T) {
		existing := []domain.CollectionPlacement{{
			ModelID: "LAM.1", CategoryID: "cat1", CollectionID: "LAC.9",
		}}
		ups, dels, err := reconcilePlacements(existing, nil, "LA", "LAM.1")
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if len(ups) != 0 || len(dels) != 1 {
			t.Fatalf("want 0 upserts 1 delete, got %d/%d", len(ups), len(dels))
		}
		if dels[0].CategoryID != "cat1" || dels[0].CollectionID != "LAC.9" {
			t.Errorf("delete key wrong: %+v", dels[0])
		}
	})

	t.Run("group still present without placement is untouched", func(t *testing.T) {
		existing := []domain.CollectionPlacement{{
			ModelID: "LAM.1", CategoryID: "cat1", CollectionID: "LAC.9", IsHidden: true,
		}}
		cats := []overrideEditorCategory{{
			CategoryID: "cat1",
			Items: []overrideEditorItem{{
				Widget: "collection-group", ID: "LAC.9",
				Placement: &domain.CollectionPlacement{IsHidden: true},
			}},
		}}
		ups, dels, err := reconcilePlacements(existing, cats, "LA", "LAM.1")
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if len(dels) != 0 {
			t.Fatalf("group present: want no deletes, got %d", len(dels))
		}
		if len(ups) != 1 || !ups[0].IsHidden {
			t.Fatalf("round-tripped placement should re-upsert: %+v", ups)
		}
	})

	t.Run("direct-fields bucket never creates placements", func(t *testing.T) {
		cats := []overrideEditorCategory{{
			CategoryID: "cat1",
			Items: []overrideEditorItem{{
				Widget:    "field-group",
				ID:        "__direct__",
				Placement: &domain.CollectionPlacement{IsRequired: true},
			}},
		}}
		ups, _, err := reconcilePlacements(nil, cats, "LA", "LAM.1")
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if len(ups) != 0 {
			t.Fatalf("direct bucket must not produce placements, got %d", len(ups))
		}
	})

	t.Run("max below min is rejected", func(t *testing.T) {
		cats := []overrideEditorCategory{{
			CategoryID: "cat1",
			Items: []overrideEditorItem{{
				Widget: "collection-group", ID: "LAC.1",
				Placement: &domain.CollectionPlacement{MinOccurs: 2, MaxOccurs: intp(1)},
			}},
		}}
		if _, _, err := reconcilePlacements(nil, cats, "LA", "LAM.1"); err == nil {
			t.Fatal("want validation error for max < min")
		}
	})

	t.Run("negative min is rejected", func(t *testing.T) {
		cats := []overrideEditorCategory{{
			CategoryID: "cat1",
			Items: []overrideEditorItem{{
				Widget: "collection-group", ID: "LAC.1",
				Placement: &domain.CollectionPlacement{MinOccurs: -1},
			}},
		}}
		if _, _, err := reconcilePlacements(nil, cats, "LA", "LAM.1"); err == nil {
			t.Fatal("want validation error for negative min")
		}
	})
}
