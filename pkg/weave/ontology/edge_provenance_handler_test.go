package ontology_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// ---------------------------------------------------------------------------
// Admin cache drop/stats handler tests (unit, no DB)
// ---------------------------------------------------------------------------

func TestAdminDropAutocompleteCache_Returns204(t *testing.T) {
	store := newRouteTestStore()
	svc := weaveontology.NewService(store, nil, nil)

	router := chi.NewMux()
	h := weaveontology.NewHandler(svc, nil, nil, nil)
	router.Post("/admin/autocomplete-cache/drop", h.DropAutocompleteCache)

	req := httptest.NewRequest(http.MethodPost, "/admin/autocomplete-cache/drop", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204", rr.Code)
	}
}

// fakeProjectsForDrop is a minimal ProjectReader that always returns one
// ontology version link so NewService wires a real IndexCache.
type fakeProjectsForDrop struct{}

func (f *fakeProjectsForDrop) ResolvedOntologyVersions(
	_ context.Context, _ string, _ domain.ResolvedOntologyVersionOpts,
) ([]domain.ResolvedOntologyVersion, error) {
	return []domain.ResolvedOntologyVersion{
		{
			Link: &domain.ProjectOntologyVersion{
				OntologyVersionID: "v-drop-test",
				IsPrimary:         true,
			},
			SourceProjectID: "",
		},
	}, nil
}

// TestAdminDropAutocompleteCache_ActuallyClearsCache builds a Service with a
// wired IndexCache, seeds an entry via SeedForTest, calls the drop handler,
// asserts 204, and confirms the cache is empty afterwards.
func TestAdminDropAutocompleteCache_ActuallyClearsCache(t *testing.T) {
	store := newRouteTestStore()
	projects := &fakeProjectsForDrop{}
	svc := weaveontology.NewService(store, projects, nil)

	// Seed an entry directly so we don't need a real DB.
	svc.SeedCacheEntryForTest("rel-drop-test")

	// Verify the entry is present before the drop.
	if n := len(svc.AutocompleteCacheStats()); n != 1 {
		t.Fatalf("expected 1 cache entry before drop, got %d", n)
	}

	router := chi.NewMux()
	h := weaveontology.NewHandler(svc, nil, nil, nil)
	router.Post("/admin/autocomplete-cache/drop", h.DropAutocompleteCache)

	req := httptest.NewRequest(http.MethodPost, "/admin/autocomplete-cache/drop", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204", rr.Code)
	}

	// The cache must be empty after drop.
	if n := len(svc.AutocompleteCacheStats()); n != 0 {
		t.Errorf("cache should be empty after drop, got %d entries", n)
	}
}

func TestAdminAutocompleteStats_ReturnsJSONArray(t *testing.T) {
	store := newRouteTestStore()
	svc := weaveontology.NewService(store, nil, nil)

	router := chi.NewMux()
	h := weaveontology.NewHandler(svc, nil, nil, nil)
	router.Get("/admin/autocomplete-cache/stats", h.AutocompleteCacheStats)

	req := httptest.NewRequest(http.MethodGet, "/admin/autocomplete-cache/stats", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("content-type=%q want application/json", ct)
	}
	var got []any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v — body: %s", err, rr.Body.String())
	}
	// When indexCache is nil (no projects wired), AutocompleteCacheStats returns
	// nil which the handler promotes to an empty JSON array.
	if got == nil {
		t.Fatal("expected non-nil JSON array (got null)")
	}
}

// ---------------------------------------------------------------------------
// Edge-provenance handler unit test — missing params → 400
// ---------------------------------------------------------------------------

