package detailview

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/go-chi/chi/v5"
)

// fakePreloader counts PreloadAutocomplete calls — one-method seam.
// Both fields use atomics so reads and writes are safe under -race.
type fakePreloader struct {
	calls     atomic.Int32
	projectID atomic.Pointer[string]
}

func (f *fakePreloader) PreloadAutocomplete(projectID string) {
	f.calls.Add(1)
	f.projectID.Store(&projectID)
}

// loadProjectID returns the last projectID passed to PreloadAutocomplete,
// or "" if PreloadAutocomplete was never called.
func (f *fakePreloader) loadProjectID() string {
	if p := f.projectID.Load(); p != nil {
		return *p
	}
	return ""
}

func TestPage_PreloadAutocomplete(t *testing.T) {
	const testProjectID = "01ABCDEFGHJKLMNPQRSTVWXYZ0"
	const testModelID = "LAM.1"

	// A minimal public project so the auth gate passes.
	testProject := &domain.Project{
		Visibility: "public",
	}

	tests := []struct {
		name      string
		snap      *weaveauth.AuthSnapshot
		wantCalls int32
	}{
		{
			name: "logged-in user triggers preload",
			snap: &weaveauth.AuthSnapshot{
				ActorID:      "actor-1",
				IsAnonymous:  false,
				IsSuperAdmin: true, // super-admin bypasses capability check cleanly
			},
			wantCalls: 1,
		},
		{
			name:      "anonymous user skips preload",
			snap:      &weaveauth.AuthSnapshot{IsAnonymous: true},
			wantCalls: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			preloader := &fakePreloader{}

			// Handler with nil renderer/weave — we only test the guard logic
			// before the renderer runs. The renderer will panic/fail, which
			// is caught by the httptest recorder and results in a non-200
			// status; that is acceptable because we only care about whether
			// PreloadAutocomplete was called.
			h := &Handler{
				logger:    nil,
				weave:     nil, // project comes from context — no DB call
				preloader: preloader,
			}

			// Build a chi router so URL params are available.
			r := chi.NewRouter()
			r.Get("/projects/{projectID}/models/{modelID}", func(w http.ResponseWriter, req *http.Request) {
				// Inject project + auth snapshot into context.
				ctx := req.Context()
				ctx = weaveauth.WithProject(ctx, testProject)
				ctx = weaveauth.WithSnapshot(ctx, tc.snap)
				req = req.WithContext(ctx)

				// Call Page; it will fail at the renderer but the preloader
				// guard runs before that.
				defer func() { recover() }() //nolint:errcheck // swallow renderer nil-deref
				h.Page("model")(w, req)
			})

			req := httptest.NewRequest(http.MethodGet,
				"/projects/"+testProjectID+"/models/"+testModelID, nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			got := preloader.calls.Load()
			if got != tc.wantCalls {
				t.Errorf("PreloadAutocomplete calls: got %d, want %d", got, tc.wantCalls)
			}
			if tc.wantCalls > 0 && preloader.loadProjectID() != testProjectID {
				t.Errorf("PreloadAutocomplete projectID: got %q, want %q", preloader.loadProjectID(), testProjectID)
			}
		})
	}
}
