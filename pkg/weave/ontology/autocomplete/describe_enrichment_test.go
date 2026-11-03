package autocomplete

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestDescribeNode_InheritedPropertiesAndLabels builds a tiny graph where a
// subclass inherits a property from its superclass and checks that DescribeNode
// separates direct from inherited properties and carries a label for every
// referenced term (so a consumer never has to render a bare opaque qname).
func TestDescribeNode_InheritedPropertiesAndLabels(t *testing.T) {
	entity := &Node{Qname: "obo:BFO_0000001", Type: "class", Label: domain.Translations{"en": "entity"}}
	person := &Node{Qname: "nfdi:NFDI_0000004", Type: "class", Label: domain.Translations{"en": "person"}}
	// person subclass_of entity
	person.Superclasses = []*Node{entity}
	entity.Subclasses = []*Node{person}
	// sdo:name declared on entity (inherited by person); sdo:givenName on person
	name := &Node{Qname: "sdo:name", Type: "property", PropertyType: "DatatypeProperty", Label: domain.Translations{"en": "name"}}
	given := &Node{Qname: "sdo:givenName", Type: "property", PropertyType: "DatatypeProperty", Label: domain.Translations{"en": "given name"}}
	entity.AsDomainOf = []*Node{name}
	person.AsDomainOf = []*Node{given}

	td := DescribeNode(person)
	if td == nil {
		t.Fatal("DescribeNode returned nil")
	}
	if got := td.AsDomainOf; len(got) != 1 || got[0] != "sdo:givenName" {
		t.Fatalf("AsDomainOf (direct) = %v; want [sdo:givenName]", got)
	}
	if got := td.InheritedProperties; len(got) != 1 || got[0] != "sdo:name" {
		t.Fatalf("InheritedProperties = %v; want [sdo:name]", got)
	}
	if got := td.Superclasses; len(got) != 1 || got[0] != "obo:BFO_0000001" {
		t.Fatalf("Superclasses = %v; want [obo:BFO_0000001]", got)
	}
	// Labels cover the term and every referenced qname by name.
	for _, q := range []string{"nfdi:NFDI_0000004", "obo:BFO_0000001", "sdo:name", "sdo:givenName"} {
		if _, ok := td.Labels[q]; !ok {
			t.Errorf("Labels missing %q; got keys %v", q, keysOf(td.Labels))
		}
	}
	if td.Labels["obo:BFO_0000001"]["en"] != "entity" {
		t.Errorf("label for obo:BFO_0000001 = %q; want entity", td.Labels["obo:BFO_0000001"]["en"])
	}
}

func keysOf(m map[string]domain.Translations) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
