package exports

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestMountRegistersPerEntityRoute verifies the slice mounts the
// per-entity CSV route. The whole-project page + per-type CSVs are
// owned by pkg/weave/project/csvexport and not tested here.
//
// The handler 503s when h.service is nil. That's enough to confirm the
// route matches and the gate (which only runs when h.projects is set)
// is bypassed in tests.
func TestMountRegistersPerEntityRoute(t *testing.T) {
	h := NewHandler(nil, nil, nil)
	r := chi.NewMux()
	h.Mount(r)

	req := httptest.NewRequest(http.MethodGet, "/models/LAM.1.csv", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want route to match and reach handler", rec.Code)
	}
}

// pe is a small helper for tests in this package that need to construct
// a PathElement value.
func pe(kind, prefix, localName string, position int) domain.PathElement {
	return domain.PathElement{
		Type:      kind,
		URI:       prefix + ":" + localName,
		Prefix:    prefix,
		LocalName: localName,
		Position:  position,
	}
}

var _ = pe // silence unused-helper warning when no test references it
