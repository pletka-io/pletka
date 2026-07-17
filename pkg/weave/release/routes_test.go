package release

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func TestMountedReleaseRootPostIsCanonicalWithoutTrailingSlash(t *testing.T) {
	t.Parallel()

	var creates, lists int

	parent := chi.NewRouter()
	parent.Use(middleware.RedirectSlashes)

	sub := chi.NewRouter()
	sub.Get("/", func(w http.ResponseWriter, r *http.Request) {
		lists++
		w.WriteHeader(http.StatusOK)
	})
	sub.Post("/", func(w http.ResponseWriter, r *http.Request) {
		creates++
		w.WriteHeader(http.StatusCreated)
	})

	parent.Mount("/projects/{projectID}/releases", sub)

	t.Run("bare", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/projects/TPD/releases", nil)
		rr := httptest.NewRecorder()

		parent.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("expected %d, got %d", http.StatusCreated, rr.Code)
		}
	})

	t.Run("slash_redirects_before_subrouter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/projects/TPD/releases/", nil)
		rr := httptest.NewRecorder()

		parent.ServeHTTP(rr, req)

		if rr.Code != http.StatusMovedPermanently {
			t.Fatalf("expected %d, got %d", http.StatusMovedPermanently, rr.Code)
		}
		if got := rr.Header().Get("Location"); got != "/projects/TPD/releases" {
			t.Fatalf("expected redirect to canonical bare path, got %q", got)
		}
	})

	if creates != 1 {
		t.Fatalf("expected 1 create hit, got %d", creates)
	}
	if lists != 0 {
		t.Fatalf("expected 0 list hits, got %d", lists)
	}
}
