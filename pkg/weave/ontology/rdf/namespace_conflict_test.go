package rdf

import (
	"encoding/xml"
	"errors"
	"strings"
	"testing"

	nsutil "github.com/pletka-io/pletka/pkg/namespace"
)

// A prefix the file spells differently from the instance binding is a
// problem with the upload, not with the server. The parser has to say so
// in a form the handler can turn into a field error -- a bare error
// string reached the curator as an unexplained 500.
func TestParseReturnsTypedPrefixConflict(t *testing.T) {
	rdfContent := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:cto="https://nfdi4culture.de/ontology/"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <rdfs:Class rdf:about="https://nfdi4culture.de/ontology/Thing">
    <rdfs:label xml:lang="en">Thing</rdfs:label>
  </rdfs:Class>
</rdf:RDF>`

	mgr := nsutil.NewStore()
	if _, err := mgr.Put("cto", "https://nfdi4culture.de/ontology#", 100); err != nil {
		t.Fatalf("seed binding: %v", err)
	}
	parser := NewRDFParser(WithNamespaceManager(mgr))

	_, err := parser.Parse(strings.NewReader(rdfContent), "cto.rdf", int64(len(rdfContent)))
	if err == nil {
		t.Fatal("expected a prefix conflict error")
	}

	var conflict *PrefixConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error=%v (%T), want *PrefixConflictError", err, err)
	}
	if conflict.Prefix != "cto" {
		t.Errorf("Prefix=%q, want cto", conflict.Prefix)
	}
	if conflict.Declared != "https://nfdi4culture.de/ontology/" {
		t.Errorf("Declared=%q, want the URI from the file", conflict.Declared)
	}
	if conflict.Existing != "https://nfdi4culture.de/ontology#" {
		t.Errorf("Existing=%q, want the URI already bound", conflict.Existing)
	}
}

// The malformed-file case shares the dead end: the curator needs the line,
// not a 500. xml.SyntaxError carries it, so the wrap must keep it reachable.
func TestParseKeepsXMLSyntaxErrorUnwrappable(t *testing.T) {
	parser := NewRDFParser()
	broken := `<?xml version="1.0"?><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"`

	_, err := parser.Parse(strings.NewReader(broken), "broken.rdf", int64(len(broken)))
	if err == nil {
		t.Fatal("expected a parse error")
	}

	var syntax *xml.SyntaxError
	if !errors.As(err, &syntax) {
		t.Fatalf("error=%v (%T), want *xml.SyntaxError in the chain", err, err)
	}
}
