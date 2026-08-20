//go:build integration

package gitmaterializer

import (
	"path/filepath"
	"testing"
)

// TestReconcileHealsDrift is the backstop's core guarantee: a scoped-write
// closure gap (a DB change materialized files were never regenerated for)
// leaves the on-disk tree stale, and Reconcile's full-rebuild-and-diff must
// notice and commit it. A second Reconcile against the now-healed tree is a
// clean no-op.
func TestReconcileHealsDrift(t *testing.T) {
	f := setupMatScopedFixture(t)
	f.model("MATSCOPED_RECONCILE_MODEL")

	if err := f.mat.InitProject(f.ctx, f.projectID); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	modelFile := filepath.Join(f.workDir(), "models", "MATSCOPED_RECONCILE_MODEL", "model.yaml")
	if !fileExists(modelFile) {
		t.Fatalf("setup: expected %s to exist after InitProject", modelFile)
	}

	// Simulate a closure gap: edit the model directly in the DB without going
	// through a change_set/scopedRewrite, so the on-disk tree drifts from
	// current DB state.
	f.updateModelUIName("MATSCOPED_RECONCILE_MODEL", "Drifted Name")

	changed, err := f.mat.Reconcile(f.ctx, f.projectID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if changed == 0 {
		t.Fatalf("reconcile should have committed the drifted model")
	}

	// Second reconcile is a clean no-op: the tree now matches current DB
	// state, so the diff is empty and no commit is made.
	changed2, err := f.mat.Reconcile(f.ctx, f.projectID)
	if err != nil {
		t.Fatalf("reconcile2: %v", err)
	}
	if changed2 != 0 {
		t.Fatalf("second reconcile should no-op, got %d", changed2)
	}
}
