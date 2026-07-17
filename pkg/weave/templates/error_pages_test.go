package templates

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestRenderer constructs a renderer with a no-op islandScripts
// function and no i18n manager — error pages then exercise their
// fallback (English) text path.
func newTestRenderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := NewRenderer(func(...string) template.HTML { return "" }, nil, AnalyticsConfig{})
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	return r
}

func TestRenderNotFound(t *testing.T) {
	r := newTestRenderer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing-page", nil)

	r.RenderNotFound(rec, req, ErrorPageDeps{Lang: "en"})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("content-type = %q; want text/html", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"404", "Page not found", "/missing-page", "Go home", "Browse projects"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestRenderMethodNotAllowed(t *testing.T) {
	r := newTestRenderer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/vision", nil)

	r.RenderMethodNotAllowed(rec, req, ErrorPageDeps{Lang: "en"})

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d; want 405", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"405", "Method not allowed", "POST", "/vision"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

// TestRenderForbidden_Anonymous covers the anon branch where the
// link block surfaces "Sign in" first instead of the default home/
// projects pair.
func TestRenderForbidden_Anonymous(t *testing.T) {
	r := newTestRenderer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/secret", nil)

	r.RenderForbidden(rec, req, ErrorPageDeps{Lang: "en"}) // Principal nil = anon

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"403", "Access denied", "Sign in", "/login"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestRenderInternalError(t *testing.T) {
	r := newTestRenderer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/some-page", nil)

	r.RenderInternalError(rec, req, ErrorPageDeps{Lang: "en"})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; want 500", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"500", "Something went wrong", "Go home"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestWantsJSON(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		accept string
		want   bool
	}{
		{"api prefix", "/api/v1/foo", "", true},
		{"schema suffix", "/projects/AME/page-schema", "", true},
		{"schema path", "/orgs/foo/schema", "", true},
		{"pane path", "/projects/AME/project-ontology-versions/pane", "", true},
		{"json accept no html", "/random", "application/json", true},
		{"browser accept", "/random", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", false},
		{"empty accept on html-y path", "/projects/AME", "", false},
		{"json + html accept = browser wins", "/random", "text/html,application/json;q=0.5", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			if got := WantsJSON(req); got != tc.want {
				t.Errorf("WantsJSON(%q, accept=%q) = %v; want %v", tc.path, tc.accept, got, tc.want)
			}
		})
	}
}
