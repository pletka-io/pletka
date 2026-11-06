package ontologyevidence

import "testing"

// An extension may redeclare a parent class under the parent's URI (Linked Art
// 0.9 carries six CRM classes). The same URI is the same term: it resolves to
// the owner declaring it under its own prefix. Foreign copies alone, or two
// different URIs for one QName, stay ambiguous.
func TestAssessPrefersNativeOwnerOfIdenticalURI(t *testing.T) {
	group := term{URI: "http://www.cidoc-crm.org/cidoc-crm/E74_Group", Prefix: "crm", LocalName: "E74_Group"}
	crm := bodyDocument{Classes: []term{group}}
	crm.Ontology.ID, crm.Ontology.Prefix = "crm", "crm"
	la := bodyDocument{Classes: []term{group}}
	la.Ontology.ID, la.Ontology.Prefix = "la", "la"
	sdh := bodyDocument{Classes: []term{group}}
	sdh.Ontology.ID, sdh.Ontology.Prefix = "sdh", "sdh"
	inv := func(ids ...string) []Body {
		out := make([]Body, 0, len(ids))
		for _, id := range ids {
			out = append(out, Body{OntologyID: id, VersionID: id + "-v"})
		}
		return out
	}
	ref := TermReference{Kind: "class", URI: "crm:E74_Group", Prefix: "crm", LocalName: "E74_Group"}
	status, c := assess(ref, inv("crm", "la"), []bodyDocument{crm, la}, nil)
	if status != Resolved || len(c) != 1 || c[0].OntologyID != "crm" || c[0].VersionID != "crm-v" {
		t.Fatalf("native owner not preferred: %s %+v", status, c)
	}
	if status, _ := assess(ref, inv("la", "sdh"), []bodyDocument{la, sdh}, nil); status != Ambiguous {
		t.Fatalf("two foreign copies: %s", status)
	}
	addons := bodyDocument{Classes: []term{{URI: "http://www.ics.forth.gr/isl/CRMsci/S19", Prefix: "crmsci", LocalName: "S19"}}}
	addons.Ontology.ID, addons.Ontology.Prefix = "addons", "addons"
	sci := bodyDocument{Classes: []term{{URI: "http://www.cidoc-crm.org/extensions/crmsci/S19", Prefix: "crmsci", LocalName: "S19"}}}
	sci.Ontology.ID, sci.Ontology.Prefix = "crmsci", "crmsci"
	if status, _ := assess(TermReference{Kind: "class", URI: "crmsci:S19", Prefix: "crmsci", LocalName: "S19"}, inv("addons", "crmsci"), []bodyDocument{addons, sci}, nil); status != Ambiguous {
		t.Fatalf("different URIs for one QName: %s", status)
	}
}