func TestEdgeProvenance_MissingParams_Returns400(t *testing.T) {
	store := newRouteTestStore()
	svc := weaveontology.NewService(store, nil, nil)

	router := chi.NewMux()
	h := weaveontology.NewHandler(svc, nil, nil, nil)
	router.Get("/api/v1/ontology/edge-provenance", h.EdgeProvenance)

	for _, tc := range []struct {
		name string
		url  string
	}{
		{"no params", "/api/v1/ontology/edge-provenance"},
		{"missing target", "/api/v1/ontology/edge-provenance?project_id=X&source=crm:E1&rel=subclass_of"},
		{"missing source", "/api/v1/ontology/edge-provenance?project_id=X&target=crm:E2&rel=subclass_of"},
		{"missing rel", "/api/v1/ontology/edge-provenance?project_id=X&source=crm:E1&target=crm:E2"},
		{"missing project_id", "/api/v1/ontology/edge-provenance?source=crm:E1&target=crm:E2&rel=subclass_of"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want 400 (url=%s)", rr.Code, tc.url)
			}
			ct := rr.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Fatalf("content-type=%q want application/json (url=%s)", ct, tc.url)
			}
			var body map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error body: %v — %s", err, rr.Body.String())
			}
			if body["error"] == "" {
				t.Fatalf("expected error field in body, got %v", body)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers — re-expose routeTestStore via the external test package.
// The routeTestStore lives in page_routes_test.go (package ontology, not
// ontology_test), so we declare a thin local store stub here that satisfies
// Store for the unit tests above that don't need real data.
// ---------------------------------------------------------------------------

type routeTestStore = testStoreAdapter

// testStoreAdapter is a zero-value stub that satisfies weaveontology.Store.
// Methods not needed by the unit tests above return zero values.
type testStoreAdapter struct{}

func newRouteTestStore() *testStoreAdapter { return &testStoreAdapter{} }

func (s *testStoreAdapter) GetFamily(context.Context, string) (*domain.OntologyFamily, error) {
	return nil, nil
}
func (s *testStoreAdapter) GetFamilyBySlug(context.Context, string) (*domain.OntologyFamily, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListFamilies(context.Context) ([]*domain.OntologyFamily, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListRootFamilies(context.Context) ([]*domain.OntologyFamily, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListChildFamilies(context.Context, string) ([]*domain.OntologyFamily, error) {
	return nil, nil
}
func (s *testStoreAdapter) GetOntology(context.Context, string) (*domain.Ontology, error) {
	return nil, nil
}
func (s *testStoreAdapter) GetOntologyByPrefix(context.Context, string) (*domain.Ontology, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListOntologies(context.Context) ([]*domain.Ontology, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListOntologiesByFamily(context.Context, string) ([]*domain.Ontology, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListOntologiesByType(context.Context, domain.OntologyType) ([]*domain.Ontology, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListOntologyExtensions(context.Context, string) ([]*domain.Ontology, error) {
	return nil, nil
}
func (s *testStoreAdapter) SearchOntologies(context.Context, string, int) ([]*domain.Ontology, error) {
	return nil, nil
}
func (s *testStoreAdapter) NamespaceBindings(context.Context) ([]weaveontology.NamespaceBinding, error) {
	return nil, nil
}
func (s *testStoreAdapter) UpsertNamespaceBindings(context.Context, []weaveontology.NamespaceBinding) error {
	return nil
}
func (s *testStoreAdapter) DeleteNamespaceBindingsBySource(context.Context, string) error {
	return nil
}
func (s *testStoreAdapter) GetVersion(context.Context, string) (*domain.OntologyVersion, error) {
	return nil, nil
}
func (s *testStoreAdapter) GetVersionByOntologyAndString(context.Context, string, string) (*domain.OntologyVersion, error) {
	return nil, nil
}
func (s *testStoreAdapter) GetActiveVersion(context.Context, string) (*domain.OntologyVersion, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListVersionsByOntology(context.Context, string) ([]*domain.OntologyVersion, error) {
	return nil, nil
}
func (s *testStoreAdapter) VersionUsageCount(context.Context, string) (int64, error) { return 0, nil }
func (s *testStoreAdapter) ListProjectsUsingVersion(context.Context, string, int) ([]weaveontology.VersionProjectUsage, error) {
	return nil, nil
}
func (s *testStoreAdapter) VersionUsageCountsForOntology(context.Context, string) (map[string]int64, error) {
	return nil, nil
}
func (s *testStoreAdapter) GetClass(context.Context, string) (*domain.OntologyClass, error) {
	return nil, nil
}
func (s *testStoreAdapter) GetClassByURI(context.Context, string, string) (*domain.OntologyClass, error) {
	return nil, nil
}
func (s *testStoreAdapter) GetClassByQname(context.Context, string, string) (*domain.OntologyClass, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListClassesByVersion(context.Context, string) ([]*domain.OntologyClass, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListClassesByQnames(context.Context, string, []string) ([]*domain.OntologyClass, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListSubclassesByQname(context.Context, string, string) ([]*domain.OntologyClass, error) {
	return nil, nil
}
func (s *testStoreAdapter) SearchClasses(context.Context, string, string, int) ([]*domain.OntologyClass, error) {
	return nil, nil
}
func (s *testStoreAdapter) GetProperty(context.Context, string) (*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *testStoreAdapter) GetPropertyByURI(context.Context, string, string) (*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *testStoreAdapter) GetPropertyByQname(context.Context, string, string) (*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListPropertiesByVersion(context.Context, string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListPropertiesByQnames(context.Context, string, []string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *testStoreAdapter) SearchProperties(context.Context, string, string, int) ([]*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *testStoreAdapter) PropertiesForDomainQname(context.Context, string, string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *testStoreAdapter) PropertiesForRangeQname(context.Context, string, string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *testStoreAdapter) GetInverseProperty(context.Context, string, string) (*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListRelationsForSource(context.Context, string, string) ([]*domain.OntologyRelation, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListRelationsForSourceByType(context.Context, string, string, string) ([]*domain.OntologyRelation, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListRelationsForTarget(context.Context, string, string) ([]*domain.OntologyRelation, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListClassesByVersions(context.Context, []string) ([]*domain.OntologyClass, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListPropertiesByVersions(context.Context, []string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListRelationsByVersionsAndTypes(context.Context, []string, []string) ([]*domain.OntologyRelationWithSource, error) {
	return nil, nil
}
func (s *testStoreAdapter) ListFieldRefs(context.Context, string) ([]*domain.FieldOntologyRef, error) {
	return nil, nil
}
func (s *testStoreAdapter) FindFieldsUsingQname(context.Context, string, string, int) ([]string, error) {
	return nil, nil
}
func (s *testStoreAdapter) CountFieldsUsingQname(context.Context, string, string) (int64, error) {
	return 0, nil
}
func (s *testStoreAdapter) CountFieldsUsingVersion(context.Context, string, string) (int64, error) {
	return 0, nil
}
func (s *testStoreAdapter) SampleFieldsUsingVersion(context.Context, string, string, int) ([]weaveontology.FieldVersionRef, error) {
	return nil, nil
}
func (s *testStoreAdapter) CreateFamily(context.Context, weaveontology.CreateFamilyInput) (*domain.OntologyFamily, error) {
	return nil, nil
}
func (s *testStoreAdapter) UpdateFamily(context.Context, string, weaveontology.UpdateFamilyInput) (*domain.OntologyFamily, error) {
	return nil, nil
}
func (s *testStoreAdapter) DeleteFamily(context.Context, string) error { return nil }
func (s *testStoreAdapter) CountOntologiesInFamily(context.Context, string) (int64, error) {
	return 0, nil
}
func (s *testStoreAdapter) CreateOntology(context.Context, weaveontology.CreateOntologyInput) (*domain.Ontology, error) {
	return nil, nil
}
func (s *testStoreAdapter) UpdateOntology(context.Context, string, weaveontology.UpdateOntologyInput) (*domain.Ontology, error) {
	return nil, nil
}
func (s *testStoreAdapter) DeleteOntology(context.Context, string) error { return nil }
func (s *testStoreAdapter) CountOntologyVersions(context.Context, string) (int64, error) {
	return 0, nil
}
func (s *testStoreAdapter) CreateVersion(context.Context, weaveontology.CreateVersionInput) (*domain.OntologyVersion, error) {
	return nil, nil
}
func (s *testStoreAdapter) UpdateVersionMetadata(context.Context, string, weaveontology.UpdateVersionMetadataInput) (*domain.OntologyVersion, error) {
	return nil, nil
}
func (s *testStoreAdapter) UpdateVersionCounts(context.Context, string, int64, int64) error {
	return nil
}
func (s *testStoreAdapter) SetActiveVersion(context.Context, string, string) error { return nil }
func (s *testStoreAdapter) DeleteVersion(context.Context, string) error            { return nil }
func (s *testStoreAdapter) ImportVersion(context.Context, weaveontology.ImportVersionInput) (*domain.OntologyVersion, error) {
	return nil, nil
}
func (s *testStoreAdapter) ReplaceFieldRefs(context.Context, string, string, []domain.FieldOntologyRef) error {
	return nil
}
