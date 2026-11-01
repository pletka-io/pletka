package vocabulary

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

type failedSKOS struct {
	scope         auth.ReadScope
	project, list string
	calls         int
}

func (f *failedSKOS) GenerateConceptListSKOS(_ context.Context, scope auth.ReadScope, project, list string, w io.Writer) error {
	f.scope, f.project, f.list = scope, project, list
	f.calls++
	_, _ = io.WriteString(w, "partial turtle must not escape")
	return apierror.Forbidden("selected source is private")
}

func TestSKOSExportRequiresScopeAndBuffersSourceFailures(t *testing.T) {
	for _, valid := range []bool{false, true} {
		source := &failedSKOS{}
		h := skosHandler{exporter: source}
		router := chi.NewRouter()
		router.Get("/projects/{projectID}/lists/{listID}/skos", h.serve)
		ctx := auth.WithProject(t.Context(), &domain.Project{Entity: domain.Entity{ID: "P"}, Visibility: "public"})
		if valid {
			ctx = auth.WithReadScope(ctx, auth.Release("7.1.3"))
		}
		out := httptest.NewRecorder()
		router.ServeHTTP(out, httptest.NewRequestWithContext(ctx, http.MethodGet, "/projects/P/lists/L/skos", nil))
		if strings.Contains(out.Body.String(), "partial turtle") || out.Header().Get("Content-Disposition") != "" {
			t.Fatal("failed export published partial download")
		}
		if !valid {
			if out.Code != 500 || source.calls != 0 {
				t.Fatalf("invalid scope reached source: %d %d", out.Code, source.calls)
			}
		} else if out.Code != 403 || source.calls != 1 || source.scope.Version() != "7.1.3" || source.project != "P" || source.list != "L" || !strings.Contains(out.Body.String(), `"code":"forbidden"`) {
			t.Fatalf("selection/error lost: %+v %d %s", source, out.Code, out.Body.String())
		}
	}
}
