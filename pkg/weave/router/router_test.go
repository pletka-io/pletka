package router

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/i18n/backend"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// newErrorTestRouter mounts one tiny route behind the stash middleware —
// exactly what Mount installs at the top — and has the route call the
// public Error one-liner. Exercises the HTML-branded and JSON-envelope
// paths without needing the full Mount wiring (which requires an
// OntologyService and panics without one).
func newErrorTestRouter(t *testing.T) *chi.Mux {
	t.Helper()

	i18nMgr, err := i18n.New(i18n.Config{
		DefaultLanguage:  "en",
		FallbackLanguage: "en",
		Storage:          backend.NewMemoryBackend(),
		Languages: []i18n.Language{
			{Code: "en", Name: "English", EnglishName: "English", Direction: "ltr", Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	renderer, err := weavetemplates.NewRenderer(func(names ...string) template.HTML {
		return ""
	}, i18nMgr, weavetemplates.AnalyticsConfig{})
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	h := ErrorPageHost{Templates: renderer, I18n: i18nMgr}

	r := chi.NewRouter()
	r.Use(stashResponder(h))
	r.Get("/boom", func(w http.ResponseWriter, r *http.Request) {
		Error(w, r, http.StatusNotFound, "not_found", "nope")
	})
	return r
}

func TestError_HTML_RendersBrandedShell(t *testing.T) {
	r := newErrorTestRouter(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/boom", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html prefix", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "nope") {
		t.Fatalf("body missing message %q: %s", "nope", body)
	}
	if !strings.Contains(body, "404") {
		t.Fatalf("body missing status badge %q: %s", "404", body)
	}
}

func TestError_JSON_WritesEnvelope(t *testing.T) {
	r := newErrorTestRouter(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/boom", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json prefix", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"code":"not_found"`) {
		t.Fatalf("body missing code field: %s", body)
	}
	if !strings.Contains(body, `"error":"nope"`) {
		t.Fatalf("body missing error message: %s", body)
	}
}

func TestError_NoResponderStashed_FallsBackToPlainHTTPError(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/boom", func(w http.ResponseWriter, r *http.Request) {
		Error(w, r, http.StatusNotFound, "not_found", "nope")
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if !strings.Contains(w.Body.String(), "nope") {
		t.Fatalf("body missing message: %s", w.Body.String())
	}
}
