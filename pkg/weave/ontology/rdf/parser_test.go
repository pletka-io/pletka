package rdf

import (
	"strings"
	"testing"

	nsutil "github.com/pletka-io/pletka/pkg/namespace"
)

func TestParseOWLObjectProperties(t *testing.T) {
	// OWL-style: <ObjectProperty> and <DatatypeProperty> (DLNarratives pattern)
	input := `<?xml version="1.0"?>
<rdf:RDF xmlns="http://www.w3.org/2002/07/owl#"
     xmlns:owl="http://www.w3.org/2002/07/owl#"
     xmlns:ex="https://example.org/ontology#"
     xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
     xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
    <Ontology rdf:about="https://example.org/ontology"/>
    <Class rdf:about="https://example.org/ontology#ClassA">
        <rdfs:label>Class A</rdfs:label>
    </Class>
    <ObjectProperty rdf:about="https://example.org/ontology#propOne">
        <rdfs:label>Property One</rdfs:label>
        <rdfs:domain rdf:resource="https://example.org/ontology#ClassA"/>
        <rdfs:range rdf:resource="https://example.org/ontology#ClassA"/>
    </ObjectProperty>
    <DatatypeProperty rdf:about="https://example.org/ontology#propTwo">
        <rdfs:label>Property Two</rdfs:label>
        <rdfs:domain rdf:resource="https://example.org/ontology#ClassA"/>
    </DatatypeProperty>
</rdf:RDF>`

	parser := NewRDFParser()
	result, err := parser.Parse(strings.NewReader(input), "test_v1.0.owl", int64(len(input)))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.Classes) != 1 {
		t.Errorf("expected 1 class, got %d", len(result.Classes))
	}

	if len(result.Properties) != 2 {
		t.Errorf("expected 2 properties, got %d", len(result.Properties))
	}
	if result.Version == nil || result.Version.RDFContent != input {
		t.Fatalf("expected RDFContent to round-trip original source")
	}

	// Verify property types
	propTypes := map[string]string{}
	for _, p := range result.Properties {
		propTypes[p.LocalName] = p.PropertyType
	}

	if propTypes["propOne"] != "ObjectProperty" {
		t.Errorf("expected propOne to be ObjectProperty, got %q", propTypes["propOne"])
	}
	if propTypes["propTwo"] != "DatatypeProperty" {
		t.Errorf("expected propTwo to be DatatypeProperty, got %q", propTypes["propTwo"])
	}
}

func TestParseOWLPropertiesWithRdfID(t *testing.T) {
	// GeoSPARQL pattern: <owl:ObjectProperty rdf:ID="..."> with namespace-prefixed elements
	input := `<?xml version="1.0"?>
<rdf:RDF
    xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
    xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"
    xmlns:owl="http://www.w3.org/2002/07/owl#"
    xmlns:ex="http://www.example.org/ont#"
    xml:base="http://www.example.org/ont">
    <owl:Ontology rdf:about="http://www.example.org/ont"/>
    <owl:Class rdf:ID="Feature">
        <rdfs:label>Feature</rdfs:label>
    </owl:Class>
    <owl:ObjectProperty rdf:ID="hasGeometry">
        <rdfs:label>has geometry</rdfs:label>
        <rdfs:domain rdf:resource="#Feature"/>
    </owl:ObjectProperty>
    <owl:DatatypeProperty rdf:ID="asWKT">
        <rdfs:label>as WKT</rdfs:label>
    </owl:DatatypeProperty>
</rdf:RDF>`

	parser := NewRDFParser()
	result, err := parser.Parse(strings.NewReader(input), "test_v1.0.rdf", int64(len(input)))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.Classes) != 1 {
		t.Errorf("expected 1 class, got %d", len(result.Classes))
	}
	if len(result.Classes) > 0 {
		// rdf:ID="Feature" with xml:base="http://www.example.org/ont" => "http://www.example.org/ont#Feature"
		expected := "http://www.example.org/ont#Feature"
		if result.Classes[0].URI != expected {
			t.Errorf("expected class URI %q, got %q", expected, result.Classes[0].URI)
		}
	}

	if len(result.Properties) != 2 {
		t.Errorf("expected 2 properties, got %d", len(result.Properties))
	}
	if len(result.Properties) > 0 {
		expected := "http://www.example.org/ont#hasGeometry"
		if result.Properties[0].URI != expected {
			t.Errorf("expected property URI %q, got %q", expected, result.Properties[0].URI)
		}
	}
}

