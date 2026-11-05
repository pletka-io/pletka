//go:build integration

package ontology_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// writeReplaceTestRDF is writeVendoredTestRDF with a configurable range on
// has_thing, so a re-import can move a relation target.
func writeReplaceTestRDF(t *testing.T, path, namespace, rangeLocal string) {
	t.Helper()
	content := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:vx="` + namespace + `"
         xmlns:owl="http://www.w3.org/2002/07/owl#"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <owl:Ontology rdf:about="` + namespace + `"><owl:versionInfo>1.0</owl:versionInfo></owl:Ontology>
  <rdfs:Class rdf:about="` + namespace + `Thing"><rdfs:label xml:lang="en">Thing</rdfs:label></rdfs:Class>
  <rdfs:Class rdf:about="` + namespace + `Actor"><rdfs:label xml:lang="en">Actor</rdfs:label></rdfs:Class>
  <rdf:Property rdf:about="` + namespace + `has_thing">
    <rdfs:label xml:lang="en">has thing</rdfs:label>
    <rdfs:domain rdf:resource="` + namespace + `Actor"/>
    <rdfs:range rdf:resource="` + namespace + rangeLocal + `"/>
  </rdf:Property>
</rdf:RDF>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// Re-importing an existing version must replace its relations rather than
// pile new rows next to the old ones (the relations table has no FK, so the
// version cascade never reached them), and must keep the version active when
// the caller did not ask to change that.
func TestImportVersion_ReplaceDropsStaleRelationsAndKeepsActive(t *testing.T) {
	pool := testdb.Pool(t)
	store := weaveontology.NewPostgresStore(pool)
	svc := weaveontology.NewService(store, nil, nil)
	ctx := context.Background()
	dir := t.TempDir()

	namespace := "https://example.org/vendored/replace/"
	ontologyID := weaveontology.GenerateOntologyID("vx-replace-test")
	versionID := weaveontology.GenerateVersionID("vx-replace-test", "1.0")
	t.Cleanup(func() {
		_ = svc.DeleteVersion(context.Background(), versionID)
		_ = svc.DeleteOntology(context.Background(), ontologyID)
	})

	writeReplaceTestRDF(t, filepath.Join(dir, "vx.rdf"), namespace, "Thing")
	if err := svc.ImportVendoredVersion(ctx, weaveontology.VendoredOntologyImportRequest{
		OntologyID: ontologyID, VersionID: versionID, VersionString: "1.0",
		Slug: "vx-replace-test", Title: "VX Replace Test", Kind: "base",
		Namespace: namespace, Prefixes: []string{"vx"}, BaseDir: dir, Files: []string{"vx.rdf"},
	}); err != nil {
		t.Fatalf("ImportVendoredVersion: %v", err)
	}
	if v, err := svc.GetVersion(ctx, versionID); err != nil || !v.IsActive {
		t.Fatalf("precondition: version active=%v err=%v", v != nil && v.IsActive, err)
	}

	// Same version id, has_thing now ranges over Actor, no activity request.
	writeReplaceTestRDF(t, filepath.Join(dir, "vx2.rdf"), namespace, "Actor")
	ont, err := svc.GetOntology(ctx, ontologyID)
	if err != nil {
		t.Fatalf("GetOntology: %v", err)
	}
	built, err := weaveontology.BuildImportVersionInputFromFile(weaveontology.ImportVersionFileOptions{
		BaseDir: dir, File: "vx2.rdf", Ontology: ont, VersionID: versionID, VersionString: "1.0",
	})
	if err != nil {
		t.Fatalf("BuildImportVersionInputFromFile: %v", err)
	}
	if _, err := svc.ImportVersion(ctx, built.Input); err != nil {
		t.Fatalf("ImportVersion (replace): %v", err)
	}

	v, err := svc.GetVersion(ctx, versionID)
	if err != nil {
		t.Fatalf("GetVersion after replace: %v", err)
	}
	if !v.IsActive {
		t.Fatal("replaced version lost its active state")
	}

	props, err := svc.ListProperties(ctx, versionID)
	if err != nil || len(props) != 1 {
		t.Fatalf("ListProperties: n=%d err=%v", len(props), err)
	}
	ranges, err := store.ListRelationsForSourceByType(ctx, props[0].ID, "property", "range")
	if err != nil {
		t.Fatalf("ListRelationsForSourceByType: %v", err)
	}
	if len(ranges) != 1 || ranges[0].TargetQname != "vx:Actor" {
		got := make([]string, 0, len(ranges))
		for _, r := range ranges {
			got = append(got, r.TargetQname)
		}
		t.Fatalf("ranges after replace = %v, want [vx:Actor]", got)
	}

	// Deleting the version takes its relations with it.
	if err := svc.DeleteVersion(ctx, versionID); err != nil {
		t.Fatalf("DeleteVersion: %v", err)
	}
	left, err := store.ListRelationsForSource(ctx, props[0].ID, "property")
	if err != nil {
		t.Fatalf("ListRelationsForSource after delete: %v", err)
	}
	if len(left) != 0 {
		t.Fatalf("%d relation rows survived the version delete", len(left))
	}
}
