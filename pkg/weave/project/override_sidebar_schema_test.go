//go:build integration

package project

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
)

// TestSidebarModelOptions_SemanticIDIsRealID is the regression test for
// ("referenced models render wrong after editing —
// 'collection Set' / 'physical_thing Physical Thing' instead of
// 'LAM.2 Set'"). For models, id IS the semantic ID (migration 004:
// "no separate semantic_id column needed — id IS the semantic ID"), but
// sidebarModelOptions built the option's SemanticID from SystemName
// instead — so a model with system_name "collection" and ui_name "Set"
// rendered as "collection Set" in the picker instead of "LAM.2 Set".
func TestSidebarModelOptions_SemanticIDIsRealID(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	projectID := ids.GenerateULID()
	modelID := ids.GenerateULID()

	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id, system_name, ui_name)
		 VALUES ($1, 'draft', $2, 'collection', $3)`,
		modelID, projectID, []byte(`{"en":"Set"}`),
	); err != nil {
		t.Fatalf("seed model: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_models WHERE project_id = $1`, projectID)
	})

	h := &Handler{weave: weave.NewPostgresStore(pool)}
	opts := h.sidebarModelOptions(ctx, projectID, "Model")

	var found bool
	for _, opt := range opts {
		if opt.Value != modelID {
			continue
		}
		found = true
		if opt.SemanticID != modelID {
			t.Errorf("SemanticID = %q, want %q (the real id — system_name %q must never surface as the badge)", opt.SemanticID, modelID, "collection")
		}
	}
	if !found {
		t.Fatalf("model option for %q not found in %+v", modelID, opts)
	}
}

// TestSidebarCollectionOptions_SemanticIDIsRealID mirrors
// TestSidebarModelOptions_SemanticIDIsRealID for collections — same bug,
// same fix.
func TestSidebarCollectionOptions_SemanticIDIsRealID(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	projectID := ids.GenerateULID()
	collectionID := ids.GenerateULID()

	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_collections (id, status, project_id, system_name, ui_name)
		 VALUES ($1, 'draft', $2, 'physical_thing', $3)`,
		collectionID, projectID, []byte(`{"en":"Physical Thing"}`),
	); err != nil {
		t.Fatalf("seed collection: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_collections WHERE project_id = $1`, projectID)
	})

	h := &Handler{weave: weave.NewPostgresStore(pool)}
	opts := h.sidebarCollectionOptions(ctx, projectID, "Collection")

	var found bool
	for _, opt := range opts {
		if opt.Value != collectionID {
			continue
		}
		found = true
		if opt.SemanticID != collectionID {
			t.Errorf("SemanticID = %q, want %q (the real id — system_name %q must never surface as the badge)", opt.SemanticID, collectionID, "physical_thing")
		}
	}
	if !found {
		t.Fatalf("collection option for %q not found in %+v", collectionID, opts)
	}
}

// TestSidebarModelOptions_CarriesSourceProjectLabel is the regression test
// for the picker-provenance fix: a model inherited from an ancestor project
// via the chain walk must carry SourceProjectLabel (the ancestor's friendly
// UI name) alongside SourceProjectID, so the sidebar model picker can
// distinguish same-named models across parent projects.
func TestSidebarModelOptions_CarriesSourceProjectLabel(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	parentID := ids.GenerateULID()
	childID := ids.GenerateULID()
	modelID := ids.GenerateULID()

	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_projects (id, owner_id, ui_name) VALUES ($1, 'fixture-user-owner', $2)`,
		parentID, []byte(`{"en":"Ancestor Project"}`),
	); err != nil {
		t.Fatalf("seed parent project: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_projects (id, owner_id, parent_project_id) VALUES ($1, 'fixture-user-owner', $2)`,
		childID, parentID,
	); err != nil {
		t.Fatalf("seed child project: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, status, project_id, system_name, ui_name)
		 VALUES ($1, 'draft', $2, 'ancestor_model', $3)`,
		modelID, parentID, []byte(`{"en":"Ancestor Model"}`),
	); err != nil {
		t.Fatalf("seed model: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_models WHERE project_id = $1`, parentID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id = $1`, childID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id = $1`, parentID)
	})

	h := &Handler{weave: weave.NewPostgresStore(pool)}
	opts := h.sidebarModelOptions(ctx, childID, "Model")

	var found bool
	for _, opt := range opts {
		if opt.Value != modelID {
			continue
		}
		found = true
		if opt.SourceProjectID != parentID {
			t.Errorf("SourceProjectID = %q, want %q", opt.SourceProjectID, parentID)
		}
		if opt.SourceProjectLabel != "Ancestor Project" {
			t.Errorf("SourceProjectLabel = %q, want %q", opt.SourceProjectLabel, "Ancestor Project")
		}
	}
	if !found {
		t.Fatalf("model option for %q not found in %+v", modelID, opts)
	}
}

// TestSidebarCollectionOptions_CarriesSourceProjectLabel mirrors
// TestSidebarModelOptions_CarriesSourceProjectLabel for collections.
func TestSidebarCollectionOptions_CarriesSourceProjectLabel(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	parentID := ids.GenerateULID()
	childID := ids.GenerateULID()
	collectionID := ids.GenerateULID()

	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_projects (id, owner_id, ui_name) VALUES ($1, 'fixture-user-owner', $2)`,
		parentID, []byte(`{"en":"Ancestor Project"}`),
	); err != nil {
		t.Fatalf("seed parent project: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_projects (id, owner_id, parent_project_id) VALUES ($1, 'fixture-user-owner', $2)`,
		childID, parentID,
	); err != nil {
		t.Fatalf("seed child project: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_collections (id, status, project_id, system_name, ui_name)
		 VALUES ($1, 'draft', $2, 'ancestor_collection', $3)`,
		collectionID, parentID, []byte(`{"en":"Ancestor Collection"}`),
	); err != nil {
		t.Fatalf("seed collection: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_collections WHERE project_id = $1`, parentID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id = $1`, childID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id = $1`, parentID)
	})

	h := &Handler{weave: weave.NewPostgresStore(pool)}
	opts := h.sidebarCollectionOptions(ctx, childID, "Collection")

	var found bool
	for _, opt := range opts {
		if opt.Value != collectionID {
			continue
		}
		found = true
		if opt.SourceProjectID != parentID {
			t.Errorf("SourceProjectID = %q, want %q", opt.SourceProjectID, parentID)
		}
		if opt.SourceProjectLabel != "Ancestor Project" {
			t.Errorf("SourceProjectLabel = %q, want %q", opt.SourceProjectLabel, "Ancestor Project")
		}
	}
	if !found {
		t.Fatalf("collection option for %q not found in %+v", collectionID, opts)
	}
}
