package override

import (
	"errors"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
)

func TestNotInRelease(t *testing.T) {
	err := NotInRelease(auth.Release("0.1.0"), "field placements", true)

	var target *NotInReleaseError
	if !errors.As(error(err), &target) {
		t.Fatal("NotInRelease is not matchable with errors.As")
	}
	if target.Version != "0.1.0" || target.What != "field placements" || !target.DraftAvailable {
		t.Errorf("got %+v, want version 0.1.0 / field placements / draft available", target)
	}
	want := "release 0.1.0 does not carry field placements"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// DraftAvailable is resolved where the error is built, from the auth
// snapshot — never inferred by a client. Telling a reader to "view the draft"
// is useless to someone who cannot see one.
func TestNotInRelease_DraftAvailableIsCarried(t *testing.T) {
	if NotInRelease(auth.Release("0.1.0"), "x", false).DraftAvailable {
		t.Error("DraftAvailable must be false when the caller says so")
	}
}
