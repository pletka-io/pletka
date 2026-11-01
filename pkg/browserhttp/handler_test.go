package browserhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/domain"
)

type recordingSource struct {
	calls int
	value browserinput.Inventory
	err   error
	scope auth.ReadScope
}

func (s *recordingSource) Inventory(_ context.Context, scope auth.ReadScope, _, _ string, _ browserinput.InventoryQuery) (browserinput.Inventory, error) {
	s.calls++
	s.scope = scope
	return s.value, s.err
}

func TestInventoryRejectsMissingScopeWrongRootAndMismatchedSelection(t *testing.T) {
	for _, tc := range []struct {
		name, project, visibility string
		scope                     auth.ReadScope
		want                      int
		calls                     int
	}{
		{"zero scope", "P", "public", auth.ReadScope{}, 500, 0},
		{"wrong root", "OTHER", "public", auth.Release("1.0.0"), 404, 0},
		{"private root", "P", "private", auth.Release("1.0.0"), 404, 0},
		{"wrong returned selection", "P", "public", auth.Release("2.0.0"), 500, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := &recordingSource{value: browserinput.Inventory{ProjectID: "P", Version: "1.0.0", Kind: "model"}}
			h := Handler{Source: source}
			router := chi.NewRouter()
			router.Get("/projects/{projectID}/models", func(w http.ResponseWriter, r *http.Request) { h.List(w, r, "model") })
			ctx := auth.WithReadScope(auth.WithProject(t.Context(), &domain.Project{Entity: domain.Entity{ID: tc.project}, Visibility: tc.visibility}), tc.scope)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequestWithContext(ctx, http.MethodGet, "/projects/P/models", nil))
			if w.Code != tc.want || source.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, source.calls, w.Body.String())
			}
		})
	}
}

func TestInventorySourceFailureDoesNotReturnPartialRows(t *testing.T) {
	source := &recordingSource{value: browserinput.Inventory{ProjectID: "P", Version: "1.0.0", Kind: "field", Items: []browserinput.InventoryRow{{ID: "partial"}}}, err: errors.New("required read failed")}
	h := Handler{Source: source}
	router := chi.NewRouter()
	router.Get("/projects/{projectID}/fields", func(w http.ResponseWriter, r *http.Request) { h.List(w, r, "field") })
	ctx := auth.WithReadScope(auth.WithProject(t.Context(), &domain.Project{Entity: domain.Entity{ID: "P"}, Visibility: "public"}), auth.Release("1.0.0"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequestWithContext(ctx, http.MethodGet, "/projects/P/fields", nil))
	if w.Code != 500 || source.calls != 1 {
		t.Fatalf("status=%d calls=%d", w.Code, source.calls)
	}
}
