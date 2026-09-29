package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

var errReaderBoom = errors.New("latest release lookup boom")

// fakeReleaseReader is a test double for LatestReleaseReader.
type fakeReleaseReader struct {
	version string
	err     error
	// failIfCalled makes the test fail if LatestReleaseVersion is invoked —
	// used to assert the reader is not consulted (explicit ?version=).
	failIfCalled *testing.T
}

func (f *fakeReleaseReader) LatestReleaseVersion(_ context.Context, _ string) (string, error) {
	if f.failIfCalled != nil {
		f.failIfCalled.Fatal("LatestReleaseVersion should not be called")
	}
	return f.version, f.err
}

func versionEchoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ProjectVersionFromContext(r.Context())))
	})
}

func readScopeEchoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ReadScopeFromContext(r.Context()).String()))
	})
}

func newRequestWithContext(t *testing.T, method, target string, project *domain.Project, snap *AuthSnapshot) *http.Request {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, target, nil)
	ctx := req.Context()
	ctx = WithProject(ctx, project)
	ctx = WithSnapshot(ctx, snap)
	return req.WithContext(ctx)
}

func TestResolveContentVersion_PublicAnonymousGET_NoExplicitVersion_UsesLatest(t *testing.T) {
	project := &domain.Project{Entity: domain.Entity{ID: "P"}, OwnerID: "P", Visibility: "public"}
	snap := &AuthSnapshot{IsAnonymous: true}
	reader := &fakeReleaseReader{version: "1.2.0"}

	req := newRequestWithContext(t, http.MethodGet, "/x", project, snap)
	w := httptest.NewRecorder()

	ResolveContentVersion(reader)(versionEchoHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != "1.2.0" {
		t.Fatalf("body = %q, want %q", got, "1.2.0")
	}
}

func TestResolveContentVersion_PublicOwnerGET_ServesHot(t *testing.T) {
	project := &domain.Project{Entity: domain.Entity{ID: "P"}, OwnerID: "P", Visibility: "public"}
	snap := &AuthSnapshot{OwnedProjectIDs: map[string]struct{}{"P": {}}}
	reader := &fakeReleaseReader{version: "1.2.0"}

	req := newRequestWithContext(t, http.MethodGet, "/x", project, snap)
	w := httptest.NewRecorder()

	ResolveContentVersion(reader)(versionEchoHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != "" {
		t.Fatalf("body = %q, want empty (hot)", got)
	}
}

func TestResolveContentVersion_ExplicitVersion_PassthroughReaderNotConsulted(t *testing.T) {
	project := &domain.Project{Entity: domain.Entity{ID: "P"}, OwnerID: "P", Visibility: "public"}
	snap := &AuthSnapshot{IsAnonymous: true}
	reader := &fakeReleaseReader{failIfCalled: t}

	// Simulate WithProjectVersionContext having already run upstream (as it
	// does in the real chain) and seeded the requested version into context
	// from ?version=0.1.0. ResolveContentVersion must leave it untouched and
	// must not consult the reader.
	req := newRequestWithContext(t, http.MethodGet, "/x?version=0.1.0", project, snap)
	req = req.WithContext(WithProjectVersion(req.Context(), "0.1.0"))
	w := httptest.NewRecorder()

	ResolveContentVersion(reader)(versionEchoHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != "0.1.0" {
		t.Fatalf("body = %q, want %q", got, "0.1.0")
	}
}

func TestResolveContentVersion_UnsafeMethod_NeverVersions(t *testing.T) {
	project := &domain.Project{Entity: domain.Entity{ID: "P"}, OwnerID: "P", Visibility: "public"}
	snap := &AuthSnapshot{IsAnonymous: true}
	reader := &fakeReleaseReader{failIfCalled: t}

	req := newRequestWithContext(t, http.MethodPost, "/x", project, snap)
	w := httptest.NewRecorder()

	ResolveContentVersion(reader)(versionEchoHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != "" {
		t.Fatalf("body = %q, want empty (hot)", got)
	}
}

func TestResolveContentVersion_PrivateMemberGET_ServesHot(t *testing.T) {
	project := &domain.Project{Entity: domain.Entity{ID: "P"}, OwnerID: "OWNER", Visibility: "private"}
	snap := &AuthSnapshot{Roles: map[string]string{"project:P": "contributor"}}
	reader := &fakeReleaseReader{failIfCalled: t}

	req := newRequestWithContext(t, http.MethodGet, "/x", project, snap)
	w := httptest.NewRecorder()

	ResolveContentVersion(reader)(versionEchoHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != "" {
		t.Fatalf("body = %q, want empty (hot)", got)
	}
}

// TestResolveContentVersion_InstallsReadScope covers every outcome of the
// version-resolution chain (WithProjectVersionContext + ResolveContentVersion)
// and asserts the ReadScope each one installs on the context. The three
// Draft() cases (non-public, public editor, lookup failure) are the ones a
// caller reading through ReadScopeFromContext instead of
// ProjectVersionFromContext would previously see as the invalid zero value —
// this is what makes them load-bearing rather than incidental coverage.
func TestResolveContentVersion_InstallsReadScope(t *testing.T) {
	publicProject := &domain.Project{Entity: domain.Entity{ID: "P"}, OwnerID: "P", Visibility: "public"}
	privateProject := &domain.Project{Entity: domain.Entity{ID: "P"}, OwnerID: "OWNER", Visibility: "private"}

	cases := []struct {
		name    string
		target  string
		project *domain.Project
		snap    *AuthSnapshot
		reader  *fakeReleaseReader
		want    string
	}{
		{
			name:    "explicit version",
			target:  "/x?version=0.1.0",
			project: publicProject,
			snap:    &AuthSnapshot{IsAnonymous: true},
			reader:  &fakeReleaseReader{failIfCalled: t},
			want:    "release 0.1.0",
		},
		{
			name:    "non-public project",
			target:  "/x",
			project: privateProject,
			snap:    &AuthSnapshot{Roles: map[string]string{"project:P": "contributor"}},
			reader:  &fakeReleaseReader{failIfCalled: t},
			want:    "draft",
		},
		{
			name:    "public editor",
			target:  "/x",
			project: publicProject,
			snap:    &AuthSnapshot{OwnedProjectIDs: map[string]struct{}{"P": {}}},
			reader:  &fakeReleaseReader{version: "1.2.0"},
			want:    "draft",
		},
		{
			name:    "latest release lookup fails",
			target:  "/x",
			project: publicProject,
			snap:    &AuthSnapshot{IsAnonymous: true},
			reader:  &fakeReleaseReader{err: errReaderBoom},
			want:    "draft",
		},
		{
			name:    "public non-editor with a release",
			target:  "/x",
			project: publicProject,
			snap:    &AuthSnapshot{IsAnonymous: true},
			reader:  &fakeReleaseReader{version: "1.2.0"},
			want:    "release 1.2.0",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := newRequestWithContext(t, http.MethodGet, c.target, c.project, c.snap)
			w := httptest.NewRecorder()

			WithProjectVersionContext(ResolveContentVersion(c.reader)(readScopeEchoHandler())).ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			if got := w.Body.String(); got != c.want {
				t.Errorf("ReadScope = %q, want %q", got, c.want)
			}
		})
	}
}

// TestWithProjectVersionContext_InstallsReadScopeAlone covers the routes
// that carry WithProjectVersionContext but never carry ResolveContentVersion
// — e.g. pkg/weave/project/routes.go's mountAt, which the override slice's
// own routes go through. On those routes a request without ?version= must
// still see Draft(), not the invalid zero value: ResolveContentVersion is
// not there to fill the gap.
func TestWithProjectVersionContext_InstallsReadScopeAlone(t *testing.T) {
	cases := []struct {
		name   string
		target string
		want   string
	}{
		{"no explicit version", "/x", "draft"},
		{"explicit version", "/x?version=0.1.0", "release 0.1.0"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, c.target, nil)
			w := httptest.NewRecorder()

			WithProjectVersionContext(readScopeEchoHandler()).ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			if got := w.Body.String(); got != c.want {
				t.Errorf("ReadScope = %q, want %q", got, c.want)
			}
		})
	}
}

func TestResolveContentVersion_ReaderError_FailsSafeToHot(t *testing.T) {
	project := &domain.Project{Entity: domain.Entity{ID: "P"}, OwnerID: "P", Visibility: "public"}
	snap := &AuthSnapshot{IsAnonymous: true}
	reader := &fakeReleaseReader{err: errReaderBoom}

	req := newRequestWithContext(t, http.MethodGet, "/x", project, snap)
	w := httptest.NewRecorder()

	ResolveContentVersion(reader)(versionEchoHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (fail-safe, not 500)", w.Code)
	}
	if got := w.Body.String(); got != "" {
		t.Fatalf("body = %q, want empty (hot)", got)
	}
}
