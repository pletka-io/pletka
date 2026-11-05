package autocomplete

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestAnnotationProperty_TerminatesInLiteral checks that an AnnotationProperty
// ends a path the way a DatatypeProperty does: with no range it yields the
// generic rdfs:Literal, and with a literal range (xsd:string) it yields that
// literal instead of a dead end.
func TestAnnotationProperty_TerminatesInLiteral(t *testing.T) {
	const (
		versionID  = "v1"
		ontologyID = "ont-own"
		ns         = "http://ex/own/"
	)
	store := &fakeIndexStore{
		classes: []*domain.OntologyClass{
			{ID: "c-a", OntologyVersionID: versionID, Prefix: "ex", LocalName: "A", URI: ns + "A", Qname: "ex:A"},
		},
		properties: []*domain.OntologyProperty{
			{ID: "p-acr", OntologyVersionID: versionID, Prefix: "ex", LocalName: "acronym", URI: ns + "acronym", Qname: "ex:acronym", PropertyType: "AnnotationProperty"},
			{ID: "p-lbl", OntologyVersionID: versionID, Prefix: "ex", LocalName: "label", URI: ns + "label", Qname: "ex:label", PropertyType: "AnnotationProperty"},
			{ID: "p-obj", OntologyVersionID: versionID, Prefix: "ex", LocalName: "rel", URI: ns + "rel", Qname: "ex:rel", PropertyType: "ObjectProperty"},
		},
		relations: []*domain.OntologyRelationWithSource{
			{OntologyRelation: domain.OntologyRelation{SourceID: "p-lbl", SourceKind: "property", RelType: "range", TargetQname: "xsd:string"}, SourceQname: "ex:label"},
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

	if got := suggestionQnames(eng.suggestRangeForProperty(idx, idx.ByQname["ex:acronym"], req)); !got[genericLiteralQname] {
		t.Errorf("range-less annotation property: expected %s, got %v", genericLiteralQname, got)
	}
	if got := suggestionQnames(eng.suggestRangeForProperty(idx, idx.ByQname["ex:label"], req)); !got["xsd:string"] {
		t.Errorf("annotation property with xsd:string range: expected xsd:string, got %v", got)
	}
	if got := suggestionQnames(eng.suggestRangeForProperty(idx, idx.ByQname["ex:rel"], req)); len(got) != 0 {
		t.Errorf("range-less object property must still be a dead end, got %v", got)
	}
}
