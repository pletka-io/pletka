// Package advisorylock holds the shared key-naming convention and busy
// sentinel for Postgres project-level advisory locks, so pkg/weave/override
// (the save path, session-level pg_advisory_lock_shared/pg_advisory_lock)
// and pkg/service/gitmaterializer (the restore path,
// transaction-scoped pg_advisory_xact_lock) can never drift on the key or
// the error they both use.
//
// This package exists purely to break an import cycle: gitmaterializer's
// production code cannot import pkg/weave/override directly, because
// override's own integration tests (package override, not override_test)
// import internal/testdb, and internal/testdb imports gitmaterializer to
// hydrate its fixtures — so override(test) -> testdb -> gitmaterializer ->
// override(prod) would cycle the moment gitmaterializer's production code
// imported override back. advisorylock imports nothing itself, so it can
// never be part of a cycle; both sides import it instead of one importing
// the other.
package advisorylock

import "errors"

// ProjectLockKey returns the advisory-lock key for a project-wide lock: the
// SHARED lock a save takes before its entity lock
// (override.Store.WithAdvisoryLock), and the EXCLUSIVE, transaction-scoped
// lock git restore takes for its whole hydration transaction
// (gitmaterializer's acquireProjectRestoreLock). Both call this so the key
// can never drift between packages — pg_advisory_lock hashes the string
// with hashtext, so a mismatched prefix would silently stop the two sides
// from contending on the same lock at all.
func ProjectLockKey(projectID string) string {
	return "project:" + projectID
}

// ErrLockBusy is returned when a project or entity advisory lock could not
// be acquired within its bounded wait — someone else is already saving
// this entity, or restoring this project. Callers use errors.Is(err,
// ErrLockBusy); pkg/weave/override.ErrLockBusy is this exact value
// (unchanged message text, predating this package), re-exported so
// existing override callers and log greps keep working unqualified.
var ErrLockBusy = errors.New("override: entity is locked by another save")
