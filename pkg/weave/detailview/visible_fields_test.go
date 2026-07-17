package detailview

import (
	"testing"

	pkgdomain "github.com/pletka-io/pletka/pkg/domain"
)

// TestFilterVisibleFieldsDropsHiddenFields is the regression test for
// (hiding a field doesn't hide it): is_hidden is persisted
// and reaches ResolvedField, but nothing in the read-only detail/browse
// view ever consumed it. buildModel/buildCollection must filter hidden
// fields out before assembling the read-only contract, and before any
// derived computation (field counts, shared path prefix) so those stay
// consistent with what's actually rendered.
//
// The override EDITOR (pkg/weave/project/override_read.go) intentionally
// keeps showing hidden fields — this filter must not be applied there.
func TestFilterVisibleFieldsDropsHiddenFields(t *testing.T) {
	fields := []pkgdomain.ResolvedField{
		{ID: "f1", SystemName: "visible_one", IsHidden: false},
		{ID: "f2", SystemName: "hidden_one", IsHidden: true},
		{ID: "f3", SystemName: "visible_two", IsHidden: false},
	}

	got := filterVisibleFields(fields)

	if len(got) != 2 {
		t.Fatalf("len(filterVisibleFields(fields)) = %d, want 2 (derived count must exclude hidden field)", len(got))
	}
	for _, f := range got {
		if f.IsHidden {
			t.Fatalf("filterVisibleFields returned a hidden field: %+v", f)
		}
		if f.ID == "f2" {
			t.Fatalf("filterVisibleFields returned the hidden field f2")
		}
	}
	if got[0].ID != "f1" || got[1].ID != "f3" {
		t.Fatalf("filterVisibleFields did not preserve order of visible fields: got %+v", got)
	}
}

// TestFilterVisibleFieldsAllHidden covers the edge case where every field
// in a bucket is hidden — the filtered result (and thus the derived field
// count) must be zero, not the original slice.
func TestFilterVisibleFieldsAllHidden(t *testing.T) {
	fields := []pkgdomain.ResolvedField{
		{ID: "f1", IsHidden: true},
		{ID: "f2", IsHidden: true},
	}

	got := filterVisibleFields(fields)

	if len(got) != 0 {
		t.Fatalf("len(filterVisibleFields(fields)) = %d, want 0", len(got))
	}
}

// TestFilterVisibleFieldsNoneHidden ensures the common case (nothing
// hidden) is a no-op that preserves every field.
func TestFilterVisibleFieldsNoneHidden(t *testing.T) {
	fields := []pkgdomain.ResolvedField{
		{ID: "f1", IsHidden: false},
		{ID: "f2", IsHidden: false},
	}

	got := filterVisibleFields(fields)

	if len(got) != 2 {
		t.Fatalf("len(filterVisibleFields(fields)) = %d, want 2", len(got))
	}
}
