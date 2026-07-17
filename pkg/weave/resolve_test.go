package weave_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
)

// pathElementsJSON builds a minimal path_elements JSONB payload from a list
// of class/property URIs, for tests that need fields with a known ontology
// path so ComputeSharedPathPrefix has something to compare.
func pathElementsJSON(t *testing.T, uris ...string) []byte {
	t.Helper()
	elems := make([]domain.PathElement, len(uris))
	for i, uri := range uris {
		elems[i] = domain.PathElement{Type: "class", URI: uri, LocalName: uri, Position: i}
	}
	b, err := json.Marshal(elems)
	if err != nil {
		t.Fatalf("marshal path elements: %v", err)
	}
	return b
}

func TestModelView_ResolvePriority(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	ctx := context.Background()

	// Create project.
	projectID := ids.GenerateULID()

	// Create model.
	modelID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id) VALUES ($1, 'draft', $2)`,
		modelID, projectID,
	); err != nil {
		t.Fatalf("setup model: %v", err)
	}

	// Create field.
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
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_categories WHERE project_id = $1`, projectID)
	})

	// Create base override: entity_type="" means base.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value)
		 VALUES ($1, $2, '', '', 0, 0, $3, $4, $5, '', '', 'literal', '')`,
		fieldID, projectID,
		[]byte(`{"en":"Base Name"}`), []byte(`{}`), []byte(`{}`),
	); err != nil {
		t.Fatalf("insert base override: %v", err)
	}

	// Create model override: entity_type="model", entity_id=modelID.
	var modelOverrideID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value)
		 VALUES ($1, $2, 'model', $3, 0, 0, $4, $5, $6, '', '', 'literal', '')
		 RETURNING id`,
		fieldID, projectID, modelID,
		[]byte(`{"en":"Model Name"}`), []byte(`{}`), []byte(`{}`),
	).Scan(&modelOverrideID); err != nil {
		t.Fatalf("insert model override: %v", err)
	}

	// Call ModelView.
	mv, err := store.ModelView(ctx, modelID, projectID)
	if err != nil {
		t.Fatalf("ModelView: %v", err)
	}

	if mv == nil {
		t.Fatal("ModelView returned nil")
	}

	// The model should have 1 field total.
	if mv.Stats.TotalFields != 1 {
		t.Fatalf("TotalFields = %d, want 1", mv.Stats.TotalFields)
	}

	// Find the resolved field in the category groups.
	var found bool
	for _, cg := range mv.Categories {
		for _, collGrp := range cg.Collections {
			for _, rf := range collGrp.Fields {
				if rf.ID != fieldID {
					continue
				}

				found = true

				if rf.DisplayName["en"] != "Model Name" {
					t.Errorf("DisplayName[en] = %q, want %q", rf.DisplayName["en"], "Model Name")
				}

				if rf.OverrideSource != "model" {
					t.Errorf("OverrideSource = %q, want %q", rf.OverrideSource, "model")
				}

				if rf.OverrideID != modelOverrideID {
					t.Errorf("OverrideID = %d, want %d", rf.OverrideID, modelOverrideID)
				}
			}
		}
	}

	if !found {
		t.Error("resolved field not found in ModelView categories")
	}
}

