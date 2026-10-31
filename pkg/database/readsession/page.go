package readsession

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Page is a bounded window over a deterministic order in the same database snapshot.
// It cannot change a reader’s project or state.
type Page struct {
	Limit  int32
	Offset int64
	// Selection nil means complete content; an explicit empty selection loads
	// interpretation metadata only. It cannot select a project or release.
	Selection projectsnapshot.Selection
}

// Validate rejects unbounded, invalid or oversized batches.
func (p Page) Validate() error {
	if p.Limit < 1 || p.Limit > 1024 || p.Offset < 0 {
		return fmt.Errorf("content page requires limit 1..1024 and nonnegative offset")
	}
	return p.Selection.Validate()
}

// Restricted distinguishes complete paging from an explicit entity closure.
func (p Page) Restricted() bool { return p.Selection != nil }

// IDs returns the selected identities of one structural kind.
func (p Page) IDs(kind string) []string { return p.Selection[kind] }

// Check rejects nil, zero, closed or canceled sessions before assembly starts.
func (s *Session) Check(ctx context.Context) error {
	if s == nil || s.lifetime == nil || s.lifetime.closed || !s.scope.Valid() {
		return fmt.Errorf("project assembly requires an open session")
	}
	return ctx.Err()
}
