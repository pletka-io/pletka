//go:build integration

package gitmaterializer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/advisorylock"
	"github.com/pletka-io/pletka/pkg/weave/override"
)

// waitForAdvisoryWaiter polls pg_locks until at least one advisory-lock
// request is queued (granted = false), or fails the test after a bounded
// wait. Used in place of a fixed sleep before an assertion that a queued
// request has NOT been granted: if the sleep turned out shorter than the
// time the other goroutine actually needed to issue its lock request, the
// request would simply not exist yet when the assertion runs, and every
// "still blocked" check after it would pass vacuously — a false pass,
// which is exactly the direction that matters for a test whose entire
// point is catching an unprotected seam. Polling for Postgres's own
// confirmation that a request is queued cannot false-pass that way: it
// only returns once a real waiter exists.
func waitForAdvisoryWaiter(t *testing.T, pool *pgxpool.Pool, lockKey string) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		// pg_locks is cluster-wide, and internal/testdb deliberately shares
		// one Postgres across parallel package binaries, several of which
		// queue advisory waiters of their own. Without the database and key
		// filters this returns another package's waiter — often before this
		// test's own goroutine has even been scheduled — and every later
		// "still blocked" assertion then passes vacuously, which is the
		// false pass this poll replaced a sleep to avoid.
		const q = `
			SELECT count(*) FROM pg_locks
			WHERE locktype = 'advisory'
			  AND NOT granted
			  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
			  AND ((classid::bigint << 32) | (objid::bigint & 4294967295)) = hashtext($1)::bigint`
		if err := pool.QueryRow(ctx, q, lockKey).Scan(&count); err != nil {
			t.Fatalf("poll pg_locks for a waiting advisory lock: %v", err)
		}
		if count > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for a queued advisory lock request to appear in pg_locks")
}

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
// hydrateProjectOverridesAndProvenanceLocked (and, before it,
// HydrateProjectOverridesAndProvenance) uses internally, so it can assert
// — deterministically, not by racing wall-clock timing against a
// near-instant empty-snapshot restore — that a save queued before the
// restore starts is still blocked immediately AFTER the overrides phase
// commits and immediately after the provenance phase commits, and only
// completes once the OUTER lock is released. Its precondition — the save's
// lock request has actually been issued and queued before the first
// "still blocked" assertion — is confirmed via pg_locks
// (waitForAdvisoryWaiter), not a fixed sleep: fix round 2 found that a
// sleep here can false-pass every assertion after it if the save simply
// hadn't gotten around to requesting its lock yet.
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

	// Wait for Postgres's own confirmation that the save's lock request is
	// actually queued — not a fixed sleep (see waitForAdvisoryWaiter).
	waitForAdvisoryWaiter(t, pool, advisorylock.ProjectLockKey(projectID))
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

