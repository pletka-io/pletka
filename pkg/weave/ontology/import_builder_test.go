package ontology

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	parsedontology "github.com/pletka-io/pletka/pkg/weave/ontology/rdf"
)

func TestBuildImportVersionInputBackfillsEntityPrefixesFromNamespaceResolver(t *testing.T) {
	ont := &domain.Ontology{
		ID:        "ont-crmarchaeo",
		Prefix:    "crmarchaeo",
		Namespace: "http://www.cidoc-crm.org/extensions/crmarchaeo/",
	}
	nsResolver := BuildImportNamespaceResolver([]NamespaceBinding{
		{Prefix: "crmarchaeo", Namespace: "http://www.cidoc-crm.org/extensions/crmarchaeo/"},
		{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
	})
	pr := &parsedontology.ParseResult{
		Version: &parsedontology.OntologyVersion{VersionString: "2.1.1"},
		Classes: []*parsedontology.OntologyClass{
			{
				URI:          "http://www.cidoc-crm.org/extensions/crmarchaeo/A10_Excavation_Interface",
				LocalName:    "A10_Excavation_Interface",
				SuperClasses: []string{"http://www.cidoc-crm.org/cidoc-crm/E26_Physical_Feature"},
			},
		},
		Properties: []*parsedontology.OntologyProperty{
			{
				URI:             "http://www.cidoc-crm.org/extensions/crmarchaeo/AP10_destroyed",
				LocalName:       "AP10_destroyed",
				DomainClasses:   []string{"http://www.cidoc-crm.org/extensions/crmarchaeo/A10_Excavation_Interface"},
				RangeClasses:    []string{"http://www.cidoc-crm.org/cidoc-crm/E18_Physical_Thing"},
				SuperProperties: []string{"http://www.cidoc-crm.org/cidoc-crm/P13_destroyed"},
			},
		},
	}

	input := BuildImportVersionInput(ont, ImportVersionBuildOptions{}, pr, nsResolver)

	if got := input.Classes[0].Prefix; got != "crmarchaeo" {
		t.Fatalf("class prefix=%q, want crmarchaeo", got)
	}
	if got := input.Properties[0].Prefix; got != "crmarchaeo" {
		t.Fatalf("property prefix=%q, want crmarchaeo", got)
	}
	wantTargets := map[string]bool{
		"crm:E26_Physical_Feature":            false,
		"crmarchaeo:A10_Excavation_Interface": false,
		"crm:E18_Physical_Thing":              false,
		"crm:P13_destroyed":                   false,
	}
	for _, rel := range input.Relations {
		if _, ok := wantTargets[rel.TargetQname]; ok {
			wantTargets[rel.TargetQname] = true
		}
		if len(rel.TargetQname) > 0 && rel.TargetQname[0] == ':' {
			t.Fatalf("relation target qname has empty prefix: %q", rel.TargetQname)
		}
	}
	for qname, found := range wantTargets {
		if !found {
			t.Fatalf("missing relation target %q in %#v", qname, input.Relations)
		}
	}
}

func TestBuildImportVersionInputUsesRDFDeclaredNamespaces(t *testing.T) {
	ont := &domain.Ontology{
		ID:        "ont-main",
		Prefix:    "main",
		Namespace: "https://example.org/main/",
	}
	rdfContent := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:ex="https://example.org/ontology/"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <rdfs:Class rdf:about="https://example.org/ontology/Thing">
    <rdfs:label xml:lang="en">Thing</rdfs:label>
  </rdfs:Class>
  <rdf:Property rdf:about="https://example.org/ontology/hasThing">
    <rdfs:label xml:lang="en">has thing</rdfs:label>
    <rdfs:domain rdf:resource="https://example.org/ontology/Thing"/>
    <rdfs:range rdf:resource="https://example.org/ontology/Thing"/>
  </rdf:Property>
</rdf:RDF>`

	bindings := []NamespaceBinding{{Prefix: ont.Prefix, Namespace: ont.Namespace}}
	mgr := BuildImportNamespaceManager(bindings)
	parser := parsedontology.NewRDFParser(parsedontology.WithNamespaceManager(mgr))
	pr, err := parser.Parse(strings.NewReader(rdfContent), "example.rdf", int64(len(rdfContent)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, ns := range pr.DeclaredNamespaces {
		bindings = append(bindings, NamespaceBinding{Prefix: ns.Prefix, Namespace: ns.Namespace})
	}

	input := BuildImportVersionInput(ont, ImportVersionBuildOptions{}, pr, BuildImportNamespaceResolver(bindings))

	if got := input.Classes[0].Prefix; got != "ex" {
		t.Fatalf("class prefix=%q, want ex", got)
	}
	if got := input.Properties[0].Prefix; got != "ex" {
		t.Fatalf("property prefix=%q, want ex", got)
	}
	foundRelation := false
	for _, rel := range input.Relations {
		if rel.TargetQname == "ex:Thing" {
			foundRelation = true
			break
		}
	}
	if !foundRelation {
		t.Fatalf("expected relation target ex:Thing in %#v", input.Relations)
	}

	var foundBinding bool
	for _, binding := range input.NamespaceBindings {
		if binding.Prefix != "ex" || binding.Namespace != "https://example.org/ontology/" {
			continue
		}
		foundBinding = true
		if binding.Weight != NamespaceBindingWeightRDFDeclared {
			t.Fatalf("binding weight=%d, want %d", binding.Weight, NamespaceBindingWeightRDFDeclared)
		}
		if binding.Source != NamespaceBindingSourceRDFDeclared {
			t.Fatalf("binding source=%q, want %q", binding.Source, NamespaceBindingSourceRDFDeclared)
		}
		if binding.OntologyID == nil || *binding.OntologyID != ont.ID {
			t.Fatalf("binding ontology_id=%v, want %s", binding.OntologyID, ont.ID)
		}
	}
	if !foundBinding {
		t.Fatalf("expected RDF-declared namespace binding in %#v", input.NamespaceBindings)
	}
}
