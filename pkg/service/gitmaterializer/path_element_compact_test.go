package gitmaterializer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestDecodePathElements_CompactTypesByPosition pins the authored compact
// form: a list of qname strings alternates property, class, property, ...
// and a literal terminal (xsd:*, rdfs:Literal, rdfs:Resource) is a literal.
func TestDecodePathElements_CompactTypesByPosition(t *testing.T) {
	cases := []struct {
		name  string
		path  []string
		types []string
	}{
		{"property then class", []string{"crm:P1_is_identified_by", "crm:E41_Appellation"}, []string{"property", "class"}},
		{"datatype terminal", []string{"sdo:name", "xsd:string"}, []string{"property", "literal"}},
		{"generic literal terminal", []string{"rdfs:comment", "rdfs:Literal"}, []string{"property", "literal"}},
		{"resource terminal", []string{"owl:sameAs", "rdfs:Resource"}, []string{"property", "literal"}},
		{"two hops", []string{"sdo:dataFeedElement", "sdo:DataFeedItem", "sdo:item", "obo:BFO_0000001"}, []string{"property", "class", "property", "class"}},
		{"rdf datatype terminal", []string{"rdfs:label", "rdf:langString"}, []string{"property", "literal"}},
		{"rdf property is a property", []string{"rdf:value", "xsd:string"}, []string{"property", "literal"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := make([]canonicalPathElement, 0, len(tc.path))
			for _, q := range tc.path {
				b, _ := json.Marshal(q)
				raw = append(raw, canonicalPathElement(b))
			}
			got, err := decodePathElements(raw)
			if err != nil {
				t.Fatal(err)
			}
			types := make([]string, 0, len(got))
			for _, pe := range got {
				types = append(types, pe.Type)
			}
			if diff := cmp.Diff(tc.types, types); diff != "" {
				t.Errorf("types mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestDecodePathElements_FullMapKeepsItsType pins that the literal rule is
// a compact-form convenience only: a full path-element map carries its own
// type and is stored verbatim, even for an rdf: or rdfs: term.
func TestDecodePathElements_FullMapKeepsItsType(t *testing.T) {
	raw := []canonicalPathElement{
		canonicalPathElement(`{"type":"property","prefix":"rdf","local_name":"value","uri":"http://www.w3.org/1999/02/22-rdf-syntax-ns#value","position":0}`),
		canonicalPathElement(`{"type":"class","prefix":"rdfs","local_name":"Resource","uri":"http://www.w3.org/2000/01/rdf-schema#Resource","position":1}`),
	}
	got, err := decodePathElements(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Type != "property" || got[1].Type != "class" {
		t.Fatalf("full-map types changed: %q, %q", got[0].Type, got[1].Type)
	}
}

// TestDecodePathElementsWith_ExpandsCompactURIs pins that a compact element
// gets a full IRI from the prefix expander, as the materializer writes for
// UI-built fields, and that an unbound prefix is an error rather than a
// qname stored where an IRI belongs.
func TestDecodePathElementsWith_ExpandsCompactURIs(t *testing.T) {
	expand := snapshotPrefixExpander(&projectManifestNamespaces{EffectiveBindings: []projectManifestNamespaceBinding{
		{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/", Source: "ontology"},
	}})
	raw := func(qs ...string) []canonicalPathElement {
		out := make([]canonicalPathElement, 0, len(qs))
		for _, q := range qs {
			b, _ := json.Marshal(q)
			out = append(out, canonicalPathElement(b))
		}
		return out
	}
	got, err := decodePathElementsWith(raw("crm:P1_is_identified_by", "xsd:string"), expand)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"http://www.cidoc-crm.org/cidoc-crm/P1_is_identified_by", "http://www.w3.org/2001/XMLSchema#string"}
	for i, w := range want {
		if got[i].URI != w {
			t.Errorf("element %d uri = %q, want %q", i, got[i].URI, w)
		}
	}
	if _, err := decodePathElementsWith(raw("zzz:unknown"), expand); err == nil || !strings.Contains(err.Error(), "unbound prefix") {
		t.Fatalf("unbound prefix must fail, got %v", err)
	}
	// A full map keeps its own uri untouched.
	full := []canonicalPathElement{canonicalPathElement(`{"type":"property","prefix":"crm","local_name":"P2_has_type","uri":"http://example.org/kept","position":0}`)}
	got, err = decodePathElementsWith(full, expand)
	if err != nil || got[0].URI != "http://example.org/kept" {
		t.Fatalf("full map uri changed: %q err %v", got[0].URI, err)
	}
}
