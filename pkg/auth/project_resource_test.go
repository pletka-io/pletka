package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
)

// fakeWeaveStore is a minimal domain.WeaveStore that only implements
// Projects().GetByID enough for the WithProjectResource middleware to
// run. Other methods panic — tests should never reach them.
type fakeWeaveStore struct {
	domain.WeaveStore
	projects fakeProjectStore
}

func (f *fakeWeaveStore) Projects() domain.WeaveProjectStore { return &f.projects }

type fakeProjectStore struct {
	domain.WeaveProjectStore
	byID        map[string]*domain.Project
	byVersionID map[string]*domain.Project
}

func (f *fakeProjectStore) GetByID(_ context.Context, id string) (*domain.Project, error) {
	return f.byID[id], nil
}

func (f *fakeProjectStore) GetByIDVersion(_ context.Context, id, version string) (*domain.Project, error) {
	if f.byVersionID == nil {
		return nil, nil
	}
	return f.byVersionID[id+"@"+version], nil
}

func TestProjectResourceFromContext_PopulatesVisibility(t *testing.T) {
	cases := []struct {
		name           string
		project        *domain.Project
		wantVisibility string
	}{
		{
			name: "public project",
			project: &domain.Project{
				Entity:     domain.Entity{ID: "PUB"},
				Visibility: "public",
			},
			wantVisibility: "public",
		},
		{
			name: "private project",
			project: &domain.Project{
				Entity:     domain.Entity{ID: "PRIV"},
				Visibility: "private",
			},
			wantVisibility: "private",
		},
		{
			name: "missing visibility defaults to private",
			project: &domain.Project{
				Entity: domain.Entity{ID: "DEF"},
			},
			wantVisibility: "private",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := WithProject(context.Background(), tc.project)
			res := ProjectResourceFromContext(ctx)
			if res.ScopeType != "project" {
				t.Errorf("ScopeType=%q want project", res.ScopeType)
			}
			if res.ID != tc.project.ID {
				t.Errorf("ID=%q want %q", res.ID, tc.project.ID)
			}
			if res.Visibility != tc.wantVisibility {
				t.Errorf("Visibility=%q want %q", res.Visibility, tc.wantVisibility)
			}
		})
	}
}

func TestProjectResourceFromContext_NoProject(t *testing.T) {
	res := ProjectResourceFromContext(context.Background())
	if res.ScopeType != "project" {
		t.Errorf("ScopeType=%q want project", res.ScopeType)
	}
	if res.ID != "" {
		t.Errorf("ID=%q want empty", res.ID)
	}
	if res.Visibility != "" {
		t.Errorf("Visibility=%q want empty (closes-fail when no project)", res.Visibility)
	}
}

// mountTestRouter mirrors the production wiring (parent.Mount with a
// sub-mux carrying the WithProjectResource middleware) so chi.URLParam
// inside the middleware sees the {projectID} from the outer pattern.
// A flat r.Use() at the top level would NOT — URL params aren't bound
// until routing matches.
func mountTestRouter(store domain.WeaveStore, handler http.HandlerFunc) chi.Router {
	parent := chi.NewRouter()
	sub := chi.NewMux()
	sub.Use(WithProjectResource(store))
	sub.Get("/check", handler)
	parent.Mount("/projects/{projectID}", sub)
	return parent
}

func mountProjectGateRouter(projects ProjectReader, gate func(ProjectReader) func(http.Handler) http.Handler, handler http.HandlerFunc) chi.Router {
	parent := chi.NewRouter()
	sub := chi.NewMux()
	sub.With(gate(projects)).Get("/check", handler)
	parent.Mount("/projects/{projectID}", sub)
	return parent
}

// TestWithProjectResource_PublicFallback proves the bug fix: an
// anonymous snapshot can read a public project once the middleware
// populates the Resource with Visibility. Before the fix, the slice
// gates constructed Resource without Visibility and the public
// fallback in EffectiveRole couldn't fire.
func TestWithProjectResource_PublicFallback(t *testing.T) {
	store := &fakeWeaveStore{
		projects: fakeProjectStore{
			byID: map[string]*domain.Project{
				"PUB": {Entity: domain.Entity{ID: "PUB"}, Visibility: "public"},
			},
		},
	}
	r := mountTestRouter(store, func(w http.ResponseWriter, r *http.Request) {
		var snap *AuthSnapshot // anonymous
		res := ProjectResourceFromContext(r.Context())
		if !snap.Can(ProjectRead, res, nil) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/projects/PUB/check", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q want 200 (public-fallback should grant viewer)", rec.Code, rec.Body.String())
	}
}

