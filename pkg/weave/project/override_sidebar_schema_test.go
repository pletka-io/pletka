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