// TestLockProjectForRestoreResetsLockTimeoutOnBusyFailure proves fix round
// 2's item 1: a FAILED acquisition (busy) must not leave lock_timeout set
// on the connection it returns to the pool. An earlier version reset
// lock_timeout only on the success path (inside the returned release
// closure), so a busy restore left one connection in the pool carrying
// restoreProjectLockTimeout (shortened here to 150ms) until it was
// retired: any later statement drawn on that connection that legitimately
// waits longer than that for a row lock would abort instead of waiting. A
// one-connection pool for the Materializer under test makes the reused
// connection the same one every time, so the leak (or its absence) is
// directly observable via SHOW lock_timeout.
func TestLockProjectForRestoreResetsLockTimeoutOnBusyFailure(t *testing.T) {
	ctx := context.Background()
	shared := hydrateTestPool(t)

	cfg, err := pgxpool.ParseConfig(shared.Config().ConnString())
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	cfg.MaxConns = 1
	singleConnPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open single-conn pool: %v", err)
	}
	defer singleConnPool.Close()

	m := NewMaterializer(singleConnPool, t.TempDir(), nil)

	orig := restoreProjectLockTimeout
	restoreProjectLockTimeout = 150 * time.Millisecond
	t.Cleanup(func() { restoreProjectLockTimeout = orig })

	const projectID = "TSTLOCKLEAK"

	// Holder: a connection from the SHARED pool (not the single-conn one
	// under test) takes the project key exclusively for the whole test,
	// so LockProjectForRestore below is guaranteed to time out busy.
	holderConn, err := shared.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire holder conn: %v", err)
	}
	defer holderConn.Release()
	key := advisorylock.ProjectLockKey(projectID)
	if _, err := holderConn.Exec(ctx, `SELECT pg_advisory_lock(hashtext($1))`, key); err != nil {
		t.Fatalf("holder take project lock: %v", err)
	}
	defer func() {
		_, _ = holderConn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext($1))`, key)
	}()

	_, lockErr := m.LockProjectForRestore(ctx, projectID)
	if lockErr == nil {
		t.Fatal("expected LockProjectForRestore to fail while the holder held the project lock")
	}
	if !errors.Is(lockErr, advisorylock.ErrLockBusy) {
		t.Fatalf("errors.Is(err, advisorylock.ErrLockBusy) = false, want true; err = %v", lockErr)
	}

	var timeout string
	if scanErr := singleConnPool.QueryRow(ctx, `SHOW lock_timeout`).Scan(&timeout); scanErr != nil {
		t.Fatalf("show lock_timeout: %v", scanErr)
	}
	if timeout != "0" {
		t.Errorf("lock_timeout left at %q on the recycled connection after a busy failure, want the server default %q", timeout, "0")
	}
}

// TestHydrateRestorePlanAcquiresLockBeforeAnyPhase proves fix round 2's
// item 2: the restore lock is acquired unconditionally at the top of the
// WHOLE pipeline, before ANY phase writes anything — not merely before
// overrides+provenance.
//
// It uses a plan with no Snapshot at all (`&RestorePlan{ProjectID:
// projectID}`), so the moment any phase from HydrateVendored onward
// actually runs against it, HydrateProjectShell returns a distinct,
// unmistakable error: "hydrate project shell: missing snapshot"
// (HydrateVendored itself tolerates a nil Snapshot as a no-op, so it's
// HydrateProjectShell, the second phase, that would surface first). If the
// lock is acquired only later — e.g. reverted to just before
// overrides+provenance — this test sees THAT error instead of the
// lock-busy one, because the vendored/shell phases would run to (this)
// failure before the lock is ever requested.
func TestHydrateRestorePlanAcquiresLockBeforeAnyPhase(t *testing.T) {
	pool := hydrateTestPool(t)
	m := NewMaterializer(pool, t.TempDir(), nil)
	overrideSvc := override.NewService(override.NewPostgresStore(pool), nil, nil)
	ctx := context.Background()

	orig := restoreProjectLockTimeout
	restoreProjectLockTimeout = 150 * time.Millisecond
	t.Cleanup(func() { restoreProjectLockTimeout = orig })

	const projectID = "TSTTOPLOCK"

	started := make(chan struct{})
	saveErr := make(chan error, 1)
	go func() {
		saveErr <- overrideSvc.WithEntityLock(ctx, projectID, "model", "TSTTOPLOCK.1", func(ctx context.Context) error {
			close(started)
			// Held well past the shortened restoreProjectLockTimeout, so
			// HydrateRestorePlan's own lock wait genuinely times out.
			time.Sleep(500 * time.Millisecond)
			return nil
		})
	}()

	select {
	case <-started:
	case err := <-saveErr:
		t.Fatalf("save failed before it took its lock: %v", err)
	}

	err := m.HydrateRestorePlan(ctx, &RestorePlan{ProjectID: projectID})
	if err == nil {
		t.Fatal("expected HydrateRestorePlan to fail while the save held the project lock")
	}
	if strings.Contains(err.Error(), "missing snapshot") {
		t.Fatalf("error reached a downstream phase (HydrateProjectShell) before the lock was ever acquired: %v", err)
	}
	if !errors.Is(err, advisorylock.ErrLockBusy) {
		t.Fatalf("errors.Is(err, advisorylock.ErrLockBusy) = false, want true; err = %v", err)
	}

	if err := <-saveErr; err != nil {
		t.Fatalf("save: %v", err)
	}
}
