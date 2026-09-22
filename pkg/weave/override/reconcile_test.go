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
