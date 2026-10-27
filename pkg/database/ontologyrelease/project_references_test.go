package ontologyrelease

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/namespace"
)

func TestResolveProjectReferenceUsesExactPublishedTerms(t *testing.T) {
	term := bodyTerm{URI: "https://old.example/A", Prefix: "ex", LocalName: "A"}
	body := bodyDocument{Classes: []bodyTerm{term}, Bindings: []namespace.Binding{
		{Prefix: "ex", Namespace: "https://phantom.example/", Weight: 80},
		{Prefix: "alias", Namespace: "https://old.example/", Weight: 20},
	}}
	p := publisher{pins: map[string]Pin{"one": {OntologyID: "one", VersionID: "v1", Digest: "digest"}}, sources: map[string]bodyDocument{"one": body}}
	for _, tc := range []struct {
		name string
		ref  TermReference
		ok   bool
	}{
		{"term QName preserves old URI", TermReference{Kind: "class", URI: "ex:A"}, true},
		{"declared alias", TermReference{Kind: "class", URI: "alias:A"}, true},
		{"absolute URI", TermReference{Kind: "class", URI: term.URI}, true},
		{"agreeing URI and QName", TermReference{Kind: "class", URI: term.URI, Prefix: "ex", LocalName: "A"}, true},
		{"undeclared prefix", TermReference{Kind: "class", URI: "guess:A"}, false},
		{"wrong kind", TermReference{Kind: "property", URI: term.URI}, false},
		{"contradictory QName", TermReference{Kind: "class", URI: term.URI, Prefix: "ex", LocalName: "B"}, false},
		{"incomplete QName", TermReference{Kind: "class", URI: term.URI, Prefix: "ex"}, false},
		{"wrong explicit version", TermReference{Kind: "class", URI: term.URI, VersionID: "v2"}, false},
		{"phantom namespace is not identity", TermReference{Kind: "class", URI: "https://phantom.example/A"}, false},
		{"empty", TermReference{Kind: "class"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveProjectReference(&p, tc.ref, nil)
			if (err == nil) != tc.ok {
				t.Fatalf("resolution = %+v, %v", got, err)
			}
			if tc.ok && (got.URI != term.URI || got.VersionID != "v1") {
				t.Fatalf("rewrote identity: %+v", got)
			}
		})
	}
	p.pins["two"] = Pin{OntologyID: "two", VersionID: "v2"}
	p.sources["two"] = body
	if _, err := resolveProjectReference(&p, TermReference{Kind: "class", URI: term.URI}, nil); err == nil {
		t.Fatal("ambiguous ownership accepted")
	}
	if got, err := resolveProjectReference(&p, TermReference{Kind: "class", URI: term.URI, VersionID: "v2"}, nil); err != nil || got.OntologyID != "two" {
		t.Fatalf("explicit version did not disambiguate: %+v, %v", got, err)
	}
}