func TestModelView_GroupByCategory(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	ctx := context.Background()

	// Create project.
	projectID := ids.GenerateULID()

	// Create model.
	modelID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id) VALUES ($1, 'draft', $2)`,
		modelID, projectID,
	); err != nil {
		t.Fatalf("setup model: %v", err)
	}

	// Create collection.
	collectionID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_collections (id, status, project_id) VALUES ($1, 'draft', $2)`,
		collectionID, projectID,
	); err != nil {
		t.Fatalf("setup collection: %v", err)
	}

	// Create 2 categories: CAT-A (position=1), CAT-B (position=2).
	catA := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_categories (id, status, project_id, ui_name, canonical_order, system_name)
		 VALUES ($1, 'draft', $2, $3, 1, 'cat_a')`,
		catA, projectID, []byte(`{"en":"Category A"}`),
	); err != nil {
		t.Fatalf("setup catA: %v", err)
	}

	catB := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_categories (id, status, project_id, ui_name, canonical_order, system_name)
		 VALUES ($1, 'draft', $2, $3, 2, 'cat_b')`,
		catB, projectID, []byte(`{"en":"Category B"}`),
	); err != nil {
		t.Fatalf("setup catB: %v", err)
	}

	// Create 3 fields.
	fieldIDs := make([]string, 3)
	for i := range fieldIDs {
		fieldIDs[i] = ids.GenerateULID()
		if _, err := pool.Exec(ctx,
			`INSERT INTO weave_fields (id, status, project_id) VALUES ($1, 'draft', $2)`,
			fieldIDs[i], projectID,
		); err != nil {
			t.Fatalf("setup field %d: %v", i, err)
		}
	}

	t.Cleanup(func() {
		bgCtx := context.Background()
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_override_refs WHERE override_id IN (SELECT id FROM weave_field_overrides WHERE project_id = $1)`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_collections WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_categories WHERE project_id = $1`, projectID)
	})

	// Create base overrides for all 3 fields (required for the resolve query to find them).
	for i, fid := range fieldIDs {
		if _, err := pool.Exec(ctx,
			`INSERT INTO weave_field_overrides
				(field_id, project_id, entity_type, entity_id, position, collection_order,
				 display_name, description, collection_name, category_id, part_of_collection_id,
				 expected_value_type, set_value)
			 VALUES ($1, $2, '', '', $3, 0, $4, $5, $6, '', '', '', '')`,
			fid, projectID, i,
			[]byte(`{"en":"Base F`+string(rune('1'+i))+`"}`), []byte(`{}`), []byte(`{}`),
		); err != nil {
			t.Fatalf("insert base override %d: %v", i, err)
		}
	}

	// Create model overrides:
	// F1 -> catA, collectionID
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value)
		 VALUES ($1, $2, 'model', $3, 0, 0, $4, $5, $6, $7, $8, '', '')`,
		fieldIDs[0], projectID, modelID,
		[]byte(`{"en":"Model F1"}`), []byte(`{}`), []byte(`{}`),
		catA, collectionID,
	); err != nil {
		t.Fatalf("insert model override F1: %v", err)
	}

	// F2 -> catA, collectionID
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value)
		 VALUES ($1, $2, 'model', $3, 1, 0, $4, $5, $6, $7, $8, '', '')`,
		fieldIDs[1], projectID, modelID,
		[]byte(`{"en":"Model F2"}`), []byte(`{}`), []byte(`{}`),
		catA, collectionID,
	); err != nil {
		t.Fatalf("insert model override F2: %v", err)
	}

	// F3 -> catB, no collection (direct field)
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value)
		 VALUES ($1, $2, 'model', $3, 0, 0, $4, $5, $6, $7, '', '', '')`,
		fieldIDs[2], projectID, modelID,
		[]byte(`{"en":"Model F3"}`), []byte(`{}`), []byte(`{}`),
		catB,
	); err != nil {
		t.Fatalf("insert model override F3: %v", err)
	}

	// Call ModelView.
	mv, err := store.ModelView(ctx, modelID, projectID)
	if err != nil {
		t.Fatalf("ModelView: %v", err)
	}

	if mv == nil {
		t.Fatal("ModelView returned nil")
	}

	// Should have 2 category groups.
	if len(mv.Categories) != 2 {
		t.Fatalf("len(Categories) = %d, want 2", len(mv.Categories))
	}

	// First category should be CAT-A (position=1).
	if mv.Categories[0].ID != catA {
		t.Errorf("Categories[0].ID = %q, want catA %q", mv.Categories[0].ID, catA)
	}

	if mv.Categories[0].Name["en"] != "Category A" {
		t.Errorf("Categories[0].Name[en] = %q, want %q", mv.Categories[0].Name["en"], "Category A")
	}

	// CAT-A should have 1 collection group with 2 fields.
	if len(mv.Categories[0].Collections) != 1 {
		t.Fatalf("len(Categories[0].Collections) = %d, want 1", len(mv.Categories[0].Collections))
	}

	if mv.Categories[0].Collections[0].ID != collectionID {
		t.Errorf("Categories[0].Collections[0].ID = %q, want %q",
			mv.Categories[0].Collections[0].ID, collectionID)
	}

	if len(mv.Categories[0].Collections[0].Fields) != 2 {
		t.Errorf("len(Categories[0].Collections[0].Fields) = %d, want 2",
			len(mv.Categories[0].Collections[0].Fields))
	}

	// Second category should be CAT-B (position=2).
	if mv.Categories[1].ID != catB {
		t.Errorf("Categories[1].ID = %q, want catB %q", mv.Categories[1].ID, catB)
	}

	// CAT-B should have 1 collection group (direct) with 1 field.
	if len(mv.Categories[1].Collections) != 1 {
		t.Fatalf("len(Categories[1].Collections) = %d, want 1", len(mv.Categories[1].Collections))
	}

	if len(mv.Categories[1].Collections[0].Fields) != 1 {
		t.Errorf("len(Categories[1].Collections[0].Fields) = %d, want 1",
			len(mv.Categories[1].Collections[0].Fields))
	}

	// Stats should reflect 3 fields, 2 categories, 1 collection.
	if mv.Stats.TotalFields != 3 {
		t.Errorf("Stats.TotalFields = %d, want 3", mv.Stats.TotalFields)
	}

	if mv.Stats.TotalCategories != 2 {
		t.Errorf("Stats.TotalCategories = %d, want 2", mv.Stats.TotalCategories)
	}

	if mv.Stats.TotalCollections != 1 {
		t.Errorf("Stats.TotalCollections = %d, want 1", mv.Stats.TotalCollections)
	}
}

func TestModelView_CompleteCopy(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	ctx := context.Background()

	// Create project.
	projectID := ids.GenerateULID()

	// Create model.
	modelID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id) VALUES ($1, 'draft', $2)`,
		modelID, projectID,
	); err != nil {
		t.Fatalf("setup model: %v", err)
	}

	// Create field. expected_value_type lives here (field-owned per
	// the post-ab5e62c rule) — overrides must not be able to change it.
	fieldID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_fields (id, status, project_id, expected_value_type) VALUES ($1, 'draft', $2, 'URI')`,
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
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_categories WHERE project_id = $1`, projectID)
	})

	// Base override carries display defaults — expected_value_type
	// stays empty here because it's field-owned (see field row above).
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value)
		 VALUES ($1, $2, '', '', 0, 0, $3, $4, $5, '', '', '', '')`,
		fieldID, projectID,
		[]byte(`{"en":"Base"}`), []byte(`{"en":"Base description"}`), []byte(`{}`),
	); err != nil {
		t.Fatalf("insert base override: %v", err)
	}

	// Model override: complete copy with different display_name and
	// description. expected_value_type stays empty — the resolver
	// pulls the field's value via the JOIN.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value)
		 VALUES ($1, $2, 'model', $3, 0, 0, $4, $5, $6, '', '', '', '')`,
		fieldID, projectID, modelID,
		[]byte(`{"en":"Override"}`), []byte(`{"en":"Override description"}`), []byte(`{}`),
	); err != nil {
		t.Fatalf("insert model override: %v", err)
	}

	// Call ModelView.
	mv, err := store.ModelView(ctx, modelID, projectID)
	if err != nil {
		t.Fatalf("ModelView: %v", err)
	}

	if mv == nil {
		t.Fatal("ModelView returned nil")
	}

	if mv.Stats.TotalFields != 1 {
		t.Fatalf("TotalFields = %d, want 1", mv.Stats.TotalFields)
	}

	// Find the resolved field.
	var found bool
	for _, cg := range mv.Categories {
		for _, collGrp := range cg.Collections {
			for _, rf := range collGrp.Fields {
				if rf.ID != fieldID {
					continue
				}

				found = true

				// The model override wins entirely (copy-on-adopt).
				if rf.DisplayName["en"] != "Override" {
					t.Errorf("DisplayName[en] = %q, want %q", rf.DisplayName["en"], "Override")
				}

				if rf.Description["en"] != "Override description" {
					t.Errorf("Description[en] = %q, want %q", rf.Description["en"], "Override description")
				}

				// ExpectedValueType comes from the base field definition, not the override.
				if rf.ExpectedValueType != "URI" {
					t.Errorf("ExpectedValueType = %q, want %q", rf.ExpectedValueType, "URI")
				}

				if rf.OverrideSource != "model" {
					t.Errorf("OverrideSource = %q, want %q", rf.OverrideSource, "model")
				}
			}
		}
	}

	if !found {
		t.Error("resolved field not found in ModelView categories")
	}
}

