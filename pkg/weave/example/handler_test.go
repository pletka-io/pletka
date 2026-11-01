package example

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// TestHandlerDeleteNotFoundReturnsAPIErrorEnvelope proves the compliance
// fix: error responses from this handler must be the canonical apierror
// JSON envelope ({"code", "error", ...} with a JSON Content-Type), not a
// plain-text http.Error body.
func TestHandlerDeleteNotFoundReturnsAPIErrorEnvelope(t *testing.T) {
	svc := newTestService(newFakeStore(), nil)
	h := NewHandler(svc.Service, nil, nil, nil)

	req := httptest.NewRequestWithContext(auth.WithReadScope(context.Background(), auth.Draft()),
		http.MethodDelete, "/PROJECT1/examples/missing-id", nil)
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

// Read endpoints must fail before acquiring content without a current root or scope.
func TestExampleReadsRequireCurrentRootScopeAndSource(t *testing.T) {
	for _, tc := range []struct {
		name, project string
		scope         auth.ReadScope
		want          int
	}{
		{"missing root", "", auth.Draft(), 404}, {"wrong root", "OTHER", auth.Draft(), 404}, {"missing scope", "P", auth.ReadScope{}, 500}, {"missing source", "P", auth.Draft(), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandler(NewService(nil), nil, nil, nil)
			ctx := auth.WithReadScope(auth.WithProject(t.Context(), &domain.Project{Entity: domain.Entity{ID: tc.project}, Visibility: "public"}), tc.scope)
			router := chi.NewRouter()
			router.Get("/projects/{projectID}/examples/{exampleID}", h.Detail)
			router.Get("/projects/{projectID}/form", h.FormSchema)
			for _, path := range []string{"/projects/P/examples/E", "/projects/P/form?mode=edit&example_id=E"} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil))
				if w.Code != tc.want {
					t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestMutationServiceRejectsMissingOrReleaseScope(t *testing.T) {
	svc := NewService(nil)
	for _, scope := range []auth.ReadScope{{}, auth.Release("1.0.0")} {
		ctx := auth.WithReadScope(t.Context(), scope)
		if _, err := svc.Create(ctx, scope, "P", CreateInput{EntityType: domain.ExampleEntityTypeModel, EntityID: "M"}); err == nil {
			t.Fatal("create accepted non-draft scope")
		}
		if _, err := svc.Update(ctx, scope, "P", "E", UpdateInput{}); err == nil {
			t.Fatal("update accepted non-draft scope")
		}
		if err := svc.Delete(ctx, scope, "P", "E"); err == nil {
			t.Fatal("delete accepted non-draft scope")
		}
	}
}
