package autocomplete

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestCollapsedNode_FillsEmptyLabelFromLaterRow pins that when two linked
// versions define the same qname and the first row seen has no label, the
// node takes the label (and comment) of a later row: a curator typing the
// label must find the term whichever ontology's row was listed first.
func TestCollapsedNode_FillsEmptyLabelFromLaterRow(t *testing.T) {
	const ns = "https://example.org/base/"
	store := &fakeIndexStore{
		classes: []*domain.OntologyClass{
			{ID: "c-thing", OntologyVersionID: "v-ext", Prefix: "base", LocalName: "Thing", URI: ns + "Thing", Qname: "base:Thing"},
		},
		properties: []*domain.OntologyProperty{
			{ID: "p-url-ext", OntologyVersionID: "v-ext", Prefix: "base", LocalName: "url", URI: ns + "url", Qname: "base:url", PropertyType: "DatatypeProperty"},
			{ID: "p-url", OntologyVersionID: "v-base", Prefix: "base", LocalName: "url", URI: ns + "url", Qname: "base:url", PropertyType: "DatatypeProperty", Label: domain.Translations{"en": "has url"}, Comment: domain.Translations{"en": "the url"}},
		},
		relations: []*domain.OntologyRelationWithSource{
			{OntologyRelation: domain.OntologyRelation{SourceID: "p-url-ext", SourceKind: "property", RelType: "domain", TargetQname: "base:Thing"}, SourceQname: "base:url"},
		},
		versions: map[string]*domain.OntologyVersion{
			"v-base": {ID: "v-base", OntologyID: "ont-base", VersionString: "1.0"},
			"v-ext":  {ID: "v-ext", OntologyID: "ont-ext", VersionString: "0.1"},
		},
		ontologies: map[string]*domain.Ontology{
			"ont-base": {ID: "ont-base", Prefix: "base", Namespace: ns},
			"ont-ext":  {ID: "ont-ext", Prefix: "ext", Namespace: "https://example.org/ext#"},
		},
	}
	versions := []string{"v-ext", "v-base"}
	idx, err := buildIndex(context.Background(), store, nil, keyForRequest(versions, "", "live"), versions, "", testLogger())
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	n := idx.ByQname["base:url"]
	if n == nil || n.Label.Get("en") != "has url" || n.Comment.Get("en") != "the url" {
		t.Fatalf("label/comment not filled from the later row: %+v", n)
	}
	got := suggestionQnames((&IndexedEngine{}).mustSuggest(t, idx, Request{CurrentPath: []string{"base:Thing"}, ScopeClass: "base:Thing", Query: "url", MaxResults: 10}))
	if !got["base:url"] {
		t.Fatalf("typing the label must find the term: %v", got)
	}
}

func (e *IndexedEngine) mustSuggest(t *testing.T, idx *Index, req Request) []Suggestion {
	t.Helper()
	sugs, err := e.suggestFromIndex(idx, req)
	if err != nil {
		t.Fatal(err)
	}
	return sugs
}
