package router

import (
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

// TestMount_SurvivesParentWithPreexistingRoutes reproduces the exact chi
// assembly order that panicked in production: pkg/app/app.go registers the
// auth Group and admin.Mount directly on the root mux BEFORE calling
// weaverouter.Mount(handler, ...) — so the mux passed to Mount already has
// routes on it. chi v5.3.1 panics "all middlewares must be defined before
// routes on a mux" if .Use() is called on a mux in that state.
//
// Mount no longer calls parent.Use(...) at all — it builds a fresh sub-mux
// (root), installs stashResponder there before root has any routes, mounts
// every weave route on root, and attaches root to parent with a single
// parent.Mount("/", root) at the end. This test drives that exact sequence
// with the same real, unexported helpers Mount uses (stashResponder,
// mountSlice) against a parent that already carries a route, and confirms:
//  1. registration does not panic;
//  2. the pre-existing parent route still works untouched;
//  3. a request into the newly-mounted slice route carries the stashed
//     responder (errresp.FromContext returns ok) — the project-gate
//     middleware in pkg/auth (RequireProjectRead/requireProject) and
//     weaverouter.Error callers both depend on this being true.
func TestMount_SurvivesParentWithPreexistingRoutes(t *testing.T) {
	errors := newAssemblyOrderErrorHost(t)

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

		// This is the exact sequence Mount performs: a fresh sub-mux,
		// stashResponder installed before root has any routes, one
		// project-scoped slice mounted via the real mountSlice helper,
		// then a single parent.Mount("/", root) attaching the whole tree
		// to the (already dirty) parent.
		root := chi.NewMux()
		root.Use(stashResponder(errors))

		mountSlice(root, "/projects/{projectID}/widgets", ProjectMiddlewareHost{}, func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				_, responderPresent = errresp.FromContext(r.Context())
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("widget-list"))
			})
		})

		parent.Mount("/", root)
	}()

	if panicked != nil {
		t.Fatalf("Mount's registration sequence panicked against a parent with pre-existing routes: %v", panicked)
	}

	// The route registered on parent before the weave tree was mounted
	// must keep working untouched.
	loginReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	loginW := httptest.NewRecorder()
	parent.ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusOK || loginW.Body.String() != "login-page" {
		t.Fatalf("pre-existing parent route broken: status=%d body=%q", loginW.Code, loginW.Body.String())
	}

	// The newly-mounted slice route must be reachable and must have seen
	// the stashed responder in its request context.
	widgetReq := httptest.NewRequest(http.MethodGet, "/projects/ABC/widgets", nil)
	widgetW := httptest.NewRecorder()
	parent.ServeHTTP(widgetW, widgetReq)
	if widgetW.Code != http.StatusOK || widgetW.Body.String() != "widget-list" {
		t.Fatalf("mounted slice route unreachable: status=%d body=%q", widgetW.Code, widgetW.Body.String())
	}
	if !responderPresent {
		t.Fatal("errresp.FromContext returned ok=false inside a route mounted by mountSlice — stashResponder is not wrapping slice routes")
	}
}

// TestStashResponder_InstalledBeforeRoutes is the narrower, structural half
// of the same guarantee: stashResponder must be the first thing installed
// on a fresh mux (a .Use() call with zero routes registered) — never
// appended after routes exist, which is what chi panics on. Mount's fix
// keeps this invariant by always calling root.Use(stashResponder(errors))
// immediately after chi.NewMux(), before any slice mounts.
func TestStashResponder_InstalledBeforeRoutes(t *testing.T) {
	errors := newAssemblyOrderErrorHost(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("stashResponder.Use on a fresh mux panicked: %v", r)
		}
	}()

	root := chi.NewMux()
	root.Use(stashResponder(errors)) // must not panic: root has zero routes here
	root.Get("/probe", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := errresp.FromContext(r.Context()); !ok {
			t.Error("responder missing on a route registered after stashResponder")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	w := httptest.NewRecorder()
	root.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// TestMount_RealEntrypointDoesNotHitChiAssemblyPanic calls the actual
// exported Mount function — the one pkg/app/app.go invokes as
// weaverouter.Mount(handler, ...) at the tail of app assembly, after
// handler.Group(...) (auth routes) and admin.Mount(handler, ...) have
// already registered routes on handler — with a parent that is dirty in
// exactly that way.
//
// A fully-populated, working Options (real OntologyService, real slice
// Services, a live Postgres pool, ...) would require reproducing most of
// pkg/app's private host-building wiring by hand; that's not attempted
// here. Instead this test uses a zero-value Options, which is guaranteed to
// fail Validate() on the very first slice Mount call inside Mount
// (actoradmin, whose Store is required) — but that failure only matters
// once Mount has already gotten past the step that used to panic.
//
// Before the fix, Mount's first statement was parent.Use(stashResponder(...))
// — called unconditionally on the (possibly dirty) parent — so a dirty
// parent panicked immediately with chi's "all middlewares must be defined
// before routes" message, regardless of Options. After the fix, that Use
// call moved onto a fresh sub-mux, so a dirty parent no longer panics at
// that point; Mount instead proceeds until the first slice Host fails its
// own Validate() check and panics with that slice's own error message. This
// test asserts Mount panics for the *expected* (Options-incompleteness)
// reason, not the chi assembly-order reason — pinning that the regression
// class described in the bug report is gone from the real entrypoint.
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
