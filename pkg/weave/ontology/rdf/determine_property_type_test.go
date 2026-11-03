package rdf

import "testing"

// TestDeterminePropertyType_XSDRanges guards the fix for plain rdf:Property
// classification: any XSD datatype range (not just string/int/date) makes a
// property a DatatypeProperty, so the path builder offers a literal terminal
// for anyURI, decimal, boolean, etc.
func TestDeterminePropertyType_XSDRanges(t *testing.T) {
	const xsd = "http://www.w3.org/2001/XMLSchema#"
	p := &RDFParser{}
	cases := []struct {
		name     string
		rangeURI string
		want     string
	}{
		{"xsd:string", xsd + "string", "DatatypeProperty"},
		{"xsd:integer", xsd + "integer", "DatatypeProperty"},
		{"xsd:date", xsd + "date", "DatatypeProperty"},
		{"xsd:anyURI", xsd + "anyURI", "DatatypeProperty"},
		{"xsd:decimal", xsd + "decimal", "DatatypeProperty"},
		{"xsd:boolean", xsd + "boolean", "DatatypeProperty"},
		{"xsd:gYear", xsd + "gYear", "DatatypeProperty"},
		{"rdfs:Literal", "http://www.w3.org/2000/01/rdf-schema#Literal", "DatatypeProperty"},
		{"class range", "http://www.cidoc-crm.org/cidoc-crm/E55_Type", "ObjectProperty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := p.determinePropertyType(&RDFProperty{Range: []Reference{{Resource: c.rangeURI}}})
			if got != c.want {
				t.Fatalf("determinePropertyType(range=%s) = %q; want %q", c.rangeURI, got, c.want)
			}
		})
	}

	if got := p.determinePropertyType(&RDFProperty{}); got != "ObjectProperty" {
		t.Fatalf("no range: got %q; want ObjectProperty", got)
	}
}