// TestModelView_BaseSetValueIsNonOverridable pins the resolver rule:
// when the base override (entity_type=”) has a non-empty set_value,
// that value wins regardless of any model/collection-level set_value
// override. The base set_value is the project's commitment that the
// field is fixed; downstream embeddings cannot silently change it.
func TestModelView_BaseSetValueIsNonOverridable(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	ctx := context.Background()

	projectID := ids.GenerateULID()

	modelID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id) VALUES ($1, 'draft', $2)`,
		modelID, projectID,
	); err != nil {
		t.Fatalf("setup model: %v", err)
	}

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
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_categories WHERE project_id = $1`, projectID)
	})

	// Base override sets a non-empty set_value: must win.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value)
		 VALUES ($1, $2, '', '', 0, 0, $3, $4, $5, '', '', 'literal', 'BASE_LOCKED')`,
		fieldID, projectID,
		[]byte(`{"en":"Base"}`), []byte(`{}`), []byte(`{}`),
	); err != nil {
		t.Fatalf("insert base override: %v", err)
	}

	// Model override tries to override set_value: must be rejected.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value)
		 VALUES ($1, $2, 'model', $3, 0, 0, $4, $5, $6, '', '', 'literal', 'MODEL_TRIES_TO_WIN')`,
		fieldID, projectID, modelID,
		[]byte(`{"en":"Model"}`), []byte(`{}`), []byte(`{}`),
	); err != nil {
		t.Fatalf("insert model override: %v", err)
	}

	mv, err := store.ModelView(ctx, modelID, projectID)
	if err != nil {
		t.Fatalf("ModelView: %v", err)
	}

	var found bool
	for _, cg := range mv.Categories {
		for _, collGrp := range cg.Collections {
			for _, rf := range collGrp.Fields {
				if rf.ID != fieldID {
					continue
				}
				found = true
				if rf.SetValue != "BASE_LOCKED" {
					t.Errorf("SetValue = %q, want %q (base must override model-level set_value)", rf.SetValue, "BASE_LOCKED")
				}
				if rf.OverrideSource != "base" {
					t.Errorf("OverrideSource = %q, want %q", rf.OverrideSource, "base")
				}
			}
		}
	}
	if !found {
		t.Error("resolved field not found")
	}
}

