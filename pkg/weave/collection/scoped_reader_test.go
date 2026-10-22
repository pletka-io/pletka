package collection

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
)

// TestAtRejectsTheZeroScope pins Review Focus 5. The zero ReadScope means a
// caller never named draft or a release; defaulting to draft here is how a
// release view silently serves working state.
func TestAtRejectsTheZeroScope(t *testing.T) {
	s := &postgresStore{}

	_, err := s.At(auth.ReadScope{})
	if err == nil {
		t.Fatal("At(zero scope) returned no error; the zero value must fail closed")
	}
	if !strings.Contains(err.Error(), "collection") {
		t.Fatalf("error %q does not name the slice; a bare message is undiagnosable in a stack", err)
	}
}

// TestAtBindsTheScope proves At binds the scope rather than storing the raw
// value.
func TestAtBindsTheScope(t *testing.T) {
	s := &postgresStore{}

	dr, err := s.At(auth.Draft())
	if err != nil {
		t.Fatalf("At(draft): %v", err)
	}
	if dr.(reader).isRelease() {
		t.Fatal("draft scope produced a release reader")
	}

	rr, err := s.At(auth.Release("1.2.0"))
	if err != nil {
		t.Fatalf("At(release): %v", err)
	}
	if got := rr.(reader).version; got != "1.2.0" {
		t.Fatalf("release reader version = %q, want 1.2.0", got)
	}
}
