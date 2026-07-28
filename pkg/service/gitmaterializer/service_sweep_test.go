//go:build integration

package gitmaterializer

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// sweepOwnerActorID is the actor seeded once and reused as owner for both
// projects created by TestSweepReconcilesEachProject.
const sweepOwnerActorID = "SWEEP_OWNER"

// TestSweepReconcilesEachProject is runSweep's core guarantee: given two
// projects that have each drifted from their materialized git tree (a
// closure gap simulated the same way TestReconcileHealsDrift does — a
// direct DB edit that bypasses the scoped-write path), a running sweep
// loop enumerates every project and calls Reconcile on it, healing both
// within the sweep's tick interval.
func TestSweepReconcilesEachProject(t *testing.T) {
	skipIfNoGit(t)
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models WHERE project_id IN ('SWEEP_P1', 'SWEEP_P2')`)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id IN ('SWEEP_P1', 'SWEEP_P2')`)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, sweepOwnerActorID)
	})

	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          sweepOwnerActorID,
		Type:        "organization",
		DisplayName: "Sweep Owner",
		Slug:        "sweep-owner",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create owner actor: %v", err)
	}

	baseDir := t.TempDir()
	mat := NewMaterializer(pool, baseDir, nil)

	p1 := sweepSeedProjectWithDrift(t, ctx, queries, mat, "SWEEP_P1", "SWEEP_M1")
	p2 := sweepSeedProjectWithDrift(t, ctx, queries, mat, "SWEEP_P2", "SWEEP_M2")

	svc := NewService(mat, nil, nil)
	svc.SetSweepInterval(10 * time.Millisecond)

	sweepCtx, cancel := context.WithCancel(ctx)
	sweepDone := make(chan struct{})
	go func() {
		defer close(sweepDone)
		svc.runSweep(sweepCtx)
	}()
	// Registered after baseDir's t.TempDir() cleanup, so it runs first
	// (t.Cleanup is LIFO): stop the sweep goroutine and wait for its current
	// tick to finish before the work dirs it's writing to get removed.
	t.Cleanup(func() {
		cancel()
		<-sweepDone
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		if headAdvanced(t, mat, p1) && headAdvanced(t, mat, p2) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for sweep to reconcile both projects")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// sweepSeededProject tracks a project's HEAD as it stood right after the
// drift-inducing DB edit, so headAdvanced can detect the sweep's reconcile
// commit landing on top of it.
type sweepSeededProject struct {
	projectID   string
	initialHead string
}

// sweepSeedProjectWithDrift creates a project with one model, materializes
// it (InitProject, mirroring TestReconcileHealsDrift), then edits the model
// directly in the DB — bypassing the scoped-write/change-log path — so the
// on-disk tree drifts from current DB state exactly like a closure gap
// would. Returns the project id and the HEAD sha captured right after the
// drifting edit, before any reconcile has run.
func sweepSeedProjectWithDrift(t *testing.T, ctx context.Context, queries *sqlcgen.Queries, mat *Materializer, projectID, modelID string) sweepSeededProject {
	t.Helper()

	uiName, _ := json.Marshal(map[string]string{"en": "Sweep Project " + projectID})
	desc, _ := json.Marshal(map[string]string{"en": "sweep test project"})
	if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID:          projectID,
		UiName:      uiName,
		Description: desc,
		Status:      "draft",
		OwnerID:     sweepOwnerActorID,
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create project %s: %v", projectID, err)
	}

	modelUIName, _ := json.Marshal(map[string]string{"en": "Sweep Model " + modelID})
	modelDesc, _ := json.Marshal(map[string]string{"en": "sweep test model"})
	scope, _ := json.Marshal(map[string]string{"prefix": "crm", "local_name": "E21_Person"})
	if _, err := queries.WeaveCreateModel(ctx, sqlcgen.WeaveCreateModelParams{
		ID:            modelID,
		SystemName:    stringPtr("sweep_model_" + modelID),
		UiName:        modelUIName,
		Description:   modelDesc,
		Status:        "draft",
		ProjectID:     projectID,
		OntologyScope: scope,
		ModelType:     "core",
	}); err != nil {
		t.Fatalf("create model %s: %v", modelID, err)
	}

	if err := mat.InitProject(ctx, projectID); err != nil {
		t.Fatalf("InitProject %s: %v", projectID, err)
	}

	// Simulate a closure gap: edit the model directly in the DB without
	// going through a change_set/scopedRewrite, so the on-disk tree drifts
	// from current DB state (same technique as TestReconcileHealsDrift).
	row, err := queries.WeaveGetModelByID(ctx, modelID)
	if err != nil {
		t.Fatalf("get model %s: %v", modelID, err)
	}
	drifted, _ := json.Marshal(map[string]string{"en": "Drifted " + modelID})
	if _, err := queries.WeaveUpdateModel(ctx, sqlcgen.WeaveUpdateModelParams{
		ID:            modelID,
		UiName:        drifted,
		Description:   row.Description,
		SystemName:    row.SystemName,
		Status:        row.Status,
		OntologyScope: row.OntologyScope,
		ModelType:     row.ModelType,
	}); err != nil {
		t.Fatalf("drift model %s: %v", modelID, err)
	}

	return sweepSeededProject{
		projectID:   projectID,
		initialHead: headSHA(t, mat, projectID),
	}
}

// headSHA returns the current HEAD commit sha for projectID's materialized
// work dir.
func headSHA(t *testing.T, mat *Materializer, projectID string) string {
	t.Helper()
	workDir := filepath.Join(mat.baseDir, projectID)
	git := newGitRunner(workDir, nil)
	sha, err := git.RevParseHead(context.Background())
	if err != nil {
		t.Fatalf("rev-parse HEAD for %s: %v", projectID, err)
	}
	return sha
}

// headAdvanced reports whether p's project HEAD has moved past the sha
// captured right after seeding drift — i.e. whether something (the sweep's
// Reconcile call) has committed on top of it.
func headAdvanced(t *testing.T, mat *Materializer, p sweepSeededProject) bool {
	t.Helper()
	return headSHA(t, mat, p.projectID) != p.initialHead
}