func TestParseOWLWithFragmentAboutURI(t *testing.T) {
	// GeoSPARQL pattern: rdf:about="#Feature" (fragment reference) with xml:base
	// The fragment must attach directly to the base without inserting a "/"
	input := `<?xml version="1.0"?>
<rdf:RDF
    xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
    xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"
    xmlns:owl="http://www.w3.org/2002/07/owl#"
    xmlns:geo="http://www.opengis.net/ont/geosparql#"
    xml:base="http://www.opengis.net/ont/geosparql">
    <owl:Ontology rdf:about=""/>
    <owl:Class rdf:about="#Feature">
        <rdfs:label>Feature</rdfs:label>
    </owl:Class>
    <owl:ObjectProperty rdf:about="#hasGeometry">
        <rdfs:label>has geometry</rdfs:label>
    </owl:ObjectProperty>
</rdf:RDF>`

	parser := NewRDFParser()
	result, err := parser.Parse(strings.NewReader(input), "geosparql_v1.0.rdf", int64(len(input)))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.Classes) != 1 {
		t.Fatalf("expected 1 class, got %d", len(result.Classes))
	}
	// rdf:about="#Feature" with xml:base="http://www.opengis.net/ont/geosparql"
	// must resolve to "http://www.opengis.net/ont/geosparql#Feature" (no "/" before "#")
	expected := "http://www.opengis.net/ont/geosparql#Feature"
	if result.Classes[0].URI != expected {
		t.Errorf("expected class URI %q, got %q", expected, result.Classes[0].URI)
	}

	if len(result.Properties) != 1 {
		t.Fatalf("expected 1 property, got %d", len(result.Properties))
	}
	expectedProp := "http://www.opengis.net/ont/geosparql#hasGeometry"
	if result.Properties[0].URI != expectedProp {
		t.Errorf("expected property URI %q, got %q", expectedProp, result.Properties[0].URI)
	}
}

