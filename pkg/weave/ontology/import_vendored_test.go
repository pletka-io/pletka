package ontology_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// writeVendoredTestRDF writes a tiny valid RDFS/OWL fixture — a couple of
// classes and a property — mirroring the inline fixture used by
// import_files_test.go's TestBuildImportVersionInputFromFileMergesCompanions.
func writeVendoredTestRDF(t *testing.T, path, namespace string) {
	t.Helper()
	content := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:vx="` + namespace + `"
         xmlns:owl="http://www.w3.org/2002/07/owl#"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <owl:Ontology rdf:about="` + namespace + `">
    <owl:versionInfo>1.0</owl:versionInfo>
  </owl:Ontology>
  <rdfs:Class rdf:about="` + namespace + `Thing">
    <rdfs:label xml:lang="en">Thing</rdfs:label>
  </rdfs:Class>
  <rdfs:Class rdf:about="` + namespace + `Actor">
    <rdfs:label xml:lang="en">Actor</rdfs:label>
  </rdfs:Class>
  <rdf:Property rdf:about="` + namespace + `has_thing">
    <rdfs:label xml:lang="en">has thing</rdfs:label>
    <rdfs:domain rdf:resource="` + namespace + `Actor"/>
    <rdfs:range rdf:resource="` + namespace + `Thing"/>
  </rdf:Property>
</rdf:RDF>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// newVendoredImportTestService wires a Service against the shared local
// test database, skipping (per testdb.Pool) when it is unavailable.
func newVendoredImportTestService(t *testing.T) *weaveontology.Service {
	t.Helper()
	pool := testdb.Pool(t)
	store := weaveontology.NewPostgresStore(pool)
	return weaveontology.NewService(store, nil, nil)
}

func TestImportVendoredVersion_FreshImportCreatesOntologyAndVersion(t *testing.T) {
	svc := newVendoredImportTestService(t)
	ctx := context.Background()
	dir := t.TempDir()

	namespace := "https://example.org/vendored/fresh/"
	writeVendoredTestRDF(t, filepath.Join(dir, "vx.rdf"), namespace)

	ontologyID := weaveontology.GenerateOntologyID("vx-fresh-test")
	versionID := weaveontology.GenerateVersionID("vx-fresh-test", "1.0")

	req := weaveontology.VendoredOntologyImportRequest{
		OntologyID:    ontologyID,
		VersionID:     versionID,
		VersionString: "1.0",
		Slug:          "vx-fresh-test",
		Title:         "VX Fresh Test Ontology",
		Kind:          "base",
		Namespace:     namespace,
		Prefixes:      []string{"vx"},
		BaseDir:       dir,
		Files:         []string{"vx.rdf"},
	}
	t.Cleanup(func() {
		_ = svc.DeleteVersion(context.Background(), versionID)
		_ = svc.DeleteOntology(context.Background(), ontologyID)
	})

	if err := svc.ImportVendoredVersion(ctx, req); err != nil {
		t.Fatalf("ImportVendoredVersion: %v", err)
	}

	ont, err := svc.GetOntology(ctx, ontologyID)
	if err != nil {
		t.Fatalf("GetOntology: %v", err)
	}
	if ont.Prefix != "vx" {
		t.Fatalf("ontology prefix=%q, want vx", ont.Prefix)
	}
	if ont.Namespace != namespace {
		t.Fatalf("ontology namespace=%q, want %q", ont.Namespace, namespace)
	}

	version, err := svc.GetVersion(ctx, versionID)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if version.VersionString != "1.0" {
		t.Fatalf("version string=%q, want 1.0", version.VersionString)
	}

	classes, err := svc.ListClasses(ctx, versionID)
	if err != nil {
		t.Fatalf("ListClassesByVersion: %v", err)
	}
	if len(classes) == 0 {
		t.Fatal("expected classes > 0")
	}

	properties, err := svc.ListProperties(ctx, versionID)
	if err != nil {
		t.Fatalf("ListPropertiesByVersion: %v", err)
	}
	if len(properties) == 0 {
		t.Fatal("expected properties > 0")
	}
}

func TestImportVendoredVersion_SecondCallIsIdempotent(t *testing.T) {
	svc := newVendoredImportTestService(t)
	ctx := context.Background()
	dir := t.TempDir()

	namespace := "https://example.org/vendored/idem/"
	writeVendoredTestRDF(t, filepath.Join(dir, "vx.rdf"), namespace)

	ontologyID := weaveontology.GenerateOntologyID("vx-idem-test")
	versionID := weaveontology.GenerateVersionID("vx-idem-test", "1.0")

	req := weaveontology.VendoredOntologyImportRequest{
		OntologyID:    ontologyID,
		VersionID:     versionID,
		VersionString: "1.0",
		Slug:          "vx-idem-test",
		Title:         "VX Idempotent Test Ontology",
		Kind:          "base",
		Namespace:     namespace,
		Prefixes:      []string{"vx"},
		BaseDir:       dir,
		Files:         []string{"vx.rdf"},
	}
	t.Cleanup(func() {
		_ = svc.DeleteVersion(context.Background(), versionID)
		_ = svc.DeleteOntology(context.Background(), ontologyID)
	})

	if err := svc.ImportVendoredVersion(ctx, req); err != nil {
		t.Fatalf("first ImportVendoredVersion: %v", err)
	}
	classesFirst, err := svc.ListClasses(ctx, versionID)
	if err != nil {
		t.Fatalf("ListClassesByVersion: %v", err)
	}

	// Second call: point BaseDir at a directory that doesn't exist to prove
	// the idempotency check short-circuits before any file is touched.
	req.BaseDir = filepath.Join(dir, "does-not-exist")
	if err := svc.ImportVendoredVersion(ctx, req); err != nil {
		t.Fatalf("second ImportVendoredVersion: %v", err)
	}

	classesSecond, err := svc.ListClasses(ctx, versionID)
	if err != nil {
		t.Fatalf("ListClassesByVersion after second call: %v", err)
	}
	if len(classesSecond) != len(classesFirst) {
		t.Fatalf("classes changed across idempotent call: first=%d second=%d", len(classesFirst), len(classesSecond))
	}
}

func TestImportVendoredVersion_RejectsUnsafeFilePaths(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{name: "path traversal", file: "../escape.rdf"},
		{name: "nested path traversal", file: "sub/../../escape.rdf"},
		{name: "absolute path", file: "/etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No store/DB access is needed: the path guard must reject the
			// request before ImportVendoredVersion ever calls the store.
			svc := weaveontology.NewService(nil, nil, nil)

			req := weaveontology.VendoredOntologyImportRequest{
				OntologyID:    "unsafe-ontology",
				VersionID:     "unsafe-ontology-v1",
				VersionString: "1.0",
				Slug:          "unsafe-ontology",
				Namespace:     "https://example.org/unsafe/",
				Prefixes:      []string{"ux"},
				BaseDir:       t.TempDir(),
				Files:         []string{tt.file},
			}

			err := svc.ImportVendoredVersion(context.Background(), req)
			if err == nil {
				t.Fatal("ImportVendoredVersion() = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.file) {
				t.Fatalf("error %q does not name the unsafe file %q", err.Error(), tt.file)
			}
		})
	}
}

func TestImportVendoredVersion_MissingFileErrorsNamingModule(t *testing.T) {
	svc := newVendoredImportTestService(t)
	ctx := context.Background()
	dir := t.TempDir()

	ontologyID := weaveontology.GenerateOntologyID("vx-missing-test")
	versionID := weaveontology.GenerateVersionID("vx-missing-test", "1.0")

	req := weaveontology.VendoredOntologyImportRequest{
		OntologyID:    ontologyID,
		VersionID:     versionID,
		VersionString: "1.0",
		Slug:          "vx-missing-test",
		Title:         "VX Missing File Test Ontology",
		Kind:          "base",
		Namespace:     "https://example.org/vendored/missing/",
		Prefixes:      []string{"vx"},
		BaseDir:       dir,
		Files:         []string{"does-not-exist.rdf"},
	}
	t.Cleanup(func() {
		_ = svc.DeleteVersion(context.Background(), versionID)
		_ = svc.DeleteOntology(context.Background(), ontologyID)
	})

	err := svc.ImportVendoredVersion(ctx, req)
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
	if got := err.Error(); !strings.Contains(got, "vx-missing-test") || !strings.Contains(got, "does-not-exist.rdf") {
		t.Fatalf("error %q does not name the module/file", got)
	}
}
