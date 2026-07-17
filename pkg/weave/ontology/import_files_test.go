package ontology

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestBuildImportVersionInputFromFileMergesCompanions(t *testing.T) {
	dir := t.TempDir()
	writeTestRDF(t, filepath.Join(dir, "base.rdf"), `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:ex="https://example.org/ontology/"
         xmlns:owl="http://www.w3.org/2002/07/owl#"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <owl:Ontology rdf:about="https://example.org/ontology/">
    <owl:versionInfo>1.0</owl:versionInfo>
  </owl:Ontology>
  <rdfs:Class rdf:about="https://example.org/ontology/BaseThing">
    <rdfs:label xml:lang="en">Base thing</rdfs:label>
  </rdfs:Class>
</rdf:RDF>`)
	writeTestRDF(t, filepath.Join(dir, "companion.rdf"), `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:ex="https://example.org/ontology/"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <rdfs:Class rdf:about="https://example.org/ontology/CompanionThing">
    <rdfs:label xml:lang="en">Companion thing</rdfs:label>
    <rdfs:subClassOf rdf:resource="https://example.org/ontology/BaseThing"/>
  </rdfs:Class>
</rdf:RDF>`)

	ont := &domain.Ontology{
		ID:        "ont-example",
		Prefix:    "ex",
		Namespace: "https://example.org/ontology/",
	}
	result, err := BuildImportVersionInputFromFile(ImportVersionFileOptions{
		BaseDir:           dir,
		File:              "base.rdf",
		Ontology:          ont,
		VersionString:     "1.0",
		NamespaceBindings: []NamespaceBinding{{Prefix: "ex", Namespace: "https://example.org/ontology/"}},
		Companions: []ImportCompanionFile{
			{File: "companion.rdf", Description: "Companion module"},
		},
	})
	if err != nil {
		t.Fatalf("BuildImportVersionInputFromFile: %v", err)
	}

	if len(result.Input.Classes) != 2 {
		t.Fatalf("classes=%d, want 2: %#v", len(result.Input.Classes), result.Input.Classes)
	}
	var companionSource string
	for _, class := range result.Input.Classes {
		if class.LocalName == "CompanionThing" {
			companionSource = class.SourceModule
		}
	}
	if companionSource != "companion.rdf" {
		t.Fatalf("companion source=%q, want companion.rdf", companionSource)
	}
	if len(result.Companions) != 1 || result.Companions[0].ClassCount != 1 {
		t.Fatalf("companions=%#v, want one class-count summary", result.Companions)
	}

	var metadata struct {
		Companions []struct {
			File          string `json:"file"`
			Description   string `json:"description"`
			ClassCount    int    `json:"class_count"`
			PropertyCount int    `json:"property_count"`
		} `json:"companions"`
	}
	if err := json.Unmarshal(result.Input.Version.OntologyMetadata, &metadata); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if len(metadata.Companions) != 1 || metadata.Companions[0].File != "companion.rdf" {
		t.Fatalf("metadata companions=%#v", metadata.Companions)
	}
}

func TestBuildImportVersionInputFromFileSkipsMissingCompanion(t *testing.T) {
	dir := t.TempDir()
	writeTestRDF(t, filepath.Join(dir, "base.rdf"), `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:ex="https://example.org/ontology/"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <rdfs:Class rdf:about="https://example.org/ontology/BaseThing">
    <rdfs:label xml:lang="en">Base thing</rdfs:label>
  </rdfs:Class>
</rdf:RDF>`)

	result, err := BuildImportVersionInputFromFile(ImportVersionFileOptions{
		BaseDir:               dir,
		File:                  "base.rdf",
		Ontology:              &domain.Ontology{ID: "ont-example", Prefix: "ex", Namespace: "https://example.org/ontology/"},
		VersionString:         "1.0",
		NamespaceBindings:     []NamespaceBinding{{Prefix: "ex", Namespace: "https://example.org/ontology/"}},
		Companions:            []ImportCompanionFile{{File: "missing.rdf"}},
		SkipMissingCompanions: true,
	})
	if err != nil {
		t.Fatalf("BuildImportVersionInputFromFile: %v", err)
	}
	if len(result.SkippedCompanions) != 1 || result.SkippedCompanions[0] != "missing.rdf" {
		t.Fatalf("skipped=%#v, want missing.rdf", result.SkippedCompanions)
	}
}

func writeTestRDF(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
