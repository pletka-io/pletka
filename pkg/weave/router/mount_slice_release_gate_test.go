package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// releaseGateWeaveStore is a minimal domain.WeaveStore that only implements
// Projects().GetByID — enough for auth.WithProjectResource to run. Other
// methods panic; this test never reaches them.
type releaseGateWeaveStore struct {
	domain.WeaveStore
	projects releaseGateProjectStore
}

func (f *releaseGateWeaveStore) Projects() domain.WeaveProjectStore { return &f.projects }

type releaseGateProjectStore struct {
	domain.WeaveProjectStore
	project *domain.Project
}

func (f *releaseGateProjectStore) GetByID(_ context.Context, _ string) (*domain.Project, error) {
	return f.project, nil
}

type releaseGateReader struct{ version string }

func (r releaseGateReader) LatestReleaseVersion(context.Context, string) (string, error) {
	return r.version, nil
}

// TestMountSlice_ExportsExcludedFromReleaseDefault proves the guard the
// exports/csvexport mount relies on: router.Mount clears LatestRelease on
// the ProjectMiddlewareHost it passes for /projects/{projectID}/exports
// (unlike every other mountSlice call, which shares the same host with a
// non-nil LatestRelease), and mountSlice only installs
// auth.ResolveContentVersion when LatestRelease is non-nil. CSV export is
// the member-only draft-verification tool (design spec §5) and must always
// reflect hot, never the release default.
func TestMountSlice_ExportsExcludedFromReleaseDefault(t *testing.T) {
	project := &domain.Project{Entity: domain.Entity{ID: "PUB"}, Visibility: "public"}
	weave := &releaseGateWeaveStore{projects: releaseGateProjectStore{project: project}}
	reader := releaseGateReader{version: "9.9.9"}

	// capture wires a stub attach handler that records the resolved
	// project version off the request context, standing in for a real
	// slice's Mount func.
	capture := func() (func(chi.Router), *string) {
		got := new(string)
		attach := func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, req *http.Request) {
				*got = auth.ProjectVersionFromContext(req.Context())
				w.WriteHeader(http.StatusOK)
			})
		}
		return attach, got
	}

	// Case 1: a normal mountSlice call (LatestRelease set, as every slice
	// except exports/csvexport receives it) resolves an anonymous/
	// non-editor reader of a public project to the latest release.
	attach, got := capture()
	h := ProjectMiddlewareHost{Weave: weave, LatestRelease: reader}
	parent := chi.NewMux()
	mountSlice(parent, "/projects/{projectID}/fields", h, attach)

	req := httptest.NewRequest(http.MethodGet, "/projects/PUB/fields/", nil)
	rec := httptest.NewRecorder()
	parent.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("normal slice mount: status = %d", rec.Code)
	}
	if *got != "9.9.9" {
		t.Fatalf("normal slice mount: resolved version = %q, want %q", *got, "9.9.9")
	}

	// Case 2: the exports mount pattern — LatestRelease cleared, exactly as
	// router.Mount does before mounting csvexport+exports — must NOT
	// resolve a version. The route stays on hot.
	attach2, got2 := capture()
	hExports := ProjectMiddlewareHost{Weave: weave} // LatestRelease intentionally nil
	parent2 := chi.NewMux()
	mountSlice(parent2, "/projects/{projectID}/exports", hExports, attach2)

	req2 := httptest.NewRequest(http.MethodGet, "/projects/PUB/exports/", nil)
	rec2 := httptest.NewRecorder()
	parent2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("exports mount: status = %d", rec2.Code)
	}
	if *got2 != "" {
		t.Fatalf("exports mount: resolved version = %q, want hot (empty)", *got2)
	}
}
