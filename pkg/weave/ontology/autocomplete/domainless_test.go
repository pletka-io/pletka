package autocomplete

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestDomainlessProperties_UniversalButOwnNamespaceOnly verifies the RDFS rule
// "a property with no rdfs:domain applies to all classes" — scoped to the
// ontology's own vocabulary:
//
//   - a domain-less property defined under the ontology's namespace is offered
//     on every class (its own domain class and an unrelated one);
//   - a domain-less property IMPORTED from another vocabulary (URI outside the
//     ontology namespace, e.g. dc:title) is NOT offered, so imported annotation
//     scaffolding does not flood every class;
//   - a property WITH a declared domain is still only offered on that lineage.
func TestDomainlessProperties_UniversalButOwnNamespaceOnly(t *testing.T) {
	const (
		versionID  = "v1"
		ontologyID = "ont-own"
		ns         = "http://ex/own/"
	)
	store := &fakeIndexStore{
		classes: []*domain.OntologyClass{
			{ID: "c-a", OntologyVersionID: versionID, Prefix: "ex", LocalName: "A", URI: ns + "A", Qname: "ex:A"},
			{ID: "c-b", OntologyVersionID: versionID, Prefix: "ex", LocalName: "B", URI: ns + "B", Qname: "ex:B"},
		},
		properties: []*domain.OntologyProperty{
			// domain-less, own namespace -> universal
			{ID: "p-open", OntologyVersionID: versionID, Prefix: "ex", LocalName: "pOpen", URI: ns + "pOpen", Qname: "ex:pOpen", PropertyType: "ObjectProperty"},
			// domained to ex:A -> lineage only
			{ID: "p-doma", OntologyVersionID: versionID, Prefix: "ex", LocalName: "pDomA", URI: ns + "pDomA", Qname: "ex:pDomA", PropertyType: "ObjectProperty"},
			// domain-less, imported external vocabulary -> excluded
			{ID: "p-dc", OntologyVersionID: versionID, Prefix: "dc", LocalName: "title", URI: "http://purl.org/dc/elements/1.1/title", Qname: "dc:title", PropertyType: "DatatypeProperty"},
		},
		relations: []*domain.OntologyRelationWithSource{
			{OntologyRelation: domain.OntologyRelation{SourceID: "p-doma", SourceKind: "property", RelType: "domain", TargetQname: "ex:A"}, SourceQname: "ex:pDomA"},
		},
		versions:   map[string]*domain.OntologyVersion{versionID: {ID: versionID, OntologyID: ontologyID, VersionString: "1.0"}},
		ontologies: map[string]*domain.Ontology{ontologyID: {ID: ontologyID, Prefix: "ex", Namespace: ns}},
	}

	key := keyForRequest([]string{versionID}, versionID, "live")
	idx, err := buildIndex(context.Background(), store, nil, key, []string{versionID}, versionID, testLogger())
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}

	eng := &IndexedEngine{}
	req := Request{MaxResults: 50}

	onA := suggestionQnames(eng.suggestPropertiesForClass(idx, idx.ByQname["ex:A"], req))
	if !onA["ex:pDomA"] {
		t.Errorf("class A: expected domained property ex:pDomA, got %v", onA)
	}
	if !onA["ex:pOpen"] {
		t.Errorf("class A: expected domain-less own property ex:pOpen, got %v", onA)
	}
	if onA["dc:title"] {
		t.Errorf("class A: imported domain-less dc:title must NOT be offered, got %v", onA)
	}

	onB := suggestionQnames(eng.suggestPropertiesForClass(idx, idx.ByQname["ex:B"], req))
	if !onB["ex:pOpen"] {
		t.Errorf("class B: expected domain-less own property ex:pOpen on unrelated class, got %v", onB)
	}
	if onB["ex:pDomA"] {
		t.Errorf("class B: domained property ex:pDomA must NOT appear on unrelated class, got %v", onB)
	}
	if onB["dc:title"] {
		t.Errorf("class B: imported domain-less dc:title must NOT be offered, got %v", onB)
	}
}

// TestDirectEngine_DomainlessPropertyOfferedOnClass checks the DirectEngine
// (DB-round-trip path) offers a domain-less property on a class via the new
// PropertiesWithoutDomain store call, keeping it consistent with the indexed
// engine regardless of dispatch mode.
func TestDirectEngine_DomainlessPropertyOfferedOnClass(t *testing.T) {
	class := &domain.OntologyClass{ID: "c-a", Qname: "ex:A", URI: "http://ex/own/A"}
	open := &domain.OntologyProperty{ID: "p-open", Qname: "ex:pOpen", LocalName: "pOpen", URI: "http://ex/own/pOpen", PropertyType: "ObjectProperty"}

	eng := &DirectEngine{store: &directDomainlessStore{class: class, open: open}}
	got, err := eng.suggestPropertiesForClass(context.Background(), []string{"v1"}, class, Request{MaxResults: 50})
	if err != nil {
		t.Fatalf("suggestPropertiesForClass: %v", err)
	}
	if qn := suggestionQnames(got); !qn["ex:pOpen"] {
		t.Errorf("DirectEngine should offer domain-less ex:pOpen, got %v", qn)
	}
}

// directDomainlessStore is a minimal autocomplete.Store: one class with no
// parents, no domain-matched properties, one domain-less property.
type directDomainlessStore struct {
	class *domain.OntologyClass
	open  *domain.OntologyProperty
}

func (s *directDomainlessStore) GetVersion(_ context.Context, id string) (*domain.OntologyVersion, error) {
	return &domain.OntologyVersion{ID: id, OntologyID: "ont"}, nil
}
func (s *directDomainlessStore) GetOntology(context.Context, string) (*domain.Ontology, error) {
	return &domain.Ontology{ID: "ont", Prefix: "ex", Namespace: "http://ex/own/"}, nil
}
func (s *directDomainlessStore) GetClassByQname(_ context.Context, _, qname string) (*domain.OntologyClass, error) {
	if s.class != nil && qname == s.class.Qname {
		return s.class, nil
	}
	return nil, nil
}
func (s *directDomainlessStore) ListRelationsForSourceByType(context.Context, string, string, string) ([]*domain.OntologyRelation, error) {
	return nil, nil
}
func (s *directDomainlessStore) PropertiesForDomainQname(context.Context, string, string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *directDomainlessStore) PropertiesWithoutDomain(context.Context, string) ([]*domain.OntologyProperty, error) {
	return []*domain.OntologyProperty{s.open}, nil
}
func (s *directDomainlessStore) ListClassesByVersion(context.Context, string) ([]*domain.OntologyClass, error) {
	return nil, nil
}
func (s *directDomainlessStore) ListPropertiesByVersion(context.Context, string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *directDomainlessStore) GetPropertyByQname(context.Context, string, string) (*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *directDomainlessStore) SearchClasses(context.Context, string, string, int) ([]*domain.OntologyClass, error) {
	return nil, nil
}
func (s *directDomainlessStore) SearchProperties(context.Context, string, string, int) ([]*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *directDomainlessStore) ListSubclassesByQname(context.Context, string, string) ([]*domain.OntologyClass, error) {
	return nil, nil
}

func suggestionQnames(sugs []Suggestion) map[string]bool {
	m := make(map[string]bool, len(sugs))
	for _, s := range sugs {
		m[s.Qname] = true
	}
	return m
}
