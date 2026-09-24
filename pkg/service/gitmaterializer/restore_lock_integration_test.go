//go:build integration

package gitmaterializer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

// TestHydrateFunctionsBlockOnInFlightSaveThenComplete proves scope item 3
// of the restore-lock design: HydrateProjectOverrides and
// HydrateProjectProvenance both take the project lock as the first
// statement of their transaction, so each blocks while a save
// (override.Service.WithEntityLock) holds the SAME project key SHARED for
// the same project, and completes once the save releases it — instead of
// racing its whole-project clear-then-reinsert against the save's writes.
//
// HydrateProjectProvenance is covered here, not just HydrateProjectOverrides:
// a save's adoption sync (h.weave.Adoptions().ReplaceForContext, called from
// pkg/weave/project/override_write.go's saveOverrides) runs inside the same
// entity-lock scope as the rest of the save, and clearProjectAdoptionsAndForks
// clears the same weave_adoptions table that sync writes to — the identical
// race the overrides side has, just one table over. See restore-lock-report.md
// for the full analysis, including why forks are NOT covered by this lock.
func TestHydrateFunctionsBlockOnInFlightSaveThenComplete(t *testing.T) {
	pool := hydrateTestPool(t)
	m := NewMaterializer(pool, t.TempDir(), nil)
	overrideSvc := override.NewService(override.NewPostgresStore(pool), nil, nil)
	ctx := context.Background()

	hydrate := map[string]func(context.Context, *RestorePlan) error{
		"HydrateProjectOverrides":  m.HydrateProjectOverrides,
		"HydrateProjectProvenance": m.HydrateProjectProvenance,
	}

	for name, fn := range hydrate {
		t.Run(name, func(t *testing.T) {
			projectID := "TSTRESTORELOCK_" + name
			started := make(chan struct{})
			saveErr := make(chan error, 1)
			go func() {
				saveErr <- overrideSvc.WithEntityLock(ctx, projectID, "model", projectID+".1", func(ctx context.Context) error {
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
			if err := fn(ctx, minimalRestorePlan(projectID)); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			waited := time.Since(waitStart)

			if err := <-saveErr; err != nil {
				t.Fatalf("save: %v", err)
			}
			if waited < 250*time.Millisecond {
				t.Fatalf("%s returned after only %s — expected it to block on the in-flight save's project lock (held ~300ms)", name, waited)
			}
		})
	}
}

// TestRestoreProjectLockTimesOutWhenSaveHeldTooLong proves scope item 4:
// the restore's own project-lock wait is bounded by its own lock_timeout
// (restoreProjectLockTimeout), not left to wait forever behind a save, and
// the resulting error names the project and reports a save in flight.
// restoreProjectLockTimeout is shortened for the duration of this test the
// way override.lockTimeout is shortened in lock_busy_integration_test.go.
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

	err := m.HydrateProjectOverrides(ctx, minimalRestorePlan(projectID))
	if err == nil {
		t.Fatal("expected HydrateProjectOverrides to fail while the save held the project lock")
	}
	if !errors.Is(err, override.ErrLockBusy) {
		t.Fatalf("errors.Is(err, override.ErrLockBusy) = false, want true; err = %v", err)
	}
	if !strings.Contains(err.Error(), projectID) {
		t.Fatalf("error %q does not name the project %q", err.Error(), projectID)
	}

	if err := <-saveErr; err != nil {
		t.Fatalf("save: %v", err)
	}
}
