package autocomplete

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// implicitRootStore builds an index store with the defaults vocabulary
// (rdfs:Resource, rdfs:label with domain Resource, rdf:type with no domain)
// plus one application ontology: Top (no superclass), Mid subClassOf Top,
// Orphan subClassOf a ghost parent, and property ex:name with domain Top.
func implicitRootStore(withDefaults bool) *fakeIndexStore {
	const (
		ownV   = "v-own"
		ownOnt = "ont-own"
		ownNS  = "http://ex/own/"
		defV   = "v-defaults"
		defOnt = "ont-defaults"
		defNS  = "https://example.org/defaults/"
		rdfsNS = "http://www.w3.org/2000/01/rdf-schema#"
	)
	s := &fakeIndexStore{
		classes: []*domain.OntologyClass{
			{ID: "c-top", OntologyVersionID: ownV, Prefix: "ex", LocalName: "Top", URI: ownNS + "Top", Qname: "ex:Top"},
			{ID: "c-mid", OntologyVersionID: ownV, Prefix: "ex", LocalName: "Mid", URI: ownNS + "Mid", Qname: "ex:Mid"},
			{ID: "c-orphan", OntologyVersionID: ownV, Prefix: "ex", LocalName: "Orphan", URI: ownNS + "Orphan", Qname: "ex:Orphan"},
		},
		properties: []*domain.OntologyProperty{
			{ID: "p-name", OntologyVersionID: ownV, Prefix: "ex", LocalName: "name", URI: ownNS + "name", Qname: "ex:name", PropertyType: "DatatypeProperty"},
		},
		relations: []*domain.OntologyRelationWithSource{
			{OntologyRelation: domain.OntologyRelation{SourceID: "c-mid", SourceKind: "class", RelType: "subclass_of", TargetQname: "ex:Top"}, SourceQname: "ex:Mid"},
			{OntologyRelation: domain.OntologyRelation{SourceID: "c-orphan", SourceKind: "class", RelType: "subclass_of", TargetQname: "other:Unknown"}, SourceQname: "ex:Orphan"},
			{OntologyRelation: domain.OntologyRelation{SourceID: "p-name", SourceKind: "property", RelType: "domain", TargetQname: "ex:Top"}, SourceQname: "ex:name"},
		},
		versions:   map[string]*domain.OntologyVersion{ownV: {ID: ownV, OntologyID: ownOnt, VersionString: "1.0"}},
		ontologies: map[string]*domain.Ontology{ownOnt: {ID: ownOnt, Prefix: "ex", Namespace: ownNS}},
	}
	if !withDefaults {
		return s
	}
	s.classes = append(s.classes,
		&domain.OntologyClass{ID: "c-res", OntologyVersionID: defV, Prefix: "rdfs", LocalName: "Resource", URI: rdfsNS + "Resource", Qname: "rdfs:Resource"},
	)
	s.properties = append(s.properties,
		&domain.OntologyProperty{ID: "p-label", OntologyVersionID: defV, Prefix: "rdfs", LocalName: "label", URI: rdfsNS + "label", Qname: "rdfs:label", PropertyType: "DatatypeProperty"},
		&domain.OntologyProperty{ID: "p-type", OntologyVersionID: defV, Prefix: "rdf", LocalName: "type", URI: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type", Qname: "rdf:type", PropertyType: "ObjectProperty"},
	)
	s.relations = append(s.relations,
		&domain.OntologyRelationWithSource{OntologyRelation: domain.OntologyRelation{SourceID: "p-label", SourceKind: "property", RelType: "domain", TargetQname: "rdfs:Resource"}, SourceQname: "rdfs:label"},
		&domain.OntologyRelationWithSource{OntologyRelation: domain.OntologyRelation{SourceID: "p-label", SourceKind: "property", RelType: "range", TargetQname: "rdfs:Literal"}, SourceQname: "rdfs:label"},
	)
	s.versions[defV] = &domain.OntologyVersion{ID: defV, OntologyID: defOnt, VersionString: "1.1"}
	s.ontologies[defOnt] = &domain.Ontology{ID: defOnt, Prefix: "defaults", Namespace: defNS}
	return s
}

func buildImplicitRootIndex(t *testing.T, withDefaults bool) *Index {
	t.Helper()
	versions := []string{"v-own"}
	if withDefaults {
		versions = append(versions, "v-defaults")
	}
	key := keyForRequest(versions, "v-own", "live")
	idx, err := buildIndex(context.Background(), implicitRootStore(withDefaults), nil, key, versions, "v-own", testLogger())
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	return idx
}

func TestImplicitRoot_BasicsOfferedOnEveryClass(t *testing.T) {
	idx := buildImplicitRootIndex(t, true)
	eng := &IndexedEngine{}
	for _, class := range []string{"ex:Top", "ex:Mid", "ex:Orphan"} {
		got := suggestionQnames(eng.suggestPropertiesForClass(idx, idx.ByQname[class], Request{MaxResults: 50}))
		if !got["rdfs:label"] {
			t.Errorf("%s: rdfs:label (domain rdfs:Resource) not offered, got %v", class, got)
		}
		if got["rdf:type"] {
			t.Errorf("%s: domain-less defaults property rdf:type must stay audit-only, got %v", class, got)
		}
	}
}

func TestImplicitRoot_OwnPropertiesSortBeforeBasics(t *testing.T) {
	idx := buildImplicitRootIndex(t, true)
	eng := &IndexedEngine{}
	got := eng.suggestPropertiesForClass(idx, idx.ByQname["ex:Mid"], Request{MaxResults: 50})
	if len(got) < 2 || got[0].Qname != "ex:name" || got[len(got)-1].Qname != "rdfs:label" {
		t.Errorf("want ex:name first and rdfs:label last, got %v", suggestionOrder(got))
	}
	// Truncation drops the basics first.
	one := eng.suggestPropertiesForClass(idx, idx.ByQname["ex:Mid"], Request{MaxResults: 1})
	if len(one) != 1 || one[0].Qname != "ex:name" {
		t.Errorf("MaxResults=1 must keep the class's own property, got %v", suggestionOrder(one))
	}
	// Dedupe: rdfs:label reached through Mid -> Top -> Resource appears once.
	count := 0
	for _, s := range got {
		if s.Qname == "rdfs:label" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("rdfs:label offered %d times, want 1", count)
	}
}

func TestImplicitRoot_QueryMatchesBasics(t *testing.T) {
	idx := buildImplicitRootIndex(t, true)
	eng := &IndexedEngine{}
	got := suggestionQnames(eng.suggestPropertiesForClass(idx, idx.ByQname["ex:Top"], Request{MaxResults: 50, Query: "lab"}))
	if !got["rdfs:label"] || got["ex:name"] {
		t.Errorf("query 'lab' should keep only rdfs:label, got %v", got)
	}
}

func TestImplicitRoot_NoDefaults_IndexUnchanged(t *testing.T) {
	idx := buildImplicitRootIndex(t, false)
	if _, ok := idx.ByQname[resourceQname]; ok {
		t.Fatalf("no defaults version: rdfs:Resource must not be synthesized")
	}
	if n := idx.ByQname["ex:Top"]; len(n.Superclasses) != 0 {
		t.Errorf("ex:Top gained superclasses without defaults: %v", n.Superclasses)
	}
	eng := &IndexedEngine{}
	got := suggestionQnames(eng.suggestPropertiesForClass(idx, idx.ByQname["ex:Top"], Request{MaxResults: 50}))
	if got["rdfs:label"] {
		t.Errorf("rdfs:label offered without defaults: %v", got)
	}
}

func TestImplicitRoot_ResourceItselfHasNoRoot(t *testing.T) {
	idx := buildImplicitRootIndex(t, true)
	res := idx.ByQname[resourceQname]
	if res == nil || res.Ghost {
		t.Fatalf("rdfs:Resource must be a real node")
	}
	if len(res.Superclasses) != 0 {
		t.Errorf("rdfs:Resource must not be its own subclass: %v", res.Superclasses)
	}
}

// suggestionOrder lists suggestion qnames in order, for failure messages.
func suggestionOrder(s []Suggestion) []string {
	out := make([]string, 0, len(s))
	for _, x := range s {
		out = append(out, x.Qname)
	}
	return out
}
