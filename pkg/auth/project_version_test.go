package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithProjectVersionContext_AttachesVersionForSafeReads(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/projects/TPC/fields?version=1.2.0", nil)
	w := httptest.NewRecorder()

	var got string
	h := WithProjectVersionContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ProjectVersionFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got != "1.2.0" {
		t.Fatalf("expected version 1.2.0, got %q", got)
	}
}

func TestWithProjectVersionContext_BlocksMutations(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/projects/TPC/settings/general?version=1.2.0", nil)
	w := httptest.NewRecorder()

	called := false
	h := WithProjectVersionContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	h.ServeHTTP(w, req)

	if called {
		t.Fatal("expected downstream handler not to be called")
	}
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
