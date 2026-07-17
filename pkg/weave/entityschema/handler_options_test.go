package entityschema

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
)

// selectOptionWire mirrors the wire shape of formschema.SelectOption for
// decoding in tests — formschema.SelectOption.Label is domain.Localizable
// (an interface), which encoding/json can't unmarshal directly.
type selectOptionWire struct {
	Value      string `json:"value"`
	SemanticID string `json:"semantic_id"`
}

// TestProjectModelOptions_CarriesSemanticID and its collection sibling
// below are the regression tests for ("referenced models
// render wrong after editing ... instead of 'LAM.2 Set'"). The ref-picker
// options endpoints never populated SelectOption.SemanticID at all, so
// callers (e.g. OverrideRefsCell.svelte) had nothing but the bare id to
// show alongside the label. Since id IS the semantic ID for models and
// collections (migration 004), the fix is to copy it across.
func TestProjectModelOptions_CarriesSemanticID(t *testing.T) {
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

	h := NewHandler(nil, weave.NewPostgresStore(pool), nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/projects/"+projectID+"/models/options", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", projectID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.ProjectModelOptions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var opts []selectOptionWire
	if err := json.Unmarshal(rec.Body.Bytes(), &opts); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}

	var found bool
	for _, opt := range opts {
		if opt.Value != modelID {
			continue
		}
		found = true
		if opt.SemanticID != modelID {
			t.Errorf("SemanticID = %q, want %q", opt.SemanticID, modelID)
		}
	}
	if !found {
		t.Fatalf("model option for %q not found in %+v", modelID, opts)
	}
}

func TestProjectCollectionOptions_CarriesSemanticID(t *testing.T) {
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

	h := NewHandler(nil, weave.NewPostgresStore(pool), nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/projects/"+projectID+"/collections/options", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", projectID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.ProjectCollectionOptions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var opts []selectOptionWire
	if err := json.Unmarshal(rec.Body.Bytes(), &opts); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}

	var found bool
	for _, opt := range opts {
		if opt.Value != collectionID {
			continue
		}
		found = true
		if opt.SemanticID != collectionID {
			t.Errorf("SemanticID = %q, want %q", opt.SemanticID, collectionID)
		}
	}
	if !found {
		t.Fatalf("collection option for %q not found in %+v", collectionID, opts)
	}
}
