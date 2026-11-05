package gitmaterializer

import (
	"encoding/json"
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
