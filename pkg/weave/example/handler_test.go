package example

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
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
