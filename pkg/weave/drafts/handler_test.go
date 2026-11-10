package drafts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
)

// TestCreate_ModelAndCollectionNeedScope pins that a model or collection
// draft without an ontology scope is refused before anything is read or
// written: a scope-less collection made this way fails strict loading of
// the tree it is materialized into.
func TestCreate_ModelAndCollectionNeedScope(t *testing.T) {
	h := NewHandler(nil, nil)
	ctx := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{ActorID: "u1"})
	for _, body := range []string{
		`{"type":"collection","project_id":"X","name":{"en":"TimeSpan"}}`,
		`{"type":"model","project_id":"X","name":{"en":"Event"}}`,
		`{"type":"collection","project_id":"X","name":{"en":"TimeSpan"},"ontology_scope":{"prefix":"crm","local_name":""}}`,
	} {
		rec := httptest.NewRecorder()
		h.Create(rec, httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/drafts", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: status %d, want a validation error", body, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "ontology_scope") {
			t.Fatalf("%s: response does not name ontology_scope: %s", body, rec.Body.String())
		}
	}
}
