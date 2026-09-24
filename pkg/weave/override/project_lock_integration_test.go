//go:build integration

package override

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestDifferentEntitySavesInOneProjectRunConcurrently proves the design's
// central property: WithEntityLock takes the project lock SHARED, so two
// saves of DIFFERENT entities in the SAME project still run concurrently —
// only two saves of the SAME entity queue. If the project lock were ever
// taken EXCLUSIVE by a save (instead of SHARED), this test would fail: the
// second save would queue behind the first and the wait would exceed the
// holder's own sleep.
func TestDifferentEntitySavesInOneProjectRunConcurrently(t *testing.T) {
	pool := testdb.Pool(t)
	store := NewPostgresStore(pool)
	svc := NewService(store, nil, nil)
	ctx := context.Background()

	const projectID = "TSTPLOCKCONC"

	firstStarted := make(chan struct{})
	firstErr := make(chan error, 1)
	go func() {
		firstErr <- svc.WithEntityLock(ctx, projectID, "model", "TSTPLOCKCONC.1", func(ctx context.Context) error {
			close(firstStarted)
			time.Sleep(300 * time.Millisecond)
			return nil
		})
	}()

	select {
	case <-firstStarted:
	case err := <-firstErr:
		t.Fatalf("first save failed before it took its lock: %v", err)
	}

	// The second save targets a DIFFERENT entity in the SAME project. It
	// must not wait for the first save's entity lock (a different key) —
	// the only thing they share is the project's SHARED lock, which both
	// can hold at once.
	waitStart := time.Now()
	secondErr := svc.WithEntityLock(ctx, projectID, "collection", "TSTPLOCKCONC.2", func(ctx context.Context) error {
		return nil
	})
	waited := time.Since(waitStart)

	if secondErr != nil {
		t.Fatalf("second save (different entity, same project): %v", secondErr)
	}
	if err := <-firstErr; err != nil {
		t.Fatalf("first save: %v", err)
	}
	if waited >= 250*time.Millisecond {
		t.Fatalf("second save waited %s — it queued behind the first save's entity lock instead of running concurrently (project lock is not SHARED)", waited)
	}
}

// TestSaveGetsErrLockBusyWhileProjectLockHeldExclusively simulates a git
// restore holding the project lock: it takes pg_advisory_xact_lock on
// ProjectLockKey(projectID) — the exact statement
// pkg/service/gitmaterializer's restore-side lock issues — from a held-open
// transaction on a separate connection, then proves a concurrent save times
// out with ErrLockBusy rather than corrupting anything (its callback never
// runs) once its own lockTimeout is exceeded.
//
// lockTimeout is shortened the way lock_busy_integration_test.go already
// does, so the wait stays well under a second instead of the production 10s.
func TestSaveGetsErrLockBusyWhileProjectLockHeldExclusively(t *testing.T) {
	pool := testdb.Pool(t)
	store := NewPostgresStore(pool)
	svc := NewService(store, nil, nil)
	ctx := context.Background()

	const projectID = "TSTPLOCKBUSY"

	orig := lockTimeout
	lockTimeout = 150 * time.Millisecond
	t.Cleanup(func() { lockTimeout = orig })

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin restore-simulating tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, ProjectLockKey(projectID)); err != nil {
		t.Fatalf("take exclusive project lock: %v", err)
	}

	// Held well past the shortened lockTimeout, so the save below genuinely
	// times out instead of merely queueing and then succeeding.
	holdDone := make(chan struct{})
	go func() {
		defer close(holdDone)
		time.Sleep(500 * time.Millisecond)
	}()

	saveErr := svc.WithEntityLock(ctx, projectID, "model", "TSTPLOCKBUSY.1", func(ctx context.Context) error {
		t.Fatal("save's callback ran — the project lock should still have been held exclusively")
		return nil
	})
	if !errors.Is(saveErr, ErrLockBusy) {
		t.Fatalf("errors.Is(err, ErrLockBusy) = false, want true; save error = %v", saveErr)
	}

	<-holdDone
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback restore-simulating tx: %v", err)
	}
}
