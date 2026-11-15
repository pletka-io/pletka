package rdfterm

import (
	"github.com/pletka-io/pletka/pkg/weave/generators"
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

func testNS() generators.NamespaceSet {
	return generators.NamespaceSet{
		ByPrefix: map[string]string{"aat": "http://vocab.getty.edu/aat/"},
		Bindings: []generators.NamespaceBinding{{Prefix: "aat", Namespace: "http://vocab.getty.edu/aat/"}},
	}
}

func TestTermExpandsCURIEAndURN(t *testing.T) {
	ns := testNS()
	for in, want := range map[string]string{
		"http://vocab.getty.edu/aat/300033618": "<http://vocab.getty.edu/aat/300033618>",
		"aat:300033618":                        "<http://vocab.getty.edu/aat/300033618>",
		"urn:uuid:1234":                        "<urn:uuid:1234>",
		"unbound:x":                            `"unbound:x"`,
		"a fixed text":                         `"a fixed text"`,
		"https://example.org/a b>":             `"https://example.org/a b>"`,
	} {
		if got := Term(in, ns); got != want {
			t.Errorf("Term(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestUnboundPrefix(t *testing.T) {
	ns := testNS()
	if p, ok := UnboundPrefix("unbound:x", ns); !ok || p != "unbound" {
		t.Errorf("UnboundPrefix(unbound:x) = %q, %v", p, ok)
	}
	for _, v := range []string{"aat:1", "http://a/b", "urn:x", "plain", "a b:c", ""} {
		if p, ok := UnboundPrefix(v, ns); ok {
			t.Errorf("UnboundPrefix(%q) = %q, want none", v, p)
		}
	}
}

func TestQuoteEscapes(t *testing.T) {
	got := Quote("a \"q\" \\ b\nc\td\x01")
	want := `"a \"q\" \\ b\nc\td\u0001"`
	if got != want {
		t.Fatalf("Quote = %s, want %s", got, want)
	}
}

func TestClassList(t *testing.T) {
	e := domain.PathElement{Type: "class", Prefix: "crmdig", LocalName: "D1", AdditionalTypes: []domain.TypeRef{{Prefix: "crm", LocalName: "E36"}}}
	if got := ClassList(e); got != "crmdig:D1, crm:E36" {
		t.Fatalf("ClassList = %q", got)
	}
}