// TestModelView_EmptyBaseSetValueDoesNotLock confirms the inverse:
// when the base set_value is empty, a model-level set_value flows
// through normally — the lock is gated on a non-empty base value.
func TestModelView_EmptyBaseSetValueDoesNotLock(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	ctx := context.Background()

	projectID := ids.GenerateULID()

	modelID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id) VALUES ($1, 'draft', $2)`,
		modelID, projectID,
	); err != nil {
		t.Fatalf("setup model: %v", err)
	}

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
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_categories WHERE project_id = $1`, projectID)
	})

	// Base override leaves set_value empty.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value)
		 VALUES ($1, $2, '', '', 0, 0, $3, $4, $5, '', '', 'literal', '')`,
		fieldID, projectID,
		[]byte(`{"en":"Base"}`), []byte(`{}`), []byte(`{}`),
	); err != nil {
		t.Fatalf("insert base override: %v", err)
	}

	// Model override sets a value: must flow through.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value)
		 VALUES ($1, $2, 'model', $3, 0, 0, $4, $5, $6, '', '', 'literal', 'MODEL_VALUE')`,
		fieldID, projectID, modelID,
		[]byte(`{"en":"Model"}`), []byte(`{}`), []byte(`{}`),
	); err != nil {
		t.Fatalf("insert model override: %v", err)
	}

	mv, err := store.ModelView(ctx, modelID, projectID)
	if err != nil {
		t.Fatalf("ModelView: %v", err)
	}

	var found bool
	for _, cg := range mv.Categories {
		for _, collGrp := range cg.Collections {
			for _, rf := range collGrp.Fields {
				if rf.ID != fieldID {
					continue
				}
				found = true
				if rf.SetValue != "MODEL_VALUE" {
					t.Errorf("SetValue = %q, want %q", rf.SetValue, "MODEL_VALUE")
				}
				if rf.OverrideSource != "model" {
					t.Errorf("OverrideSource = %q, want %q", rf.OverrideSource, "model")
				}
			}
		}
	}
	if !found {
		t.Error("resolved field not found")
	}
}

