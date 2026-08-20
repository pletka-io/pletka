//go:build integration

package gitmaterializer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// scopedRewriteOwnerActorID/scopedRewriteProjectID are the fixed ids seeded
// by setupScopedRewriteFixture. Deleted in t.Cleanup at the end of each test
// (tests in this file run sequentially, not in parallel), matching the
// convention established by closureFixture/setupOneModel.
const (
	scopedRewriteOwnerActorID = "SCOPEDREWRITE_OWNER"
	scopedRewriteProjectID    = "SCOPEDREWRITE_PROJECT"
)

// scopedRewriteFixture seeds one project (with owner actor) in the
// per-package test clone and provides helpers to build the models/fields/
// overrides scopedRewrite() reads, plus a way to materialize a full initial
// tree to rewrite against.
type scopedRewriteFixture struct {
	t         *testing.T
	ctx       context.Context
	queries   *sqlcgen.Queries
	mat       *Materializer
	projectID string
}

func setupScopedRewriteFixture(t *testing.T) *scopedRewriteFixture {
	t.Helper()
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, scopedRewriteProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models WHERE project_id = $1`, scopedRewriteProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_collections WHERE project_id = $1`, scopedRewriteProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id = $1`, scopedRewriteProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, scopedRewriteProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, scopedRewriteOwnerActorID)
	})

	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          scopedRewriteOwnerActorID,
		Type:        "organization",
		DisplayName: "Scoped Rewrite Owner",
		Slug:        "scoped-rewrite-owner",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create owner actor: %v", err)
	}

	uiName, _ := json.Marshal(map[string]string{"en": "Scoped Rewrite Project"})
	desc, _ := json.Marshal(map[string]string{"en": "Project for scopedRewrite() tests"})
	if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID:          scopedRewriteProjectID,
		UiName:      uiName,
		Description: desc,
		Status:      "draft",
		OwnerID:     scopedRewriteOwnerActorID,
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	return &scopedRewriteFixture{
		t:         t,
		ctx:       ctx,
		queries:   queries,
		mat:       NewMaterializer(pool, t.TempDir(), nil),
		projectID: scopedRewriteProjectID,
	}
}

func (f *scopedRewriteFixture) model(id string) {
	f.t.Helper()
	uiName, _ := json.Marshal(map[string]string{"en": "Scoped Rewrite Model " + id})
	desc, _ := json.Marshal(map[string]string{"en": "scoped rewrite test model"})
	scope, _ := json.Marshal(map[string]string{"prefix": "crm", "local_name": "E21_Person"})
	if _, err := f.queries.WeaveCreateModel(f.ctx, sqlcgen.WeaveCreateModelParams{
		ID:            id,
		SystemName:    stringPtr("scoped_rewrite_model_" + id),
		UiName:        uiName,
		Description:   desc,
		Status:        "draft",
		ProjectID:     f.projectID,
		OntologyScope: scope,
		ModelType:     "core",
	}); err != nil {
		f.t.Fatalf("create model %s: %v", id, err)
	}
}

func (f *scopedRewriteFixture) field(id string) {
	f.t.Helper()
	uiName, _ := json.Marshal(map[string]string{"en": "Scoped Rewrite Field " + id})
	desc, _ := json.Marshal(map[string]string{"en": "scoped rewrite test field"})
	scope, _ := json.Marshal(map[string]string{"prefix": "crm", "local_name": "P1_is_identified_by"})
	if _, err := f.queries.WeaveCreateField(f.ctx, sqlcgen.WeaveCreateFieldParams{
		ID:            id,
		SystemName:    stringPtr("scoped_rewrite_field_" + id),
		UiName:        uiName,
		Description:   desc,
		Status:        "draft",
		ProjectID:     f.projectID,
		OntologyScope: scope,
	}); err != nil {
		f.t.Fatalf("create field %s: %v", id, err)
	}
}

// placeFieldOnModel creates a model-owned override row placing fieldID on
// modelID, and returns the override row's id.
func (f *scopedRewriteFixture) placeFieldOnModel(fieldID, modelID string) int64 {
	f.t.Helper()
	o, err := f.queries.WeaveCreateOverride(f.ctx, sqlcgen.WeaveCreateOverrideParams{
		FieldID:    fieldID,
		ProjectID:  f.projectID,
		EntityType: "model",
		EntityID:   modelID,
	})
	if err != nil {
		f.t.Fatalf("create override placing field %s on model %s: %v", fieldID, modelID, err)
	}
	return o.ID
}

// materializeAll writes categories/fields/models/collections plus the
// model/collection override subtrees into workDir, mirroring the subset of
// writeProjectTree that scopedRewrite() needs to have a baseline tree to
// rewrite against (skips project/adoption/fork manifests — irrelevant here).
func (f *scopedRewriteFixture) materializeAll(workDir string) {
	f.t.Helper()
	if err := f.mat.writeFields(f.ctx, workDir, f.projectID); err != nil {
		f.t.Fatalf("writeFields: %v", err)
	}
	if err := f.mat.writeModels(f.ctx, workDir, f.projectID); err != nil {
		f.t.Fatalf("writeModels: %v", err)
	}
	if err := f.mat.writeCollections(f.ctx, workDir, f.projectID); err != nil {
		f.t.Fatalf("writeCollections: %v", err)
	}
	if err := f.mat.writeScopedOverridesByProject(f.ctx, workDir, f.projectID, "model"); err != nil {
		f.t.Fatalf("writeScopedOverridesByProject(model): %v", err)
	}
	if err := f.mat.writeScopedOverridesByProject(f.ctx, workDir, f.projectID, "collection"); err != nil {
		f.t.Fatalf("writeScopedOverridesByProject(collection): %v", err)
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// TestScopedRewriteWritesAndDeletes is the brief's canonical scenario:
// a delete ref removes the whole entity directory, a non-delete ref
// (re)writes the entity file via the same single-entity writer the
// whole-tree path uses.
func TestScopedRewriteWritesAndDeletes(t *testing.T) {
	f := setupScopedRewriteFixture(t)
	f.model("SCOPEDREWRITE_MODEL_DEL")
	f.field("SCOPEDREWRITE_FIELD_1")
	workDir := t.TempDir()
	f.materializeAll(workDir)

	// Sanity: the tree exists before the rewrite.
	if !dirExists(filepath.Join(workDir, "models", "SCOPEDREWRITE_MODEL_DEL")) {
		t.Fatalf("setup: expected models/SCOPEDREWRITE_MODEL_DEL to exist before rewrite")
	}

	refs := []ScopedRef{
		{EntityType: "model", EntityID: "SCOPEDREWRITE_MODEL_DEL", Delete: true},
		{EntityType: "field", EntityID: "SCOPEDREWRITE_FIELD_1"},
	}
	if err := f.mat.scopedRewrite(f.ctx, workDir, f.projectID, refs); err != nil {
		t.Fatalf("scopedRewrite: %v", err)
	}

	if dirExists(filepath.Join(workDir, "models", "SCOPEDREWRITE_MODEL_DEL")) {
		t.Fatalf("models/SCOPEDREWRITE_MODEL_DEL should be removed")
	}
	if !fileExists(filepath.Join(workDir, "fields", "SCOPEDREWRITE_FIELD_1", "field.yaml")) {
		t.Fatalf("fields/SCOPEDREWRITE_FIELD_1/field.yaml should be (re)written")
	}
}

// TestScopedRewriteRegeneratesModelOverrides proves writeOverridesForOwner's
// clear-then-rewrite behavior: after a model ref is rewritten, its
// overrides/ subtree reflects the CURRENT DB state, not whatever was
// materialized before. A placement removed from the DB between the initial
// materialization and the scoped rewrite must drop out of the tree, and a
// placement still present must be (re)written.
func TestScopedRewriteRegeneratesModelOverrides(t *testing.T) {
	f := setupScopedRewriteFixture(t)
	f.model("SCOPEDREWRITE_MODEL_OV")
	f.field("SCOPEDREWRITE_FIELD_KEEP")
	f.field("SCOPEDREWRITE_FIELD_DROP")
	f.placeFieldOnModel("SCOPEDREWRITE_FIELD_KEEP", "SCOPEDREWRITE_MODEL_OV")
	dropID := f.placeFieldOnModel("SCOPEDREWRITE_FIELD_DROP", "SCOPEDREWRITE_MODEL_OV")

	workDir := t.TempDir()
	f.materializeAll(workDir)

	overridesDir := filepath.Join(workDir, "models", "SCOPEDREWRITE_MODEL_OV", "overrides")
	keepOverridePath := filepath.Join(overridesDir, "SCOPEDREWRITE_FIELD_KEEP@"+idStr(t, f.queries, f.ctx, "SCOPEDREWRITE_FIELD_KEEP", "SCOPEDREWRITE_MODEL_OV")+".yaml")
	dropOverridePath := filepath.Join(overridesDir, "SCOPEDREWRITE_FIELD_DROP@"+idStr(t, f.queries, f.ctx, "SCOPEDREWRITE_FIELD_DROP", "SCOPEDREWRITE_MODEL_OV")+".yaml")
	if !fileExists(keepOverridePath) {
		t.Fatalf("setup: expected %s to exist before rewrite", keepOverridePath)
	}
	if !fileExists(dropOverridePath) {
		t.Fatalf("setup: expected %s to exist before rewrite", dropOverridePath)
	}

	// Remove the "drop" placement from the DB, simulating an edit that
	// unplaced the field from the model before the scoped rewrite runs.
	if err := f.queries.WeaveDeleteOverride(f.ctx, dropID); err != nil {
		t.Fatalf("delete override %d: %v", dropID, err)
	}

	refs := []ScopedRef{{EntityType: "model", EntityID: "SCOPEDREWRITE_MODEL_OV"}}
	if err := f.mat.scopedRewrite(f.ctx, workDir, f.projectID, refs); err != nil {
		t.Fatalf("scopedRewrite: %v", err)
	}

	if !fileExists(keepOverridePath) {
		t.Fatalf("expected %s to survive the rewrite (still placed in DB)", keepOverridePath)
	}
	if fileExists(dropOverridePath) {
		t.Fatalf("expected %s to be removed by the rewrite (placement deleted in DB)", dropOverridePath)
	}
}

// idStr looks up the override id placing fieldID on modelID so the test can
// build the exact override file name (domain.FilePath embeds the override's
// bigserial id in "<fieldKey>@<overrideID>.yaml").
func idStr(t *testing.T, q *sqlcgen.Queries, ctx context.Context, fieldID, modelID string) string {
	t.Helper()
	rows, err := q.WeaveListOverridesForOwner(ctx, sqlcgen.WeaveListOverridesForOwnerParams{
		ProjectID:  scopedRewriteProjectID,
		EntityType: "model",
		EntityID:   modelID,
	})
	if err != nil {
		t.Fatalf("list overrides for owner %s: %v", modelID, err)
	}
	for _, row := range rows {
		if row.FieldID == fieldID {
			return fmt.Sprintf("%d", row.ID)
		}
	}
	t.Fatalf("no override found placing field %s on model %s", fieldID, modelID)
	return ""
}
