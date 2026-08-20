//go:build integration

package gitmaterializer

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// sortRefs makes ScopedRef slice comparisons order-independent — closure()
// makes no ordering promise beyond "change_log entries in id order", and the
// placingOwners fan-out has no defined order at all.
var sortRefs = cmpopts.SortSlices(func(a, b ScopedRef) bool {
	if a.EntityType != b.EntityType {
		return a.EntityType < b.EntityType
	}
	return a.EntityID < b.EntityID
})

// closureOwnerActorID/closureProjectID are the fixed ids seeded by
// setupClosureFixture. Deleted in t.Cleanup at the end of each test (tests
// in this file run sequentially, not in parallel), so reusing fixed ids
// across tests in this file is safe — matches the convention established by
// scoped_writer_test.go's setupOneModel.
const (
	closureOwnerActorID = "CLOSURE_OWNER"
	closureProjectID    = "CLOSURE_PROJECT"
)

// closureFixture seeds one project (with owner actor) in the per-package
// test clone and provides helpers to build the change_set/change_log/
// override rows closure() reads.
type closureFixture struct {
	t         *testing.T
	ctx       context.Context
	pool      *pgxpool.Pool
	queries   *sqlcgen.Queries
	mat       *Materializer
	projectID string
}

func setupClosureFixture(t *testing.T) *closureFixture {
	t.Helper()
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_change_log WHERE project_id = $1`, closureProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_change_set WHERE project_id = $1`, closureProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, closureProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models WHERE project_id = $1`, closureProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_collections WHERE project_id = $1`, closureProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id = $1`, closureProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, closureProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, closureOwnerActorID)
	})

	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          closureOwnerActorID,
		Type:        "organization",
		DisplayName: "Closure Owner",
		Slug:        "closure-owner",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create owner actor: %v", err)
	}

	uiName, _ := json.Marshal(map[string]string{"en": "Closure Project"})
	desc, _ := json.Marshal(map[string]string{"en": "Project for closure() tests"})
	if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID:          closureProjectID,
		UiName:      uiName,
		Description: desc,
		Status:      "draft",
		OwnerID:     closureOwnerActorID,
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	return &closureFixture{
		t:         t,
		ctx:       ctx,
		pool:      pool,
		queries:   queries,
		mat:       NewMaterializer(pool, t.TempDir(), nil),
		projectID: closureProjectID,
	}
}

// changeSet creates a change_set and appends one change_log entry per entry
// spec, returning the change_set row ready to pass to closure().
func (f *closureFixture) changeSet(entries ...changeLogSpec) sqlcgen.WeaveChangeSet {
	f.t.Helper()
	cs, err := f.queries.WeaveCreateChangeSet(f.ctx, sqlcgen.WeaveCreateChangeSetParams{
		ProjectID:     f.projectID,
		ActorName:     "closure-test",
		ActorEmail:    "closure-test@example.org",
		CommitMessage: "closure test change set",
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

type changeLogSpec struct {
	entityType      string
	entityID        string
	operation       string
	previousPayload []byte
}

func entry(entityType, entityID, operation string) changeLogSpec {
	return changeLogSpec{entityType: entityType, entityID: entityID, operation: operation}
}

// entryWithPreviousPayload is entry() plus a previous_payload blob, used by
// tests that exercise closure()'s category-delete SemanticID resolution
// (categorySemanticIDFromPayload reads previous_payload, not payload).
func entryWithPreviousPayload(entityType, entityID, operation string, previousPayload []byte) changeLogSpec {
	return changeLogSpec{entityType: entityType, entityID: entityID, operation: operation, previousPayload: previousPayload}
}

// model creates a bare model row in the fixture's project.
func (f *closureFixture) model(id string) {
	f.t.Helper()
	uiName, _ := json.Marshal(map[string]string{"en": "Closure Model " + id})
	desc, _ := json.Marshal(map[string]string{"en": "closure test model"})
	scope, _ := json.Marshal(map[string]string{"prefix": "crm", "local_name": "E21_Person"})
	if _, err := f.queries.WeaveCreateModel(f.ctx, sqlcgen.WeaveCreateModelParams{
		ID:            id,
		SystemName:    stringPtr("closure_model_" + id),
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

// collection creates a bare collection row in the fixture's project.
func (f *closureFixture) collection(id string) {
	f.t.Helper()
	uiName, _ := json.Marshal(map[string]string{"en": "Closure Collection " + id})
	desc, _ := json.Marshal(map[string]string{"en": "closure test collection"})
	scope, _ := json.Marshal(map[string]string{"prefix": "crm", "local_name": "E67_Birth"})
	if _, err := f.queries.WeaveCreateCollection(f.ctx, sqlcgen.WeaveCreateCollectionParams{
		ID:            id,
		SystemName:    stringPtr("closure_collection_" + id),
		UiName:        uiName,
		Description:   desc,
		Status:        "draft",
		ProjectID:     f.projectID,
		OntologyScope: scope,
	}); err != nil {
		f.t.Fatalf("create collection %s: %v", id, err)
	}
}

// field creates a bare field row in the fixture's project.
func (f *closureFixture) field(id string) {
	f.t.Helper()
	uiName, _ := json.Marshal(map[string]string{"en": "Closure Field " + id})
	desc, _ := json.Marshal(map[string]string{"en": "closure test field"})
	scope, _ := json.Marshal(map[string]string{"prefix": "crm", "local_name": "P1_is_identified_by"})
	if _, err := f.queries.WeaveCreateField(f.ctx, sqlcgen.WeaveCreateFieldParams{
		ID:            id,
		SystemName:    stringPtr("closure_field_" + id),
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
func (f *closureFixture) placeFieldOnModel(fieldID, modelID string) int64 {
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
func (f *closureFixture) placeFieldOnCollection(fieldID, collectionID string) int64 {
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

// baseOverride creates a base (entity_type "") override row for fieldID and
// returns the override row's id.
func (f *closureFixture) baseOverride(fieldID string) int64 {
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

func TestClosureModelDelete(t *testing.T) {
	f := setupClosureFixture(t)
	f.model("CLOSURE_MODEL_DEL")
	cs := f.changeSet(entry("model", "CLOSURE_MODEL_DEL", "delete"))

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []ScopedRef{{EntityType: "model", EntityID: "CLOSURE_MODEL_DEL", Delete: true}}
	if diff := cmp.Diff(want, refs, sortRefs); diff != "" {
		t.Fatalf("closure mismatch (-want +got):\n%s", diff)
	}
}

func TestClosureFieldUpdateIncludesPlacingModels(t *testing.T) {
	f := setupClosureFixture(t)
	f.field("CLOSURE_FIELD_UPD")
	f.model("CLOSURE_MODEL_PLACES_UPD")
	f.placeFieldOnModel("CLOSURE_FIELD_UPD", "CLOSURE_MODEL_PLACES_UPD")
	cs := f.changeSet(entry("field", "CLOSURE_FIELD_UPD", "update"))

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []ScopedRef{
		{EntityType: "field", EntityID: "CLOSURE_FIELD_UPD"},
		{EntityType: "model", EntityID: "CLOSURE_MODEL_PLACES_UPD"},
	}
	if diff := cmp.Diff(want, refs, sortRefs); diff != "" {
		t.Fatalf("closure mismatch (-want +got):\n%s", diff)
	}
}

func TestClosureFieldUpdateIncludesPlacingCollections(t *testing.T) {
	f := setupClosureFixture(t)
	f.field("CLOSURE_FIELD_UPD_C")
	f.collection("CLOSURE_COLLECTION_PLACES_UPD")
	f.placeFieldOnCollection("CLOSURE_FIELD_UPD_C", "CLOSURE_COLLECTION_PLACES_UPD")
	cs := f.changeSet(entry("field", "CLOSURE_FIELD_UPD_C", "update"))

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []ScopedRef{
		{EntityType: "field", EntityID: "CLOSURE_FIELD_UPD_C"},
		{EntityType: "collection", EntityID: "CLOSURE_COLLECTION_PLACES_UPD"},
	}
	if diff := cmp.Diff(want, refs, sortRefs); diff != "" {
		t.Fatalf("closure mismatch (-want +got):\n%s", diff)
	}
}

// TestClosureFieldDeleteStillRewritesPlacingModels covers the brief's edge
// case explicitly: a field DELETE still emits the placing model as a
// non-delete rewrite, even though the placement override row is gone by the
// time closure() runs (the field row and its overrides are removed in the
// same transaction that wrote the change_log "delete" entry, so we simulate
// that here — the field is never actually placed on the model in the DB,
// only referenced by the change_log entry — matching the real ordering of
// events in production).
func TestClosureFieldDeleteStillRewritesPlacingModels(t *testing.T) {
	f := setupClosureFixture(t)
	f.model("CLOSURE_MODEL_PLACES_DEL")
	cs := f.changeSet(entry("field", "CLOSURE_FIELD_DEL", "delete"))

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []ScopedRef{{EntityType: "field", EntityID: "CLOSURE_FIELD_DEL", Delete: true}}
	if diff := cmp.Diff(want, refs, sortRefs); diff != "" {
		t.Fatalf("closure mismatch (-want +got):\n%s", diff)
	}
}

func TestClosureOverrideEntryResolvesOwningModel(t *testing.T) {
	f := setupClosureFixture(t)
	f.field("CLOSURE_FIELD_OV")
	f.model("CLOSURE_MODEL_OV")
	overrideID := f.placeFieldOnModel("CLOSURE_FIELD_OV", "CLOSURE_MODEL_OV")
	cs := f.changeSet(entry("override", fmt.Sprintf("%d", overrideID), "update"))

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []ScopedRef{{EntityType: "model", EntityID: "CLOSURE_MODEL_OV"}}
	if diff := cmp.Diff(want, refs, sortRefs); diff != "" {
		t.Fatalf("closure mismatch (-want +got):\n%s", diff)
	}
}

func TestClosureOverrideEntryResolvesOwningCollection(t *testing.T) {
	f := setupClosureFixture(t)
	f.field("CLOSURE_FIELD_OV_C")
	f.collection("CLOSURE_COLLECTION_OV")
	overrideID := f.placeFieldOnCollection("CLOSURE_FIELD_OV_C", "CLOSURE_COLLECTION_OV")
	cs := f.changeSet(entry("override", fmt.Sprintf("%d", overrideID), "update"))

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []ScopedRef{{EntityType: "collection", EntityID: "CLOSURE_COLLECTION_OV"}}
	if diff := cmp.Diff(want, refs, sortRefs); diff != "" {
		t.Fatalf("closure mismatch (-want +got):\n%s", diff)
	}
}

func TestClosureOverrideEntryBaseResolvesToField(t *testing.T) {
	f := setupClosureFixture(t)
	f.field("CLOSURE_FIELD_BASE")
	overrideID := f.baseOverride("CLOSURE_FIELD_BASE")
	cs := f.changeSet(entry("override", fmt.Sprintf("%d", overrideID), "update"))

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []ScopedRef{{EntityType: "field", EntityID: "CLOSURE_FIELD_BASE"}}
	if diff := cmp.Diff(want, refs, sortRefs); diff != "" {
		t.Fatalf("closure mismatch (-want +got):\n%s", diff)
	}
}

// TestClosureOverrideEntryMissingRowSkipped covers the ErrNoRows edge: the
// override row referenced by the change_log entry has since been deleted
// (e.g. by a later change_set in the same poll batch). closure() must not
// error — it skips the entry and leaves the reconcile backstop to catch it.
func TestClosureOverrideEntryMissingRowSkipped(t *testing.T) {
	f := setupClosureFixture(t)
	cs := f.changeSet(entry("override", "999999999", "update"))

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no refs for a missing override row, got %+v", refs)
	}
}

// TestClosureDedupFirstOccurrenceWins proves closure()'s dedup-by-
// (EntityType, EntityID) behavior documented on closure()'s doc comment: two
// change_log entries that resolve to the SAME ref — here a direct "model
// delete" entry and a later "field update" entry whose sole placing owner is
// that same model — collapse to exactly one ScopedRef, keyed on the first
// occurrence. add() no-ops on a repeat key (see closure.go), so the first
// occurrence's Delete flag wins outright: a later non-delete placement ref
// for the same entity does NOT downgrade an earlier delete to a rewrite.
// (The code has no symmetric protection the other way — an earlier non-delete
// occurrence would likewise not be *upgraded* by a later delete for the same
// key, since the second add() call is a no-op regardless of its Delete
// argument. That asymmetry-free "first wins, period" behavior is what this
// test pins down.)
func TestClosureDedupFirstOccurrenceWins(t *testing.T) {
	f := setupClosureFixture(t)
	f.model("CLOSURE_MODEL_DEDUP")
	f.field("CLOSURE_FIELD_DEDUP")
	f.placeFieldOnModel("CLOSURE_FIELD_DEDUP", "CLOSURE_MODEL_DEDUP")
	// change_log entries are processed in id (insertion) order: the direct
	// model delete is written first, then the field update whose
	// placingOwners() fan-out re-adds the same model as a non-delete ref.
	cs := f.changeSet(
		entry("model", "CLOSURE_MODEL_DEDUP", "delete"),
		entry("field", "CLOSURE_FIELD_DEDUP", "update"),
	)

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []ScopedRef{
		// Exactly one ref for the model — the field-placement fan-out's
		// duplicate add() for the same key is a no-op, so Delete stays true
		// from the first (direct model delete) occurrence.
		{EntityType: "model", EntityID: "CLOSURE_MODEL_DEDUP", Delete: true},
		{EntityType: "field", EntityID: "CLOSURE_FIELD_DEDUP", Delete: false},
	}
	if diff := cmp.Diff(want, refs, sortRefs); diff != "" {
		t.Fatalf("closure mismatch (-want +got):\n%s", diff)
	}
}

// TestClosureNamespaceBindingMapsToProjectManifest is the final-review Fix 1
// regression: before this fix, closure() had no case for a "namespace_binding"
// change_log entity type, so it produced NO refs at all — the change_set
// would drain as a noop with no commit (the DB change unmaterialized until the
// nightly reconcile sweep, under system attribution). namespace_binding
// changes materialize into project.yaml, so they must map to a single
// "project_manifest" ref, never a delete.
func TestClosureNamespaceBindingMapsToProjectManifest(t *testing.T) {
	f := setupClosureFixture(t)
	cs := f.changeSet(entry("namespace_binding", "SOME_BINDING_ID", "update"))

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []ScopedRef{{EntityType: "project_manifest", EntityID: f.projectID, Delete: false}}
	if diff := cmp.Diff(want, refs, sortRefs); diff != "" {
		t.Fatalf("closure mismatch (-want +got):\n%s", diff)
	}
}

// TestClosureProjectOntologyVersionMapsToProjectManifest is
// TestClosureNamespaceBindingMapsToProjectManifest's sibling for the other
// entity type Fix 1 covers: "project-ontology-version" changes materialize
// into both project.yaml (linked_versions) and pletka.mod (ontology
// requirements), so they map to the same synthetic "project_manifest" ref —
// scopedRewrite() decides which files that ref rewrites.
func TestClosureProjectOntologyVersionMapsToProjectManifest(t *testing.T) {
	f := setupClosureFixture(t)
	cs := f.changeSet(entry("project-ontology-version", "SOME_LINK_ID", "create"))

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []ScopedRef{{EntityType: "project_manifest", EntityID: f.projectID, Delete: false}}
	if diff := cmp.Diff(want, refs, sortRefs); diff != "" {
		t.Fatalf("closure mismatch (-want +got):\n%s", diff)
	}
}

// TestClosureNamespaceAndOntologyDedupToOneProjectManifestRef proves the
// closure() doc comment's dedup claim for the project-manifest case: a
// change_set with BOTH a namespace_binding entry and a project-ontology-version
// entry (e.g. one poller batch draining two related edits) collapses to
// exactly one "project_manifest" ref, not two duplicate rewrites of the same
// files.
func TestClosureNamespaceAndOntologyDedupToOneProjectManifestRef(t *testing.T) {
	f := setupClosureFixture(t)
	cs := f.changeSet(
		entry("namespace_binding", "SOME_BINDING_ID", "update"),
		entry("project-ontology-version", "SOME_LINK_ID", "create"),
	)

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []ScopedRef{{EntityType: "project_manifest", EntityID: f.projectID, Delete: false}}
	if diff := cmp.Diff(want, refs, sortRefs); diff != "" {
		t.Fatalf("closure mismatch (-want +got):\n%s", diff)
	}
}

// TestClosureCategoryDeleteResolvesSemanticIDFromPreviousPayload is the
// final-review Fix 2 regression: categories are the one entity where ID
// (a ULID) != SemanticID, but files are written at
// categories/<SemanticID>.yaml. A delete change_log entry's EntityID is the
// ULID (matching production: category.Service.Delete logs the row's raw ID),
// so closure() must resolve the SemanticID from previous_payload and use
// THAT as the ref's EntityID — otherwise deletePathsFor computes the wrong
// path and the real file lingers as noop drift.
func TestClosureCategoryDeleteResolvesSemanticIDFromPreviousPayload(t *testing.T) {
	f := setupClosureFixture(t)
	prevPayload, err := json.Marshal(map[string]string{"semantic_id": "CLOSURE_PROJECT.CAT.7"})
	if err != nil {
		t.Fatalf("marshal previous payload: %v", err)
	}
	cs := f.changeSet(entryWithPreviousPayload("category", "01CATEGORY_ULID_NOT_SEMANTIC", "delete", prevPayload))

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []ScopedRef{{EntityType: "category", EntityID: "CLOSURE_PROJECT.CAT.7", Delete: true}}
	if diff := cmp.Diff(want, refs, sortRefs); diff != "" {
		t.Fatalf("closure mismatch (-want +got):\n%s", diff)
	}
}

// TestClosureCategoryDeleteFallsBackToRawIDWhenPayloadMissing covers the
// degraded-input edge closure()'s doc comment promises: when previous_payload
// is empty or doesn't parse (e.g. a hand-crafted or legacy change_log row),
// closure() falls back to the raw entity id rather than erroring — the
// reconcile backstop heals any resulting drift.
func TestClosureCategoryDeleteFallsBackToRawIDWhenPayloadMissing(t *testing.T) {
	f := setupClosureFixture(t)
	cs := f.changeSet(entry("category", "CLOSURE_CATEGORY_NO_PAYLOAD", "delete"))

	refs, err := f.mat.closure(f.ctx, cs)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []ScopedRef{{EntityType: "category", EntityID: "CLOSURE_CATEGORY_NO_PAYLOAD", Delete: true}}
	if diff := cmp.Diff(want, refs, sortRefs); diff != "" {
		t.Fatalf("closure mismatch (-want +got):\n%s", diff)
	}
}
