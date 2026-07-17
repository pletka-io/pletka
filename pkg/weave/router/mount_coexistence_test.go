package router_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestMountCoexistenceWithProjectsRouteGroup pins the assumption that
// pkg/weave/project's chi.Mount calls on sub-paths under /projects can
// coexist with direct routes on the same /projects group. If chi changes
// behaviour and starts panicking on overlapping prefixes, this catches it
// before the binary boots.
func TestMountCoexistenceWithProjectsRouteGroup(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("chi panicked at registration: %v", r)
		}
	}()
	mux := chi.NewMux()
	mux.Route("/projects", func(r chi.Router) {
		r.Get("/api", func(http.ResponseWriter, *http.Request) {})
		r.Post("/", func(http.ResponseWriter, *http.Request) {})
		r.Get("/{projectID}", func(http.ResponseWriter, *http.Request) {})
	})
	for _, path := range []string{
		"/projects/data",
		"/projects/check-prefix",
		"/projects/entity-list-schema",
		"/projects/filters/institutions",
		"/projects/form-schema/project",
		"/projects/{projectID}/inheritance-tree",
		"/projects/{projectID}/exports",
	} {
		sub := chi.NewMux()
		sub.Get("/", func(http.ResponseWriter, *http.Request) {})
		mux.Mount(path, sub)
	}
}