// TestModelView_StatsExcludeHiddenFields covers the follow-up:
// a field hidden via model override must not be counted in mv.Stats (total
// fields, fields breakdown), while still being present in mv.Categories —
// the override editor and generators still need to see hidden fields so
// curators can unhide them.
func TestModelView_StatsExcludeHiddenFields(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	ctx := context.Background()

	projectID := ids.GenerateULID()

	modelID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id) VALUES ($1, 'draft', $2)`,
		modelID, projectID,
	); err != nil {
		t.Fatalf("setup model: %v", err)
	}

	visibleFieldID := ids.GenerateULID()
	hiddenFieldID := ids.GenerateULID()
	for _, fid := range []string{visibleFieldID, hiddenFieldID} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO weave_fields (id, status, project_id) VALUES ($1, 'draft', $2)`,
			fid, projectID,
		); err != nil {
			t.Fatalf("setup field %s: %v", fid, err)
		}
	}

	t.Cleanup(func() {
		bgCtx := context.Background()
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_override_refs WHERE override_id IN (SELECT id FROM weave_field_overrides WHERE project_id = $1)`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
	})

	// Base overrides (required for resolveForModel to pick the fields up).
	for i, fid := range []string{visibleFieldID, hiddenFieldID} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO weave_field_overrides
				(field_id, project_id, entity_type, entity_id, position, collection_order,
				 display_name, description, collection_name, category_id, part_of_collection_id,
				 expected_value_type, set_value)
			 VALUES ($1, $2, '', '', $3, 0, $4, $5, $6, '', '', '', '')`,
			fid, projectID, i,
			[]byte(`{"en":"Base"}`), []byte(`{}`), []byte(`{}`),
		); err != nil {
			t.Fatalf("insert base override %s: %v", fid, err)
		}
	}

	// Model override for the visible field: is_hidden defaults to false.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value, is_hidden)
		 VALUES ($1, $2, 'model', $3, 0, 0, $4, $5, $6, '', '', '', '', false)`,
		visibleFieldID, projectID, modelID,
		[]byte(`{"en":"Visible Field"}`), []byte(`{}`), []byte(`{}`),
	); err != nil {
		t.Fatalf("insert model override visible field: %v", err)
	}

	// Model override for the hidden field: is_hidden=true.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value, is_hidden)
		 VALUES ($1, $2, 'model', $3, 1, 0, $4, $5, $6, '', '', '', '', true)`,
		hiddenFieldID, projectID, modelID,
		[]byte(`{"en":"Hidden Field"}`), []byte(`{}`), []byte(`{}`),
	); err != nil {
		t.Fatalf("insert model override hidden field: %v", err)
	}

	mv, err := store.ModelView(ctx, modelID, projectID)
	if err != nil {
		t.Fatalf("ModelView: %v", err)
	}
	if mv == nil {
		t.Fatal("ModelView returned nil")
	}

	if mv.Stats.TotalFields != 1 {
		t.Errorf("Stats.TotalFields = %d, want 1 (hidden field must be excluded)", mv.Stats.TotalFields)
	}

	for _, item := range mv.Stats.FieldsBreakdown {
		if item.Name == "Hidden Field" {
			t.Errorf("Stats.FieldsBreakdown contains hidden field %q, want excluded", item.Name)
		}
	}

	// mv.Categories must still carry BOTH fields — the override editor and
	// generators consume view.Categories directly and need hidden fields
	// intact (so curators can unhide them). Only Stats should filter.
	var categoriesFieldCount int
	for _, cg := range mv.Categories {
		for _, coll := range cg.Collections {
			categoriesFieldCount += len(coll.Fields)
		}
	}
	if categoriesFieldCount != 2 {
		t.Errorf("mv.Categories field count = %d, want 2 (hidden field must remain for the editor)", categoriesFieldCount)
	}
}

// TestModelView_DirectBucketNeverHoistsSharedPrefix is the regression test
// for: direct fields (no PartOfCollectionID) share a display
// category, not an ontology root, so hoisting an incidental shared prefix
// into the bucket header is wrong even when every direct field happens to
// share a full path (the pathological "1 field" / "all fields identical"
// case that previously hid the field's own path entirely).
func TestModelView_DirectBucketNeverHoistsSharedPrefix(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	ctx := context.Background()

	projectID := ids.GenerateULID()

	modelID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id) VALUES ($1, 'draft', $2)`,
		modelID, projectID,
	); err != nil {
		t.Fatalf("setup model: %v", err)
	}

	pathJSON := pathElementsJSON(t, "crm:E21_Person", "crm:P1_is_identified_by")

	fieldIDs := make([]string, 2)
	for i := range fieldIDs {
		fieldIDs[i] = ids.GenerateULID()
		if _, err := pool.Exec(ctx,
			`INSERT INTO weave_fields (id, status, project_id, path_elements) VALUES ($1, 'draft', $2, $3)`,
			fieldIDs[i], projectID, pathJSON,
		); err != nil {
			t.Fatalf("setup field %d: %v", i, err)
		}
	}

	t.Cleanup(func() {
		bgCtx := context.Background()
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_override_refs WHERE override_id IN (SELECT id FROM weave_field_overrides WHERE project_id = $1)`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
	})

	// Both fields are model overrides with no part_of_collection_id, so they
	// both land in the direct-fields ("__direct__") bucket, and both carry
	// the exact same PathElements — the case that used to hoist the whole
	// path to the bucket header, leaving the field row with nothing to show.
	for i, fid := range fieldIDs {
		if _, err := pool.Exec(ctx,
			`INSERT INTO weave_field_overrides
				(field_id, project_id, entity_type, entity_id, position, collection_order,
				 display_name, description, collection_name, category_id, part_of_collection_id,
				 expected_value_type, set_value)
			 VALUES ($1, $2, 'model', $3, $4, 0, $5, $6, $7, '', '', '', '')`,
			fid, projectID, modelID, i,
			[]byte(`{"en":"Direct Field"}`), []byte(`{}`), []byte(`{}`),
		); err != nil {
			t.Fatalf("insert model override %d: %v", i, err)
		}
	}

	mv, err := store.ModelView(ctx, modelID, projectID)
	if err != nil {
		t.Fatalf("ModelView: %v", err)
	}
	if mv == nil {
		t.Fatal("ModelView returned nil")
	}

	var direct *domain.CollectionGroup
	for i := range mv.Categories {
		for j := range mv.Categories[i].Collections {
			if mv.Categories[i].Collections[j].ID == "__direct__" {
				direct = &mv.Categories[i].Collections[j]
			}
		}
	}
	if direct == nil {
		t.Fatal("direct-fields bucket not found in ModelView.Categories")
	}
	if len(direct.Fields) != 2 {
		t.Fatalf("direct bucket field count = %d, want 2", len(direct.Fields))
	}
	if len(direct.SharedPathPrefix) != 0 {
		t.Errorf("direct.SharedPathPrefix = %+v, want empty (direct fields must never hoist a shared prefix)", direct.SharedPathPrefix)
	}
}

