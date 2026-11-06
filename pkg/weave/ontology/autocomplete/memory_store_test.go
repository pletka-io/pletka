package autocomplete

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func memoryFixture() []MemoryVersion {
	const ns = "https://example.org/base/"
	return []MemoryVersion{{
		Version:  &domain.OntologyVersion{ID: "v-base", OntologyID: "ont-base", VersionString: "1.0"},
		Ontology: &domain.Ontology{ID: "ont-base", Prefix: "base", Namespace: ns},
		Classes: []*domain.OntologyClass{
			{ID: "c-thing", OntologyVersionID: "v-base", Prefix: "base", LocalName: "Thing", URI: ns + "Thing", Qname: "base:Thing"},
			{ID: "c-person", OntologyVersionID: "v-base", Prefix: "base", LocalName: "Person", URI: ns + "Person", Qname: "base:Person"},
		},
		Properties: []*domain.OntologyProperty{
			{ID: "p-name", OntologyVersionID: "v-base", Prefix: "base", LocalName: "name", URI: ns + "name", Qname: "base:name", PropertyType: "DatatypeProperty"},
			{ID: "p-knows", OntologyVersionID: "v-base", Prefix: "base", LocalName: "knows", URI: ns + "knows", Qname: "base:knows", PropertyType: "ObjectProperty"},
		},
		Relations: []domain.OntologyRelationWithSource{
			{OntologyRelation: domain.OntologyRelation{SourceID: "c-person", SourceKind: "class", RelType: "subclass_of", TargetQname: "base:Thing"}, SourceQname: "base:Person"},
			{OntologyRelation: domain.OntologyRelation{SourceID: "p-name", SourceKind: "property", RelType: "domain", TargetQname: "base:Thing"}, SourceQname: "base:name"},
			{OntologyRelation: domain.OntologyRelation{SourceID: "p-name", SourceKind: "property", RelType: "range", TargetQname: "xsd:string"}, SourceQname: "base:name"},
			{OntologyRelation: domain.OntologyRelation{SourceID: "p-knows", SourceKind: "property", RelType: "domain", TargetQname: "base:Person"}, SourceQname: "base:knows"},
			{OntologyRelation: domain.OntologyRelation{SourceID: "p-knows", SourceKind: "property", RelType: "range", TargetQname: "base:Person"}, SourceQname: "base:knows"},
		},
	}}
}

// TestOfflineEngine_StepsLikeTheBuilder pins that an index built from
// in-memory rows answers the builder's three questions: root classes,
// properties on a class (inherited through subclass_of), ranges of a
// property (classes and literals).
func TestOfflineEngine_StepsLikeTheBuilder(t *testing.T) {
	eng, err := NewOfflineEngine(context.Background(), NewMemoryStore(memoryFixture()...), nil, testLogger())
	if err != nil {
		t.Fatalf("NewOfflineEngine: %v", err)
	}
	roots := suggestionQnames(mustSuggest(t, eng, Request{MaxResults: 50}))
	if !roots["base:Thing"] || !roots["base:Person"] {
		t.Fatalf("root classes: got %v", roots)
	}
	props := suggestionQnames(mustSuggest(t, eng, Request{CurrentPath: []string{"base:Person"}, ScopeClass: "base:Person", MaxResults: 50}))
	if !props["base:name"] || !props["base:knows"] {
		t.Fatalf("properties on base:Person (name inherited from Thing): got %v", props)
	}
	ranges := suggestionQnames(mustSuggest(t, eng, Request{CurrentPath: []string{"base:Person", "base:name"}, ScopeClass: "base:Person", MaxResults: 50, IncludeRangeSuggestions: true}))
	if !ranges["xsd:string"] {
		t.Fatalf("range of base:name: got %v", ranges)
	}
}

func mustSuggest(t *testing.T, eng *OfflineEngine, req Request) []Suggestion {
	t.Helper()
	sugs, err := eng.Suggest(context.Background(), req)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	return sugs
}
