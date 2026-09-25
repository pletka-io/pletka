// Package advisorylock holds the shared key-naming convention, busy
// sentinel, and the hardened session-level acquire/release primitives for
// Postgres project- and entity-scoped advisory locks, so pkg/weave/override
// (the save path — SHARED project lock then EXCLUSIVE entity lock, both
// session-level, on one pooled connection) and pkg/service/gitmaterializer
// (the restore path — one EXCLUSIVE, session-level project lock held across
// its whole overrides+provenance pipeline) can never drift on the key, the
// error, or the partial-acquisition/cancellation-free-unlock handling they
// both need.
//
// This package exists purely to break an import cycle: gitmaterializer's
// production code cannot import pkg/weave/override directly, because
// override's own integration tests (package override, not override_test)
// import internal/testdb, and internal/testdb imports gitmaterializer to
// hydrate its fixtures — so override(test) -> testdb -> gitmaterializer ->
// override(prod) would cycle the moment gitmaterializer's production code
// imported override back. advisorylock imports nothing of this repo's own,
// so it can never be part of a cycle; both sides import it instead of one
// importing the other.
package advisorylock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GraceTimeout bounds a Release call's own context, deliberately detached
// from the caller's ctx cancellation — see Release's doc comment for why.
const GraceTimeout = 5 * time.Second

// lockNotAvailableSQLState is Postgres's SQLSTATE for "lock_timeout
// exceeded while waiting for a lock" (55P03) — the signal Acquire
// translates into ErrLockBusy.
const lockNotAvailableSQLState = "55P03"

// Mode selects which family of session-level Postgres advisory-lock
// function Acquire/Release uses.
type Mode int

const (
	// Shared takes/releases pg_advisory_lock_shared/pg_advisory_unlock_shared:
	// any number of sessions may hold a key SHARED at once.
	Shared Mode = iota
	// Exclusive takes/releases pg_advisory_lock/pg_advisory_unlock: only one
	// session may hold a key EXCLUSIVE, and it excludes every SHARED holder too.
	Exclusive
)

func lockSQL(mode Mode) string {
	if mode == Shared {
		return `SELECT pg_advisory_lock_shared(hashtext($1))`
	}
	return `SELECT pg_advisory_lock(hashtext($1))`
}

func unlockSQL(mode Mode) string {
	if mode == Shared {
		return `SELECT pg_advisory_unlock_shared(hashtext($1))`
	}
	return `SELECT pg_advisory_unlock(hashtext($1))`
}

// ProjectLockKey returns the advisory-lock key for a project-wide lock: the
// SHARED lock a save takes before its entity lock
// (pkg/weave/override.Store.WithAdvisoryLock), and the EXCLUSIVE lock git
// restore holds across its whole overrides+provenance pipeline
// (gitmaterializer's Materializer.LockProjectForRestore). Both call this so
// the key can never drift between packages — pg_advisory_lock hashes the
// string with hashtext, so a mismatched prefix would silently stop the two
// sides from contending on the same lock at all.
func ProjectLockKey(projectID string) string {
	return "project:" + projectID
}

// ErrLockBusy is returned when a project or entity advisory lock could not
// be acquired within its bounded wait — someone else is already saving
// this entity, or restoring this project. Callers use errors.Is(err,
// ErrLockBusy). The message is deliberately generic: it surfaces verbatim
// (joined onto richer, caller-specific context) on both the save path and
// the restore path, so it must read sensibly on either — it does not name
// "override" or "entity" specifically the way an earlier version of this
// sentinel did.
var ErrLockBusy = errors.New("advisorylock: locked by another save or restore in progress")

// Acquire takes a session-level advisory lock on key in the given mode, on
// conn, translating a lock_timeout expiry (SQLSTATE 55P03) into ErrLockBusy
// — joined onto the underlying Postgres error (via errors.Join) rather than
// replacing it, so errors.Is(err, ErrLockBusy) still matches while an
// operator staring at the failure keeps the key and the server's own
// message too. conn must already have `lock_timeout` set (via a `SET
// lock_timeout` statement) to whatever bounds this specific attempt — that
// is the caller's responsibility, not Acquire's, since a caller taking two
// locks in sequence (a save's project lock then its entity lock) needs a
// different budget for each (see docs-oss and the save side's caller for
// why).
func Acquire(ctx context.Context, conn *pgxpool.Conn, mode Mode, key string) error {
	if _, err := conn.Exec(ctx, lockSQL(mode), key); err != nil {
		if isLockNotAvailable(err) {
			return errors.Join(ErrLockBusy, err)
		}
		return err
	}
	return nil
}

// Release releases a session-level advisory lock on key in the given mode
// on conn.
//
// The unlock must not run on ctx once ctx may already be done: pgx
// short-circuits an already-canceled ctx (newContextAlreadyDoneError)
// without ever touching the wire, so the UNLOCK never reaches Postgres, the
// session survives, and returning conn to a pool afterward would hand back
// a connection that is still holding the lock. Release always runs the
// unlock on a context that ignores the caller's cancellation, bounded
// instead by graceTimeout, so it always gets a real chance to reach the
// server; if it still fails, or Postgres reports the lock wasn't held by
// this session, the connection must not go back to a pool holding the
// lock, so Release closes it here — the caller's own conn.Release() then
// destroys the resource instead of recycling it. A non-nil return means
// exactly that: the caller must treat conn as already closed, not merely
// "maybe still holding the lock."
func Release(ctx context.Context, conn *pgxpool.Conn, mode Mode, key string, graceTimeout time.Duration) error {
	unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), graceTimeout)
	defer cancel()

	var released bool
	unlockErr := conn.QueryRow(unlockCtx, unlockSQL(mode), key).Scan(&released)
	if unlockErr == nil && released {
		return nil
	}
	if unlockErr == nil {
		unlockErr = fmt.Errorf("advisory lock %q was not held by this session at unlock", key)
	}
	_ = conn.Conn().Close(unlockCtx) //nolint:errcheck // best-effort; the caller's conn.Release() destroys the resource regardless
	return unlockErr
}

// isLockNotAvailable reports whether err is Postgres aborting a lock wait
// because lock_timeout fired (SQLSTATE 55P03).
func isLockNotAvailable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == lockNotAvailableSQLState
}
