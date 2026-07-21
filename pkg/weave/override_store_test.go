//go:build integration

package weave_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// overrideTestSetup holds shared test fixtures.
type overrideTestSetup struct {
	store     *weave.PostgresStore
	overrides domain.OverrideStore
	projectID string
	fieldID   string
}

func newOverrideTestSetup(t *testing.T) overrideTestSetup {
	t.Helper()

	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	overrides := store.Overrides()
	ctx := context.Background()

	// Create a temporary project.
	projectID := ids.GenerateULID()

	// Create a temporary weave_fields row.
	fieldID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_fields (id, status, project_id) VALUES ($1, 'draft', $2)`,
		fieldID, projectID,
	); err != nil {
		t.Fatalf("setup field: %v", err)
	}

	t.Cleanup(func() {
		bgCtx := context.Background()
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_override_refs WHERE override_id IN (SELECT id FROM weave_field_overrides WHERE project_id = $1)`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_collections WHERE project_id = $1`, projectID)
	})

	return overrideTestSetup{
		store:     store,
		overrides: overrides,
		projectID: projectID,
		fieldID:   fieldID,
	}
}

func TestOverrideStore_CreateAndGetBase(t *testing.T) {
	s := newOverrideTestSetup(t)
	ctx := context.Background()

	override := &domain.FieldOverride{
		FieldID:     s.fieldID,
		ProjectID:   s.projectID,
		EntityType:  "",
		EntityID:    "",
		DisplayName: domain.Translations{"en": "Test Field", "nl": "Testveld"},
		Description: domain.Translations{"en": "A test field description"},
		Position:    1,
	}

	if err := s.overrides.Create(ctx, override); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if override.ID == 0 {
		t.Fatal("expected ID to be generated after Create")
	}

	if override.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be populated after Create")
	}

	// GetBase should return the same override.
	got, err := s.overrides.GetBase(ctx, s.fieldID, s.projectID)
	if err != nil {
		t.Fatalf("GetBase: %v", err)
	}

	if got == nil {
		t.Fatal("GetBase returned nil")
	}

	if got.ID != override.ID {
		t.Errorf("GetBase ID = %d, want %d", got.ID, override.ID)
	}

	if diff := cmp.Diff(
		domain.Translations{"en": "Test Field", "nl": "Testveld"},
		got.DisplayName,
	); diff != "" {
		t.Errorf("DisplayName mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(
		domain.Translations{"en": "A test field description"},
		got.Description,
	); diff != "" {
		t.Errorf("Description mismatch (-want +got):\n%s", diff)
	}

	if got.EntityType != "" {
		t.Errorf("EntityType = %q, want empty", got.EntityType)
	}

	// GetBase for a non-existent field should return nil, nil.
	notFound, err := s.overrides.GetBase(ctx, "01NONEXISTENT0000000000000", s.projectID)
	if err != nil {
		t.Fatalf("GetBase for non-existent: %v", err)
	}

	if notFound != nil {
		t.Error("expected nil for non-existent field")
	}
}

func TestOverrideStore_SetAndGetRefs(t *testing.T) {
	s := newOverrideTestSetup(t)
	ctx := context.Background()

	// Create a base override to attach refs to.
	override := &domain.FieldOverride{
		FieldID:     s.fieldID,
		ProjectID:   s.projectID,
		EntityType:  "",
		EntityID:    "",
		DisplayName: domain.Translations{"en": "Field with refs"},
	}

	if err := s.overrides.Create(ctx, override); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Set 2 refs: one resource_model and one collection_model.
	refs := []domain.OverrideRef{
		{
			RefType:    "resource_model",
			TargetID:   ids.GenerateULID(),
			SemanticID: "OVRM.1",
			Position:   0,
		},
		{
			RefType:    "collection_model",
			TargetID:   ids.GenerateULID(),
			SemanticID: "OVRC.1",
			Position:   0,
		},
	}

	if err := s.overrides.SetRefs(ctx, override.ID, refs); err != nil {
		t.Fatalf("SetRefs: %v", err)
	}

	// GetRefs should return 2 refs.
	got, err := s.overrides.GetRefs(ctx, override.ID)
	if err != nil {
		t.Fatalf("GetRefs: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("GetRefs returned %d refs, want 2", len(got))
	}

	// Verify ref types are present (ordered by ref_type, then position).
	refTypes := make([]string, len(got))
	for i, r := range got {
		refTypes[i] = r.RefType
	}

	if diff := cmp.Diff(
		[]string{"collection_model", "resource_model"},
		refTypes,
		cmpopts.SortSlices(func(a, b string) bool { return a < b }),
	); diff != "" {
		t.Errorf("RefTypes mismatch (-want +got):\n%s", diff)
	}

	// Verify semantic IDs are correct.
	semanticIDs := make(map[string]string)
	for _, r := range got {
		semanticIDs[r.RefType] = r.SemanticID
	}

	if semanticIDs["resource_model"] != "OVRM.1" {
		t.Errorf("resource_model SemanticID = %q, want %q", semanticIDs["resource_model"], "OVRM.1")
	}

	if semanticIDs["collection_model"] != "OVRC.1" {
		t.Errorf("collection_model SemanticID = %q, want %q", semanticIDs["collection_model"], "OVRC.1")
	}

	// Replace with 1 ref.
	replacedRefs := []domain.OverrideRef{
		{
			RefType:    "resource_model",
			TargetID:   ids.GenerateULID(),
			SemanticID: "OVRM.2",
			Position:   0,
		},
	}

	if err := s.overrides.SetRefs(ctx, override.ID, replacedRefs); err != nil {
		t.Fatalf("SetRefs (replace): %v", err)
	}

	got2, err := s.overrides.GetRefs(ctx, override.ID)
	if err != nil {
		t.Fatalf("GetRefs after replace: %v", err)
	}

	if len(got2) != 1 {
		t.Fatalf("GetRefs after replace returned %d refs, want 1", len(got2))
	}

	if got2[0].SemanticID != "OVRM.2" {
		t.Errorf("replaced ref SemanticID = %q, want %q", got2[0].SemanticID, "OVRM.2")
	}
}

func TestOverrideStore_ListForField(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	overrides := store.Overrides()
	ctx := context.Background()

	// Create a temporary project.
	projectID := ids.GenerateULID()

	// Create a field.
	fieldID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_fields (id, status, project_id) VALUES ($1, 'draft', $2)`,
		fieldID, projectID,
	); err != nil {
		t.Fatalf("setup field: %v", err)
	}

	// Create a model.
	modelID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id) VALUES ($1, 'draft', $2)`,
		modelID, projectID,
	); err != nil {
		t.Fatalf("setup model: %v", err)
	}

	// Create a collection.
	collectionID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_collections (id, status, project_id) VALUES ($1, 'draft', $2)`,
		collectionID, projectID,
	); err != nil {
		t.Fatalf("setup collection: %v", err)
	}

	t.Cleanup(func() {
		bgCtx := context.Background()
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_override_refs WHERE override_id IN (SELECT id FROM weave_field_overrides WHERE project_id = $1)`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_collections WHERE project_id = $1`, projectID)
	})

	// Create 3 overrides: base, model, collection.
	baseOverride := &domain.FieldOverride{
		FieldID:     fieldID,
		ProjectID:   projectID,
		EntityType:  "",
		EntityID:    "",
		DisplayName: domain.Translations{"en": "Base"},
		Position:    1,
	}
	if err := overrides.Create(ctx, baseOverride); err != nil {
		t.Fatalf("Create base: %v", err)
	}

	modelOverride := &domain.FieldOverride{
		FieldID:     fieldID,
		ProjectID:   projectID,
		EntityType:  "model",
		EntityID:    modelID,
		DisplayName: domain.Translations{"en": "Model Override"},
		Position:    1,
	}
	if err := overrides.Create(ctx, modelOverride); err != nil {
		t.Fatalf("Create model override: %v", err)
	}

	collectionOverride := &domain.FieldOverride{
		FieldID:     fieldID,
		ProjectID:   projectID,
		EntityType:  "collection",
		EntityID:    collectionID,
		DisplayName: domain.Translations{"en": "Collection Override"},
		Position:    1,
	}
	if err := overrides.Create(ctx, collectionOverride); err != nil {
		t.Fatalf("Create collection override: %v", err)
	}

	// ListForField should return 3 overrides in priority order:
	// model (0), collection (1), base (2).
	list, err := overrides.ListForField(ctx, fieldID)
	if err != nil {
		t.Fatalf("ListForField: %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("ListForField returned %d overrides, want 3", len(list))
	}

	wantOrder := []string{"model", "collection", ""}
	gotOrder := make([]string, len(list))
	for i, o := range list {
		gotOrder[i] = o.EntityType
	}

	if diff := cmp.Diff(wantOrder, gotOrder); diff != "" {
		t.Errorf("ListForField order mismatch (-want +got):\n%s", diff)
	}

	// Verify display names.
	if list[0].DisplayName["en"] != "Model Override" {
		t.Errorf("first override DisplayName = %q, want %q", list[0].DisplayName["en"], "Model Override")
	}

	if list[1].DisplayName["en"] != "Collection Override" {
		t.Errorf("second override DisplayName = %q, want %q", list[1].DisplayName["en"], "Collection Override")
	}

	if list[2].DisplayName["en"] != "Base" {
		t.Errorf("third override DisplayName = %q, want %q", list[2].DisplayName["en"], "Base")
	}
}

