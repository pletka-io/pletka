package autocomplete

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestBuildIndex_MergesHashPrefixedTargetIntoRealNode is the prefix-drift
// regression. One ontology defines a shared class under a readable prefix
// (obo:BFO_0000001); another references that class as a property domain by a
// hash prefix for the same URI (x59a354be:BFO_0000001) with no class row of
// its own. Without canonicalisation the hash reference becomes a ghost and the
// property's domain edge is stranded on it, so the property never autocompletes
// from the readable scope. The index must merge the hash reference into the
// real node instead.
func TestBuildIndex_MergesHashPrefixedTargetIntoRealNode(t *testing.T) {
	const (
		versionID = "v-primary"
		oboURI    = "http://purl.obolibrary.org/obo/BFO_0000001"
	)
	store := &fakeIndexStore{
		classes: []*domain.OntologyClass{
			{
				ID:                "cls-bfo",
				OntologyVersionID: versionID,
				Prefix:            "obo",
				LocalName:         "BFO_0000001",
				URI:               oboURI,
				Qname:             "obo:BFO_0000001",
				Label:             domain.Translations{"en": "entity"},
			},
		},
		properties: []*domain.OntologyProperty{
			{
				ID:                "prop-desc",
				OntologyVersionID: versionID,
				Prefix:            "sdo",
				LocalName:         "description",
				URI:               "https://schema.org/description",
				Qname:             "sdo:description",
				PropertyType:      "DatatypeProperty",
				Label:             domain.Translations{"en": "description"},
			},
		},
		// sdo:description declares its domain by a hash prefix for the SAME URI
		// as the real obo:BFO_0000001 class — the schema-extension signature.
		relations: []*domain.OntologyRelationWithSource{
			{
				OntologyRelation: domain.OntologyRelation{
					SourceID:    "prop-desc",
					SourceKind:  "property",
					RelType:     "domain",
					TargetQname: "x59a354be:BFO_0000001",
				},
				SourceQname: "sdo:description",
			},
		},
		versions: map[string]*domain.OntologyVersion{
			versionID: {ID: versionID, OntologyID: "ont-1", VersionString: "1.0"},
		},
		ontologies: map[string]*domain.Ontology{"ont-1": {ID: "ont-1", Prefix: "obo"}},
	}

	bindings := func(context.Context) ([]NamespaceBinding, error) {
		return []NamespaceBinding{
			{Prefix: "obo", Namespace: "http://purl.obolibrary.org/obo/"},
			{Prefix: "x59a354be", Namespace: "http://purl.obolibrary.org/obo/"},
		}, nil
	}

	versionIDs := []string{versionID}
	key := keyForRequest(versionIDs, versionID, "live")
	idx, err := buildIndex(context.Background(), store, bindings, key, versionIDs, versionID, testLogger())
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}

	// No ghost under the hash prefix — it merged into the real node.
	if g := idx.ByQname["x59a354be:BFO_0000001"]; g != nil {
		t.Fatalf("hash-prefixed target should have merged, not created node %+v", g)
	}
	bfo := idx.ByQname["obo:BFO_0000001"]
	if bfo == nil || bfo.Ghost {
		t.Fatal("obo:BFO_0000001 should be a real node")
	}
	// sdo:description's domain edge lands on the real node.
	if len(bfo.AsDomainOf) != 1 || bfo.AsDomainOf[0].Qname != "sdo:description" {
		t.Fatalf("obo:BFO_0000001.AsDomainOf = %v; want [sdo:description]", bfo.AsDomainOf)
	}

	// End to end: scoping at the readable class now offers the property.
	eng := &IndexedEngine{}
	got := eng.suggestPropertiesForClass(idx, bfo, Request{MaxResults: 10})
	if len(got) != 1 || got[0].Qname != "sdo:description" {
		t.Fatalf("suggestPropertiesForClass = %v; want [sdo:description]", got)
	}
}

// TestIndexLookupNodeResolvesByURIAcrossPrefixes covers the request-side half
// of the prefix-drift fix: the import stored a shared class under a hash prefix
// (x59a354be:BFO_0000001), but a higher-weight display binding makes the path
// builder emit the readable prefix (obo:BFO_0000001). lookupNode must resolve
// the readable request qname to the hash-stored node via their shared URI, so
// the properties whose domain is that class still autocomplete.
func TestIndexLookupNodeResolvesByURIAcrossPrefixes(t *testing.T) {
	const uri = "http://purl.obolibrary.org/obo/BFO_0000001"
	node := &Node{Qname: "x59a354be:BFO_0000001", URI: uri, Type: "class"}

	mgr, err := newNamespaceManager(context.Background(), func(context.Context) ([]NamespaceBinding, error) {
		return []NamespaceBinding{
			{Prefix: "obo", Namespace: "http://purl.obolibrary.org/obo/"},
		}, nil
	})
	if err != nil {
		t.Fatalf("newNamespaceManager: %v", err)
	}

	idx := &Index{
		ByQname:    map[string]*Node{node.Qname: node},
		ByURI:      map[string]*Node{uri: node},
		namespaces: mgr,
	}

	tests := []struct {
		name  string
		qname string
		want  *Node
	}{
		{"stored hash prefix hits ByQname", "x59a354be:BFO_0000001", node},
		{"readable prefix resolves via URI", "obo:BFO_0000001", node},
		{"unknown prefix resolves to nothing", "nope:BFO_0000001", nil},
		{"unknown local under known prefix", "obo:BFO_9999999", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := idx.lookupNode(tt.qname); got != tt.want {
				t.Fatalf("lookupNode(%q) = %v; want %v", tt.qname, got, tt.want)
			}
		})
	}
}
