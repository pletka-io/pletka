package vocabulary

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
)

// Store must expose At, and reads must live on the Reader it returns. These
// are compile-time assertions: they cost nothing at run time and fail the
// build if the shape drifts, which is the only moment the check matters.
//
// Asserted via an anonymous interface rather than by calling anything — a
// method value taken from a nil interface panics, so a "does it have At" test
// written that way tests the nil check and not the shape.
var (
	_ interface {
		At(auth.ReadScope) (Reader, error)
	} = (Store)(nil)

	// A Reader must NOT carry writes. If a mutation is ever added to Reader,
	// this assertion still compiles -- the ratchet is what catches that -- so
	// the comment carries the intent the type cannot.
	_ Reader = (Reader)(nil)
)

// TestStoreExposesTheVersionedSliceShape exists so the assertions above are
// attached to a named, runnable test: a build failure then names this file and
// this slice rather than surfacing as an unexplained compile error.
func TestStoreExposesTheVersionedSliceShape(t *testing.T) {
	t.Log("Store.At and Reader are asserted at compile time; reaching this line means the shape holds")
}
