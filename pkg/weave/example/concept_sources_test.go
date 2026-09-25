package example

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestConceptSearchURL_OpenVsClosed guards #3599 B1: an open bound list offers
// its source vocabulary (source-entries), a sealed one only its own entries.
func TestConceptSearchURL_OpenVsClosed(t *testing.T) {
	openRef := domain.EntityRef{ID: "CL1", URL: "/projects/LA/concept-lists/CL1", IsClosed: false}
	if got, want := conceptSearchURL(openRef), "/api/v2/projects/LA/concept-lists/CL1/source-entries/search"; got != want {
		t.Fatalf("open list: got %q want %q", got, want)
	}

	closedRef := domain.EntityRef{ID: "CL1", URL: "/projects/LA/concept-lists/CL1", IsClosed: true}
	if got, want := conceptSearchURL(closedRef), "/api/v2/concept-lists/CL1/entries/search"; got != want {
		t.Fatalf("closed list: got %q want %q", got, want)
	}

	// No recoverable project → safe list-only fallback, never a broken URL.
	noURL := domain.EntityRef{ID: "CL9", IsClosed: false}
	if got, want := conceptSearchURL(noURL), "/api/v2/concept-lists/CL9/entries/search"; got != want {
		t.Fatalf("no-url fallback: got %q want %q", got, want)
	}
}