// TestModelView_RealCollectionKeepsSharedPrefix pins down that the
// fix is scoped to the direct-fields bucket only — a real
// collection whose fields genuinely share an ontology root still hoists
// that shared prefix into the collection header, unchanged.
func TestModelView_RealCollectionKeepsSharedPrefix(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	ctx := context.Background()

	projectID := ids.GenerateULID()

	modelID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id) VALUES ($1, 'draft', $2)`,
		modelID, projectID,
	); err != nil {
		t.Fatalf("setup model: %v", err)
	}

	collectionID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_collections (id, status, project_id) VALUES ($1, 'draft', $2)`,
		collectionID, projectID,
	); err != nil {
		t.Fatalf("setup collection: %v", err)
	}

	// Two fields sharing a 2-element root, diverging on the third element.
	path1 := pathElementsJSON(t, "crm:E67_Birth", "crm:P4_has_time-span", "crm:E52_Time-Span")
	path2 := pathElementsJSON(t, "crm:E67_Birth", "crm:P4_has_time-span", "crm:E49_Time_Appellation")

	fieldIDs := make([]string, 2)
	paths := [][]byte{path1, path2}
	for i := range fieldIDs {
		fieldIDs[i] = ids.GenerateULID()
		if _, err := pool.Exec(ctx,
			`INSERT INTO weave_fields (id, status, project_id, path_elements) VALUES ($1, 'draft', $2, $3)`,
			fieldIDs[i], projectID, paths[i],
		); err != nil {
			t.Fatalf("setup field %d: %v", i, err)
		}
	}

	t.Cleanup(func() {
		bgCtx := context.Background()
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_override_refs WHERE override_id IN (SELECT id FROM weave_field_overrides WHERE project_id = $1)`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_collections WHERE project_id = $1`, projectID)
	})

	for i, fid := range fieldIDs {
		if _, err := pool.Exec(ctx,
			`INSERT INTO weave_field_overrides
				(field_id, project_id, entity_type, entity_id, position, collection_order,
				 display_name, description, collection_name, category_id, part_of_collection_id,
				 expected_value_type, set_value)
			 VALUES ($1, $2, 'model', $3, $4, 0, $5, $6, $7, '', $8, '', '')`,
			fid, projectID, modelID, i,
			[]byte(`{"en":"Collection Field"}`), []byte(`{}`), []byte(`{}`),
			collectionID,
		); err != nil {
			t.Fatalf("insert model override %d: %v", i, err)
		}
	}

	mv, err := store.ModelView(ctx, modelID, projectID)
	if err != nil {
		t.Fatalf("ModelView: %v", err)
	}
	if mv == nil {
		t.Fatal("ModelView returned nil")
	}

	var coll *domain.CollectionGroup
	for i := range mv.Categories {
		for j := range mv.Categories[i].Collections {
			if mv.Categories[i].Collections[j].ID == collectionID {
				coll = &mv.Categories[i].Collections[j]
			}
		}
	}
	if coll == nil {
		t.Fatal("collection bucket not found in ModelView.Categories")
	}
	if len(coll.SharedPathPrefix) != 2 {
		t.Fatalf("coll.SharedPathPrefix length = %d, want 2: %+v", len(coll.SharedPathPrefix), coll.SharedPathPrefix)
	}
	if coll.SharedPathPrefix[0].URI != "crm:E67_Birth" || coll.SharedPathPrefix[1].URI != "crm:P4_has_time-span" {
		t.Errorf("coll.SharedPathPrefix = %+v, want [crm:E67_Birth crm:P4_has_time-span]", coll.SharedPathPrefix)
	}
}

