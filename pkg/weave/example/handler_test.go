package example

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestHandlerDeleteNotFoundReturnsAPIErrorEnvelope proves the compliance
// fix: error responses from this handler must be the canonical apierror
// JSON envelope ({"code", "error", ...} with a JSON Content-Type), not a
// plain-text http.Error body.
func TestHandlerDeleteNotFoundReturnsAPIErrorEnvelope(t *testing.T) {
	svc := NewService(newFakeStore(), nil)
	h := NewHandler(svc, nil, nil, nil)

	req := httptest.NewRequest(http.MethodDelete, "/PROJECT1/examples/missing-id", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", "PROJECT1")
	rctx.URLParams.Add("exampleID", "missing-id")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want JSON", ct)
	}

	var envelope struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v; body=%s", err, rec.Body.String())
	}
	if envelope.Code != "not_found" {
		t.Fatalf("code = %q, want %q", envelope.Code, "not_found")
	}
	if envelope.Error == "" {
		t.Fatal("error message is empty")
	}
}

// seedExample stores one example the handler tests can fetch.
func seedExample(t *testing.T, store *fakeStore, projectID, id string) {
	t.Helper()
	ex := &domain.Example{ID: id, ProjectID: projectID, EntityType: domain.ExampleEntityTypeModel, EntityID: "M1", Status: domain.ExampleStatusDraft}
	if err := store.CreateWithValues(context.Background(), ex, nil); err != nil {
		t.Fatal(err)
	}
}

func detailRequest(projectID, exampleID, accept string) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/"+projectID+"/examples/"+exampleID, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", projectID)
	rctx.URLParams.Add("exampleID", exampleID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// TestHandlerDetailRedirectsBrowsersToProjectPage verifies a browser
// navigation to the example's URL lands on the project page with the
// example open, while the JSON API contract is untouched.
func TestHandlerDetailRedirectsBrowsersToProjectPage(t *testing.T) {
	store := newFakeStore()
	seedExample(t, store, "LA", "01EXAMPLE")
	h := NewHandler(NewService(store, fakeViews{models: map[string]*domain.ModelView{"M1": singleFieldModelView(11, "F1", "String", false, 0, nil)}}), nil, nil, nil)

	rec := httptest.NewRecorder()
	h.Detail(rec, detailRequest("LA", "01EXAMPLE", "text/html,application/xhtml+xml,*/*;q=0.8"))
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302; body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/projects/LA#tab=examples&item=01EXAMPLE" {
		t.Fatalf("Location = %q", loc)
	}
}

func TestHandlerDetailServesJSONForAPIClients(t *testing.T) {
	store := newFakeStore()
	seedExample(t, store, "LA", "01EXAMPLE")
	h := NewHandler(NewService(store, fakeViews{models: map[string]*domain.ModelView{"M1": singleFieldModelView(11, "F1", "String", false, 0, nil)}}), nil, nil, nil)

	for _, accept := range []string{"", "*/*", "application/json"} {
		rec := httptest.NewRecorder()
		h.Detail(rec, detailRequest("LA", "01EXAMPLE", accept))
		if rec.Code != http.StatusOK {
			t.Fatalf("Accept %q: status = %d, want 200; body=%s", accept, rec.Code, rec.Body.String())
		}
		var out struct {
			Example struct {
				ID string `json:"id"`
			} `json:"example"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Example.ID != "01EXAMPLE" {
			t.Fatalf("Accept %q: body = %s (err %v)", accept, rec.Body.String(), err)
		}
	}
}

func TestHandlerDetailUnknownIDIs404ForBrowsersToo(t *testing.T) {
	h := NewHandler(NewService(newFakeStore(), fakeViews{}), nil, nil, nil)
	rec := httptest.NewRecorder()
	h.Detail(rec, detailRequest("LA", "missing", "text/html"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestHandlerListCarriesModelName verifies list rows name the target model
// next to its id (George: "you have to have memorized the Pletka numbers").
func TestHandlerListCarriesModelName(t *testing.T) {
	store := newFakeStore()
	seedExample(t, store, "LA", "01EXAMPLE")
	views := namingViews{
		fakeViews: fakeViews{models: map[string]*domain.ModelView{"M1": singleFieldModelView(11, "F1", "String", false, 0, nil)}},
		names:     map[string]domain.Translations{"M1": {"en": "Physical Thing"}},
	}
	h := NewHandler(NewService(store, views), nil, nil, nil)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/LA/examples", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", "LA")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.List(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items []struct {
			EntityID   string `json:"entity_id"`
			EntityName string `json:"entity_name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Items) != 1 {
		t.Fatalf("body = %s (err %v)", rec.Body.String(), err)
	}
	if out.Items[0].EntityName != "Physical Thing" || out.Items[0].EntityID != "M1" {
		t.Fatalf("row = %+v", out.Items[0])
	}
}
