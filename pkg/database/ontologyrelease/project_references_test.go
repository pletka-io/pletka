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

// An extension may redeclare a parent class under the parent's URI (Linked Art
// 0.9 redeclares six CRM classes). The same URI is the same term: a reference
// resolves to the owner whose own prefix the term carries, not to "two
// matches". Two foreign copies, or two different URIs for one QName, stay
// ambiguous.
func TestResolveProjectReferencePrefersNativeOwnerOfIdenticalURI(t *testing.T) {
	group := bodyTerm{URI: "http://www.cidoc-crm.org/cidoc-crm/E74_Group", Prefix: "crm", LocalName: "E74_Group"}
	crm := bodyDocument{Classes: []bodyTerm{group}}
	crm.Ontology.ID, crm.Ontology.Prefix = "crm", "crm"
	la := bodyDocument{Classes: []bodyTerm{group}}
	la.Ontology.ID, la.Ontology.Prefix = "la", "la"
	p := publisher{
		pins:    map[string]Pin{"crm": {OntologyID: "crm", VersionID: "crm-7"}, "la": {OntologyID: "la", VersionID: "la-0.9"}},
		sources: map[string]bodyDocument{"crm": crm, "la": la},
	}
	got, err := resolveProjectReference(&p, TermReference{Kind: "class", URI: "crm:E74_Group", Prefix: "crm", LocalName: "E74_Group"}, nil)
	if err != nil || got.OntologyID != "crm" || got.VersionID != "crm-7" || got.URI != group.URI {
		t.Fatalf("native owner not preferred: %+v, %v", got, err)
	}
	// Two foreign copies and no native owner: still ambiguous.
	sdh := bodyDocument{Classes: []bodyTerm{group}}
	sdh.Ontology.ID, sdh.Ontology.Prefix = "sdh", "sdh"
	q := publisher{pins: map[string]Pin{"la": p.pins["la"], "sdh": {OntologyID: "sdh", VersionID: "sdh-1"}}, sources: map[string]bodyDocument{"la": la, "sdh": sdh}}
	if _, err := resolveProjectReference(&q, TermReference{Kind: "class", URI: "crm:E74_Group", Prefix: "crm", LocalName: "E74_Group"}, nil); err == nil {
		t.Fatal("two foreign copies accepted")
	}
	// Same QName, different URIs (la-addons 1.0's S19 against crmsci 2.0's): ambiguous.
	other := bodyDocument{Classes: []bodyTerm{{URI: "http://www.ics.forth.gr/isl/CRMsci/S19", Prefix: "crmsci", LocalName: "S19"}}}
	other.Ontology.ID, other.Ontology.Prefix = "addons", "addons"
	sci := bodyDocument{Classes: []bodyTerm{{URI: "http://www.cidoc-crm.org/extensions/crmsci/S19", Prefix: "crmsci", LocalName: "S19"}}}
	sci.Ontology.ID, sci.Ontology.Prefix = "crmsci", "crmsci"
	r := publisher{pins: map[string]Pin{"addons": {OntologyID: "addons", VersionID: "a1"}, "crmsci": {OntologyID: "crmsci", VersionID: "s2"}}, sources: map[string]bodyDocument{"addons": other, "crmsci": sci}}
	if _, err := resolveProjectReference(&r, TermReference{Kind: "class", URI: "crmsci:S19", Prefix: "crmsci", LocalName: "S19"}, nil); err == nil {
		t.Fatal("different URIs for one QName accepted")
	}
}