// TestModelView_RealCollectionPrefixIgnoresHiddenField is the regression
// test for the follow-up nit: a hidden field's divergent path
// must not shorten (or blank out) a real collection's shared prefix, since
// the hidden field isn't rendered. The fix must not remove the hidden field
// from Categories[].Collections[].Fields — only the prefix computation
// changes; the override editor and generators still need it there.
func TestModelView_RealCollectionPrefixIgnoresHiddenField(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	ctx := context.Background()

	projectID := ids.GenerateULID()

	modelID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id) VALUES ($1, 'draft', $2)`,
		modelID, projectID,
	); err != nil {
		t.Fatalf("setup model: %v", err)
	}

	collectionID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_collections (id, status, project_id) VALUES ($1, 'draft', $2)`,
		collectionID, projectID,
	); err != nil {
		t.Fatalf("setup collection: %v", err)
	}

	// Two visible fields share a 2-element root; the hidden field's path
	// diverges completely (no shared elements at all) so if it leaked into
	// the computation the shared prefix would collapse to empty.
	visiblePath1 := pathElementsJSON(t, "crm:E67_Birth", "crm:P4_has_time-span", "crm:E52_Time-Span")
	visiblePath2 := pathElementsJSON(t, "crm:E67_Birth", "crm:P4_has_time-span", "crm:E49_Time_Appellation")
	hiddenPath := pathElementsJSON(t, "crm:E39_Actor", "crm:P131_is_identified_by")

	visibleFieldID1 := ids.GenerateULID()
	visibleFieldID2 := ids.GenerateULID()
	hiddenFieldID := ids.GenerateULID()

	for fid, p := range map[string][]byte{
		visibleFieldID1: visiblePath1,
		visibleFieldID2: visiblePath2,
		hiddenFieldID:   hiddenPath,
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO weave_fields (id, status, project_id, path_elements) VALUES ($1, 'draft', $2, $3)`,
			fid, projectID, p,
		); err != nil {
			t.Fatalf("setup field %s: %v", fid, err)
		}
	}

	t.Cleanup(func() {
		bgCtx := context.Background()
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_override_refs WHERE override_id IN (SELECT id FROM weave_field_overrides WHERE project_id = $1)`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_collections WHERE project_id = $1`, projectID)
	})

	insertOverride := func(fid string, pos int, isHidden bool) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO weave_field_overrides
				(field_id, project_id, entity_type, entity_id, position, collection_order,
				 display_name, description, collection_name, category_id, part_of_collection_id,
				 expected_value_type, set_value, is_hidden)
			 VALUES ($1, $2, 'model', $3, $4, 0, $5, $6, $7, '', $8, '', '', $9)`,
			fid, projectID, modelID, pos,
			[]byte(`{"en":"Field"}`), []byte(`{}`), []byte(`{}`),
			collectionID, isHidden,
		); err != nil {
			t.Fatalf("insert model override for %s: %v", fid, err)
		}
	}
	insertOverride(visibleFieldID1, 0, false)
	insertOverride(visibleFieldID2, 1, false)
	insertOverride(hiddenFieldID, 2, true)

	mv, err := store.ModelView(ctx, modelID, projectID)
	if err != nil {
		t.Fatalf("ModelView: %v", err)
	}
	if mv == nil {
		t.Fatal("ModelView returned nil")
	}

	var coll *domain.CollectionGroup
	for i := range mv.Categories {
		for j := range mv.Categories[i].Collections {
			if mv.Categories[i].Collections[j].ID == collectionID {
				coll = &mv.Categories[i].Collections[j]
			}
		}
	}
	if coll == nil {
		t.Fatal("collection bucket not found in ModelView.Categories")
	}

	// The hidden field must remain in Fields for the editor/generators.
	if len(coll.Fields) != 3 {
		t.Fatalf("coll.Fields length = %d, want 3 (hidden field must remain)", len(coll.Fields))
	}

	// But the shared prefix must be computed as if the hidden field wasn't
	// there — i.e. the 2-element root shared by the two visible fields,
	// not collapsed to empty by the hidden field's divergent path.
	if len(coll.SharedPathPrefix) != 2 {
		t.Fatalf("coll.SharedPathPrefix length = %d, want 2 (must ignore hidden field's divergent path): %+v",
			len(coll.SharedPathPrefix), coll.SharedPathPrefix)
	}
	if coll.SharedPathPrefix[0].URI != "crm:E67_Birth" || coll.SharedPathPrefix[1].URI != "crm:P4_has_time-span" {
		t.Errorf("coll.SharedPathPrefix = %+v, want [crm:E67_Birth crm:P4_has_time-span]", coll.SharedPathPrefix)
	}
}

