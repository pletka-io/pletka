package rdf

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func ontologyDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "..", "configs", "ontologies", "cidoc-crm")
}

func TestParseRealDLNarratives(t *testing.T) {
	path := filepath.Join(ontologyDir(), "DLNarratives_v2.0.owl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("DLNarratives file not found, skipping")
	}

	parser := NewRDFParser()
	result, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if result.Version.ClassCount != len(result.Classes) {
		t.Errorf("ClassCount mismatch: version says %d, got %d", result.Version.ClassCount, len(result.Classes))
	}

	if len(result.Classes) < 20 {
		t.Errorf("expected at least 20 classes, got %d", len(result.Classes))
	}

	if len(result.Properties) < 20 {
		t.Errorf("expected at least 20 properties (ObjectProperty + DatatypeProperty), got %d", len(result.Properties))
	}

	t.Logf("DLNarratives: %d classes, %d properties", len(result.Classes), len(result.Properties))
}

func TestParseRealGeoSPARQL(t *testing.T) {
	path := filepath.Join(ontologyDir(), "GeoSPARQL_v1.0.rdf")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("GeoSPARQL file not found, skipping")
	}

	parser := NewRDFParser()
	result, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.Classes) < 3 {
		t.Errorf("expected at least 3 classes, got %d", len(result.Classes))
	}

	if len(result.Properties) < 30 {
		t.Errorf("expected at least 30 properties (ObjectProperty + DatatypeProperty), got %d", len(result.Properties))
	}

	// Verify URIs are properly resolved (rdf:ID should become full URIs)
	for _, c := range result.Classes {
		if c.URI == "" {
			t.Error("found class with empty URI")
		}
		if c.LocalName == "" {
			t.Errorf("found class with empty local name: %s", c.URI)
		}
	}

	t.Logf("GeoSPARQL: %d classes, %d properties", len(result.Classes), len(result.Properties))
}

func TestParseRealCRMarchaeo211(t *testing.T) {
	path := filepath.Join(ontologyDir(), "CRMarchaeo_v2.1.1.rdf")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("CRMarchaeo 2.1.1 file not found, skipping")
	}

	parser := NewRDFParser()
	result, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.Classes) < 8 {
		t.Errorf("expected at least 8 classes (rdf:Description pattern), got %d", len(result.Classes))
	}

	if len(result.Properties) < 30 {
		t.Errorf("expected at least 30 properties (rdf:Description pattern), got %d", len(result.Properties))
	}

	t.Logf("CRMarchaeo 2.1.1: %d classes, %d properties", len(result.Classes), len(result.Properties))
}

func TestParseRealCRMtex20(t *testing.T) {
	path := filepath.Join(ontologyDir(), "CRMtex_v2.rdf")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("CRMtex 2.0 file not found, skipping")
	}

	parser := NewRDFParser()
	result, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.Classes) < 5 {
		t.Errorf("expected at least 5 classes (rdf:Description pattern), got %d", len(result.Classes))
	}

	if len(result.Properties) < 10 {
		t.Errorf("expected at least 10 properties (rdf:Description pattern), got %d", len(result.Properties))
	}

	t.Logf("CRMtex 2.0: %d classes, %d properties", len(result.Classes), len(result.Properties))
}
