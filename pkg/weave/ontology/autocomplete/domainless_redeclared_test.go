package autocomplete

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestDomainlessProperty_RedeclaredByExtension pins that a property an
// ontology defines without a domain stays offered on every class when a
// linked extension re-declares the same qname under its own namespace (to
// add a range). The node is collapsed by qname; the owning namespace is any
// linked ontology whose namespace covers the property's URI.
func TestDomainlessProperty_RedeclaredByExtension(t *testing.T) {
	const (
		baseV, baseOnt, baseNS = "v-base", "ont-base", "https://example.org/base/"
		extV, extOnt, extNS    = "v-ext", "ont-ext", "https://example.org/ext#"
	)
	// the extension's rows come first, as a real listing may order them
	store := &fakeIndexStore{
		classes: []*domain.OntologyClass{
			{ID: "c-thing", OntologyVersionID: baseV, Prefix: "base", LocalName: "Thing", URI: baseNS + "Thing", Qname: "base:Thing"},
		},
		properties: []*domain.OntologyProperty{
			{ID: "p-acr-ext", OntologyVersionID: extV, Prefix: "base", LocalName: "acronym", URI: baseNS + "acronym", Qname: "base:acronym", PropertyType: "AnnotationProperty"},
			{ID: "p-acr", OntologyVersionID: baseV, Prefix: "base", LocalName: "acronym", URI: baseNS + "acronym", Qname: "base:acronym", PropertyType: "AnnotationProperty"},
		},
		relations: []*domain.OntologyRelationWithSource{
			{OntologyRelation: domain.OntologyRelation{SourceID: "p-acr-ext", SourceKind: "property", RelType: "range", TargetQname: "xsd:string"}, SourceQname: "base:acronym"},
		},
		versions: map[string]*domain.OntologyVersion{
			baseV: {ID: baseV, OntologyID: baseOnt, VersionString: "1.0"},
			extV:  {ID: extV, OntologyID: extOnt, VersionString: "0.1"},
		},
		ontologies: map[string]*domain.Ontology{
			baseOnt: {ID: baseOnt, Prefix: "base", Namespace: baseNS},
			extOnt:  {ID: extOnt, Prefix: "ext", Namespace: extNS},
		},
	}
	versions := []string{extV, baseV}
	key := keyForRequest(versions, "", "live")
	idx, err := buildIndex(context.Background(), store, nil, key, versions, "", testLogger())
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	eng := &IndexedEngine{}
	got := suggestionQnames(eng.suggestPropertiesForClass(idx, idx.ByQname["base:Thing"], Request{MaxResults: 50}))
	if !got["base:acronym"] {
		t.Fatalf("domain-less base:acronym must be offered on base:Thing although the extension declared it first; got %v", got)
	}
}
