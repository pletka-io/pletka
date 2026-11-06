package ontology

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

const offlineTestRDF = `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"
         xmlns:owl="http://www.w3.org/2002/07/owl#"
         xmlns:base="https://example.org/base/">
  <owl:Ontology rdf:about="https://example.org/base/"/>
  <owl:Class rdf:about="https://example.org/base/Thing"><rdfs:label xml:lang="en">Thing</rdfs:label></owl:Class>
  <owl:Class rdf:about="https://example.org/base/Person"><rdfs:subClassOf rdf:resource="https://example.org/base/Thing"/></owl:Class>
  <owl:DatatypeProperty rdf:about="https://example.org/base/name">
    <rdfs:domain rdf:resource="https://example.org/base/Thing"/>
    <rdfs:range rdf:resource="http://www.w3.org/2001/XMLSchema#string"/>
  </owl:DatatypeProperty>
  <owl:ObjectProperty rdf:about="https://example.org/base/knows">
    <rdfs:domain rdf:resource="https://example.org/base/Person"/>
    <rdfs:range rdf:resource="https://example.org/base/Person"/>
  </owl:ObjectProperty>
</rdf:RDF>`

// TestLoadOfflineVersion_RowsCarryQnames pins that an RDF file becomes the
// rows the index needs: qnames on classes and properties, SourceQname on
// relations, the synthetic version id on every row.
func TestLoadOfflineVersion_RowsCarryQnames(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.rdf"), []byte(offlineTestRDF), 0o644); err != nil {
		t.Fatal(err)
	}
	mv, err := LoadOfflineVersion(OfflineVersionSpec{
		Ontology:  &domain.Ontology{ID: "ont-base", Prefix: "base", Namespace: "https://example.org/base/"},
		VersionID: "base@1.0", Version: "1.0", BaseDir: dir, File: "base.rdf",
	})
	if err != nil {
		t.Fatalf("LoadOfflineVersion: %v", err)
	}
	classes := map[string]bool{}
	for _, c := range mv.Classes {
		classes[c.Qname] = true
		if c.OntologyVersionID != "base@1.0" {
			t.Errorf("class %s version id %q", c.Qname, c.OntologyVersionID)
		}
	}
	if !classes["base:Thing"] || !classes["base:Person"] {
		t.Fatalf("classes: %v", classes)
	}
	var sawRange, sawSub bool
	for _, r := range mv.Relations {
		if r.SourceQname == "base:name" && r.RelType == "range" && r.TargetQname == "xsd:string" {
			sawRange = true
		}
		if r.SourceQname == "base:Person" && r.RelType == "subclass_of" && r.TargetQname == "base:Thing" {
			sawSub = true
		}
	}
	if !sawRange || !sawSub {
		t.Fatalf("relations missing: range=%v subclass=%v (%d relations)", sawRange, sawSub, len(mv.Relations))
	}
	if mv.Version.ID != "base@1.0" || mv.Ontology.Prefix != "base" {
		t.Fatalf("version/ontology: %+v %+v", mv.Version, mv.Ontology)
	}
}
