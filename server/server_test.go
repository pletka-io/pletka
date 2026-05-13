package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerShellRoutes(t *testing.T) {
	srv, err := New(Config{
		BuildInfo: BuildInfo{
			Version: "test",
			Commit:  "abc123",
			Date:    "2026-05-13T00:00:00Z",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Run("health", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /healthz status = %d; want %d", rr.Code, http.StatusOK)
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("health JSON: %v", err)
		}
		if body["status"] != "ok" {
			t.Fatalf("health status = %v; want ok", body["status"])
		}
	})

	t.Run("version", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/version", nil)
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /version status = %d; want %d", rr.Code, http.StatusOK)
		}
		var body BuildInfo
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("version JSON: %v", err)
		}
		if body.Version != "test" || body.Commit != "abc123" {
			t.Fatalf("version body = %+v", body)
		}
	})

	t.Run("home shell", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET / status = %d; want %d", rr.Code, http.StatusOK)
		}
		if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Fatalf("content-type = %q; want text/html", ct)
		}
		if !strings.Contains(rr.Body.String(), "data-page-schema") {
			t.Fatalf("home shell does not include page schema")
		}
	})
}
