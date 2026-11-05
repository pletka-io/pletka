package autocomplete

import (
	"context"
	"testing"
)

// buildTerminalRangeIndex builds the terminalRangeStore index, in which
// rdfs:Resource, rdfs:Literal and xsd:string are real class nodes.
func buildTerminalRangeIndex(t *testing.T) *Index {
	t.Helper()
	versions := []string{"v-own", "v-defaults"}
	key := keyForRequest(versions, "v-own", "live")
	idx, err := buildIndex(context.Background(), terminalRangeStore(), nil, key, versions, "v-own", testLogger())
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	return idx
}

// TestRootClasses_ExcludeLiteralTypesAndResource pins that the scope-class
// picker (empty current_path) never offers a datatype, rdfs:Literal or the
// implicit root, even though the defaults ontology stores them as class rows.
func TestRootClasses_ExcludeLiteralTypesAndResource(t *testing.T) {
	idx := buildTerminalRangeIndex(t)
	eng := &IndexedEngine{}
	got := suggestionQnames(eng.suggestRootClasses(idx, Request{MaxResults: 50}))
	for _, bad := range []string{"xsd:string", "rdfs:Literal", "rdfs:Resource"} {
		if got[bad] {
			t.Errorf("scope picker offered %s; got %v", bad, got)
		}
	}
	if !got["ex:Thing"] {
		t.Errorf("scope picker must still offer ordinary classes; got %v", got)
	}
	// A query that matches a datatype's local name must not surface it either.
	if q := suggestionQnames(eng.suggestRootClasses(idx, Request{MaxResults: 50, Query: "str"})); q["xsd:string"] {
		t.Errorf("query 'str' offered xsd:string; got %v", q)
	}
}

// TestGetSuggestions_LiteralTerminusCannotContinue pins that a path whose last
// element is a literal type or rdfs:Resource is finished: no properties are
// offered after it, even though those qnames now resolve to real nodes.
func TestGetSuggestions_LiteralTerminusCannotContinue(t *testing.T) {
	idx := buildTerminalRangeIndex(t)
	eng := &IndexedEngine{}
	for _, last := range []string{"xsd:string", "rdfs:Literal", "rdfs:Resource"} {
		got, err := eng.suggestFromIndex(idx, Request{CurrentPath: []string{"ex:Thing", "ex:sameAs", last}, MaxResults: 50})
		if err != nil {
			t.Fatalf("%s: %v", last, err)
		}
		if len(got) != 0 {
			t.Errorf("path ending in %s must offer nothing, got %v", last, suggestionOrder(got))
		}
	}
	// Control: a class terminus still continues.
	got, err := eng.suggestFromIndex(idx, Request{CurrentPath: []string{"ex:Thing"}, MaxResults: 50})
	if err != nil || len(got) == 0 {
		t.Errorf("class terminus must still offer properties, got %v err %v", suggestionOrder(got), err)
	}
}