func TestRequireProjectRead_PublicFallback(t *testing.T) {
	projects := &fakeProjectStore{
		byID: map[string]*domain.Project{
			"PUB": {Entity: domain.Entity{ID: "PUB"}, Visibility: "public"},
		},
	}
	r := mountProjectGateRouter(projects, RequireProjectRead, func(w http.ResponseWriter, r *http.Request) {
		project := ProjectFromContext(r.Context())
		if project == nil || project.ID != "PUB" {
			http.Error(w, "missing project", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/projects/PUB/check", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q want 200", rec.Code, rec.Body.String())
	}
}

// TestWithProjectResource_PrivateAnonymousDenied confirms that the
// public fallback only fires for public projects — anonymous users
// remain locked out of private projects.
func TestWithProjectResource_PrivateAnonymousDenied(t *testing.T) {
	store := &fakeWeaveStore{
		projects: fakeProjectStore{
			byID: map[string]*domain.Project{
				"PRIV": {Entity: domain.Entity{ID: "PRIV"}, Visibility: "private"},
			},
		},
	}
	r := mountTestRouter(store, func(w http.ResponseWriter, r *http.Request) {
		var snap *AuthSnapshot
		res := ProjectResourceFromContext(r.Context())
		if !snap.Can(ProjectRead, res, nil) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/projects/PRIV/check", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 (anonymous on private project)", rec.Code)
	}
}

func TestRequireProjectRead_PrivateAnonymousDenied(t *testing.T) {
	projects := &fakeProjectStore{
		byID: map[string]*domain.Project{
			"PRIV": {Entity: domain.Entity{ID: "PRIV"}, Visibility: "private"},
		},
	}
	called := false
	r := mountProjectGateRouter(projects, RequireProjectRead, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/projects/PRIV/check", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
	if called {
		t.Fatal("handler ran despite private anonymous project")
	}
}

// TestWithProjectResource_NotFound covers the 404 path for a missing
// project. The handler should never run.
func TestWithProjectResource_NotFound(t *testing.T) {
	store := &fakeWeaveStore{projects: fakeProjectStore{byID: map[string]*domain.Project{}}}
	handlerCalled := false
	r := mountTestRouter(store, func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/projects/MISSING/check", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status=%d want 404", rec.Code)
	}
	if handlerCalled {
		t.Error("handler ran despite missing project; middleware should short-circuit")
	}
}

func TestWithProjectResource_UsesArchivedProjectWhenVersionRequested(t *testing.T) {
	store := &fakeWeaveStore{
		projects: fakeProjectStore{
			byID: map[string]*domain.Project{
				"TPC": {Entity: domain.Entity{ID: "TPC"}, Visibility: "private"},
			},
			byVersionID: map[string]*domain.Project{
				"TPC@1.0.0": {
					Entity:     domain.Entity{ID: "TPC", VersionNumber: "1.0.0"},
					Visibility: "public",
				},
			},
		},
	}
	r := mountTestRouter(store, func(w http.ResponseWriter, r *http.Request) {
		p := ProjectFromContext(r.Context())
		if p == nil {
			http.Error(w, "missing project", http.StatusInternalServerError)
			return
		}
		if p.VersionNumber != "1.0.0" {
			http.Error(w, "wrong version", http.StatusInternalServerError)
			return
		}
		res := ProjectResourceFromContext(r.Context())
		if res.Visibility != "public" {
			http.Error(w, "wrong visibility", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/projects/TPC/check?version=1.0.0", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q want 200", rec.Code, rec.Body.String())
	}
}

// TestWithProjectResource_NoProjectID confirms the middleware no-ops
// for routes without a {projectID} URL parameter (e.g. /api/v1/drafts).
// Such routes mount the middleware on a sub-mux that doesn't have a
// {projectID} segment, and the middleware passes through unchanged.
func TestWithProjectResource_NoProjectID(t *testing.T) {
	store := &fakeWeaveStore{projects: fakeProjectStore{byID: map[string]*domain.Project{}}}
	parent := chi.NewRouter()
	sub := chi.NewMux()
	sub.Use(WithProjectResource(store))
	sub.Get("/no-project-here", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	parent.Mount("/", sub)

	rec := httptest.NewRecorder()
	parent.ServeHTTP(rec, httptest.NewRequest("GET", "/no-project-here", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status=%d want 200 (middleware should be a no-op without {projectID})", rec.Code)
	}
}
