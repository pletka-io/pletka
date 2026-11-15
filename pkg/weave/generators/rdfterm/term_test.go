package rdfterm

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestIsDatatype(t *testing.T) {
	for _, tc := range []struct {
		e    domain.PathElement
		want bool
	}{
		{domain.PathElement{Type: "class", Prefix: "xsd", LocalName: "date"}, true},
		{domain.PathElement{Type: "class", URI: "http://www.w3.org/2001/XMLSchema#dateTime"}, true},
		{domain.PathElement{Type: "class", Prefix: "rdf", LocalName: "langString"}, true},
		{domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E52_Time-Span"}, false},
	} {
		if got := IsDatatype(tc.e); got != tc.want {
			t.Errorf("IsDatatype(%s) = %v, want %v", tc.e.PrefixedName(), got, tc.want)
		}
	}
}