func TestParseRDFDescriptionPattern(t *testing.T) {
	// rdf:Description pattern (CRMarchaeo 2.1.1, CRMtex 2.0)
	input := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:owl="http://www.w3.org/2002/07/owl#"
    xmlns:ex="http://example.org/ontology/"
    xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
    xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
    <rdf:Description rdf:about="http://example.org/ontology/">
        <rdf:type rdf:resource="http://www.w3.org/2002/07/owl#Ontology"/>
        <owl:versionInfo xml:lang="en">Test Ontology 1.0</owl:versionInfo>
    </rdf:Description>
    <rdf:Description rdf:about="http://example.org/ontology/ClassA">
        <rdf:type rdf:resource="http://www.w3.org/2000/01/rdf-schema#Class"/>
        <rdfs:label xml:lang="en">Class A</rdfs:label>
        <rdfs:comment xml:lang="en">A test class</rdfs:comment>
        <rdfs:subClassOf rdf:resource="http://www.cidoc-crm.org/cidoc-crm/E1_CRM_Entity"/>
    </rdf:Description>
    <rdf:Description rdf:about="http://example.org/ontology/ClassB">
        <rdf:type rdf:resource="http://www.w3.org/2000/01/rdf-schema#Class"/>
        <rdfs:label xml:lang="en">Class B</rdfs:label>
    </rdf:Description>
    <rdf:Description rdf:about="http://example.org/ontology/AP1_has_thing">
        <rdf:type rdf:resource="http://www.w3.org/1999/02/22-rdf-syntax-ns#Property"/>
        <rdfs:label xml:lang="en">has thing</rdfs:label>
        <rdfs:domain rdf:resource="http://example.org/ontology/ClassA"/>
        <rdfs:range rdf:resource="http://example.org/ontology/ClassB"/>
    </rdf:Description>
</rdf:RDF>`

	parser := NewRDFParser()
	result, err := parser.Parse(strings.NewReader(input), "test_v1.0.rdf", int64(len(input)))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.Classes) != 2 {
		t.Errorf("expected 2 classes, got %d", len(result.Classes))
	}

	if len(result.Properties) != 1 {
		t.Errorf("expected 1 property, got %d", len(result.Properties))
	}

	// Verify class details
	if len(result.Classes) >= 1 {
		c := result.Classes[0]
		if c.URI != "http://example.org/ontology/ClassA" {
			t.Errorf("expected ClassA URI, got %q", c.URI)
		}
		if c.Label.Get("en") != "Class A" {
			t.Errorf("expected label 'Class A', got %q", c.Label.Get("en"))
		}
		if len(c.SuperClasses) != 1 || c.SuperClasses[0] != "http://www.cidoc-crm.org/cidoc-crm/E1_CRM_Entity" {
			t.Errorf("expected superclass E1_CRM_Entity, got %v", c.SuperClasses)
		}
	}

	// Verify property details
	if len(result.Properties) >= 1 {
		p := result.Properties[0]
		if p.URI != "http://example.org/ontology/AP1_has_thing" {
			t.Errorf("expected AP1_has_thing URI, got %q", p.URI)
		}
		if len(p.DomainClasses) != 1 || p.DomainClasses[0] != "http://example.org/ontology/ClassA" {
			t.Errorf("expected domain ClassA, got %v", p.DomainClasses)
		}
	}
}

func TestParseMixedStyles(t *testing.T) {
	// Mix of RDFS Property and OWL ObjectProperty in the same document
	input := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
     xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"
     xmlns:owl="http://www.w3.org/2002/07/owl#"
     xmlns:ex="http://example.org/ont#"
     xml:base="http://example.org/ont">
    <owl:Ontology rdf:about="http://example.org/ont"/>
    <rdfs:Class rdf:about="http://example.org/ont#Thing">
        <rdfs:label>Thing</rdfs:label>
    </rdfs:Class>
    <rdf:Property rdf:about="http://example.org/ont#rdfsProperty">
        <rdfs:label>RDFS property</rdfs:label>
    </rdf:Property>
    <owl:ObjectProperty rdf:about="http://example.org/ont#owlObjProp">
        <rdfs:label>OWL object property</rdfs:label>
    </owl:ObjectProperty>
    <owl:DatatypeProperty rdf:about="http://example.org/ont#owlDtProp">
        <rdfs:label>OWL datatype property</rdfs:label>
    </owl:DatatypeProperty>
</rdf:RDF>`

	parser := NewRDFParser()
	result, err := parser.Parse(strings.NewReader(input), "mixed_v1.0.rdf", int64(len(input)))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.Classes) != 1 {
		t.Errorf("expected 1 class, got %d", len(result.Classes))
	}

	if len(result.Properties) != 3 {
		t.Errorf("expected 3 properties, got %d", len(result.Properties))
	}
}

