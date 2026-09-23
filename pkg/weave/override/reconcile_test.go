package override

import (
	"slices"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

//nolint:unparam // cat is part of the helper's general (id, field, cat, coll, pos) shape; every case here happens to pass "C".
func ov(id int64, field, cat, coll string, pos int) domain.FieldOverride {
	return domain.FieldOverride{ID: id, FieldID: field, CategoryID: cat, PartOfCollectionID: coll, Position: pos}
}

func TestMatchOverridesByID(t *testing.T) {
	existing := []domain.FieldOverride{ov(10, "F1", "C", "", 1), ov(11, "F2", "C", "", 2)}
	desired := []domain.FieldOverride{ov(11, "F2", "C", "", 1), ov(10, "F1", "C", "", 2)}
	p := matchOverrides(existing, desired)
	if !slices.Equal(p.update, []int64{11, 10}) || len(p.remove) != 0 {
		t.Fatalf("plan = %+v", p)
	}
}

func TestMatchOverridesByKeyWhenIDMissing(t *testing.T) {
	existing := []domain.FieldOverride{ov(10, "F1", "C", "COL", 1), ov(11, "F2", "C", "", 2)}
	desired := []domain.FieldOverride{ov(0, "F1", "C", "COL", 1), ov(0, "F3", "C", "", 2)}
	p := matchOverrides(existing, desired)
	if !slices.Equal(p.update, []int64{10, 0}) || !slices.Equal(p.remove, []int64{11}) {
		t.Fatalf("plan = %+v", p)
	}
}

func TestMatchOverridesSameFieldTwicePairsByPosition(t *testing.T) {
	existing := []domain.FieldOverride{ov(20, "F1", "C", "", 5), ov(21, "F1", "C", "", 2)}
	desired := []domain.FieldOverride{ov(0, "F1", "C", "", 9), ov(0, "F1", "C", "", 1)}
	p := matchOverrides(existing, desired)
	// existing by position: 21 (2), 20 (5); desired by position: idx1 (1), idx0 (9)
	if !slices.Equal(p.update, []int64{20, 21}) || len(p.remove) != 0 {
		t.Fatalf("plan = %+v", p)
	}
}

func TestMatchOverridesForeignOrUnknownIDInserts(t *testing.T) {
	existing := []domain.FieldOverride{ov(10, "F1", "C", "", 1)}
	desired := []domain.FieldOverride{ov(999, "F9", "C", "", 1)}
	p := matchOverrides(existing, desired)
	if !slices.Equal(p.update, []int64{0}) || !slices.Equal(p.remove, []int64{10}) {
		t.Fatalf("plan = %+v", p)
	}
}

func TestMatchOverridesDuplicateIDClaimsOnce(t *testing.T) {
	existing := []domain.FieldOverride{ov(10, "F1", "C", "", 1)}
	desired := []domain.FieldOverride{ov(10, "F1", "C", "", 1), ov(10, "F1", "C", "", 2)}
	p := matchOverrides(existing, desired)
	if !slices.Equal(p.update, []int64{10, 0}) || len(p.remove) != 0 {
		t.Fatalf("plan = %+v", p)
	}
}

func TestMatchOverridesKnownIDWithOtherFieldIsNotClaimed(t *testing.T) {
	existing := []domain.FieldOverride{ov(10, "F1", "C", "", 1)}
	desired := []domain.FieldOverride{ov(10, "F2", "C", "", 1)}
	p := matchOverrides(existing, desired)
	if !slices.Equal(p.update, []int64{0}) || !slices.Equal(p.remove, []int64{10}) {
		t.Fatalf("plan = %+v", p)
	}
}

func TestStampMatchedIDsFillsIDLessKeptRow(t *testing.T) {
	existing := []domain.FieldOverride{ov(10, "F1", "C", "COL", 1)}
	desired := []domain.FieldOverride{ov(0, "F1", "C", "COL", 1)}
	plan := matchOverrides(existing, desired)
	stampMatchedIDs(desired, plan)
	if desired[0].ID != 10 {
		t.Fatalf("desired[0].ID = %d, want 10", desired[0].ID)
	}
	if len(addedIndices(existing, desired)) != 0 {
		t.Fatalf("stamped row still classified as added: %+v", addedIndices(existing, desired))
	}
}

func TestStampMatchedIDsLeavesUnclaimedRowsAlone(t *testing.T) {
	existing := []domain.FieldOverride{ov(10, "F1", "C", "", 1)}
	desired := []domain.FieldOverride{ov(0, "F2", "C", "", 1), ov(999, "F9", "C", "", 2)}
	plan := matchOverrides(existing, desired)
	stampMatchedIDs(desired, plan)
	if desired[0].ID != 0 {
		t.Fatalf("genuinely new row got stamped: ID = %d", desired[0].ID)
	}
	if desired[1].ID != 999 {
		t.Fatalf("stale-id row was changed: ID = %d, want 999 unchanged", desired[1].ID)
	}
}

func TestAddedIndicesMatchesComputeDiffAdded(t *testing.T) {
	existing := []domain.FieldOverride{ov(10, "F1", "C", "COL", 1), ov(11, "F2", "C", "", 2)}
	// idx0: id-less resend of the kept F1 row (matched by key, not counted).
	// idx1: genuinely new row (ID==0, no key match).
	// idx2: stale/unknown id (not present in existing).
	desired := []domain.FieldOverride{
		ov(0, "F1", "C", "COL", 1),
		ov(0, "F3", "C", "", 3),
		ov(999, "F9", "C", "", 4),
	}
	plan := matchOverrides(existing, desired)
	stampMatchedIDs(desired, plan)

	diff := ComputeDiff(existing, desired)
	idx := addedIndices(existing, desired)
	if len(idx) != len(diff.Added) {
		t.Fatalf("addedIndices len = %d, ComputeDiff Added len = %d", len(idx), len(diff.Added))
	}
	if !slices.Equal(idx, []int{1, 2}) {
		t.Fatalf("addedIndices = %v, want [1 2]", idx)
	}
	for j, i := range idx {
		if diff.Added[j].FieldID != desired[i].FieldID {
			t.Fatalf("addedIndices[%d]=%d does not line up with diff.Added[%d] (field %s vs %s)",
				j, i, j, desired[i].FieldID, diff.Added[j].FieldID)
		}
	}
}
