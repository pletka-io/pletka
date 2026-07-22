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
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// writeExternalNSOntologyFixture writes a tiny RDFS/OWL fixture whose class
// declares an rdfs:subClassOf pointing at an *external* namespace by full URI
// (the AAAo -> crm shape reproduced tiny). The fixture declares no xmlns
// prefix for that external namespace, so the RDF parser can only resolve the
// full URI to a qname if the caller supplies a prefix binding for it.
func writeExternalNSOntologyFixture(t *testing.T, path, namespace, externalNS string) {
	t.Helper()
	content := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:ext="` + namespace + `"
         xmlns:owl="http://www.w3.org/2002/07/owl#"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <owl:Ontology rdf:about="` + namespace + `">
    <owl:versionInfo>1.0</owl:versionInfo>
  </owl:Ontology>
  <rdfs:Class rdf:about="` + namespace + `ZE19_Naming">
    <rdfs:label xml:lang="en">Naming</rdfs:label>
    <rdfs:subClassOf rdf:resource="` + externalNS + `E13_Attribute_Assignment"/>
  </rdfs:Class>
</rdf:RDF>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestRestoreVendoredOntologyExternalNamespace exercises the vendored-ontology
// restore path for the real bug the AME/LA/ING fixtures caught: a vendored
// ontology references an external namespace (crm) by full URI in an
// rdfs:subClassOf but declares no prefix for it. The prefix binding lives only
// in the project snapshot's effective namespace bindings, so a git-only
// restore must thread those bindings into the vendored ontology import — the
// ontology's own prefixes are not enough. Before the fix the hydrate fails
// with "rdf namespace resolution failed: missing prefix binding for <crm ns>".
func TestRestoreVendoredOntologyExternalNamespace(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}

	pool := testdb.Pool(t)
	ctx := context.Background()
	queries := sqlcgen.New(pool)

	const (
		ownerID      = "EXTNS_RESTORE_OWNER"
		projectID    = "EXTNS_RESTORE_PROJECT"
		ontologySlug = "pletka-extns-restore"
		namespace = "https://example.org/extns-restore/"
		// A fully synthetic external namespace + prefix, deliberately NOT one the
		// test fixtures already bind (e.g. crm), so this test is independent of
		// the fixture template's namespace bindings and exercises only the
		// threading of a project-scoped external binding into vendored restore.
		externalNS  = "https://example.org/extns-external/"
		externalPfx = "extns"
	)
	ontologyID := weaveontology.GenerateOntologyID(ontologySlug)
	versionID := weaveontology.GenerateVersionID(ontologySlug, "1.0")

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_relations r USING weave_ontology_classes c WHERE r.source_id = c.id AND c.ontology_version_id = $1`, versionID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_classes WHERE ontology_version_id = $1`, versionID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE id = $1`, versionID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, ontologyID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	}
	cleanup()
	t.Cleanup(cleanup)

	// 1. Owner actor.
	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          ownerID,
		Type:        "organization",
		DisplayName: "ExtNS Restore Org",
		Slug:        "extns-restore-org",
		Role:        "",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create owner actor: %v", err)
	}

	ontologyStore := weaveontology.NewPostgresStore(pool)
	ontologySvc := weaveontology.NewService(ontologyStore, nil, nil)

	rdfDir := t.TempDir()
	writeExternalNSOntologyFixture(t, filepath.Join(rdfDir, "extns.rdf"), namespace, externalNS)

	// Seed the DB ontology by supplying the external binding directly (the live
	// import path already has these via the project's namespace table).
	if err := ontologySvc.ImportVendoredVersion(ctx, weaveontology.VendoredOntologyImportRequest{
		OntologyID:    ontologyID,
		VersionID:     versionID,
		VersionString: "1.0",
		Slug:          ontologySlug,
		Title:         "ExtNS Restore Ontology",
		Kind:          "base",
		Namespace:     namespace,
		Prefixes:      []string{"ext"},
		BaseDir:       rdfDir,
		Files:         []string{"extns.rdf"},
		ExternalBindings: []weaveontology.NamespaceBinding{
			{Prefix: externalPfx, Namespace: externalNS},
		},
	}); err != nil {
		t.Fatalf("seed ImportVendoredVersion: %v", err)
	}

	// 2. Project + external namespace binding + ontology link. The crm binding
	// is a project-scoped user binding so it lands in the snapshot's
	// effective_bindings when the project is materialized.
	uiName, _ := json.Marshal(map[string]string{"en": "ExtNS Restore Project"})
	description, _ := json.Marshal(map[string]string{"en": "Project for TestRestoreVendoredOntologyExternalNamespace"})
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

	pid := projectID
	if _, err := queries.WeaveCreateUserNamespaceBinding(ctx, sqlcgen.WeaveCreateUserNamespaceBindingParams{
		ID:        ids.GenerateULID(),
		ProjectID: &pid,
		Prefix:    externalPfx,
		Namespace: externalNS,
		Weight:    0,
	}); err != nil {
		t.Fatalf("create external namespace binding: %v", err)
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

	// 3. Materialize a self-contained snapshot (vendors the ontology and writes
	// effective_bindings, including crm, into project.yaml).
	sourceDir := t.TempDir()
	mat := gitmaterializer.NewMaterializer(pool, sourceDir, nil)
	if err := mat.InitProjectWithOptions(ctx, projectID, gitmaterializer.InitProjectOptions{SelfContained: true}); err != nil {
		t.Fatalf("InitProjectWithOptions(self-contained): %v", err)
	}
	rootDir := filepath.Join(sourceDir, projectID)

	// 4. Simulate an empty target DB.
	if _, err := pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE project_id = $1`, projectID); err != nil {
		t.Fatalf("delete project namespace bindings: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID); err != nil {
		t.Fatalf("delete project ontology link: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM weave_ontology_relations r USING weave_ontology_classes c WHERE r.source_id = c.id AND c.ontology_version_id = $1`, versionID); err != nil {
		t.Fatalf("delete ontology relations: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM weave_ontology_classes WHERE ontology_version_id = $1`, versionID); err != nil {
		t.Fatalf("delete ontology classes: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE id = $1`, versionID); err != nil {
		t.Fatalf("delete ontology version: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, ontologyID); err != nil {
		t.Fatalf("delete ontology: %v", err)
	}

	// 5. Hydrate from the on-disk snapshot alone. This is the step that fails
	// before the fix: the vendored ontology import only receives the
	// ontology's own prefix, so resolving the external crm URI errors.
	hydrator := gitmaterializer.NewMaterializer(pool, t.TempDir(), nil).
		WithOntologyImporter(app.NewOntologyVendorImporter(ontologySvc))

	if _, err := hydrator.HydrateProjectSnapshot(ctx, rootDir); err != nil {
		t.Fatalf("HydrateProjectSnapshot: %v", err)
	}

	// 6. Assert the class restored and the cross-namespace subclass_of edge is
	// present (target resolved to a crm: qname via the threaded binding).
	classes, err := queries.WeaveListOntologyClassesByVersion(ctx, versionID)
	if err != nil {
		t.Fatalf("list restored classes: %v", err)
	}
	var namingID string
	for _, c := range classes {
		if c.LocalName == "ZE19_Naming" {
			namingID = c.ID
		}
	}
	if namingID == "" {
		t.Fatalf("expected class ZE19_Naming to be restored, got %#v", classes)
	}

	var targetQname string
	if err := pool.QueryRow(ctx, `
		SELECT target_qname FROM weave_ontology_relations
		WHERE source_id = $1 AND source_kind = 'class' AND rel_type = 'subclass_of'
	`, namingID).Scan(&targetQname); err != nil {
		t.Fatalf("expected cross-namespace subclass_of edge for %s: %v", namingID, err)
	}
	if targetQname != externalPfx+":E13_Attribute_Assignment" {
		t.Fatalf("expected subclass_of target %q, got %q", externalPfx+":E13_Attribute_Assignment", targetQname)
	}
}
