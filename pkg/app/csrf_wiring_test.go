package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/session"
)

// A cross-site-style non-exempt POST (no JSON content-type, no X-Requested-With)
// without a CSRF token must be rejected once CSRF is wired.
//
// The probe route is mounted under /projects/ because the CSRF exemption in
// pkg/session/middleware.go (noSurf's ExemptFunc) only ever exempts JSON/XHR
// requests for /projects/ paths (and unconditionally for /api/ paths); a bare
// path like /csrf-probe is never exempt, which would make the JSON-POST
// assertion below indistinguishable from the form-POST one. /projects/ is the
// real prefix the schema-driven UI's fetch() calls use, so this also matches
// production behavior.
func TestCSRFRejectsUntokenedFormPost(t *testing.T) {
	r := chi.NewRouter()
	setupGlobalMiddleware(r, rootDependencies{Session: session.New(session.DefaultConfig())})
	r.Post("/projects/csrf-probe", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })

	req := httptest.NewRequest(http.MethodPost, "/projects/csrf-probe", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("untokened form POST: got %d, want 400 (nosurf reject)", rec.Code)
	}
}

// A JSON POST (schema-driven UI fetch()) to the same probe must still pass —
// nosurf's JSON exemption for /projects/ routes must survive being wired in.
func TestCSRFExemptsJSONPost(t *testing.T) {
	r := chi.NewRouter()
	setupGlobalMiddleware(r, rootDependencies{Session: session.New(session.DefaultConfig())})
	r.Post("/projects/csrf-probe", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })

	req := httptest.NewRequest(http.MethodPost, "/projects/csrf-probe", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("JSON POST: got %d, want 200 (nosurf JSON exemption)", rec.Code)
	}
}
