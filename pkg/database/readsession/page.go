package readsession

import (
	"context"
	"fmt"
)

// Page is a bounded window over a deterministic order in the same database snapshot.
// It cannot change a reader’s project or state.
type Page struct {
	Limit  int32
	Offset int64
}

// Validate rejects unbounded, invalid or oversized batches.
func (p Page) Validate() error {
	if p.Limit < 1 || p.Limit > 1024 || p.Offset < 0 {
		return fmt.Errorf("content page requires limit 1..1024 and nonnegative offset")
	}
	return nil
}

// Check rejects nil, zero, closed or canceled sessions before assembly starts.
func (s *Session) Check(ctx context.Context) error {
	if s == nil || s.lifetime == nil || s.lifetime.closed || !s.scope.Valid() {
		return fmt.Errorf("project assembly requires an open session")
	}
	return ctx.Err()
}