func TestModelView_CollectionGroupCarriesPlacement(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	ctx := context.Background()

	projectID := ids.GenerateULID()
	modelID := ids.GenerateULID()
	collectionID := ids.GenerateULID()

	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id) VALUES ($1, 'draft', $2)`,
		modelID, projectID); err != nil {
		t.Fatalf("setup model: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_collections (id, status, project_id) VALUES ($1, 'draft', $2)`,
		collectionID, projectID); err != nil {
		t.Fatalf("setup collection: %v", err)
	}
	fieldID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_fields (id, status, project_id, path_elements) VALUES ($1, 'draft', $2, $3)`,
		fieldID, projectID,
		pathElementsJSON(t, "crm:E67_Birth", "crm:P4_has_time-span", "crm:E52_Time-Span")); err != nil {
		t.Fatalf("setup field: %v", err)
	}
	t.Cleanup(func() {
		bgCtx := context.Background()
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_collection_placements WHERE model_id = $1`, modelID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bgCtx, `DELETE FROM weave_collections WHERE project_id = $1`, projectID)
	})
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, collection_order,
			 display_name, description, collection_name, category_id, part_of_collection_id,
			 expected_value_type, set_value)
		 VALUES ($1, $2, 'model', $3, 0, 0, $4, $5, $6, '', $7, '', '')`,
		fieldID, projectID, modelID,
		[]byte(`{"en":"Collection Field"}`), []byte(`{}`), []byte(`{}`), collectionID); err != nil {
		t.Fatalf("insert model override: %v", err)
	}
	// Placement: required, 1..3, category '' (uncategorized bucket).
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_collection_placements
			(project_id, model_id, category_id, collection_id, is_required, min_occurs, max_occurs, is_hidden)
		 VALUES ($1, $2, '', $3, true, 1, 3, false)`,
		projectID, modelID, collectionID); err != nil {
		t.Fatalf("insert placement: %v", err)
	}

	mv, err := store.ModelView(ctx, modelID, projectID)
	if err != nil {
		t.Fatalf("ModelView: %v", err)
	}
	var coll *domain.CollectionGroup
	for i := range mv.Categories {
		for j := range mv.Categories[i].Collections {
			if mv.Categories[i].Collections[j].ID == collectionID {
				coll = &mv.Categories[i].Collections[j]
			}
		}
	}
	if coll == nil {
		t.Fatal("collection group not found in model view")
	}
	if coll.Placement == nil {
		t.Fatal("expected placement on collection group, got nil")
	}
	if !coll.Placement.IsRequired || coll.Placement.MinOccurs != 1 ||
		coll.Placement.MaxOccurs == nil || *coll.Placement.MaxOccurs != 3 {
		t.Errorf("placement constraints wrong: %+v", coll.Placement)
	}

	// Hidden placement: stats exclude the group's fields, but the model
	// view (editor payload) still contains the group flagged hidden.
	if _, err := pool.Exec(ctx,
		`UPDATE weave_collection_placements SET is_hidden = true WHERE model_id = $1`, modelID); err != nil {
		t.Fatalf("hide placement: %v", err)
	}
	mvHidden, err := store.ModelView(ctx, modelID, projectID)
	if err != nil {
		t.Fatalf("ModelView hidden: %v", err)
	}
	if mvHidden.Stats.TotalFields != 0 {
		t.Errorf("stats: want 0 fields with group hidden, got %d", mvHidden.Stats.TotalFields)
	}
	foundHidden := false
	for i := range mvHidden.Categories {
		for j := range mvHidden.Categories[i].Collections {
			c := mvHidden.Categories[i].Collections[j]
			if c.ID == collectionID && c.Placement != nil && c.Placement.IsHidden {
				foundHidden = true
			}
		}
	}
	if !foundHidden {
		t.Error("editor payload must retain the hidden group with is_hidden=true")
	}

	// A group with no placement row carries nil (defaults).
	if _, err := pool.Exec(ctx, `DELETE FROM weave_collection_placements WHERE model_id = $1`, modelID); err != nil {
		t.Fatalf("cleanup placement: %v", err)
	}
	mv2, err := store.ModelView(ctx, modelID, projectID)
	if err != nil {
		t.Fatalf("ModelView second: %v", err)
	}
	for i := range mv2.Categories {
		for j := range mv2.Categories[i].Collections {
			if mv2.Categories[i].Collections[j].ID == collectionID &&
				mv2.Categories[i].Collections[j].Placement != nil {
				t.Error("expected nil placement when no row exists")
			}
		}
	}
}
