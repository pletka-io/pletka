package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pletka-io/pletka/pkg/database/advisorylock"
)

// restoreProjectLockTimeout bounds how long LockProjectForRestore waits to
// acquire the project's advisory lock before it gives up. A save takes the
// SAME key SHARED (see override.WithAdvisoryLock) for the whole span of one
// entity save, bounded by its own 10s lockTimeout — so a restore that
// simply waited forever could queue behind a slow-but-healthy save
// indefinitely. A restore is rare and operator-initiated, though, so it is
// correct for it to wait considerably longer than a save's own 10s before
// it gives up: an operator watching a restore job would rather it wait out
// an in-flight save than fail outright over a save that was merely slow,
// not stuck. 30s — 3x a save's own budget — gives real margin over
// ordinary save latency while still failing within a bounded,
// human-noticeable time instead of hanging forever.
//
// A var, not a const, solely so an integration test can shorten it to
// exercise a real lock-busy wait in well under 10 seconds — see
// restore_lock_integration_test.go. Production code never assigns to it.
var restoreProjectLockTimeout = 30 * time.Second //nolint:gochecknoglobals // test-only override hook, mirrors override.lockTimeout

// LockProjectForRestore takes a session-level EXCLUSIVE Postgres advisory
// lock on projectID, on its own pooled connection, and returns a release
// func that must be called exactly once (calling it again is a safe no-op)
// when the caller is done. Not named acquireProjectLock (see lock.go, the
// pre-existing flock acquirer for a project's git working directory) so the
// two "project lock" concepts read as distinct at every call site — this
// one is a Postgres advisory lock, that one is a filesystem flock, and they
// protect entirely different things.
//
// The lock is SESSION-level, held on its own connection independent of any
// transaction, NOT `pg_advisory_xact_lock` scoped to a single transaction.
// That distinction is load-bearing: a restore that hydrates overrides then
// provenance is a PIPELINE of two separately committed transactions
// (HydrateProjectOverrides, then HydrateProjectProvenance). An earlier
// version of this lock was taken per-transaction — the first statement of
// each hydration transaction, released at that transaction's own commit.
// pg_advisory_xact_lock releases exactly at commit, and Postgres wakes the
// next queued waiter right there: a save queued behind the overrides
// phase's lock was granted the instant that phase committed, ran to
// completion, and wrote its own adoption row — which the provenance
// phase then deleted a moment later along with every other adoption,
// reinserting only the snapshot's. The curator's save returned 200 and the
// categories they had just adopted were silently gone. Holding ONE lock
// across the WHOLE pipeline — acquired here, before ANYTHING in the
// pipeline is written, released only once every phase has finished —
// closes that seam: there is no point during the restore where the lock is
// not held, so no save can be granted mid-pipeline.
//
// This also means a lock-acquisition failure (busy, or a genuine
// connection error) now happens before the pipeline's first write, so a
// project can no longer end up with its shell/entities at snapshot state
// while its overrides/provenance stay at pre-restore state with nothing to
// roll either half back — see the busy-path error message below for what
// this function tells an operator when it does still fail (vendored/shell/
// entities may already be committed from EARLIER, separately-locked
// pipeline phases that this lock does not cover, since they don't
// interleave with anything a save writes).
//
// Reuses the same hardened session-level acquire/release primitives the
// save path uses (pkg/database/advisorylock, also used by
// pkg/weave/override) rather than re-deriving the partial-acquisition and
// cancellation-free-unlock handling here.
//
// Not re-entrant: nothing in gitmaterializer nests a second
// LockProjectForRestore call for the same project inside the first, and
// nothing should — see override.Store.WithAdvisoryLock's doc comment for
// the deadlock a nested project-scoped acquisition can create against a
// concurrently-queued save.
func (m *Materializer) LockProjectForRestore(ctx context.Context, projectID string) (release func() error, err error) {
	conn, err := m.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("lock project %s for restore: acquire lock conn: %w", projectID, err)
	}

	setLockTimeout := fmt.Sprintf(`SET lock_timeout = '%dms'`, restoreProjectLockTimeout.Milliseconds())
	if _, execErr := conn.Exec(ctx, setLockTimeout); execErr != nil {
		conn.Release()
		return nil, fmt.Errorf("lock project %s for restore: set lock_timeout: %w", projectID, execErr)
	}

	key := advisorylock.ProjectLockKey(projectID)
	if lockErr := advisorylock.Acquire(ctx, conn, advisorylock.Exclusive, key); lockErr != nil {
		conn.Release()
		if errors.Is(lockErr, advisorylock.ErrLockBusy) {
			// Joins advisorylock.ErrLockBusy (the same value
			// pkg/weave/override.ErrLockBusy re-exports) onto an
			// operator-facing message: names the project, says a save is
			// in flight, and says plainly the project may already be
			// partially restored (earlier pipeline phases run and commit
			// before this lock is ever requested) and that re-running the
			// restore is the correct next step once the save clears.
			return nil, fmt.Errorf(
				"restore project %s: a save is in flight for this project — the project may be partially restored (earlier phases may already be applied); re-run the restore once the save completes: %w",
				projectID, lockErr)
		}
		return nil, fmt.Errorf("lock project %s for restore: take advisory lock %q: %w", projectID, key, lockErr)
	}

	var released bool
	release = func() error {
		if released {
			return nil
		}
		released = true
		defer conn.Release()

		if unlockErr := advisorylock.Release(ctx, conn, advisorylock.Exclusive, key, advisorylock.GraceTimeout); unlockErr != nil {
			return fmt.Errorf("release project restore lock %q: %w", key, unlockErr)
		}

		resetCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), advisorylock.GraceTimeout)
		defer cancel()
		if _, resetErr := conn.Exec(resetCtx, `RESET lock_timeout`); resetErr != nil {
			_ = conn.Conn().Close(resetCtx) //nolint:errcheck // best-effort; the deferred conn.Release() destroys the resource regardless
			return fmt.Errorf("reset lock_timeout: %w", resetErr)
		}
		return nil
	}
	return release, nil
}
