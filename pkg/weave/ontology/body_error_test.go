package ontology

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pletka-io/pletka/pkg/database/ontologybody"
)

func TestSealedBodyWriteUsesCanonicalConflict(t *testing.T) {
	cause := &pgconn.PgError{Code: "23514", ConstraintName: "ontology_body_sealed"}
	err := ontologybody.MapError(cause)
	if !errors.Is(err, cause) {
		t.Fatal("database cause lost")
	}
	h := NewHandler(nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/admin/ontologies/example", nil)
	h.writeServiceError(w, r, err, "version")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"code":"conflict"`) {
		t.Fatalf("sealed write response: %d %s", w.Code, w.Body.String())
	}
}
