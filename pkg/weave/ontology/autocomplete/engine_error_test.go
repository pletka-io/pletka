package autocomplete

import (
	"context"
	"errors"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// sentinelErr is the sentinel DB error used in error-propagation tests.
var sentinelErr = errors.New("db unavailable")

// errorStore is a fake Store that returns sentinelErr from GetClassByQname and
// GetPropertyByQname for a configured qname, and nil/nil for all others.
// All other methods return empty results with no error.
type errorStore struct {
	// failQname causes GetClassByQname and GetPropertyByQname to return
	// sentinelErr when the qname argument matches.
	failQname string
}

func (s *errorStore) GetVersion(_ context.Context, _ string) (*domain.OntologyVersion, error) {
	return nil, nil
}

func (s *errorStore) GetOntology(_ context.Context, _ string) (*domain.Ontology, error) {
	return nil, nil
}

func (s *errorStore) ListClassesByVersion(_ context.Context, _ string) ([]*domain.OntologyClass, error) {
	return nil, nil
}

func (s *errorStore) ListPropertiesByVersion(_ context.Context, _ string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}

func (s *errorStore) GetClassByQname(_ context.Context, _, qname string) (*domain.OntologyClass, error) {
	if qname == s.failQname {
		return nil, sentinelErr
	}
	return nil, nil
}

func (s *errorStore) GetPropertyByQname(_ context.Context, _, qname string) (*domain.OntologyProperty, error) {
	if qname == s.failQname {
		return nil, sentinelErr
	}
	return nil, nil
}

func (s *errorStore) SearchClasses(_ context.Context, _, _ string, _ int) ([]*domain.OntologyClass, error) {
	return nil, nil
}

func (s *errorStore) SearchProperties(_ context.Context, _, _ string, _ int) ([]*domain.OntologyProperty, error) {
	return nil, nil
}

func (s *errorStore) PropertiesForDomainQname(_ context.Context, _, _ string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}

func (s *errorStore) ListSubclassesByQname(_ context.Context, _, _ string) ([]*domain.OntologyClass, error) {
	return nil, nil
}

func (s *errorStore) ListRelationsForSourceByType(_ context.Context, _, _, _ string) ([]*domain.OntologyRelation, error) {
	return nil, nil
}

// projectReaderForVersions builds a fakeProjectReader for a list of version IDs.
func projectReaderForVersions(versionIDs ...string) *fakeProjectReader {
	links := make([]domain.ResolvedOntologyVersion, 0, len(versionIDs))
	for _, id := range versionIDs {
		links = append(links, minimalVersionLink(id, false))
	}
	return &fakeProjectReader{links: links}
}

// notFoundStore behaves like a real store for not-found: returns (nil, nil)
// for every GetClassByQname / GetPropertyByQname call. Used to verify the
// cross-version fall-through still works when an item is genuinely absent in
// one version but present in another.
type notFoundThenFoundStore struct {
	// presentQname is the qname that appears (as a class) in version v2 only.
	presentQname string
	presentClass *domain.OntologyClass
	// domainProp is returned by PropertiesForDomainQname for presentQname,
	// enabling tests to assert that the cross-version fall-through actually
	// produced suggestion output (not just "no error").
	domainProp *domain.OntologyProperty
}

func (s *notFoundThenFoundStore) GetVersion(_ context.Context, _ string) (*domain.OntologyVersion, error) {
	return nil, nil
}

func (s *notFoundThenFoundStore) GetOntology(_ context.Context, _ string) (*domain.Ontology, error) {
	return nil, nil
}

func (s *notFoundThenFoundStore) ListClassesByVersion(_ context.Context, _ string) ([]*domain.OntologyClass, error) {
	return nil, nil
}

func (s *notFoundThenFoundStore) ListPropertiesByVersion(_ context.Context, _ string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}

func (s *notFoundThenFoundStore) GetClassByQname(_ context.Context, versionID, qname string) (*domain.OntologyClass, error) {
	if qname == s.presentQname && versionID == "v2" {
		return s.presentClass, nil
	}
	return nil, nil // not found — genuine nil, nil
}

func (s *notFoundThenFoundStore) GetPropertyByQname(_ context.Context, _, _ string) (*domain.OntologyProperty, error) {
	return nil, nil
}

func (s *notFoundThenFoundStore) SearchClasses(_ context.Context, _, _ string, _ int) ([]*domain.OntologyClass, error) {
	return nil, nil
}

func (s *notFoundThenFoundStore) SearchProperties(_ context.Context, _, _ string, _ int) ([]*domain.OntologyProperty, error) {
	return nil, nil
}

func (s *notFoundThenFoundStore) PropertiesForDomainQname(_ context.Context, _, domainQname string) ([]*domain.OntologyProperty, error) {
	if s.domainProp != nil && domainQname == s.presentQname {
		return []*domain.OntologyProperty{s.domainProp}, nil
	}
	return nil, nil
}

func (s *notFoundThenFoundStore) ListSubclassesByQname(_ context.Context, _, _ string) ([]*domain.OntologyClass, error) {
	return nil, nil
}

func (s *notFoundThenFoundStore) ListRelationsForSourceByType(_ context.Context, _, _, _ string) ([]*domain.OntologyRelation, error) {
	return nil, nil
}

// TestDirectEngine_GetSuggestions_PropagatesClassLookupError verifies that a
// real (non-nil) error from GetClassByQname inside GetSuggestions is
// propagated to the caller — not silently swallowed into an empty result.
func TestDirectEngine_GetSuggestions_PropagatesClassLookupError(t *testing.T) {
	store := &errorStore{failQname: "x:BadClass"}
	projects := projectReaderForVersions("v1")
	engine := NewDirect(store, projects)

	_, err := engine.GetSuggestions(context.Background(), Request{
		ProjectID:   "proj-1",
		CurrentPath: []string{"x:BadClass"},
		MaxResults:  10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected sentinel error in chain, got: %v", err)
	}
}

// TestDirectEngine_GetSuggestions_PropagatesPropertyLookupError verifies that
// a real error from GetPropertyByQname inside GetSuggestions is propagated
// when the class lookup returns (nil, nil) but the property lookup errors.
func TestDirectEngine_GetSuggestions_PropagatesPropertyLookupError(t *testing.T) {
	// Use a store where GetClassByQname returns nil for x:BadProp (not found)
	// but GetPropertyByQname returns an error. We achieve this by having failQname
	// match the qname — both methods check failQname so GetClassByQname will also
	// fail. We need a store where class returns nil but property errors.
	store := &classNotFoundPropertyErrorStore{failQname: "x:BadProp"}
	projects := projectReaderForVersions("v1")
	engine := NewDirect(store, projects)

	_, err := engine.GetSuggestions(context.Background(), Request{
		ProjectID:   "proj-1",
		CurrentPath: []string{"x:BadProp"},
		MaxResults:  10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected sentinel error in chain, got: %v", err)
	}
}

// classNotFoundPropertyErrorStore: GetClassByQname returns (nil, nil); GetPropertyByQname
// returns (nil, sentinelErr) for the configured qname.
type classNotFoundPropertyErrorStore struct {
	failQname string
}

func (s *classNotFoundPropertyErrorStore) GetVersion(_ context.Context, _ string) (*domain.OntologyVersion, error) {
	return nil, nil
}

func (s *classNotFoundPropertyErrorStore) GetOntology(_ context.Context, _ string) (*domain.Ontology, error) {
	return nil, nil
}

func (s *classNotFoundPropertyErrorStore) ListClassesByVersion(_ context.Context, _ string) ([]*domain.OntologyClass, error) {
	return nil, nil
}

func (s *classNotFoundPropertyErrorStore) ListPropertiesByVersion(_ context.Context, _ string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}

func (s *classNotFoundPropertyErrorStore) GetClassByQname(_ context.Context, _, _ string) (*domain.OntologyClass, error) {
	return nil, nil // always not found
}

func (s *classNotFoundPropertyErrorStore) GetPropertyByQname(_ context.Context, _, qname string) (*domain.OntologyProperty, error) {
	if qname == s.failQname {
		return nil, sentinelErr
	}
	return nil, nil
}

func (s *classNotFoundPropertyErrorStore) SearchClasses(_ context.Context, _, _ string, _ int) ([]*domain.OntologyClass, error) {
	return nil, nil
}

func (s *classNotFoundPropertyErrorStore) SearchProperties(_ context.Context, _, _ string, _ int) ([]*domain.OntologyProperty, error) {
	return nil, nil
}

func (s *classNotFoundPropertyErrorStore) PropertiesForDomainQname(_ context.Context, _, _ string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}

func (s *classNotFoundPropertyErrorStore) ListSubclassesByQname(_ context.Context, _, _ string) ([]*domain.OntologyClass, error) {
	return nil, nil
}

func (s *classNotFoundPropertyErrorStore) ListRelationsForSourceByType(_ context.Context, _, _, _ string) ([]*domain.OntologyRelation, error) {
	return nil, nil
}

// TestDirectEngine_ClassLineage_PropagatesError verifies that classLineageQnames
// propagates a DB error from GetClassByQname instead of silently swallowing it.
// We set up a two-version scenario where the second version's lookup for the
// class errors.
func TestDirectEngine_ClassLineage_PropagatesError(t *testing.T) {
	// Store that errors on qname "x:Root" in any version.
	store := &errorStore{failQname: "x:Root"}
	projects := projectReaderForVersions("v1", "v2")
	engine := NewDirect(store, projects)

	_, err := engine.classLineageQnames(context.Background(), []string{"v1", "v2"}, "x:Root")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected sentinel error in chain, got: %v", err)
	}
}

// TestDirectEngine_GetSuggestions_NotFoundContinues verifies that a genuine
// not-found response (nil, nil) from GetClassByQname in one version does NOT
// abort the loop — the engine continues to the next version, finds the class
// there, resolves its properties, and returns at least one suggestion.
func TestDirectEngine_GetSuggestions_NotFoundContinues(t *testing.T) {
	class := &domain.OntologyClass{
		ID:        "cls-found",
		Qname:     "x:FoundClass",
		Prefix:    "x",
		LocalName: "FoundClass",
		URI:       "http://example.org/FoundClass",
	}
	// prop is returned by PropertiesForDomainQname for x:FoundClass, so the
	// engine produces at least one suggestion after the cross-version fall-through.
	prop := &domain.OntologyProperty{
		ID:        "prop-p1",
		Qname:     "x:hasName",
		Prefix:    "x",
		LocalName: "hasName",
		URI:       "http://example.org/hasName",
	}
	store := &notFoundThenFoundStore{
		presentQname: "x:FoundClass",
		presentClass: class,
		domainProp:   prop,
	}
	projects := projectReaderForVersions("v1", "v2")
	engine := NewDirect(store, projects)

	// x:FoundClass is absent in v1 (nil, nil) but present in v2.
	// GetSuggestions must find it in v2, call suggestPropertiesForClass, and
	// return x:hasName — proving the cross-version fall-through produced output.
	suggestions, err := engine.GetSuggestions(context.Background(), Request{
		ProjectID:   "proj-1",
		CurrentPath: []string{"x:FoundClass"},
		MaxResults:  10,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(suggestions) == 0 {
		t.Fatal("expected at least one suggestion from cross-version fall-through, got none")
	}
	if suggestions[0].Qname != "x:hasName" {
		t.Errorf("expected suggestion qname %q, got %q", "x:hasName", suggestions[0].Qname)
	}
}

// TestClassLineage_NotFoundContinues verifies that a genuine not-found
// (nil, nil) from GetClassByQname inside classLineageQnames causes the loop
// to continue to the next version rather than returning an error.
func TestClassLineage_NotFoundContinues(t *testing.T) {
	// x:Root is absent in v1 but present in v2 with no relations.
	class := &domain.OntologyClass{
		ID:        "cls-root",
		Qname:     "x:Root",
		Prefix:    "x",
		LocalName: "Root",
		URI:       "http://example.org/Root",
	}
	store := &notFoundThenFoundStore{
		presentQname: "x:Root",
		presentClass: class,
	}
	engine := NewDirect(store, projectReaderForVersions())

	qnames, err := engine.classLineageQnames(context.Background(), []string{"v1", "v2"}, "x:Root")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// x:Root itself must be in the output even though v1 returned nil.
	found := false
	for _, q := range qnames {
		if q == "x:Root" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected x:Root in lineage output, got %v", qnames)
	}
}
