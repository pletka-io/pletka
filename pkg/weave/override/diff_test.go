package override

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestComputeDiffKeptRowIsChanged covers the ordinary update path: an
// existing row's id is present in desired with different content.
func TestComputeDiffKeptRowIsChanged(t *testing.T) {
	existing := []domain.FieldOverride{ov(10, "F1", "C", "", 1)}
	desired := []domain.FieldOverride{ov(10, "F1", "C", "", 2)} // position changed
	diff := ComputeDiff(existing, desired)
	if len(diff.Changed) != 1 || len(diff.Added) != 0 || len(diff.Removed) != 0 {
		t.Fatalf("diff = %+v", diff)
	}
	if diff.Changed[0].Before.ID != 10 || diff.Changed[0].After.Position != 2 {
		t.Fatalf("changed pair = %+v", diff.Changed[0])
	}
}

// TestComputeDiffNewRowIsAdded covers a desired row with ID==0 — always
// Added, regardless of whether existing is empty or not.
func TestComputeDiffNewRowIsAdded(t *testing.T) {
	existing := []domain.FieldOverride{}
	desired := []domain.FieldOverride{ov(0, "F1", "C", "", 1)}
	diff := ComputeDiff(existing, desired)
	if len(diff.Added) != 1 || len(diff.Removed) != 0 || len(diff.Changed) != 0 {
		t.Fatalf("diff = %+v", diff)
	}
	if diff.Added[0].FieldID != "F1" {
		t.Fatalf("added = %+v", diff.Added[0])
	}
}

// TestComputeDiffDroppedRowIsRemoved covers an existing row whose id is
// absent from desired entirely.
func TestComputeDiffDroppedRowIsRemoved(t *testing.T) {
	existing := []domain.FieldOverride{ov(10, "F1", "C", "", 1)}
	desired := []domain.FieldOverride{}
	diff := ComputeDiff(existing, desired)
	if len(diff.Removed) != 1 || len(diff.Added) != 0 || len(diff.Changed) != 0 {
		t.Fatalf("diff = %+v", diff)
	}
	if diff.Removed[0].ID != 10 {
		t.Fatalf("removed = %+v", diff.Removed[0])
	}
}

// TestComputeDiffIDNamingOtherFieldIsRemovedPlusAdded pins the fix for
// finding 1 (fix round 1): a client that kept an override_id but swapped
// the field in that slot. matchOverrides refuses to let an id claim a row
// of a different field (see TestMatchOverridesKnownIDWithOtherFieldIsNotClaimed
// in reconcile_test.go), so ReplaceForEntity deletes the old row and inserts
// a new one. Before this fix, ComputeDiff paired the two by id alone and
// reported a single Update naming the deleted row — a change log entry for
// a row that no longer exists, with neither real operation recorded.
func TestComputeDiffIDNamingOtherFieldIsRemovedPlusAdded(t *testing.T) {
	existing := []domain.FieldOverride{ov(10, "F1", "C", "", 1)}
	desired := []domain.FieldOverride{ov(10, "F2", "C", "", 1)}
	diff := ComputeDiff(existing, desired)
	if len(diff.Changed) != 0 {
		t.Fatalf("expected no Changed pairing across fields, got %+v", diff.Changed)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].ID != 10 || diff.Removed[0].FieldID != "F1" {
		t.Fatalf("removed = %+v", diff.Removed)
	}
	if len(diff.Added) != 1 || diff.Added[0].FieldID != "F2" {
		t.Fatalf("added = %+v", diff.Added)
	}
}

// TestComputeDiffUnknownIDIsAdded covers a desired row carrying a non-zero
// id that names no row in existing at all (not merely a different field) —
// also Added, and the existing row it did not claim is Removed.
func TestComputeDiffUnknownIDIsAdded(t *testing.T) {
	existing := []domain.FieldOverride{ov(10, "F1", "C", "", 1)}
	desired := []domain.FieldOverride{ov(999, "F9", "C", "", 1)}
	diff := ComputeDiff(existing, desired)
	if len(diff.Added) != 1 || diff.Added[0].FieldID != "F9" {
		t.Fatalf("added = %+v", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].ID != 10 {
		t.Fatalf("removed = %+v", diff.Removed)
	}
}
