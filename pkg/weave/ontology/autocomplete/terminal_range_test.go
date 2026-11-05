package autocomplete

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// terminalRangeStore has the defaults vocabulary as real nodes (rdfs:Resource,
// rdfs:Literal, xsd:string) plus application properties ranged on them, and a
// class ex:Thing with no superclass that the implicit root will attach.
func terminalRangeStore() *fakeIndexStore {
	const (
		ownV, ownOnt, ownNS = "v-own", "ont-own", "http://ex/own/"
		defV, defOnt        = "v-defaults", "ont-defaults"
		rdfsNS              = "http://www.w3.org/2000/01/rdf-schema#"
		xsdNS               = "http://www.w3.org/2001/XMLSchema#"
	)
	return &fakeIndexStore{
		classes: []*domain.OntologyClass{
			{ID: "c-thing", OntologyVersionID: ownV, Prefix: "ex", LocalName: "Thing", URI: ownNS + "Thing", Qname: "ex:Thing"},
			{ID: "c-res", OntologyVersionID: defV, Prefix: "rdfs", LocalName: "Resource", URI: rdfsNS + "Resource", Qname: "rdfs:Resource"},
			{ID: "c-lit", OntologyVersionID: defV, Prefix: "rdfs", LocalName: "Literal", URI: rdfsNS + "Literal", Qname: "rdfs:Literal"},
			{ID: "c-str", OntologyVersionID: defV, Prefix: "xsd", LocalName: "string", URI: xsdNS + "string", Qname: "xsd:string"},
		},
		properties: []*domain.OntologyProperty{
			{ID: "p-title", OntologyVersionID: ownV, Prefix: "ex", LocalName: "title", URI: ownNS + "title", Qname: "ex:title", PropertyType: "DatatypeProperty"},
			{ID: "p-note", OntologyVersionID: ownV, Prefix: "ex", LocalName: "note", URI: ownNS + "note", Qname: "ex:note", PropertyType: "DatatypeProperty"},
			{ID: "p-same", OntologyVersionID: ownV, Prefix: "ex", LocalName: "sameAs", URI: ownNS + "sameAs", Qname: "ex:sameAs", PropertyType: "ObjectProperty"},
			{ID: "p-rel", OntologyVersionID: ownV, Prefix: "ex", LocalName: "rel", URI: ownNS + "rel", Qname: "ex:rel", PropertyType: "ObjectProperty"},
		},
		relations: []*domain.OntologyRelationWithSource{
			{OntologyRelation: domain.OntologyRelation{SourceID: "c-lit", SourceKind: "class", RelType: "subclass_of", TargetQname: "rdfs:Resource"}, SourceQname: "rdfs:Literal"},
			{OntologyRelation: domain.OntologyRelation{SourceID: "p-title", SourceKind: "property", RelType: "range", TargetQname: "xsd:string"}, SourceQname: "ex:title"},
			{OntologyRelation: domain.OntologyRelation{SourceID: "p-note", SourceKind: "property", RelType: "range", TargetQname: "rdfs:Literal"}, SourceQname: "ex:note"},
			{OntologyRelation: domain.OntologyRelation{SourceID: "p-same", SourceKind: "property", RelType: "range", TargetQname: "rdfs:Resource"}, SourceQname: "ex:sameAs"},
			{OntologyRelation: domain.OntologyRelation{SourceID: "p-rel", SourceKind: "property", RelType: "range", TargetQname: "ex:Thing"}, SourceQname: "ex:rel"},
		},
		versions: map[string]*domain.OntologyVersion{
			ownV: {ID: ownV, OntologyID: ownOnt, VersionString: "1.0"},
			defV: {ID: defV, OntologyID: defOnt, VersionString: "1.1"},
		},
		ontologies: map[string]*domain.Ontology{
			ownOnt: {ID: ownOnt, Prefix: "ex", Namespace: ownNS},
			defOnt: {ID: defOnt, Prefix: "defaults", Namespace: "https://example.org/defaults/"},
		},
	}
}

func TestTerminalRange_RealLiteralAndResourceNodes(t *testing.T) {
	versions := []string{"v-own", "v-defaults"}
	key := keyForRequest(versions, "v-own", "live")
	idx, err := buildIndex(context.Background(), terminalRangeStore(), nil, key, versions, "v-own", testLogger())
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if n := idx.ByQname["xsd:string"]; n == nil || n.Ghost {
		t.Fatalf("precondition: xsd:string must be a real node in this index")
	}
	eng := &IndexedEngine{}
	req := Request{MaxResults: 50}

	tests := []struct {
		prop      string
		wantQname string
		wantType  string
	}{
		{"ex:title", "xsd:string", "literal"},
		{"ex:note", "rdfs:Literal", "literal"},
		{"ex:sameAs", "rdfs:Resource", "literal"},
	}
	for _, tc := range tests {
		got := eng.suggestRangeForProperty(idx, idx.ByQname[tc.prop], req)
		if len(got) != 1 {
			t.Errorf("%s: want exactly one terminal suggestion, got %v", tc.prop, suggestionOrder(got))
			continue
		}
		if got[0].Qname != tc.wantQname || got[0].Type != tc.wantType {
			t.Errorf("%s: got %s/%s, want %s/%s", tc.prop, got[0].Qname, got[0].Type, tc.wantQname, tc.wantType)
		}
	}

	// An ordinary class range still expands to the class (and its subclasses).
	got := suggestionQnames(eng.suggestRangeForProperty(idx, idx.ByQname["ex:rel"], req))
	if !got["ex:Thing"] {
		t.Errorf("ex:rel: class range must still suggest ex:Thing, got %v", got)
	}
	if got["rdfs:Resource"] {
		t.Errorf("ex:rel: rdfs:Resource must not leak into a class range, got %v", got)
	}
}
