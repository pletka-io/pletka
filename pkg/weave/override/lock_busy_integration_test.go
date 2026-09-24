//go:build integration

package override

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestWithEntityLockReturnsRealErrLockBusy is lock_busy_classification_test.go's
// integration counterpart (Task 3 fix round 2, finding 3): that test proves
// errors.Is survives the exact shape WithAdvisoryLock returns, but it builds
// that shape itself, so it would stay green even if store_postgres.go
// stopped returning it (a %w -> %v regression, say). This test asserts
// errors.Is(err, ErrLockBusy) on the error WithEntityLock actually returns,
// from a real lock-timeout wait against Postgres.
//
// lockTimeout is shortened for the duration of this test (restored via
// t.Cleanup) so the wait stays well under a second instead of the
// production 10s — see the var's doc comment in store_postgres.go.
func TestWithEntityLockReturnsRealErrLockBusy(t *testing.T) {
	pool := testdb.Pool(t)
	store := NewPostgresStore(pool)
	svc := NewService(store, nil, nil)
	ctx := context.Background()

	orig := lockTimeout
	lockTimeout = 150 * time.Millisecond
	t.Cleanup(func() { lockTimeout = orig })

	holderErr := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		holderErr <- svc.WithEntityLock(ctx, "TSTLOCKBUSY", "model", "TSTLOCKBUSY.1", func(ctx context.Context) error {
			close(started)
			// Held well past the shortened lockTimeout, so the waiter
			// below genuinely times out instead of merely queueing.
			time.Sleep(500 * time.Millisecond)
			return nil
		})
	}()

	// As in fingerprint_integration_test.go's TestWithEntityLockSerialises:
	// take whichever of "holder started" or "holder failed" arrives first,
	// so a holder that errors before ever taking the lock doesn't hang
	// this test waiting on started.
	select {
	case <-started:
	case hErr := <-holderErr:
		t.Fatalf("holder failed before it took the lock: %v", hErr)
	}

	err := svc.WithEntityLock(ctx, "TSTLOCKBUSY", "model", "TSTLOCKBUSY.1", func(ctx context.Context) error {
		t.Fatal("waiter's callback ran — the lock should still have been held")
		return nil
	})
	if !errors.Is(err, ErrLockBusy) {
		t.Fatalf("errors.Is(err, ErrLockBusy) = false, want true; real WithEntityLock error = %v", err)
	}

	if hErr := <-holderErr; hErr != nil {
		t.Fatalf("holder WithEntityLock: %v", hErr)
	}
}
