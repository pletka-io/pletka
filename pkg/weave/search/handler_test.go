package search

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/go-cmp/cmp"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

func TestParseSearchParams(t *testing.T) {
	t.Parallel()

	const testProjectID = "01HZABCDEF0000000000000001"

	tests := []struct {
		name      string
		query     string
		projectID string
		want      domain.SearchParams
	}{
		{
			name:      "empty query uses defaults",
			query:     "",
			projectID: testProjectID,
			want: domain.SearchParams{
				ProjectID:    testProjectID,
				Scope:        "project",
				Sort:         "relevance",
				Limit:        25,
				Offset:       0,
				PathFieldIDs: nil,
			},
		},
		{
			name:      "path_contains single with prefix",
			query:     "path_contains=crm:P1",
			projectID: testProjectID,
			want: domain.SearchParams{
				ProjectID:      testProjectID,
				Scope:          "project",
				Sort:           "relevance",
				Limit:          25,
				PathPrefix:     "crm",
				PathLocalName:  "P1",
				PathStartsWith: "",
			},
		},
		{
			name:      "path_contains multi element routes to PathStartsWith",
			query:     "path_contains=P1->E33",
			projectID: testProjectID,
			want: domain.SearchParams{
				ProjectID:      testProjectID,
				Scope:          "project",
				Sort:           "relevance",
				Limit:          25,
				PathStartsWith: "P1->E33",
				PathPrefix:     "",
				PathLocalName:  "",
			},
		},
		{
			name:      "explicit path_starts_with overrides path_contains",
			query:     "path_starts_with=A->B&path_contains=X",
			projectID: testProjectID,
			want: domain.SearchParams{
				ProjectID:      testProjectID,
				Scope:          "project",
				Sort:           "relevance",
				Limit:          25,
				PathStartsWith: "A->B",
				PathPrefix:     "",
				PathLocalName:  "X",
			},
		},
		{
			name:      "ontology_class bare no prefix",
			query:     "ontology_class=E21_Person",
			projectID: testProjectID,
			want: domain.SearchParams{
				ProjectID:      testProjectID,
				Scope:          "project",
				Sort:           "relevance",
				Limit:          25,
				OntologyClass:  "E21_Person",
				OntologyPrefix: "",
			},
		},
		{
			name:      "ontology_class with prefix",
			query:     "ontology_class=crm:E21_Person",
			projectID: testProjectID,
			want: domain.SearchParams{
				ProjectID:      testProjectID,
				Scope:          "project",
				Sort:           "relevance",
				Limit:          25,
				OntologyClass:  "E21_Person",
				OntologyPrefix: "crm",
			},
		},
		{
			name:      "path_depth parsed correctly",
			query:     "path_depth=4",
			projectID: testProjectID,
			want: domain.SearchParams{
				ProjectID: testProjectID,
				Scope:     "project",
				Sort:      "relevance",
				Limit:     25,
				PathDepth: 4,
			},
		},
		{
			name:      "path_depth garbage value becomes 0",
			query:     "path_depth=abc",
			projectID: testProjectID,
			want: domain.SearchParams{
				ProjectID: testProjectID,
				Scope:     "project",
				Sort:      "relevance",
				Limit:     25,
				PathDepth: 0,
			},
		},
		{
			name:      "scope garbage falls back to project",
			query:     "scope=foo",
			projectID: testProjectID,
			want: domain.SearchParams{
				ProjectID: testProjectID,
				Scope:     "project",
				Sort:      "relevance",
				Limit:     25,
			},
		},
		{
			name:      "limit over cap falls back to 25",
			query:     "limit=1000",
			projectID: testProjectID,
			want: domain.SearchParams{
				ProjectID: testProjectID,
				Scope:     "project",
				Sort:      "relevance",
				Limit:     25,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			target := "/api/v1/projects/" + tc.projectID + "/search"
			if tc.query != "" {
				target += "?" + tc.query
			}

			r := httptest.NewRequest(http.MethodGet, target, nil)
			got := parseSearchParams(r, tc.projectID)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("parseSearchParams(%q) mismatch (-want +got):\n%s", tc.query, diff)
			}
		})
	}
}

func TestReleaseModeSearchEndpointsReturnNotFound(t *testing.T) {
	t.Parallel()

	h := NewHandler(nil, slog.Default())
	r := chi.NewMux()
	r.With(weaveauth.WithProjectVersionContext).Get("/api/v1/projects/{projectID}/search", h.EntitySearch)
	r.With(weaveauth.WithProjectVersionContext).Get("/api/v1/projects/{projectID}/path-suggestions", h.PathSuggestionsHandler)

	tests := []string{
		"/api/v1/projects/TPC/search?version=0.1.3-test&type=field&q=actor",
		"/api/v1/projects/TPC/path-suggestions?version=0.1.3-test&current_path=crm:P2_has_type",
	}

	for _, target := range tests {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d body=%q", target, rec.Code, rec.Body.String())
		}
	}
}
