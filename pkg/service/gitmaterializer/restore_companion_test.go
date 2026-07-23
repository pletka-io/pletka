//go:build integration

package gitmaterializer_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/app"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

func writeCompanionFixtures(t *testing.T, dir, namespace string) {
	t.Helper()
	primary := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:cmp="` + namespace + `"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"
         xmlns:owl="http://www.w3.org/2002/07/owl#">
  <owl:Ontology rdf:about="` + namespace + `"><owl:versionInfo>1.0</owl:versionInfo></owl:Ontology>
  <rdfs:Class rdf:about="` + namespace + `E1_Main">
    <rdfs:label xml:lang="en">Main</rdfs:label>
  </rdfs:Class>
</rdf:RDF>`
	companion := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:cmp="` + namespace + `"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <rdfs:Class rdf:about="` + namespace + `PC1_Companion_Class">
    <rdfs:label xml:lang="en">Companion Class</rdfs:label>
  </rdfs:Class>
</rdf:RDF>`
	if err := os.WriteFile(filepath.Join(dir, "cmp.rdf"), []byte(primary), 0o644); err != nil {
		t.Fatalf("write primary fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmp_pc.rdf"), []byte(companion), 0o644); err != nil {
		t.Fatalf("write companion fixture: %v", err)
	}
}

// TestRestoreVendoredOntologyCompanions proves companion RDF sources
// round-trip through a self-contained snapshot: the vendor tree carries the
// companion file + manifest primary marker, and a git-only restore
// re-imports the companion's classes (with the same bare source_module the
// live import records) and re-persists the companion row.
func TestRestoreVendoredOntologyCompanions(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}

	pool := testdb.Pool(t)
	ctx := context.Background()
	queries := sqlcgen.New(pool)

	const (
		ownerID      = "CMP_RESTORE_OWNER"
		projectID    = "CMP_RESTORE_PROJECT"
		ontologySlug = "pletka-cmp-restore"
		namespace    = "https://example.org/cmp-restore/"
	)
	ontologyID := weaveontology.GenerateOntologyID(ontologySlug)
	versionID := weaveontology.GenerateVersionID(ontologySlug, "1.0")

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_classes WHERE ontology_version_id = $1`, versionID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE id = $1`, versionID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, ontologyID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID: ownerID, Type: "organization", DisplayName: "Cmp Restore Org",
		Slug: "cmp-restore-org", Visibility: "private",
	}); err != nil {
		t.Fatalf("create owner actor: %v", err)
	}

	ontologyStore := weaveontology.NewPostgresStore(pool)
	ontologySvc := weaveontology.NewService(ontologyStore, nil, nil)

	rdfDir := t.TempDir()
	writeCompanionFixtures(t, rdfDir, namespace)

	if err := ontologySvc.ImportVendoredVersion(ctx, weaveontology.VendoredOntologyImportRequest{
		OntologyID:    ontologyID,
		VersionID:     versionID,
		VersionString: "1.0",
		Slug:          ontologySlug,
		Title:         "Companion Restore Ontology",
		Kind:          "base",
		Namespace:     namespace,
		Prefixes:      []string{"cmp"},
		BaseDir:       rdfDir,
		Files:         []string{"cmp.rdf", "cmp_pc.rdf"},
		Primary:       "cmp.rdf",
		Companions: []weaveontology.VendoredOntologyCompanion{
			{File: "cmp_pc.rdf", Description: "companion module"},
		},
	}); err != nil {
		t.Fatalf("seed ImportVendoredVersion: %v", err)
	}

	uiName, _ := json.Marshal(map[string]string{"en": "Cmp Restore Project"})
	if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID: projectID, UiName: uiName, Description: uiName,
		Status: "draft", OwnerID: ownerID, Visibility: "private",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	isPrimary := true
	usageNotes := ""
	if _, err := queries.WeaveCreateProjectOntologyVersion(ctx, sqlcgen.WeaveCreateProjectOntologyVersionParams{
		ProjectID: projectID, OntologyVersionID: versionID,
		IsPrimary: &isPrimary, UsageNotes: &usageNotes,
	}); err != nil {
		t.Fatalf("link project to ontology version: %v", err)
	}

	sourceDir := t.TempDir()
	mat := gitmaterializer.NewMaterializer(pool, sourceDir, nil)
	if err := mat.InitProjectWithOptions(ctx, projectID, gitmaterializer.InitProjectOptions{SelfContained: true}); err != nil {
		t.Fatalf("InitProjectWithOptions(self-contained): %v", err)
	}
	rootDir := filepath.Join(sourceDir, projectID)

	// The vendor tree must carry both source files.
	vendorSrc := ""
	_ = filepath.Walk(filepath.Join(rootDir, "vendor", "ontologies"), func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && info.Name() == "cmp_pc.rdf" {
			vendorSrc = path
		}
		return nil
	})
	if vendorSrc == "" {
		t.Fatal("companion file cmp_pc.rdf not vendored into the snapshot")
	}

	// Wipe and restore git-only.
	for _, stmt := range []string{
		`DELETE FROM weave_project_ontology_versions WHERE project_id = $1`,
		`DELETE FROM weave_projects WHERE id = $1`,
	} {
		if _, err := pool.Exec(ctx, stmt, projectID); err != nil {
			t.Fatalf("wipe project: %v", err)
		}
	}
	for _, stmt := range []string{
		`DELETE FROM weave_ontology_classes WHERE ontology_version_id = $1`,
		`DELETE FROM weave_ontology_versions WHERE id = $1`,
	} {
		if _, err := pool.Exec(ctx, stmt, versionID); err != nil {
			t.Fatalf("wipe ontology version: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, ontologyID); err != nil {
		t.Fatalf("wipe ontology: %v", err)
	}

	hydrator := gitmaterializer.NewMaterializer(pool, t.TempDir(), nil).
		WithOntologyImporter(app.NewOntologyVendorImporter(ontologySvc))
	if _, err := hydrator.HydrateProjectSnapshot(ctx, rootDir); err != nil {
		t.Fatalf("HydrateProjectSnapshot: %v", err)
	}

	// Companion class restored with the bare source_module the live import
	// records (not the vendored "src/..." path).
	var sourceModule *string
	if err := pool.QueryRow(ctx, `
		SELECT source_module FROM weave_ontology_classes
		WHERE ontology_version_id = $1 AND local_name = 'PC1_Companion_Class'
	`, versionID).Scan(&sourceModule); err != nil {
		t.Fatalf("companion class not restored: %v", err)
	}
	if sourceModule == nil || *sourceModule != "cmp_pc.rdf" {
		t.Fatalf("companion source_module: want %q, got %v", "cmp_pc.rdf", sourceModule)
	}

	// Companion raw content re-persisted, so the NEXT snapshot generation
	// from the restored database is source-complete too.
	var companionBytes int
	if err := pool.QueryRow(ctx, `
		SELECT length(content) FROM weave_ontology_version_companions
		WHERE ontology_version_id = $1 AND filename = 'cmp_pc.rdf'
	`, versionID).Scan(&companionBytes); err != nil {
		t.Fatalf("companion content not re-persisted on restore: %v", err)
	}
	if companionBytes == 0 {
		t.Fatal("companion content re-persisted empty")
	}
}
