package ontology_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

type schemaLookupStore struct {
	weaveontology.Store
	err error
}

func (s schemaLookupStore) GetOntology(context.Context, string) (*domain.Ontology, error) {
	return nil, s.err
}

func (s schemaLookupStore) GetVersion(context.Context, string) (*domain.OntologyVersion, error) {
	return nil, s.err
}

func TestFormSchemasPreserveLookupErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{name: "missing", status: http.StatusNotFound},
		{name: "unavailable", err: errors.New("database unavailable"), status: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := weaveontology.NewHandler(weaveontology.NewService(schemaLookupStore{err: tc.err}, nil, nil), nil, nil, nil)
			for name, handler := range map[string]http.HandlerFunc{
				"ontology": h.FormSchema,
				"version":  h.VersionFormSchema,
			} {
				t.Run(name, func(t *testing.T) {
					r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/form-schema?mode=edit&entity_id=missing", nil)
					w := httptest.NewRecorder()
					handler(w, r)
					if w.Code != tc.status {
						t.Fatalf("status=%d, want %d; body=%s", w.Code, tc.status, w.Body.String())
					}
				})
			}
		})
	}
}
