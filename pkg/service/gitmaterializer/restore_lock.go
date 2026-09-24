package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pletka-io/pletka/pkg/database/advisorylock"
)

// restoreProjectLockTimeout bounds how long a restore's exclusive project
// lock wait can take before it gives up. A save takes the SAME key SHARED
// (see override.WithAdvisoryLock) for the whole span of one entity save,
// bounded by its own 10s lockTimeout — so a restore that simply waited
// forever could queue behind a slow-but-healthy save indefinitely. A
// restore is rare and operator-initiated, though, so it is correct for it
// to wait considerably longer than a save's own 10s before it gives up: an
// operator watching a restore job would rather it wait out an in-flight
// save than fail outright over a save that was merely slow, not stuck.
// 30s — 3x a save's own budget — gives real margin over ordinary save
// latency while still failing within a bounded, human-noticeable time
// instead of hanging forever.
//
// A var, not a const, solely so an integration test can shorten it to
// exercise a real lock-busy wait in well under 10 seconds — see
// restore_lock_integration_test.go. Production code never assigns to it.
var restoreProjectLockTimeout = 30 * time.Second //nolint:gochecknoglobals // test-only override hook, mirrors override.lockTimeout

// restoreLockNotAvailableSQLState is Postgres's SQLSTATE for "lock_timeout
// exceeded while waiting for a lock" (55P03) — the same code
// override.postgresStore classifies into ErrLockBusy.
const restoreLockNotAvailableSQLState = "55P03"

// acquireProjectRestoreLock takes an EXCLUSIVE, transaction-scoped Postgres
// advisory lock (pg_advisory_xact_lock) on projectID as the first statement
// of tx, so tx's whole-project clear-then-reinsert cannot interleave with a
// save — which holds the SAME key SHARED for the whole span of one save
// (see override.Store.WithAdvisoryLock). pg_advisory_xact_lock releases
// automatically on commit or rollback, so — unlike the save side's
// session-level lock — nothing here needs an explicit unlock or a RESET.
//
// Lock ordering is project-then-entity everywhere a save takes both, and a
// restore only ever takes the project lock, so the two lock users can never
// form a deadlock cycle between them.
func acquireProjectRestoreLock(ctx context.Context, tx pgx.Tx, projectID string) error {
	// SET LOCAL does not accept a bind parameter, so the bound duration is
	// formatted into the statement text; restoreProjectLockTimeout is a
	// package variable only so a test can shorten it, never caller input.
	// SET LOCAL (not SET) confines the timeout to this transaction — it
	// reverts automatically at commit or rollback, so this needs no RESET
	// counterpart the way the save side's session-level SET does.
	setLockTimeout := fmt.Sprintf(`SET LOCAL lock_timeout = '%dms'`, restoreProjectLockTimeout.Milliseconds())
	if _, err := tx.Exec(ctx, setLockTimeout); err != nil {
		return fmt.Errorf("set restore lock_timeout: %w", err)
	}

	key := advisorylock.ProjectLockKey(projectID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, key); err != nil {
		if isRestoreLockNotAvailable(err) {
			// Joins advisorylock.ErrLockBusy (the same value
			// pkg/weave/override.ErrLockBusy re-exports) onto the
			// project-named message so a caller that only cares "is
			// something else holding this lock" can use the same
			// errors.Is check on either side, while an operator staring
			// at the failed restore job still gets the project id and an
			// explanation, not just the sentinel.
			return fmt.Errorf("restore project %s: a save is in flight for this project: %w", projectID, errors.Join(advisorylock.ErrLockBusy, err))
		}
		return fmt.Errorf("take project restore lock %q: %w", key, err)
	}
	return nil
}

// isRestoreLockNotAvailable reports whether err is Postgres aborting a lock
// wait because lock_timeout fired (SQLSTATE 55P03).
func isRestoreLockNotAvailable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == restoreLockNotAvailableSQLState
}
