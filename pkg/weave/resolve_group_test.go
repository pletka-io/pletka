package weave

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func rf(id, coll string, collOrder, pos int) domain.ResolvedField {
	return domain.ResolvedField{ID: id, CategoryID: "CAT", PartOfCollectionID: coll, CollectionOrder: collOrder, Position: pos}
}

// TestGroupByCategoryCollectionOrderIsDeterministic: a collection's position
// is the smallest collection_order among its fields (not whichever field the
// map handed over first), ties break on field position, then id, and the
// direct bucket sits at its own position between collections.
func TestGroupByCategoryCollectionOrderIsDeterministic(t *testing.T) {
	cats := map[string]*categoryInfo{"CAT": {ID: "CAT", Name: domain.Translations{"en": "Cat"}, Position: 1}}
	fields := []domain.ResolvedField{
		rf("f1", "C2", 9, 30), // C2 has a stray high order on one field...
		rf("f2", "C2", 2, 31), // ...but its true order is 2
		rf("f3", "", 3, 40),   // direct bucket, order 3
		rf("f4", "C1", 1, 10), // C1 first
		rf("f5", "C3", 3, 50), // ties with the direct bucket on order; later position → after it
	}
	for i := 0; i < 20; i++ { // map iteration order varies; the result must not
		groups := groupByCategory(fields, cats, nil)
		if len(groups) != 1 {
			t.Fatalf("want 1 category, got %d", len(groups))
		}
		ids := make([]string, 0, len(groups[0].Collections))
		for _, g := range groups[0].Collections {
			ids = append(ids, g.ID)
		}
		want := []string{"C1", "C2", "__direct__", "C3"}
		if len(ids) != len(want) {
			t.Fatalf("run %d: got %v, want %v", i, ids, want)
		}
		for j := range want {
			if ids[j] != want[j] {
				t.Fatalf("run %d: got %v, want %v", i, ids, want)
			}
		}
	}
}
