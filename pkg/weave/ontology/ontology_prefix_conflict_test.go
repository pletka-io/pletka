package ontology

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
)

// uniqueViolationStore fails every ontology write with a Postgres unique
// violation, the way a taken prefix does (the id is derived from the prefix,
// so the collision lands on the primary key or on the prefix key).
type uniqueViolationStore struct{ *routeTestStore }

func (uniqueViolationStore) CreateOntology(context.Context, CreateOntologyInput) (*domain.Ontology, error) {
	return nil, &pgconn.PgError{Code: "23505", ConstraintName: "weave_ontologies_pkey"}
}

func (uniqueViolationStore) UpdateOntology(context.Context, string, UpdateOntologyInput) (*domain.Ontology, error) {
	return nil, &pgconn.PgError{Code: "23505", ConstraintName: "weave_ontologies_prefix_key"}
}

func TestOntologyWriteWithTakenPrefixReturnsPrefixFieldError(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
	}{
		{name: "create", method: http.MethodPost, path: "/admin/ontologies"},
		{name: "update", method: http.MethodPut, path: "/admin/ontologies/ont-crm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := chi.NewRouter()
			handler := NewHandler(NewService(uniqueViolationStore{newRouteTestStore()}, nil, nil), nil, nil, nil)
			router.Route("/admin/ontologies", handler.Mount)

			body := bytes.NewBufferString(`{"prefix":"crm","namespace":"http://example.org/x#","name":"Dup"}`)
			ctx := auth.WithSnapshot(context.Background(), &auth.AuthSnapshot{IsSuperAdmin: true})
			req := httptest.NewRequestWithContext(ctx, tc.method, tc.path, body)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status=%d want 422, body=%s", rec.Code, rec.Body.String())
			}
			var got struct {
				Errors map[string][]string `json:"errors"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode body: %v (%s)", err, rec.Body.String())
			}
			if len(got.Errors["prefix"]) == 0 {
				t.Fatalf("want a prefix field error, got %v", got.Errors)
			}
		})
	}
}

type failingDeleteStore struct{ *routeTestStore }

func (failingDeleteStore) DeleteOntology(context.Context, string) error {
	return errors.New("delete ontology: connection reset")
}

func TestOntologyServiceErrorRecordsCause(t *testing.T) {
	router := chi.NewRouter()
	handler := NewHandler(NewService(failingDeleteStore{newRouteTestStore()}, nil, nil), nil, nil, nil)
	router.Route("/admin/ontologies", handler.Mount)

	ctx, cause := errresp.WithCauseSlot(auth.WithSnapshot(context.Background(), &auth.AuthSnapshot{IsSuperAdmin: true}))
	req := httptest.NewRequestWithContext(ctx, http.MethodDelete, "/admin/ontologies/ont-crm", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500, body=%s", rec.Code, rec.Body.String())
	}
	if cause.Err() == nil || cause.Err().Error() != "delete ontology: connection reset" {
		t.Fatalf("cause=%v, want the store error", cause.Err())
	}
}
