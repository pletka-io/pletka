package visualization

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/weave/generators"
)

func TestSPARQLKindOption(t *testing.T) {
	for _, tt := range []struct {
		query     string
		wantCount bool
		wantLimit int
	}{
		{"", false, 100},
		{"kind=entries", false, 100},
		{"kind=count", true, 0},
		{"count=1", true, 0},
	} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x?"+tt.query, nil)
		opts, err := sparqlOptionsFromRequest(r, generators.EntityField)
		if err != nil {
			t.Fatalf("%q: %v", tt.query, err)
		}
		if opts.SPARQLCount != tt.wantCount || opts.SPARQLLimit != tt.wantLimit {
			t.Fatalf("%q: count=%v limit=%d", tt.query, opts.SPARQLCount, opts.SPARQLLimit)
		}
	}
}

func TestSPARQLKindRejectsUnofferedKind(t *testing.T) {
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x?kind=coverage", nil)
	_, err := sparqlOptionsFromRequest(r, generators.EntityField)
	if err == nil || err.Error() != "kind must be one of entries, count" {
		t.Fatalf("err = %v", err)
	}
}

func TestSPARQLKindsRoute(t *testing.T) {
	h := &Handler{}
	get := func(target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.GetSPARQLKinds(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
		return w
	}
	w := get("/gen/sparql/kinds?level=model")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"level":"model","kinds":["entries","count"]}` {
		t.Fatalf("body %s", got)
	}
	for _, target := range []string{"/gen/sparql/kinds?level=project", "/gen/sparql/kinds"} {
		if w := get(target); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d", target, w.Code)
		}
	}
}
