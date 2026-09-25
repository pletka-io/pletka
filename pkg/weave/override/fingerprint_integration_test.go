//go:build integration

package override

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Scratch coordinates for fingerprint/lock tests: real LA fixture fields
// (LAF.5, LAF.6) placed on a scratch model id that does not collide with any
// real LA model or the ReplaceForEntity scratch id (TSTSTABLE.1).
const (
	fpTestProjectID  = "LA"
	fpTestEntityType = "model"
	fpTestEntityID   = "TSTFP.1"
)

// cleanupFPEntity registers cleanup of every override row on the scratch
// entity, so consecutive tests in this file don't see each other's leftovers.
func cleanupFPEntity(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM weave_field_overrides WHERE entity_type = $1 AND entity_id = $2`,
			fpTestEntityType, fpTestEntityID)
	})
}

func TestEntityFingerprintChangesWithSave(t *testing.T) {
	pool := testdb.Pool(t)
	store := NewPostgresStore(pool)
	svc := NewService(store, nil, nil)
	ctx := context.Background()
	cleanupFPEntity(t, pool)

	f5 := laFieldID(t, pool, "LAF.5")
	f6 := laFieldID(t, pool, "LAF.6")

	overrides := []domain.FieldOverride{
		{FieldID: f5, ProjectID: fpTestProjectID, Position: 1, DisplayName: domain.Translations{"en": "Name Type"}},
		{FieldID: f6, ProjectID: fpTestProjectID, Position: 2, DisplayName: domain.Translations{"en": "Name Value"}},
	}
	if err := store.ReplaceForEntity(ctx, fpTestEntityType, fpTestEntityID, overrides); err != nil {
		t.Fatalf("first save: %v", err)
	}

	fpA, err := svc.EntityFingerprint(ctx, fpTestEntityType, fpTestEntityID)
	if err != nil {
		t.Fatalf("fingerprint A: %v", err)
	}
	if fpA == "" {
		t.Fatal("expected non-empty fingerprint")
	}

	// Save again unchanged.
	if err := store.ReplaceForEntity(ctx, fpTestEntityType, fpTestEntityID, overrides); err != nil {
		t.Fatalf("second save: %v", err)
	}
	fpB, err := svc.EntityFingerprint(ctx, fpTestEntityType, fpTestEntityID)
	if err != nil {
		t.Fatalf("fingerprint B: %v", err)
	}
	if fpB != fpA {
		t.Fatalf("fingerprint changed on unchanged save: %s -> %s", fpA, fpB)
	}

	// Change one display name.
	overrides[0].DisplayName = domain.Translations{"en": "Name Type Changed"}
	if err := store.ReplaceForEntity(ctx, fpTestEntityType, fpTestEntityID, overrides); err != nil {
		t.Fatalf("third save: %v", err)
	}
	fpC, err := svc.EntityFingerprint(ctx, fpTestEntityType, fpTestEntityID)
	if err != nil {
		t.Fatalf("fingerprint C: %v", err)
	}
	if fpC == fpB {
		t.Fatal("fingerprint unchanged after display name edit")
	}
}

func TestEntityFingerprintCoversRefs(t *testing.T) {
	pool := testdb.Pool(t)
	store := NewPostgresStore(pool)
	svc := NewService(store, nil, nil)
	ctx := context.Background()
	cleanupFPEntity(t, pool)

	f5 := laFieldID(t, pool, "LAF.5")
	overrides := []domain.FieldOverride{
		{FieldID: f5, ProjectID: fpTestProjectID, Position: 1},
	}
	if err := store.ReplaceForEntity(ctx, fpTestEntityType, fpTestEntityID, overrides); err != nil {
		t.Fatalf("save: %v", err)
	}
	overrideID := overrides[0].ID

	before, err := svc.EntityFingerprint(ctx, fpTestEntityType, fpTestEntityID)
	if err != nil {
		t.Fatalf("fingerprint before: %v", err)
	}

	if err := store.SetRefs(ctx, overrideID, []domain.OverrideRef{
		{RefType: "resource_model", TargetID: "M1", Position: 1},
	}); err != nil {
		t.Fatalf("set refs: %v", err)
	}

	after, err := svc.EntityFingerprint(ctx, fpTestEntityType, fpTestEntityID)
	if err != nil {
		t.Fatalf("fingerprint after: %v", err)
	}
	if after == before {
		t.Fatal("fingerprint unchanged after setting a ref")
	}
}

func TestWithEntityLockSerialises(t *testing.T) {
	pool := testdb.Pool(t)
	store := NewPostgresStore(pool)
	svc := NewService(store, nil, nil)
	ctx := context.Background()

	holderErr := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		holderErr <- svc.WithEntityLock(ctx, fpTestProjectID, "model", "TSTFPLOCK.1", func(ctx context.Context) error {
			close(started)
			time.Sleep(300 * time.Millisecond)
			return nil
		})
	}()

	// The holder closes started from inside its callback, so a holder that
	// fails before it ever runs would leave this blocked until the test
	// binary's own panic. Take whichever arrives first.
	select {
	case <-started:
	case hErr := <-holderErr:
		t.Fatalf("holder failed before it took the lock: %v", hErr)
	}
	waitStart := time.Now()
	err := svc.WithEntityLock(ctx, fpTestProjectID, "model", "TSTFPLOCK.1", func(ctx context.Context) error {
		return nil
	})
	waited := time.Since(waitStart)

	if err != nil {
		t.Fatalf("WithEntityLock (waiter): %v", err)
	}
	if hErr := <-holderErr; hErr != nil {
		t.Fatalf("WithEntityLock (holder): %v", hErr)
	}
	if waited < 200*time.Millisecond {
		t.Fatalf("expected to wait at least 200ms for the held lock, waited %s", waited)
	}
}

// TestWithAdvisoryLockReleasesOnCancelledContext proves the fix for the
// leaked session-level lock: when the caller's ctx is already done by the
// time WithAdvisoryLock's deferred unlock runs, the unlock must still reach
// Postgres (on a context that ignores that cancellation) instead of being
// silently skipped and leaving the lock held by the connection sitting back
// in the pool. RED before the fix: pg_try_advisory_lock below returns false
// because the "unlock" never touched the wire.
//
// Verification deliberately opens its own connection OUTSIDE the pool
// (pgx.Connect, not pool.Acquire): pg_advisory_lock is re-entrant within one
// session, so if verification happened to reuse the very pooled connection
// that held the lock, pg_try_advisory_lock would trivially "succeed" against
// its own still-held lock and mask the leak instead of detecting it.
func TestWithAdvisoryLockReleasesOnCancelledContext(t *testing.T) {
	pool := testdb.Pool(t)
	store := NewPostgresStore(pool)

	const key = "model:TSTFPCANCEL.1"

	ctx, cancel := context.WithCancel(context.Background())
	err := store.WithAdvisoryLock(ctx, fpTestProjectID, key, func(cbCtx context.Context) error {
		// Cancel the ctx WithAdvisoryLock was called with before its
		// deferred unlock runs, so that unlock sees an already-done ctx —
		// the exact scenario a curator closing a tab mid-save produces.
		cancel()
		return cbCtx.Err()
	})
	if err == nil {
		t.Fatal("expected the callback's ctx.Err() to propagate")
	}

	verifyCtx := context.Background()
	rawConn, connErr := pgx.Connect(verifyCtx, pool.Config().ConnString())
	if connErr != nil {
		t.Fatalf("open dedicated verification conn: %v", connErr)
	}
	defer rawConn.Close(verifyCtx) //nolint:errcheck

	var acquired bool
	if scanErr := rawConn.QueryRow(verifyCtx, `SELECT pg_try_advisory_lock(hashtext($1))`, key).Scan(&acquired); scanErr != nil {
		t.Fatalf("try lock: %v", scanErr)
	}
	if acquired {
		_, _ = rawConn.Exec(verifyCtx, `SELECT pg_advisory_unlock(hashtext($1))`, key)
	}
	if !acquired {
		t.Fatal("advisory lock still held after WithAdvisoryLock returned on a canceled ctx — it leaked back into the pool")
	}
}

// TestWithAdvisoryLockResetsLockTimeout proves the lock does not leave its
// 10s lock_timeout on a connection the rest of the app then reuses: an
// unrelated release tag or git restore drawing that connection would abort
// on a row lock instead of waiting for it. A one-connection pool makes the
// reused connection the same one every time.
func TestWithAdvisoryLockResetsLockTimeout(t *testing.T) {
	ctx := context.Background()
	shared := testdb.Pool(t)

	cfg, err := pgxpool.ParseConfig(shared.Config().ConnString())
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open single-conn pool: %v", err)
	}
	defer pool.Close()

	store := NewPostgresStore(pool)
	if lockErr := store.WithAdvisoryLock(ctx, fpTestProjectID, "model:TSTFPRESET.1", func(context.Context) error {
		return nil
	}); lockErr != nil {
		t.Fatalf("WithAdvisoryLock: %v", lockErr)
	}

	var timeout string
	if scanErr := pool.QueryRow(ctx, `SHOW lock_timeout`).Scan(&timeout); scanErr != nil {
		t.Fatalf("show lock_timeout: %v", scanErr)
	}
	if timeout != "0" {
		t.Errorf("lock_timeout left at %q on the recycled connection, want the server default %q", timeout, "0")
	}
}
