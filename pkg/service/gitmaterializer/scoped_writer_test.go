//go:build integration

package gitmaterializer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// scopedWriterOwnerActorID/scopedWriterProjectID/scopedWriterModelID are the
// fixed ids seeded by setupOneModel. Deleted in t.Cleanup, so a fixed id is
// safe: this is the only test that touches them and cleanup runs even on
// failure.
const (
	scopedWriterOwnerActorID = "SCOPEDWRITE_OWNER"
	scopedWriterProjectID    = "SCOPEDWRITE_PROJECT"
	scopedWriterModelID      = "SCOPEDWRITE_MODEL"
)

// setupOneModel seeds one project with one model in the per-package test
// clone and returns a Materializer plus the ids needed to exercise
// writeModels/writeOneModel against it.
func setupOneModel(t *testing.T) (mat *Materializer, projectID, modelID string) {
	t.Helper()
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)

	projectID = scopedWriterProjectID
	modelID = scopedWriterModelID

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models WHERE id = $1`, modelID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, scopedWriterOwnerActorID)
	})

	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          scopedWriterOwnerActorID,
		Type:        "organization",
		DisplayName: "Scoped Writer Owner",
		Slug:        "scoped-writer-owner",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create owner actor: %v", err)
	}

	projectUIName, _ := json.Marshal(map[string]string{"en": "Scoped Writer Project"})
	projectDesc, _ := json.Marshal(map[string]string{"en": "Project for the writeOneModel drift test"})
	if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID:          projectID,
		UiName:      projectUIName,
		Description: projectDesc,
		Status:      "draft",
		OwnerID:     scopedWriterOwnerActorID,
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	modelUIName, _ := json.Marshal(map[string]string{"en": "Scoped Writer Model"})
	modelDesc, _ := json.Marshal(map[string]string{"en": "Model for the writeOneModel drift test"})
	ontologyScope, _ := json.Marshal(map[string]string{"prefix": "crm", "local_name": "E21_Person"})
	if _, err := queries.WeaveCreateModel(ctx, sqlcgen.WeaveCreateModelParams{
		ID:            modelID,
		SystemName:    stringPtr("scoped_writer_model"),
		UiName:        modelUIName,
		Description:   modelDesc,
		Status:        "draft",
		ProjectID:     projectID,
		OntologyScope: ontologyScope,
		ModelType:     "core",
	}); err != nil {
		t.Fatalf("create model: %v", err)
	}

	mat = NewMaterializer(pool, t.TempDir(), nil)
	return mat, projectID, modelID
}

// modelRelPath returns the work-tree-relative path a model file is written
// to, matching the PathSpec writeModels/writeOneModel both build.
func modelRelPath(modelID string) string {
	return domain.FilePath(domain.PathSpec{EntityType: "model", EntityID: modelID})
}

// cmpFileBytes reads both files and diffs their contents as strings, failing
// the test helper's caller with a useful message if either read fails.
func cmpFileBytes(t *testing.T, pathA, pathB string) string {
	t.Helper()
	a, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatalf("read %s: %v", pathA, err)
	}
	b, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatalf("read %s: %v", pathB, err)
	}
	return cmp.Diff(string(a), string(b))
}

// TestWriteOneModelMatchesWriteModels is the drift gate for Task 1: the
// scoped single-entity writer must byte-for-byte match what the whole-tree
// writer produces for the same model. If this ever fails, writeOneModel has
// drifted from writeModels and the scoped rewrite path is no longer safe.
func TestWriteOneModelMatchesWriteModels(t *testing.T) {
	m, projectID, modelID := setupOneModel(t)
	ctx := context.Background()
	workA := t.TempDir()
	workB := t.TempDir()

	if err := m.writeModels(ctx, workA, projectID); err != nil {
		t.Fatalf("writeModels: %v", err)
	}
	if err := m.writeOneModel(ctx, workB, projectID, modelID); err != nil {
		t.Fatalf("writeOneModel: %v", err)
	}

	rel := modelRelPath(modelID)
	if diff := cmpFileBytes(t, filepath.Join(workA, rel), filepath.Join(workB, rel)); diff != "" {
		t.Fatalf("writeOneModel drifted from writeModels:\n%s", diff)
	}
}
