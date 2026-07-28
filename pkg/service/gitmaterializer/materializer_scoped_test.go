//go:build integration

package gitmaterializer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// matScopedOwnerActorID/matScopedProjectID are the fixed ids seeded by
// setupMatScopedFixture. Deleted in t.Cleanup at the end of each test (tests
// in this file run sequentially, not in parallel), matching the convention
// established by closureFixture/scopedRewriteFixture.
const (
	matScopedOwnerActorID = "MATSCOPED_OWNER"
	matScopedProjectID    = "MATSCOPED_PROJECT"
)

// matScopedFixture seeds one project (with owner actor) in the per-package
// test clone and drives the same DB mutations a real edit/delete would make,
// so processChangeSet's draft path (the hot path under test) can be exercised
// end to end: closure() + scopedRewrite() + git add/commit/mark-processed.
type matScopedFixture struct {
	t         *testing.T
	ctx       context.Context
	pool      *pgxpool.Pool
	queries   *sqlcgen.Queries
	mat       *Materializer
	projectID string
	baseDir   string
}

func setupMatScopedFixture(t *testing.T) *matScopedFixture {
	t.Helper()
	skipIfNoGit(t)
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_change_log WHERE project_id = $1`, matScopedProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_change_set WHERE project_id = $1`, matScopedProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, matScopedProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE project_id = $1`, matScopedProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, matScopedProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE ontology_id IN (SELECT id FROM weave_ontologies WHERE prefix = 'mso')`)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE prefix = 'mso'`)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models WHERE project_id = $1`, matScopedProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_collections WHERE project_id = $1`, matScopedProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id = $1`, matScopedProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_categories WHERE project_id = $1`, matScopedProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, matScopedProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, matScopedOwnerActorID)
	})

	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          matScopedOwnerActorID,
		Type:        "organization",
		DisplayName: "Materializer Scoped Owner",
		Slug:        "materializer-scoped-owner",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create owner actor: %v", err)
	}

	uiName, _ := json.Marshal(map[string]string{"en": "Materializer Scoped Project"})
	desc, _ := json.Marshal(map[string]string{"en": "Project for the scoped-rewrite hot-path tests"})
	if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID:          matScopedProjectID,
		UiName:      uiName,
		Description: desc,
		Status:      "draft",
		OwnerID:     matScopedOwnerActorID,
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	baseDir := t.TempDir()
	return &matScopedFixture{
		t:         t,
		ctx:       ctx,
		pool:      pool,
		queries:   queries,
		mat:       NewMaterializer(pool, baseDir, nil),
		projectID: matScopedProjectID,
		baseDir:   baseDir,
	}
}

func (f *matScopedFixture) workDir() string {
	return filepath.Join(f.baseDir, f.projectID)
}

// model creates a bare model row in the fixture's project.
func (f *matScopedFixture) model(id string) {
	f.t.Helper()
	uiName, _ := json.Marshal(map[string]string{"en": "Matscoped Model " + id})
	desc, _ := json.Marshal(map[string]string{"en": "matscoped test model"})
	scope, _ := json.Marshal(map[string]string{"prefix": "crm", "local_name": "E21_Person"})
	if _, err := f.queries.WeaveCreateModel(f.ctx, sqlcgen.WeaveCreateModelParams{
		ID:            id,
		SystemName:    stringPtr("matscoped_model_" + strings.ReplaceAll(id, ".", "_")),
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

// field creates a bare field row in the fixture's project.
func (f *matScopedFixture) field(id string) {
	f.t.Helper()
	uiName, _ := json.Marshal(map[string]string{"en": "Matscoped Field " + id})
	desc, _ := json.Marshal(map[string]string{"en": "matscoped test field"})
	scope, _ := json.Marshal(map[string]string{"prefix": "crm", "local_name": "P1_is_identified_by"})
	if _, err := f.queries.WeaveCreateField(f.ctx, sqlcgen.WeaveCreateFieldParams{
		ID:            id,
		SystemName:    stringPtr("matscoped_field_" + strings.ReplaceAll(id, ".", "_")),
		UiName:        uiName,
		Description:   desc,
		Status:        "draft",
		ProjectID:     f.projectID,
		OntologyScope: scope,
	}); err != nil {
		f.t.Fatalf("create field %s: %v", id, err)
	}
}

// collection creates a bare collection row in the fixture's project.
func (f *matScopedFixture) collection(id string) {
	f.t.Helper()
	uiName, _ := json.Marshal(map[string]string{"en": "Matscoped Collection " + id})
	desc, _ := json.Marshal(map[string]string{"en": "matscoped test collection"})
	scope, _ := json.Marshal(map[string]string{"prefix": "crm", "local_name": "E67_Birth"})
	if _, err := f.queries.WeaveCreateCollection(f.ctx, sqlcgen.WeaveCreateCollectionParams{
		ID:            id,
		SystemName:    stringPtr("matscoped_collection_" + strings.ReplaceAll(id, ".", "_")),
		UiName:        uiName,
		Description:   desc,
		Status:        "draft",
		ProjectID:     f.projectID,
		OntologyScope: scope,
	}); err != nil {
		f.t.Fatalf("create collection %s: %v", id, err)
	}
}

// category creates a bare category row in the fixture's project.
func (f *matScopedFixture) category(id string) {
	f.t.Helper()
	uiName, _ := json.Marshal(map[string]string{"en": "Matscoped Category " + id})
	desc, _ := json.Marshal(map[string]string{"en": "matscoped test category"})
	if _, err := f.queries.WeaveCreateCategory(f.ctx, sqlcgen.WeaveCreateCategoryParams{
		ID:             id,
		SystemName:     stringPtr("matscoped_category_" + strings.ReplaceAll(id, ".", "_")),
		UiName:         uiName,
		Description:    desc,
		Status:         "draft",
		ProjectID:      f.projectID,
		CanonicalOrder: 0,
	}); err != nil {
		f.t.Fatalf("create category %s: %v", id, err)
	}
}

// categoryWithSemanticID creates a category row whose SemanticID differs
// from its ULID id, matching how real categories are created
// (category.Service.Create always mints a fresh ULID for ID separately from
// the project-scoped SemanticID) — see Fix 2's category-delete path bug.
func (f *matScopedFixture) categoryWithSemanticID(id, semanticID string) {
	f.t.Helper()
	uiName, _ := json.Marshal(map[string]string{"en": "Matscoped Category " + id})
	desc, _ := json.Marshal(map[string]string{"en": "matscoped test category"})
	if _, err := f.queries.WeaveCreateCategory(f.ctx, sqlcgen.WeaveCreateCategoryParams{
		ID:             id,
		SemanticID:     &semanticID,
		SystemName:     stringPtr("matscoped_category_" + strings.ReplaceAll(id, ".", "_")),
		UiName:         uiName,
		Description:    desc,
		Status:         "draft",
		ProjectID:      f.projectID,
		CanonicalOrder: 0,
	}); err != nil {
		f.t.Fatalf("create category %s (semantic id %s): %v", id, semanticID, err)
	}
}

// deleteCategory performs a real DB delete of a category row, mirroring what
// a delete in the UI does.
func (f *matScopedFixture) deleteCategory(id string) {
	f.t.Helper()
	if err := f.queries.WeaveDeleteCategory(f.ctx, id); err != nil {
		f.t.Fatalf("delete category %s: %v", id, err)
	}
}

// ontologyVersion creates a bare ontology + one active version, for tests
// that link a project-ontology-version. Returns the version id.
func (f *matScopedFixture) ontologyVersion(ontologyID, versionID string) string {
	f.t.Helper()
	if _, err := f.queries.WeaveCreateOntology(f.ctx, sqlcgen.WeaveCreateOntologyParams{
		ID:           ontologyID,
		Prefix:       "mso",
		Namespace:    "https://example.org/matscoped/",
		Name:         "Matscoped Ontology",
		Description:  []byte(`{"en":"Matscoped test ontology"}`),
		OntologyType: "base",
	}); err != nil {
		f.t.Fatalf("create ontology %s: %v", ontologyID, err)
	}
	if _, err := f.queries.WeaveCreateOntologyVersion(f.ctx, sqlcgen.WeaveCreateOntologyVersionParams{
		ID:                     versionID,
		OntologyID:             ontologyID,
		VersionString:          "1.0.0",
		IsActive:               true,
		CompatibleBaseVersions: []string{},
		VersionInfo:            []byte(`{"en":"1.0.0"}`),
		ImportedOntologies:     []string{},
		OntologyLabel:          []byte(`{"en":"Matscoped Ontology"}`),
		OntologyComment:        []byte(`{}`),
		OntologyMetadata:       []byte(`{}`),
		ClassCount:             1,
		PropertyCount:          1,
	}); err != nil {
		f.t.Fatalf("create ontology version %s: %v", versionID, err)
	}
	return versionID
}

// linkProjectOntologyVersion links versionID to the fixture's project via a
// weave_project_ontology_versions row, mirroring what
// projectontologyversion.Service.Create does to a real project.
func (f *matScopedFixture) linkProjectOntologyVersion(versionID string) {
	f.t.Helper()
	if _, err := f.queries.WeaveCreateProjectOntologyVersion(f.ctx, sqlcgen.WeaveCreateProjectOntologyVersionParams{
		ProjectID:         f.projectID,
		OntologyVersionID: versionID,
	}); err != nil {
		f.t.Fatalf("link project ontology version %s: %v", versionID, err)
	}
}

// namespaceBinding creates a project-scoped namespace binding row, mirroring
// what namespacebinding.Service.Create does. Returns the binding id.
func (f *matScopedFixture) namespaceBinding(id, prefix, namespace string) string {
	f.t.Helper()
	projectID := f.projectID
	if _, err := f.queries.WeaveCreateUserNamespaceBinding(f.ctx, sqlcgen.WeaveCreateUserNamespaceBindingParams{
		ID:        id,
		ProjectID: &projectID,
		Prefix:    prefix,
		Namespace: namespace,
		Weight:    0,
	}); err != nil {
		f.t.Fatalf("create namespace binding %s: %v", id, err)
	}
	return id
}

// placeFieldOnModel creates a model-owned override row placing fieldID on
// modelID, and returns the override row's id.
func (f *matScopedFixture) placeFieldOnModel(fieldID, modelID string) int64 {
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

// placeFieldOnCollection creates a collection-owned override row placing
// fieldID on collectionID, and returns the override row's id.
func (f *matScopedFixture) placeFieldOnCollection(fieldID, collectionID string) int64 {
	f.t.Helper()
	o, err := f.queries.WeaveCreateOverride(f.ctx, sqlcgen.WeaveCreateOverrideParams{
		FieldID:    fieldID,
		ProjectID:  f.projectID,
		EntityType: "collection",
		EntityID:   collectionID,
	})
	if err != nil {
		f.t.Fatalf("create override placing field %s on collection %s: %v", fieldID, collectionID, err)
	}
	return o.ID
}

// updateFieldUIName performs a real DB edit of a field's UI name, mirroring
// what a save in the UI does — used by the equivalence test's "field EDIT"
// case.
func (f *matScopedFixture) updateFieldUIName(id, newName string) {
	f.t.Helper()
	row, err := f.queries.WeaveGetFieldByID(f.ctx, id)
	if err != nil {
		f.t.Fatalf("get field %s: %v", id, err)
	}
	uiName, _ := json.Marshal(map[string]string{"en": newName})
	if _, err := f.queries.WeaveUpdateField(f.ctx, sqlcgen.WeaveUpdateFieldParams{
		ID:                id,
		UiName:            uiName,
		Description:       row.Description,
		SystemName:        row.SystemName,
		Status:            row.Status,
		OntologyScope:     row.OntologyScope,
		OntologyPath:      row.OntologyPath,
		PathElements:      row.PathElements,
		ExpectedValueType: row.ExpectedValueType,
		Examples:          row.Examples,
	}); err != nil {
		f.t.Fatalf("update field %s: %v", id, err)
	}
}

// deleteField performs a real DB delete of a field row, mirroring what a
// delete in the UI does.
func (f *matScopedFixture) deleteField(id string) {
	f.t.Helper()
	if err := f.queries.WeaveDeleteField(f.ctx, id); err != nil {
		f.t.Fatalf("delete field %s: %v", id, err)
	}
}

// updateCollectionUIName performs a real DB edit of a collection's UI name,
// mirroring what a save in the UI does — used by the equivalence test's
// "collection EDIT" case.
func (f *matScopedFixture) updateCollectionUIName(id, newName string) {
	f.t.Helper()
	row, err := f.queries.WeaveGetCollectionByID(f.ctx, id)
	if err != nil {
		f.t.Fatalf("get collection %s: %v", id, err)
	}
	uiName, _ := json.Marshal(map[string]string{"en": newName})
	if _, err := f.queries.WeaveUpdateCollection(f.ctx, sqlcgen.WeaveUpdateCollectionParams{
		ID:                       id,
		UiName:                   uiName,
		Description:              row.Description,
		SystemName:               row.SystemName,
		Status:                   row.Status,
		OntologyScope:            row.OntologyScope,
		CollectionNumber:         row.CollectionNumber,
		CanonicalCollectionOrder: row.CanonicalCollectionOrder,
		DefaultCategoryID:        row.DefaultCategoryID,
	}); err != nil {
		f.t.Fatalf("update collection %s: %v", id, err)
	}
}

// deleteCollection performs a real DB delete of a collection row, mirroring
// what a delete in the UI does.
func (f *matScopedFixture) deleteCollection(id string) {
	f.t.Helper()
	if err := f.queries.WeaveDeleteCollection(f.ctx, id); err != nil {
		f.t.Fatalf("delete collection %s: %v", id, err)
	}
}

// updateCategoryUIName performs a real DB edit of a category's UI name,
// mirroring what a save in the UI does — used by the equivalence test's
// "category EDIT" case.
func (f *matScopedFixture) updateCategoryUIName(id, newName string) {
	f.t.Helper()
	row, err := f.queries.WeaveGetCategoryByID(f.ctx, id)
	if err != nil {
		f.t.Fatalf("get category %s: %v", id, err)
	}
	uiName, _ := json.Marshal(map[string]string{"en": newName})
	if _, err := f.queries.WeaveUpdateCategory(f.ctx, sqlcgen.WeaveUpdateCategoryParams{
		ID:             id,
		UiName:         uiName,
		Description:    row.Description,
		SystemName:     row.SystemName,
		Status:         row.Status,
		CanonicalOrder: row.CanonicalOrder,
	}); err != nil {
		f.t.Fatalf("update category %s: %v", id, err)
	}
}

// updateModelUIName performs a real DB edit of a model's UI name, mirroring
// what a save in the UI does — used by the equivalence test's "model EDIT"
// case.
func (f *matScopedFixture) updateModelUIName(id, newName string) {
	f.t.Helper()
	row, err := f.queries.WeaveGetModelByID(f.ctx, id)
	if err != nil {
		f.t.Fatalf("get model %s: %v", id, err)
	}
	uiName, _ := json.Marshal(map[string]string{"en": newName})
	if _, err := f.queries.WeaveUpdateModel(f.ctx, sqlcgen.WeaveUpdateModelParams{
		ID:            id,
		UiName:        uiName,
		Description:   row.Description,
		SystemName:    row.SystemName,
		Status:        row.Status,
		OntologyScope: row.OntologyScope,
		ModelType:     row.ModelType,
	}); err != nil {
		f.t.Fatalf("update model %s: %v", id, err)
	}
}

// deleteModel performs a real DB delete of a model row, mirroring what a
// delete in the UI does.
func (f *matScopedFixture) deleteModel(id string) {
	f.t.Helper()
	if err := f.queries.WeaveDeleteModel(f.ctx, id); err != nil {
		f.t.Fatalf("delete model %s: %v", id, err)
	}
}

// baseOverride creates a base (entity_type "") override row for fieldID and
// returns the override row's id.
func (f *matScopedFixture) baseOverride(fieldID string) int64 {
	f.t.Helper()
	o, err := f.queries.WeaveCreateOverride(f.ctx, sqlcgen.WeaveCreateOverrideParams{
		FieldID:    fieldID,
		ProjectID:  f.projectID,
		EntityType: "",
		EntityID:   "",
	})
	if err != nil {
		f.t.Fatalf("create base override for field %s: %v", fieldID, err)
	}
	return o.ID
}

// changeSet creates a change_set with commitMessage and appends one
// change_log entry per entry spec (see closure_test.go's changeLogSpec/entry
// helpers), returning the change_set row ready to pass to closure()/
// processChangeSet().
func (f *matScopedFixture) changeSet(commitMessage string, entries ...changeLogSpec) sqlcgen.WeaveChangeSet {
	f.t.Helper()
	cs, err := f.queries.WeaveCreateChangeSet(f.ctx, sqlcgen.WeaveCreateChangeSetParams{
		ProjectID:     f.projectID,
		ActorName:     "matscoped-test",
		ActorEmail:    "matscoped-test@example.org",
		CommitMessage: commitMessage,
	})
	if err != nil {
		f.t.Fatalf("create change set: %v", err)
	}
	for _, e := range entries {
		if _, err := f.queries.WeaveCreateChangeLogEntry(f.ctx, sqlcgen.WeaveCreateChangeLogEntryParams{
			ChangeSetID:     cs.ID,
			EntityType:      e.entityType,
			EntityID:        e.entityID,
			Operation:       e.operation,
			ProjectID:       f.projectID,
			FilePath:        "irrelevant.yaml",
			Payload:         []byte(`{}`),
			PreviousPayload: e.previousPayload,
		}); err != nil {
			f.t.Fatalf("create change log entry (%s %s %s): %v", e.operation, e.entityType, e.entityID, err)
		}
	}
	return cs
}

// copyMatTree recursively copies src into dst, preserving relative
// structure. Used to seed a scoped-rewrite baseline from a prior full
// materialization.
func copyMatTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy tree %s -> %s: %v", src, dst, err)
	}
}

// diffTrees walks both dirs (excluding .git) and returns a cmp.Diff of
// relative-path -> file-bytes maps, empty when the trees match exactly.
func diffTrees(t *testing.T, dirA, dirB string) string {
	t.Helper()
	return cmp.Diff(treeContents(t, dirA), treeContents(t, dirB))
}

func treeContents(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk tree %s: %v", dir, err)
	}
	return out
}

// commitTouchesPath reports whether sha's commit changed any file under
// relDir (a repo-relative directory, no trailing slash).
func commitTouchesPath(t *testing.T, repoDir, sha, relDir string) bool {
	t.Helper()
	out, err := exec.Command("git", "-C", repoDir, "diff-tree", "--no-commit-id", "--name-only", "-r", sha).Output()
	if err != nil {
		t.Fatalf("git diff-tree %s: %v", sha, err)
	}
	prefix := relDir + "/"
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == relDir || strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// commitFullMessage returns sha's full commit message (subject + body),
// trailing newline stripped.
func commitFullMessage(t *testing.T, repoDir, sha string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repoDir, "log", "-1", "--format=%B", sha).Output()
	if err != nil {
		t.Fatalf("git log -1 %s: %v", sha, err)
	}
	return strings.TrimRight(string(out), "\n")
}

// TestScopedEqualsFullRebuild_ModelEdit is the equivalence (drift guard)
// gate: for a model UI-name edit, the tree scopedRewrite() produces starting
// from the pre-edit baseline must be byte-identical to a full rebuild at the
// post-edit DB state. Any drift here is a real closure()/scopedRewrite() bug,
// not a test-tuning problem.
func TestScopedEqualsFullRebuild_ModelEdit(t *testing.T) {
	f := setupMatScopedFixture(t)
	f.model("MATSCOPED_MODEL_EDIT")

	// S1: full rebuild before the edit — this is the scoped path's baseline.
	full1 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full1, f.projectID); err != nil {
		t.Fatalf("full rebuild at S1: %v", err)
	}

	// S1 -> S2: a real DB edit, recorded as a change_set.
	f.updateModelUIName("MATSCOPED_MODEL_EDIT", "Edited Name")
	cs := f.changeSet("edit model", entry("model", "MATSCOPED_MODEL_EDIT", "update"))

	// S2: full rebuild — the ground truth the scoped rewrite must match.
	full2 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full2, f.projectID); err != nil {
		t.Fatalf("full rebuild at S2: %v", err)
	}

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}

	scoped := t.TempDir()
	copyMatTree(t, full1, scoped)
	if err := f.mat.scopedRewrite(f.ctx, scoped, f.projectID, refs); err != nil {
		t.Fatalf("scopedRewrite: %v", err)
	}

	if diff := diffTrees(t, full2, scoped); diff != "" {
		t.Fatalf("scoped tree drifted from full rebuild after a model edit:\n%s", diff)
	}
}

// TestScopedEqualsFullRebuild_ModelDelete is the delete-case sibling of
// TestScopedEqualsFullRebuild_ModelEdit: scopedRewrite() must remove the
// deleted model's directory and land on exactly the same tree a full rebuild
// at the post-delete DB state produces.
func TestScopedEqualsFullRebuild_ModelDelete(t *testing.T) {
	f := setupMatScopedFixture(t)
	f.model("MATSCOPED_MODEL_DEL")

	full1 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full1, f.projectID); err != nil {
		t.Fatalf("full rebuild at S1: %v", err)
	}

	f.deleteModel("MATSCOPED_MODEL_DEL")
	cs := f.changeSet("delete model", entry("model", "MATSCOPED_MODEL_DEL", "delete"))

	full2 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full2, f.projectID); err != nil {
		t.Fatalf("full rebuild at S2: %v", err)
	}

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}

	scoped := t.TempDir()
	copyMatTree(t, full1, scoped)
	if err := f.mat.scopedRewrite(f.ctx, scoped, f.projectID, refs); err != nil {
		t.Fatalf("scopedRewrite: %v", err)
	}

	if diff := diffTrees(t, full2, scoped); diff != "" {
		t.Fatalf("scoped tree drifted from full rebuild after a model delete:\n%s", diff)
	}
}

// TestScopedEqualsFullRebuild_FieldEdit is the field-edit sibling of
// TestScopedEqualsFullRebuild_ModelEdit: the field is placed on a model via
// a model-owned override, so closure()'s field->placing-owner fan-out must
// rewrite both the field's own file and the placing model's overrides/
// subtree. scopedRewrite() starting from the pre-edit baseline must land on
// exactly the tree a full rebuild at the post-edit DB state produces.
func TestScopedEqualsFullRebuild_FieldEdit(t *testing.T) {
	f := setupMatScopedFixture(t)
	f.field("MATSCOPED_FIELD_EDIT")
	f.model("MATSCOPED_MODEL_PLACES_EDIT")
	f.placeFieldOnModel("MATSCOPED_FIELD_EDIT", "MATSCOPED_MODEL_PLACES_EDIT")

	full1 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full1, f.projectID); err != nil {
		t.Fatalf("full rebuild at S1: %v", err)
	}

	f.updateFieldUIName("MATSCOPED_FIELD_EDIT", "Edited Field Name")
	cs := f.changeSet("edit field", entry("field", "MATSCOPED_FIELD_EDIT", "update"))

	full2 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full2, f.projectID); err != nil {
		t.Fatalf("full rebuild at S2: %v", err)
	}

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}

	scoped := t.TempDir()
	copyMatTree(t, full1, scoped)
	if err := f.mat.scopedRewrite(f.ctx, scoped, f.projectID, refs); err != nil {
		t.Fatalf("scopedRewrite: %v", err)
	}

	if diff := diffTrees(t, full2, scoped); diff != "" {
		t.Fatalf("scoped tree drifted from full rebuild after a field edit (placed on a model):\n%s", diff)
	}
}

// TestScopedEqualsFullRebuild_FieldDelete is the delete-case sibling of
// TestScopedEqualsFullRebuild_FieldEdit: an unplaced field (no non-base
// placements, so the delete is allowed) is deleted; scopedRewrite() must
// remove the field's directory and land on exactly the same tree a full
// rebuild at the post-delete DB state produces.
func TestScopedEqualsFullRebuild_FieldDelete(t *testing.T) {
	f := setupMatScopedFixture(t)
	f.field("MATSCOPED_FIELD_DEL")

	full1 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full1, f.projectID); err != nil {
		t.Fatalf("full rebuild at S1: %v", err)
	}

	f.deleteField("MATSCOPED_FIELD_DEL")
	cs := f.changeSet("delete field", entry("field", "MATSCOPED_FIELD_DEL", "delete"))

	full2 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full2, f.projectID); err != nil {
		t.Fatalf("full rebuild at S2: %v", err)
	}

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}

	scoped := t.TempDir()
	copyMatTree(t, full1, scoped)
	if err := f.mat.scopedRewrite(f.ctx, scoped, f.projectID, refs); err != nil {
		t.Fatalf("scopedRewrite: %v", err)
	}

	if diff := diffTrees(t, full2, scoped); diff != "" {
		t.Fatalf("scoped tree drifted from full rebuild after a field delete:\n%s", diff)
	}
}

// TestScopedEqualsFullRebuild_CollectionEdit mirrors the model-edit
// equivalence case for collections. The collection has a field placement, so
// its overrides/ subtree is materialized and must regenerate identically
// under scopedRewrite().
func TestScopedEqualsFullRebuild_CollectionEdit(t *testing.T) {
	f := setupMatScopedFixture(t)
	f.field("MATSCOPED_FIELD_FOR_COLLECTION_EDIT")
	f.collection("MATSCOPED_COLLECTION_EDIT")
	f.placeFieldOnCollection("MATSCOPED_FIELD_FOR_COLLECTION_EDIT", "MATSCOPED_COLLECTION_EDIT")

	full1 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full1, f.projectID); err != nil {
		t.Fatalf("full rebuild at S1: %v", err)
	}

	f.updateCollectionUIName("MATSCOPED_COLLECTION_EDIT", "Edited Collection Name")
	cs := f.changeSet("edit collection", entry("collection", "MATSCOPED_COLLECTION_EDIT", "update"))

	full2 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full2, f.projectID); err != nil {
		t.Fatalf("full rebuild at S2: %v", err)
	}

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}

	scoped := t.TempDir()
	copyMatTree(t, full1, scoped)
	if err := f.mat.scopedRewrite(f.ctx, scoped, f.projectID, refs); err != nil {
		t.Fatalf("scopedRewrite: %v", err)
	}

	if diff := diffTrees(t, full2, scoped); diff != "" {
		t.Fatalf("scoped tree drifted from full rebuild after a collection edit:\n%s", diff)
	}
}

// TestScopedEqualsFullRebuild_CollectionDelete is the delete-case sibling of
// TestScopedEqualsFullRebuild_CollectionEdit: scopedRewrite() must remove the
// deleted collection's directory and land on exactly the same tree a full
// rebuild at the post-delete DB state produces.
func TestScopedEqualsFullRebuild_CollectionDelete(t *testing.T) {
	f := setupMatScopedFixture(t)
	f.collection("MATSCOPED_COLLECTION_DEL")

	full1 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full1, f.projectID); err != nil {
		t.Fatalf("full rebuild at S1: %v", err)
	}

	f.deleteCollection("MATSCOPED_COLLECTION_DEL")
	cs := f.changeSet("delete collection", entry("collection", "MATSCOPED_COLLECTION_DEL", "delete"))

	full2 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full2, f.projectID); err != nil {
		t.Fatalf("full rebuild at S2: %v", err)
	}

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}

	scoped := t.TempDir()
	copyMatTree(t, full1, scoped)
	if err := f.mat.scopedRewrite(f.ctx, scoped, f.projectID, refs); err != nil {
		t.Fatalf("scopedRewrite: %v", err)
	}

	if diff := diffTrees(t, full2, scoped); diff != "" {
		t.Fatalf("scoped tree drifted from full rebuild after a collection delete:\n%s", diff)
	}
}

// TestScopedEqualsFullRebuild_CategoryEdit mirrors the model-edit
// equivalence case for categories. Categories own a single file (no
// overrides/ subtree), so this exercises the simplest closure()/
// scopedRewrite() path.
func TestScopedEqualsFullRebuild_CategoryEdit(t *testing.T) {
	f := setupMatScopedFixture(t)
	f.category("MATSCOPED_CATEGORY_EDIT")

	full1 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full1, f.projectID); err != nil {
		t.Fatalf("full rebuild at S1: %v", err)
	}

	f.updateCategoryUIName("MATSCOPED_CATEGORY_EDIT", "Edited Category Name")
	cs := f.changeSet("edit category", entry("category", "MATSCOPED_CATEGORY_EDIT", "update"))

	full2 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full2, f.projectID); err != nil {
		t.Fatalf("full rebuild at S2: %v", err)
	}

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}

	scoped := t.TempDir()
	copyMatTree(t, full1, scoped)
	if err := f.mat.scopedRewrite(f.ctx, scoped, f.projectID, refs); err != nil {
		t.Fatalf("scopedRewrite: %v", err)
	}

	if diff := diffTrees(t, full2, scoped); diff != "" {
		t.Fatalf("scoped tree drifted from full rebuild after a category edit:\n%s", diff)
	}
}

// TestNoCrossEntityStealing is the GRPM.2 race regression: two change_sets
// touching DIFFERENT entities, drained in the order that caused the bug
// (the unrelated one first). Before the scoped-rewrite swap, processChangeSet
// full-rebuilt the tree from CURRENT DB state on every change_set — so cs1's
// rebuild, running after BOTH DB mutations had already landed, silently
// absorbed cs2's deletion and committed it under cs1's unrelated message,
// leaving cs2 a no-op with no commit at all. After the swap, each change_set
// commits only the files its own closure() touches.
func TestNoCrossEntityStealing(t *testing.T) {
	f := setupMatScopedFixture(t)
	f.model("GRPM.2")
	f.field("MATSCOPED_FIELD_OTHER")

	if err := f.mat.InitProject(f.ctx, f.projectID); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	repoDir := f.workDir()
	if !fileExists(filepath.Join(repoDir, "models", "GRPM.2", "model.yaml")) {
		t.Fatalf("setup: expected models/GRPM.2/model.yaml to exist after InitProject")
	}

	// cs1: an unrelated implicit override edit — a brand-new base override on
	// a field that has nothing to do with GRPM.2.
	overrideID := f.baseOverride("MATSCOPED_FIELD_OTHER")
	cs1 := f.changeSet("implicit change", entry("override", fmt.Sprintf("%d", overrideID), "update"))

	// cs2: the real delete, recorded after cs1 — both DB mutations are
	// committed before either change_set is drained, matching production
	// where the poller runs asynchronously after both writes have landed.
	f.deleteModel("GRPM.2")
	cs2 := f.changeSet("DELETE /projects/GRP/models/GRPM.2", entry("model", "GRPM.2", "delete"))

	// Drain cs1 first (the race order that caused the bug), then cs2.
	if err := f.mat.processChangeSet(f.ctx, cs1); err != nil {
		t.Fatalf("process cs1: %v", err)
	}
	if err := f.mat.processChangeSet(f.ctx, cs2); err != nil {
		t.Fatalf("process cs2: %v", err)
	}

	cs2After, err := f.queries.WeaveGetChangeSet(f.ctx, cs2.ID)
	if err != nil {
		t.Fatalf("get cs2: %v", err)
	}
	if cs2After.GitCommitSha == nil || *cs2After.GitCommitSha == "" {
		t.Fatalf("cs2 (real delete) must have committed, got noop")
	}
	sha2 := *cs2After.GitCommitSha

	if !commitTouchesPath(t, repoDir, sha2, "models/GRPM.2") {
		t.Fatalf("cs2's commit must remove models/GRPM.2")
	}
	wantMsg := fmt.Sprintf("DELETE /projects/GRP/models/GRPM.2\n\nChange-Set: %d", cs2.ID)
	if got := commitFullMessage(t, repoDir, sha2); got != wantMsg {
		t.Fatalf("cs2 commit message = %q, want %q", got, wantMsg)
	}

	cs1After, err := f.queries.WeaveGetChangeSet(f.ctx, cs1.ID)
	if err != nil {
		t.Fatalf("get cs1: %v", err)
	}
	if cs1After.GitCommitSha != nil && *cs1After.GitCommitSha != "" {
		if commitTouchesPath(t, repoDir, *cs1After.GitCommitSha, "models/GRPM.2") {
			t.Fatalf("cs1's commit must not touch models/GRPM.2 (cross-entity stealing)")
		}
	}
}

// TestScopedEqualsFullRebuild_CategoryDelete is the final-review Fix 2
// regression: categories are the one entity where ID (a ULID) != SemanticID,
// but files are written at categories/<SemanticID>.yaml while a delete
// change_log entry's EntityID is the ULID. Before the fix, closure() used the
// raw entity id for deletePathsFor, computing categories/<ULID>.yaml — a path
// that was never written, so os.RemoveAll no-opped and the real file lingered
// as noop drift. This test fails before the fix (the stale category file
// survives in `scoped`) and passes after (scoped == full2, both missing the
// deleted category's file).
func TestScopedEqualsFullRebuild_CategoryDelete(t *testing.T) {
	f := setupMatScopedFixture(t)
	const categoryULID = "MATSCOPED_CATEGORY_DEL_ULID"
	const categorySemanticID = "MATSCOPED_PROJECT.CAT.99"
	f.categoryWithSemanticID(categoryULID, categorySemanticID)

	full1 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full1, f.projectID); err != nil {
		t.Fatalf("full rebuild at S1: %v", err)
	}
	if !fileExists(filepath.Join(full1, "categories", categorySemanticID+".yaml")) {
		t.Fatalf("setup: expected categories/%s.yaml to exist before delete", categorySemanticID)
	}

	// The change_log "delete" entry's EntityID is the category's ULID (not
	// its SemanticID) — matching production, where category.Service.Delete
	// logs the row's raw id. previous_payload carries the SemanticID closure()
	// must resolve to find the real on-disk path.
	prevPayload, err := json.Marshal(map[string]string{"semantic_id": categorySemanticID})
	if err != nil {
		t.Fatalf("marshal previous payload: %v", err)
	}
	f.deleteCategory(categoryULID)
	cs := f.changeSet("delete category", entryWithPreviousPayload("category", categoryULID, "delete", prevPayload))

	full2 := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, full2, f.projectID); err != nil {
		t.Fatalf("full rebuild at S2: %v", err)
	}
	if fileExists(filepath.Join(full2, "categories", categorySemanticID+".yaml")) {
		t.Fatalf("setup: expected categories/%s.yaml to be gone after delete", categorySemanticID)
	}

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}

	scoped := t.TempDir()
	copyMatTree(t, full1, scoped)
	if err := f.mat.scopedRewrite(f.ctx, scoped, f.projectID, refs); err != nil {
		t.Fatalf("scopedRewrite: %v", err)
	}

	if diff := diffTrees(t, full2, scoped); diff != "" {
		t.Fatalf("scoped tree drifted from full rebuild after a category delete (stale file not removed):\n%s", diff)
	}
}

// TestProcessChangeSet_ConcurrentlyDeletedRefDoesNotWedgeChangeSet is the
// final-review Fix 3 regression: a change_set's closure resolved to a
// non-delete ref (model "update") for an entity that was deleted from the DB
// before scopedRewrite() ran — an update-then-delete race between two
// change_sets. Before the fix, writeOneModel's pgx.ErrNoRows propagated as a
// hard error, so processChangeSet returned an error, the change_set was never
// marked processed (materialized_error recorded instead), and the poller
// would retry it forever. After the fix, the ref is skipped and the
// change_set completes (noop: the model file was never materialized in this
// tree, so there's nothing to write or remove).
func TestProcessChangeSet_ConcurrentlyDeletedRefDoesNotWedgeChangeSet(t *testing.T) {
	f := setupMatScopedFixture(t)
	f.model("MATSCOPED_MODEL_RACE")

	if err := f.mat.InitProject(f.ctx, f.projectID); err != nil {
		t.Fatalf("InitProject: %v", err)
	}

	// The change_set records an "update" for the model — but by the time
	// processChangeSet drains it, the model row is gone (deleted by another,
	// already-processed change_set; see closure()'s dedup-asymmetry comment
	// for how this can happen even from a single poll batch).
	cs := f.changeSet("edit model", entry("model", "MATSCOPED_MODEL_RACE", "update"))
	f.deleteModel("MATSCOPED_MODEL_RACE")

	if err := f.mat.processChangeSet(f.ctx, cs); err != nil {
		t.Fatalf("processChangeSet must not fail on a concurrently-deleted non-delete ref, got: %v", err)
	}

	csAfter, err := f.queries.WeaveGetChangeSet(f.ctx, cs.ID)
	if err != nil {
		t.Fatalf("get change set: %v", err)
	}
	if !csAfter.ProcessedAt.Valid {
		t.Fatalf("change set must be marked processed, not left for the poller to retry forever")
	}
	if csAfter.MaterializedError != nil {
		t.Fatalf("change set must not record a materialization error, got %q", *csAfter.MaterializedError)
	}
	if csAfter.MaterializedOutcome == nil || *csAfter.MaterializedOutcome != "noop" {
		got := "<nil>"
		if csAfter.MaterializedOutcome != nil {
			got = *csAfter.MaterializedOutcome
		}
		t.Fatalf("expected outcome %q (nothing to write once the ref is skipped), got %q", "noop", got)
	}
}

// TestProcessChangeSet_ProjectOntologyVersionMaterializesPletkaMod is the
// final-review Fix 1 regression (project-ontology-version half): before the
// fix, closure() had no case for a "project-ontology-version" change_log
// entity type, so the change_set drained as a noop with no commit — the DB
// change (a linked ontology version) went unmaterialized until the nightly
// reconcile sweep, under system attribution instead of the actor's. After the
// fix, such a change_set commits and pletka.mod reflects the new requirement.
func TestProcessChangeSet_ProjectOntologyVersionMaterializesPletkaMod(t *testing.T) {
	f := setupMatScopedFixture(t)

	if err := f.mat.InitProject(f.ctx, f.projectID); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	pletkaModPath := filepath.Join(f.workDir(), "pletka.mod")
	before, err := os.ReadFile(pletkaModPath)
	if err != nil {
		t.Fatalf("read pletka.mod before: %v", err)
	}
	if strings.Contains(string(before), "MATSCOPED_ONTOLOGY") {
		t.Fatalf("setup: pletka.mod should not reference the ontology before it is linked")
	}

	versionID := f.ontologyVersion("MATSCOPED_ONTOLOGY", "MATSCOPED_ONTOLOGY_V1")
	f.linkProjectOntologyVersion(versionID)
	cs := f.changeSet("link ontology version", entry("project-ontology-version", versionID, "create"))

	if err := f.mat.processChangeSet(f.ctx, cs); err != nil {
		t.Fatalf("processChangeSet: %v", err)
	}

	csAfter, err := f.queries.WeaveGetChangeSet(f.ctx, cs.ID)
	if err != nil {
		t.Fatalf("get change set: %v", err)
	}
	if csAfter.GitCommitSha == nil || *csAfter.GitCommitSha == "" {
		t.Fatalf("change set must commit (not drain as a noop) once closure() covers project-ontology-version entries")
	}

	after, err := os.ReadFile(pletkaModPath)
	if err != nil {
		t.Fatalf("read pletka.mod after: %v", err)
	}
	if !strings.Contains(string(after), "MATSCOPED_ONTOLOGY") {
		t.Fatalf("pletka.mod must reflect the newly linked ontology version, got:\n%s", after)
	}
}

// TestProcessChangeSet_NamespaceBindingMaterializesProjectManifest is the
// final-review Fix 1 regression (namespace_binding half): sibling of
// TestProcessChangeSet_ProjectOntologyVersionMaterializesPletkaMod, covering
// the other project-level "draft" change_log entity type closure() was
// missing a case for. namespace_binding changes materialize into
// project.yaml's namespaces.effective_bindings.
func TestProcessChangeSet_NamespaceBindingMaterializesProjectManifest(t *testing.T) {
	f := setupMatScopedFixture(t)

	if err := f.mat.InitProject(f.ctx, f.projectID); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	projectManifestPath := filepath.Join(f.workDir(), "project.yaml")
	before, err := os.ReadFile(projectManifestPath)
	if err != nil {
		t.Fatalf("read project.yaml before: %v", err)
	}
	if strings.Contains(string(before), "matscoped-ext") {
		t.Fatalf("setup: project.yaml should not reference the binding's namespace before it is created")
	}

	bindingID := f.namespaceBinding("MATSCOPED_NS_BINDING", "matscoped-ext", "https://example.org/matscoped-ext/")
	cs := f.changeSet("add namespace binding", entry("namespace_binding", bindingID, "create"))

	if err := f.mat.processChangeSet(f.ctx, cs); err != nil {
		t.Fatalf("processChangeSet: %v", err)
	}

	csAfter, err := f.queries.WeaveGetChangeSet(f.ctx, cs.ID)
	if err != nil {
		t.Fatalf("get change set: %v", err)
	}
	if csAfter.GitCommitSha == nil || *csAfter.GitCommitSha == "" {
		t.Fatalf("change set must commit (not drain as a noop) once closure() covers namespace_binding entries")
	}

	after, err := os.ReadFile(projectManifestPath)
	if err != nil {
		t.Fatalf("read project.yaml after: %v", err)
	}
	if !strings.Contains(string(after), "matscoped-ext") {
		t.Fatalf("project.yaml must reflect the newly created namespace binding, got:\n%s", after)
	}
}
