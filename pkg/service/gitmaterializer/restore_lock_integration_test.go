//go:build integration

package gitmaterializer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/database/advisorylock"
	"github.com/pletka-io/pletka/pkg/weave/override"
)

// minimalRestorePlan returns the smallest RestorePlan HydrateProjectOverrides
// and HydrateProjectProvenance accept: a non-nil Snapshot with every entity
// slice empty. Both functions operate by project_id WHERE clause and hydrate
// nothing from an empty snapshot, so no weave_projects row (or any other
// fixture) needs to exist for projectID — these tests are only exercising
// the project lock, not real hydration.
func minimalRestorePlan(projectID string) *RestorePlan {
	return &RestorePlan{ProjectID: projectID, Snapshot: &ProjectSnapshot{}}
}

// TestSeamNoSaveCanCommitBetweenOverridesAndProvenance is the seam test
// fix round 1 requires (item 1): a save must NOT be able to commit between
// the overrides phase and the provenance phase.
//
// It drives LockProjectForRestore, HydrateProjectOverrides, and
// HydrateProjectProvenance directly, in the exact sequence
// HydrateProjectOverridesAndProvenance uses internally, so it can assert
// — deterministically, not by racing wall-clock timing against a
// near-instant empty-snapshot restore — that a save queued before the
// restore starts is still blocked immediately AFTER the overrides phase
// commits and immediately after the provenance phase commits, and only
// completes once the OUTER lock is released.
//
// This is the property an earlier version of the fix broke: it took a
// pg_advisory_xact_lock per hydration transaction, which released at that
// transaction's own commit — exactly the "overrides phase committed"
// instant this test checks — and Postgres woke the queued save right
// there.
func TestSeamNoSaveCanCommitBetweenOverridesAndProvenance(t *testing.T) {
	pool := hydrateTestPool(t)
	m := NewMaterializer(pool, t.TempDir(), nil)
	overrideSvc := override.NewService(override.NewPostgresStore(pool), nil, nil)
	ctx := context.Background()
	const projectID = "TSTSEAM"

	// The restore takes its lock FIRST, so the save started right after is
	// guaranteed to have something to queue behind — nothing else
	// contends for a fresh project key otherwise, so starting the save
	// first (with only a head-start sleep before the restore) would let
	// it race the restore for an uncontended lock instead of reliably
	// queuing behind it.
	release, err := m.LockProjectForRestore(ctx, projectID)
	if err != nil {
		t.Fatalf("LockProjectForRestore: %v", err)
	}
	// Guards every early return below (including a t.Fatal partway through
	// the sabotage/seam checks): release always targets whichever closure
	// `release` currently holds, since a Go closure captures the variable,
	// not its value at defer time — without this, a Fatal between an
	// acquire and its matching release would leave the pinned connection
	// checked out forever, and the pool's t.Cleanup(pool.Close) would hang
	// waiting for it.
	defer func() { _ = release() }()

	saveStarted := make(chan struct{})
	saveDone := make(chan error, 1)
	go func() {
		saveDone <- overrideSvc.WithEntityLock(ctx, projectID, "model", "TSTSEAM.1", func(ctx context.Context) error {
			close(saveStarted)
			return nil
		})
	}()

	// Give the save time to actually issue its (now-blocked) lock request.
	time.Sleep(100 * time.Millisecond)
	select {
	case <-saveStarted:
		t.Fatal("save's callback ran before the overrides phase even started — the restore lock did not block it at all")
	default:
	}

	if err := m.HydrateProjectOverrides(ctx, minimalRestorePlan(projectID)); err != nil {
		t.Fatalf("HydrateProjectOverrides: %v", err)
	}
	// THE SEAM: HydrateProjectOverrides has committed its own transaction.
	// The queued save must still be blocked here — only the OUTER lock
	// (still held via `release`, not yet called) may stop it, since
	// HydrateProjectOverrides takes none of its own.
	select {
	case <-saveStarted:
		t.Fatal("save's callback ran between the overrides and provenance phases — the seam is unprotected")
	default:
	}

	if err := m.HydrateProjectProvenance(ctx, minimalRestorePlan(projectID)); err != nil {
		t.Fatalf("HydrateProjectProvenance: %v", err)
	}
	select {
	case <-saveStarted:
		t.Fatal("save's callback ran while the outer restore lock was still held")
	default:
	}

	if err := release(); err != nil {
		t.Fatalf("release: %v", err)
	}

	select {
	case err := <-saveDone:
		if err != nil {
			t.Fatalf("save: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("save never completed after the restore released its lock")
	}
	select {
	case <-saveStarted:
	default:
		t.Fatal("save's callback never ran even after the restore released its lock")
	}
}

// TestHydrateProjectOverridesAndProvenanceBlocksOnInFlightSaveThenCompletes
// exercises the real, public wrapped entry point end to end: it blocks
// while a save holds the project lock, and — the point "also worth
// closing" in the fix-round review — the save itself SUCCEEDS once the
// restore releases the lock, not merely "is refused while the restore
// holds it."
func TestHydrateProjectOverridesAndProvenanceBlocksOnInFlightSaveThenCompletes(t *testing.T) {
	pool := hydrateTestPool(t)
	m := NewMaterializer(pool, t.TempDir(), nil)
	overrideSvc := override.NewService(override.NewPostgresStore(pool), nil, nil)
	ctx := context.Background()
	const projectID = "TSTRESTOREWRAP"

	started := make(chan struct{})
	saveErr := make(chan error, 1)
	go func() {
		saveErr <- overrideSvc.WithEntityLock(ctx, projectID, "model", "TSTRESTOREWRAP.1", func(ctx context.Context) error {
			close(started)
			time.Sleep(300 * time.Millisecond)
			return nil
		})
	}()

	select {
	case <-started:
	case err := <-saveErr:
		t.Fatalf("save failed before it took its lock: %v", err)
	}

	waitStart := time.Now()
	if err := m.HydrateProjectOverridesAndProvenance(ctx, minimalRestorePlan(projectID)); err != nil {
		t.Fatalf("HydrateProjectOverridesAndProvenance: %v", err)
	}
	waited := time.Since(waitStart)

	if err := <-saveErr; err != nil {
		t.Fatalf("save: %v", err)
	}
	if waited < 250*time.Millisecond {
		t.Fatalf("HydrateProjectOverridesAndProvenance returned after only %s — expected it to block on the in-flight save's project lock (held ~300ms)", waited)
	}
}

// TestRestoreProjectLockTimesOutWhenSaveHeldTooLong proves scope item 4 of
// the original brief (restore's own bounded wait): LockProjectForRestore
// does not wait forever behind a save, and the resulting error names the
// project, reports a save in flight, and says the project may be partially
// restored. restoreProjectLockTimeout is shortened for the duration of
// this test the way override.lockTimeout is shortened in
// lock_busy_integration_test.go.
func TestRestoreProjectLockTimesOutWhenSaveHeldTooLong(t *testing.T) {
	pool := hydrateTestPool(t)
	m := NewMaterializer(pool, t.TempDir(), nil)
	overrideSvc := override.NewService(override.NewPostgresStore(pool), nil, nil)
	ctx := context.Background()

	orig := restoreProjectLockTimeout
	restoreProjectLockTimeout = 150 * time.Millisecond
	t.Cleanup(func() { restoreProjectLockTimeout = orig })

	const projectID = "TSTRESTORELOCKBUSY"

	started := make(chan struct{})
	saveErr := make(chan error, 1)
	go func() {
		saveErr <- overrideSvc.WithEntityLock(ctx, projectID, "model", "TSTRESTORELOCKBUSY.1", func(ctx context.Context) error {
			close(started)
			// Held well past the shortened restoreProjectLockTimeout, so
			// the restore below genuinely times out instead of merely
			// queueing and then succeeding.
			time.Sleep(500 * time.Millisecond)
			return nil
		})
	}()

	select {
	case <-started:
	case err := <-saveErr:
		t.Fatalf("save failed before it took its lock: %v", err)
	}

	release, err := m.LockProjectForRestore(ctx, projectID)
	if err == nil {
		_ = release()
		t.Fatal("expected LockProjectForRestore to fail while the save held the project lock")
	}
	if !errors.Is(err, advisorylock.ErrLockBusy) {
		t.Fatalf("errors.Is(err, advisorylock.ErrLockBusy) = false, want true; err = %v", err)
	}
	if !strings.Contains(err.Error(), projectID) {
		t.Fatalf("error %q does not name the project %q", err.Error(), projectID)
	}
	if !strings.Contains(err.Error(), "partially restored") {
		t.Fatalf("error %q does not tell the operator the project may be partially restored", err.Error())
	}

	if err := <-saveErr; err != nil {
		t.Fatalf("save: %v", err)
	}
}
