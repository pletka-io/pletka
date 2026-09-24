//go:build integration

package vocabulary_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

// fakeProjectReader satisfies auth.ProjectReader, returning a project with a
// fixed visibility so the gate's read check can be exercised without a real
// project store.
type fakeProjectReader struct{ visibility string }

func (f fakeProjectReader) GetByID(_ context.Context, id string) (*domain.Project, error) {
	return &domain.Project{Entity: domain.Entity{ID: id}, Visibility: f.visibility}, nil
}

// TestSearchConceptListEntries_AuthGate proves the previously-ungated global
// value-autocomplete route is no longer cross-tenant readable: an anonymous
// caller gets the list of a PUBLIC project but is refused (404) for an INTERNAL
// project's list.
func TestSearchConceptListEntries_AuthGate(t *testing.T) {
	pool := testdb.Pool(t)
	mustExec(t, pool, `INSERT INTO weave_actors (id, display_name, slug) VALUES ('own3','O3','own3')`)
	mustExec(t, pool, `INSERT INTO weave_projects (id, owner_id, visibility) VALUES ('PUBP','own3','public')`)
	mustExec(t, pool, `INSERT INTO weave_projects (id, owner_id, visibility) VALUES ('INTP','own3','internal')`)
	mustExec(t, pool, `INSERT INTO weave_concept_lists (id, project_id) VALUES ('CLPUB','PUBP')`)
	mustExec(t, pool, `INSERT INTO weave_concept_lists (id, project_id) VALUES ('CLINT','INTP')`)

	svc := vocabulary.NewService(pool, nil)

	serve := func(vis, listID string) int {
		r := chi.NewRouter()
		vocabulary.Mount(r, vocabulary.Host{Service: svc, Projects: fakeProjectReader{visibility: vis}})
		req := httptest.NewRequest(http.MethodGet, "/api/v2/concept-lists/"+listID+"/entries/search?q=x", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	if code := serve("public", "CLPUB"); code != http.StatusOK {
		t.Fatalf("anonymous read of a PUBLIC project's list: want 200, got %d", code)
	}
	if code := serve("internal", "CLINT"); code != http.StatusNotFound {
		t.Fatalf("anonymous read of an INTERNAL project's list: want 404, got %d", code)
	}
}
