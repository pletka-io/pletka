//go:build integration

package override

import (
	"context"
	"sync"
	"testing"
	"time"

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

	var wg sync.WaitGroup
	wg.Add(1)
	started := make(chan struct{})
	go func() {
		defer wg.Done()
		_ = svc.WithEntityLock(ctx, "model", "TSTFPLOCK.1", func(ctx context.Context) error {
			close(started)
			time.Sleep(300 * time.Millisecond)
			return nil
		})
	}()

	<-started
	waitStart := time.Now()
	err := svc.WithEntityLock(ctx, "model", "TSTFPLOCK.1", func(ctx context.Context) error {
		return nil
	})
	waited := time.Since(waitStart)
	wg.Wait()

	if err != nil {
		t.Fatalf("WithEntityLock: %v", err)
	}
	if waited < 200*time.Millisecond {
		t.Fatalf("expected to wait at least 200ms for the held lock, waited %s", waited)
	}
}