func TestParseNamespaceFromClassURI(t *testing.T) {
	// RDFS file with no xml:base and no owl:Ontology element
	// (like CRMdig_v3.2.1.rdfs, CRMpe_v3.1.2.rdfs)
	rdfContent := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <rdfs:Class rdf:about="http://www.ics.forth.gr/isl/CRMdig/D1_Digital_Object">
    <rdfs:label xml:lang="en">D1 Digital Object</rdfs:label>
  </rdfs:Class>
  <rdfs:Class rdf:about="http://www.ics.forth.gr/isl/CRMdig/D2_Digitization_Process">
    <rdfs:label xml:lang="en">D2 Digitization Process</rdfs:label>
  </rdfs:Class>
</rdf:RDF>`

	mgr := nsutil.NewDefaultManager()
	mgr.Put("crmdig", "http://www.ics.forth.gr/isl/CRMdig/", 100)
	parser := NewRDFParser(WithNamespaceManager(mgr))
	result, err := parser.Parse(strings.NewReader(rdfContent), "CRMdig_v3.2.1.rdfs", int64(len(rdfContent)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Namespace should be inferred from class URIs
	if result.Ontology.Namespace == "" {
		t.Error("Namespace should be inferred from class URIs, got empty")
	}
	if result.Ontology.Namespace != "http://www.ics.forth.gr/isl/CRMdig/" {
		t.Errorf("Namespace = %q, want %q", result.Ontology.Namespace, "http://www.ics.forth.gr/isl/CRMdig/")
	}
}

func TestParseClassPrefixFromNamespace(t *testing.T) {
	// RDF with xml:base that is NOT cidoc-crm.org
	rdfContent := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"
         xmlns:owl="http://www.w3.org/2002/07/owl#"
         xml:base="https://linked.art/ns/terms/">
  <owl:Ontology rdf:about="https://linked.art/ns/terms/">
    <rdfs:label>Linked Art</rdfs:label>
  </owl:Ontology>
  <rdfs:Class rdf:about="https://linked.art/ns/terms/Payment">
    <rdfs:label xml:lang="en">Payment</rdfs:label>
  </rdfs:Class>
  <rdfs:Class rdf:about="http://www.cidoc-crm.org/cidoc-crm/E21_Person">
    <rdfs:label xml:lang="en">Person</rdfs:label>
  </rdfs:Class>
</rdf:RDF>`

	mgr := nsutil.NewDefaultManager()
	mgr.Put("la", "https://linked.art/ns/terms/", 100)
	mgr.Put("crm", "http://www.cidoc-crm.org/cidoc-crm/", 100)
	parser := NewRDFParser(WithNamespaceManager(mgr))
	result, err := parser.Parse(strings.NewReader(rdfContent), "linked_art_test.rdf", int64(len(rdfContent)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Ontology namespace must be captured
	if result.Ontology.Namespace != "https://linked.art/ns/terms/" {
		t.Errorf("Namespace = %q, want %q", result.Ontology.Namespace, "https://linked.art/ns/terms/")
	}

	// Find the Payment class (belongs to linked art namespace)
	// and E21_Person (belongs to CRM namespace)
	var paymentPrefix, personPrefix string
	for _, c := range result.Classes {
		switch c.LocalName {
		case "Payment":
			paymentPrefix = c.Prefix
		case "E21_Person":
			personPrefix = c.Prefix
		}
	}

	// Payment's URI is in linked art namespace → should get ontology's prefix
	if paymentPrefix == "" {
		t.Error("Payment class has empty prefix")
	}

	// E21_Person's URI is in CRM namespace → should NOT get linked art prefix
	if personPrefix == paymentPrefix && personPrefix != "" {
		t.Errorf("E21_Person and Payment should have different prefixes, both got %q", personPrefix)
	}
}

func TestParseAddsDeclaredNamespacesToManager(t *testing.T) {
	rdfContent := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:ex="https://example.org/ontology/"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <rdfs:Class rdf:about="https://example.org/ontology/Thing">
    <rdfs:label xml:lang="en">Thing</rdfs:label>
  </rdfs:Class>
</rdf:RDF>`

	mgr := nsutil.NewStore()
	parser := NewRDFParser(WithNamespaceManager(mgr))
	result, err := parser.Parse(strings.NewReader(rdfContent), "example.rdf", int64(len(rdfContent)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(result.DeclaredNamespaces) == 0 {
		t.Fatal("expected RDF namespace declarations to be captured")
	}
	if len(result.MissingNamespaces) != 0 {
		t.Fatalf("expected declared namespace to resolve prefixes, got declared %#v missing %#v", result.DeclaredNamespaces, result.MissingNamespaces)
	}
	if got := result.Classes[0].Prefix; got != "ex" {
		t.Fatalf("class prefix=%q, want ex", got)
	}
	ns, err := mgr.GetWithBase("https://example.org/ontology/")
	if err != nil {
		t.Fatalf("manager should contain declared namespace: %v", err)
	}
	if ns.Prefix != "ex" {
		t.Fatalf("manager prefix=%q, want ex", ns.Prefix)
	}
}

func TestParseFailsOnMissingNamespaceByDefault(t *testing.T) {
	rdfContent := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <rdfs:Class rdf:about="https://missing.example.org/Thing">
    <rdfs:label xml:lang="en">Thing</rdfs:label>
  </rdfs:Class>
</rdf:RDF>`

	parser := NewRDFParser()
	_, err := parser.Parse(strings.NewReader(rdfContent), "missing.rdf", int64(len(rdfContent)))
	if err == nil {
		t.Fatal("expected missing namespace error")
	}
	nsErr, ok := err.(*NamespaceResolutionError)
	if !ok {
		t.Fatalf("error type=%T want *NamespaceResolutionError: %v", err, err)
	}
	if len(nsErr.MissingNamespaces) != 1 || nsErr.MissingNamespaces[0].Namespace != "https://missing.example.org/" {
		t.Fatalf("missing namespaces=%#v", nsErr.MissingNamespaces)
	}
}

func TestParseAllowsMissingNamespacesWhenRequested(t *testing.T) {
	rdfContent := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <rdfs:Class rdf:about="https://missing.example.org/Thing">
    <rdfs:label xml:lang="en">Thing</rdfs:label>
  </rdfs:Class>
</rdf:RDF>`

	parser := NewRDFParser(WithAllowMissingNamespaces())
	result, err := parser.Parse(strings.NewReader(rdfContent), "missing.rdf", int64(len(rdfContent)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.MissingNamespaces) != 1 {
		t.Fatalf("missing namespaces=%#v, want one entry", result.MissingNamespaces)
	}
}

func TestParseFailsOnDeclaredNamespaceConflict(t *testing.T) {
	rdfContent := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:ex="https://example.org/rdf/"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <rdfs:Class rdf:about="https://example.org/rdf/Thing">
    <rdfs:label xml:lang="en">Thing</rdfs:label>
  </rdfs:Class>
</rdf:RDF>`

	mgr := nsutil.NewStore()
	mgr.Put("ex", "https://example.org/manager/", 100)
	parser := NewRDFParser(WithNamespaceManager(mgr))
	_, err := parser.Parse(strings.NewReader(rdfContent), "conflict.rdf", int64(len(rdfContent)))
	if err == nil {
		t.Fatal("expected namespace declaration conflict")
	}
	if !strings.Contains(err.Error(), "namespace declaration conflict") {
		t.Fatalf("error=%v, want namespace declaration conflict", err)
	}
}

func TestResolvePrefixesReportsMissing(t *testing.T) {
	mgr := nsutil.NewDefaultManager()
	mgr.Put("crm", "http://www.cidoc-crm.org/cidoc-crm/", 10)

	classes := []*OntologyClass{
		{URI: "http://www.cidoc-crm.org/cidoc-crm/E21_Person", LocalName: "E21_Person"},
		{URI: "https://missing.example.org/Thing", LocalName: "Thing"},
	}
	properties := []*OntologyProperty{
		{URI: "https://missing.example.org/prop", LocalName: "prop"},
	}

	missing := ResolvePrefixes(classes, properties, mgr)

	// CRM class should be resolved
	if classes[0].Prefix != "crm" {
		t.Fatalf("expected CRM class prefix to be resolved, got %q", classes[0].Prefix)
	}

	// Missing namespace should be reported once for class and once for property (count 2)
	if len(missing) != 1 {
		t.Fatalf("expected 1 missing namespace entry, got %d", len(missing))
	}
	if missing[0].Namespace != "https://missing.example.org/" {
		t.Errorf("missing namespace = %q, want %q", missing[0].Namespace, "https://missing.example.org/")
	}
	if missing[0].Count != 2 {
		t.Errorf("missing count = %d, want 2", missing[0].Count)
	}
}
