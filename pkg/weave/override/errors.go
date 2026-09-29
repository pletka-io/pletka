package override

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/auth"
)

// ErrCodeNotInRelease is returned when the data exists but not in the release
// that was asked for.
//
// Justification, which .claude/rules/api-patterns.md requires for a new code:
// none of the canonical codes means "the data exists, just not in what you
// asked for". not_found is wrong — the entity exists and a reader with draft
// access can see it. forbidden is wrong — nothing is being withheld. This is
// the second addition after vocabulary_service_unavailable, so api-patterns.md
// should settle whether the set is closed or extensible.
const ErrCodeNotInRelease = "not_in_release"

// NotInReleaseError carries what was asked for and whether a draft exists to
// redirect to.
type NotInReleaseError struct {
	Version        string
	What           string
	DraftAvailable bool
}

func (e *NotInReleaseError) Error() string {
	return fmt.Sprintf("release %s does not carry %s", e.Version, e.What)
}

// NotInRelease builds the error. draftAvailable is resolved from the auth
// snapshot at the call site and never inferred by the client: the rendering
// differs by who is asking, and a reader who cannot see a draft must not be
// told to go look at one.
func NotInRelease(scope auth.ReadScope, what string, draftAvailable bool) *NotInReleaseError {
	return &NotInReleaseError{Version: scope.Version(), What: what, DraftAvailable: draftAvailable}
}
