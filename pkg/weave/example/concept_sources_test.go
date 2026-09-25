package example

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestConceptSources_AlwaysListOnly guards #3599: a field bound to a control
// list autocompletes against only that list's own entries — open or sealed.
// The value picker never searches the source vocabulary or another list.
func TestConceptSources_AlwaysListOnly(t *testing.T) {
	for _, closed := range []bool{false, true} {
		refs := []domain.EntityRef{{ID: "CL1", URL: "/projects/LA/concept-lists/CL1", IsClosed: closed}}
		got := conceptSources(refs)
		if len(got) != 1 {
			t.Fatalf("closed=%v: expected 1 source, got %d", closed, len(got))
		}
		if want := "/api/v2/concept-lists/CL1/entries/search"; got[0].SearchURL != want {
			t.Fatalf("closed=%v: SearchURL = %q, want %q", closed, got[0].SearchURL, want)
		}
	}
}