func TestOverrideStore_ReplaceForEntity(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	overrides := store.Overrides()
	ctx := context.Background()

	// Create a temporary project.
	projectID := ids.GenerateULID()

	// Create two fields.
	fieldID1 := ids.GenerateULID()
	fieldID2 := ids.GenerateULID()

	for _, fid := range []string{fieldID1, fieldID2} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO weave_fields (id, status, project_id) VALUES ($1, 'draft', $2)`,
			fid, projectID,
		); err != nil {
			t.Fatalf("setup field %s: %v", fid, err)
		}
	}

	// Create a model.
	modelID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id) VALUES ($1, 'draft', $2)`,
		modelID, projectID,
	); err != nil {
		t.Fatalf("setup model: %v", err)
	}

	t.Cleanup(func() {
		bgCtx := context.Background()
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_override_refs WHERE override_id IN (SELECT id FROM weave_field_overrides WHERE project_id = $1)`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
	})

	// Create 2 model overrides for different fields.
	override1 := &domain.FieldOverride{
		FieldID:     fieldID1,
		ProjectID:   projectID,
		EntityType:  "model",
		EntityID:    modelID,
		DisplayName: domain.Translations{"en": "Override 1"},
		Position:    0,
	}
	override2 := &domain.FieldOverride{
		FieldID:     fieldID2,
		ProjectID:   projectID,
		EntityType:  "model",
		EntityID:    modelID,
		DisplayName: domain.Translations{"en": "Override 2"},
		Position:    1,
	}

	if err := overrides.Create(ctx, override1); err != nil {
		t.Fatalf("Create override1: %v", err)
	}

	if err := overrides.Create(ctx, override2); err != nil {
		t.Fatalf("Create override2: %v", err)
	}

	// Verify we have 2 overrides.
	list, err := overrides.ListForEntity(ctx, "model", modelID)
	if err != nil {
		t.Fatalf("ListForEntity: %v", err)
	}

	if len(list) != 2 {
		t.Fatalf("ListForEntity returned %d overrides, want 2", len(list))
	}

	// ReplaceForEntity with 1 override.
	newOverrides := []domain.FieldOverride{
		{
			FieldID:     fieldID1,
			ProjectID:   projectID,
			DisplayName: domain.Translations{"en": "Replaced Override"},
			Position:    0,
		},
	}

	if err := overrides.ReplaceForEntity(ctx, "model", modelID, newOverrides); err != nil {
		t.Fatalf("ReplaceForEntity: %v", err)
	}

	// ListForEntity should now return exactly 1 override.
	list2, err := overrides.ListForEntity(ctx, "model", modelID)
	if err != nil {
		t.Fatalf("ListForEntity after replace: %v", err)
	}

	if len(list2) != 1 {
		t.Fatalf("ListForEntity after replace returned %d overrides, want 1", len(list2))
	}

	if list2[0].DisplayName["en"] != "Replaced Override" {
		t.Errorf("replaced override DisplayName = %q, want %q", list2[0].DisplayName["en"], "Replaced Override")
	}

	if list2[0].EntityType != "model" {
		t.Errorf("replaced override EntityType = %q, want %q", list2[0].EntityType, "model")
	}

	if list2[0].EntityID != modelID {
		t.Errorf("replaced override EntityID = %q, want %q", list2[0].EntityID, modelID)
	}
}
