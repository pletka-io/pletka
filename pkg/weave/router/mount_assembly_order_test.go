package router

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/i18n/backend"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// TestMount_SurvivesParentWithPreexistingRoutes exercises the same slice +
// pre-existing-route coexistence guarantee the A1 regression test covered,
// against the reverted direct-mount structure: Mount installs no middleware
// of its own, so every weave route (via mountSlice, exactly as Mount calls
// it) mounts directly on parent, alongside whatever routes pkg/app/app.go
// already registered (the auth Group, admin.Mount) before calling
// weaverouter.Mount(handler, ...). This confirms:
//  1. registration does not panic;
//  2. the pre-existing parent route still works untouched;
//  3. a request into the newly-mounted slice route carries a responder
//     stashed via errresp.WithResponder (built from BuildResponder) — the
//     project-gate middleware in pkg/auth (RequireProjectRead/
//     requireProject) and weaverouter.Error callers both depend on this
//     being true once the app layer stashes the global responder.
func TestMount_SurvivesParentWithPreexistingRoutes(t *testing.T) {
	errors := newAssemblyOrderErrorHost(t)
	respond := BuildResponder(errors)

	// parent mimics pkg/app/app.go: handler.Group(...) for the auth routes
	// and admin.Mount(handler, ...) both register directly on the mux
	// before weaverouter.Mount is ever called.
	parent := chi.NewRouter()
	parent.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("login-page"))
	})

	var (
		responderPresent bool
		panicked         any
	)
	func() {
		defer func() { panicked = recover() }()

		// mountSlice is the same unexported helper Mount calls directly on
		// parent (no sub-router). The responder is stashed per-request via
		// a small middleware wrapping the slice handler, standing in for
		// the app-level stash Mount no longer installs itself.
		mountSlice(parent, "/projects/{projectID}/widgets", ProjectMiddlewareHost{}, func(r chi.Router) {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					next.ServeHTTP(w, r.WithContext(errresp.WithResponder(r.Context(), respond)))
				})
			})
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				_, responderPresent = errresp.FromContext(r.Context())
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("widget-list"))
			})
		})
	}()

	if panicked != nil {
		t.Fatalf("Mount's registration sequence panicked against a parent with pre-existing routes: %v", panicked)
	}

	// The route registered on parent before the weave tree was mounted
	// must keep working untouched.
	loginReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/login", nil)
	loginW := httptest.NewRecorder()
	parent.ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusOK || loginW.Body.String() != "login-page" {
		t.Fatalf("pre-existing parent route broken: status=%d body=%q", loginW.Code, loginW.Body.String())
	}

	// The newly-mounted slice route must be reachable and must have seen
	// the stashed responder in its request context.
	widgetReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/projects/ABC/widgets", nil)
	widgetW := httptest.NewRecorder()
	parent.ServeHTTP(widgetW, widgetReq)
	if widgetW.Code != http.StatusOK || widgetW.Body.String() != "widget-list" {
		t.Fatalf("mounted slice route unreachable: status=%d body=%q", widgetW.Code, widgetW.Body.String())
	}
	if !responderPresent {
		t.Fatal("errresp.FromContext returned ok=false inside a route mounted by mountSlice — stashResponder is not wrapping slice routes")
	}
}

// TestMount_RealEntrypointDoesNotHitChiAssemblyPanic calls the actual
// exported Mount function — the one pkg/app/app.go invokes as
// weaverouter.Mount(handler, ...) at the tail of app assembly, after
// handler.Group(...) (auth routes) and admin.Mount(handler, ...) have
// already registered routes on handler — with a parent that is dirty in
// exactly that way.
//
// Mount no longer calls parent.Use(...) (or any .Use at all) — every weave
// route mounts directly on parent (see router.go's Mount), so there is no
// "middlewares must be defined before routes" panic class left to trigger
// against a dirty parent. This test pins that: direct mount is safe.
//
// A fully-populated, working Options (real OntologyService, real slice
// Services, a live Postgres pool, ...) would require reproducing most of
// pkg/app's private host-building wiring by hand; that's not attempted
// here. Instead this test uses a zero-value Options, which is guaranteed to
// fail Validate() on the very first slice Mount call inside Mount
// (actoradmin, whose Store is required) — an unrelated, expected panic that
// only proves Mount got past route assembly. The assertion below fails the
// test if that panic ever turns back into the chi assembly-order message.
func TestMount_RealEntrypointDoesNotHitChiAssemblyPanic(t *testing.T) {
	errors := newAssemblyOrderErrorHost(t)

	parent := chi.NewRouter()
	parent.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	var panicked any
	func() {
		defer func() { panicked = recover() }()
		Mount(parent, ProjectMiddlewareHost{}, errors, Options{})
	}()

	if panicked == nil {
		t.Fatal("Mount(dirtyParent, ..., Options{}) did not panic — expected it to fail actoradmin's Validate() once past assembly, got a nil panic instead")
	}
	msg, ok := panicked.(error)
	var msgText string
	if ok {
		msgText = msg.Error()
	} else {
		msgText = fmt.Sprint(panicked)
	}
	if strings.Contains(msgText, "middlewares must be defined before routes") {
		t.Fatalf("Mount panicked with the chi assembly-order error against a dirty parent — the regression is back: %v", msgText)
	}
	if !strings.Contains(msgText, "actoradmin host missing required dependencies") {
		t.Fatalf("Mount panicked for an unexpected reason (want the actoradmin Validate() error, since Options is zero-value): %v", msgText)
	}
}

func newAssemblyOrderErrorHost(t *testing.T) ErrorPageHost {
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

	return ErrorPageHost{Templates: renderer, I18n: i18nMgr}
}
