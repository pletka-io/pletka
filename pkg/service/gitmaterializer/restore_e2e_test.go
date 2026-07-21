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

// writeE2EOntologyFixture writes a tiny valid RDFS/OWL fixture — mirroring
// the fixture used by pkg/weave/ontology/import_vendored_test.go — so the
// vendored ontology snapshot has real classes/properties to restore.
func writeE2EOntologyFixture(t *testing.T, path, namespace string) {
	t.Helper()
	content := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:e2e="` + namespace + `"
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

// TestRestoreIntoEmptyDB exercises the full vendored-ontology restore path
// end to end: a project with a linked ontology version is materialized to a
// self-contained git snapshot on disk, the corresponding database rows are
// then deleted (simulating an empty target database), and
// HydrateProjectSnapshot — wired with the real pkg/app adapter — must
// restore both the project and the vendored ontology version from disk
// alone. A second hydrate run must succeed unchanged (idempotency).
func TestRestoreIntoEmptyDB(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}

	pool := testdb.Pool(t)
	ctx := context.Background()
	queries := sqlcgen.New(pool)

	const (
		ownerID      = "E2E_RESTORE_OWNER"
		projectID    = "E2E_RESTORE_PROJECT"
		ontologySlug = "pletka-e2e-restore"
		namespace    = "https://example.org/e2e-restore/"
	)
	ontologyID := weaveontology.GenerateOntologyID(ontologySlug)
	versionID := weaveontology.GenerateVersionID(ontologySlug, "1.0")

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE id = $1`, versionID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, ontologyID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	}
	cleanup()
	t.Cleanup(cleanup)

	// 1. Create the owner actor and a small project with a linked ontology
	// version, through the service/store layer.
	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          ownerID,
		Type:        "organization",
		DisplayName: "E2E Restore Org",
		Slug:        "e2e-restore-org",
		Role:        "",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create owner actor: %v", err)
	}

	ontologyStore := weaveontology.NewPostgresStore(pool)
	ontologySvc := weaveontology.NewService(ontologyStore, nil, nil)

	rdfDir := t.TempDir()
	writeE2EOntologyFixture(t, filepath.Join(rdfDir, "e2e.rdf"), namespace)

	if err := ontologySvc.ImportVendoredVersion(ctx, weaveontology.VendoredOntologyImportRequest{
		OntologyID:    ontologyID,
		VersionID:     versionID,
		VersionString: "1.0",
		Slug:          ontologySlug,
		Title:         "E2E Restore Ontology",
		Kind:          "base",
		Namespace:     namespace,
		Prefixes:      []string{"e2e"},
		BaseDir:       rdfDir,
		Files:         []string{"e2e.rdf"},
	}); err != nil {
		t.Fatalf("seed ImportVendoredVersion: %v", err)
	}

	uiName, _ := json.Marshal(map[string]string{"en": "E2E Restore Project"})
	description, _ := json.Marshal(map[string]string{"en": "Project used by TestRestoreIntoEmptyDB"})
	if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID:          projectID,
		UiName:      uiName,
		Description: description,
		Status:      "draft",
		OwnerID:     ownerID,
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	isPrimary := true
	usageNotes := ""
	if _, err := queries.WeaveCreateProjectOntologyVersion(ctx, sqlcgen.WeaveCreateProjectOntologyVersionParams{
		ProjectID:         projectID,
		OntologyVersionID: versionID,
		IsPrimary:         &isPrimary,
		UsageNotes:        &usageNotes,
	}); err != nil {
		t.Fatalf("link project to ontology version: %v", err)
	}

	// 2. Materialize the project to a temp dir as a self-contained snapshot,
	// which vendors the linked ontology version alongside the project tree.
	sourceDir := t.TempDir()
	mat := gitmaterializer.NewMaterializer(pool, sourceDir, nil)
	if err := mat.InitProjectWithOptions(ctx, projectID, gitmaterializer.InitProjectOptions{SelfContained: true}); err != nil {
		t.Fatalf("InitProjectWithOptions(self-contained): %v", err)
	}
	rootDir := filepath.Join(sourceDir, projectID)

	// 3. Simulate an empty target database by deleting the ontology and
	// project rows the snapshot depends on.
	if _, err := pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID); err != nil {
		t.Fatalf("delete project ontology link: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE id = $1`, versionID); err != nil {
		t.Fatalf("delete ontology version: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, ontologyID); err != nil {
		t.Fatalf("delete ontology: %v", err)
	}

	// 4. Hydrate from the on-disk snapshot with a Materializer wired with
	// the real pkg/app adapter — the same wiring production uses.
	hydrator := gitmaterializer.NewMaterializer(pool, t.TempDir(), nil).
		WithOntologyImporter(app.NewOntologyVendorImporter(ontologySvc))

	if _, err := hydrator.HydrateProjectSnapshot(ctx, rootDir); err != nil {
		t.Fatalf("HydrateProjectSnapshot: %v", err)
	}

	// 5. Assert the ontology version and project were restored, and that
	// the project->ontology-version link is back in place.
	if _, err := queries.WeaveGetOntologyVersionByID(ctx, versionID); err != nil {
		t.Fatalf("expected ontology version %s to be restored: %v", versionID, err)
	}
	if _, err := queries.WeaveGetProjectByID(ctx, projectID); err != nil {
		t.Fatalf("expected project %s to be restored: %v", projectID, err)
	}
	links, err := queries.WeaveListProjectOntologyVersions(ctx, projectID)
	if err != nil {
		t.Fatalf("list project ontology versions: %v", err)
	}
	if len(links) != 1 || links[0].OntologyVersionID != versionID {
		t.Fatalf("expected project->ontology-version link to %s, got %#v", versionID, links)
	}

	// 6. A second hydrate run against the same snapshot must succeed
	// unchanged, proving the restore path (including the ontology import)
	// is idempotent.
	if _, err := hydrator.HydrateProjectSnapshot(ctx, rootDir); err != nil {
		t.Fatalf("HydrateProjectSnapshot (second run): %v", err)
	}
	linksAfter, err := queries.WeaveListProjectOntologyVersions(ctx, projectID)
	if err != nil {
		t.Fatalf("list project ontology versions after second run: %v", err)
	}
	if len(linksAfter) != 1 || linksAfter[0].OntologyVersionID != versionID {
		t.Fatalf("expected project->ontology-version link to remain %s after second run, got %#v", versionID, linksAfter)
	}
}
