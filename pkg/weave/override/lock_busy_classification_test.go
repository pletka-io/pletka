package override

import (
	"errors"
	"fmt"
	"testing"
)

// TestErrLockBusyClassificationSurvivesWrapAndJoin guards the
// errors.Is(err, ErrLockBusy) classification every caller (today, just
// pkg/weave/project's saveOverrides) relies on to turn lock contention into
// a 409 instead of a 500. It needs no database and no ten-second wait: it
// is a pure assertion over the exact error shapes WithAdvisoryLock (see
// store_postgres.go) actually returns. If someone changes a %w to %v, or
// drops the errors.Join, this test goes red without ever touching
// Postgres — Task 3 fix round 1, finding 7.
func TestErrLockBusyClassificationSurvivesWrapAndJoin(t *testing.T) {
	pgErr := errors.New("pg: lock_timeout")
	teardownErr := errors.New("reset lock_timeout: connection already closed")

	// The exact shape WithAdvisoryLock returns when the lock could not be
	// acquired in time:
	//   fmt.Errorf("take advisory lock %q: %w", key, errors.Join(ErrLockBusy, lockErr))
	wrappedAndJoined := fmt.Errorf("take advisory lock %q: %w", "model:X", errors.Join(ErrLockBusy, pgErr))

	tests := []struct {
		name   string
		err    error
		wantIs bool
	}{
		{"bare sentinel", ErrLockBusy, true},
		{"wrapped and joined with the pg error, as the store actually returns it", wrappedAndJoined, true},
		{"that form with a teardown error joined on top as well", errors.Join(wrappedAndJoined, teardownErr), true},
		{"unrelated error must not match", errors.New("completely unrelated failure"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := errors.Is(tc.err, ErrLockBusy); got != tc.wantIs {
				t.Fatalf("errors.Is(err, ErrLockBusy) = %v, want %v (err: %v)", got, tc.wantIs, tc.err)
			}
		})
	}
}
