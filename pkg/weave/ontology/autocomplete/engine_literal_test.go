package autocomplete

import "testing"

// TestGenericLiteralSuggestionIsRDFSLiteral pins the convention that a literal
// path terminus without a specific datatype is emitted as rdfs:Literal (the
// RDFS class of all literals) with Type "literal" — not the non-standard
// "rdf:literal" that has no such term. path audits skip Type=="literal", so
// this leaf never reports as a missing class.
func TestGenericLiteralSuggestionIsRDFSLiteral(t *testing.T) {
	if genericLiteralQname != "rdfs:Literal" {
		t.Fatalf("generic literal qname = %q, want rdfs:Literal", genericLiteralQname)
	}

	s := literalSuggestionFromQname(genericLiteralQname)
	if s.Type != "literal" {
		t.Errorf("Type = %q, want literal", s.Type)
	}
	if s.Qname != "rdfs:Literal" || s.URI != "rdfs:Literal" {
		t.Errorf("Qname/URI = %q/%q, want rdfs:Literal", s.Qname, s.URI)
	}
	if s.Prefix != "rdfs" || s.LocalName != "Literal" {
		t.Errorf("prefix/local = %q/%q, want rdfs/Literal", s.Prefix, s.LocalName)
	}
	if s.Datatype != "rdfs:Literal" {
		t.Errorf("Datatype = %q, want rdfs:Literal", s.Datatype)
	}
}
