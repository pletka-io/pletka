//go:build integration

package gitmaterializer_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer/ontologyvendor"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestReleasedOntologyTreeUsesCapturedArtifacts(t *testing.T) {
	pool := testPool(t)
	ctx := seedZMAT(t, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM weave_releases WHERE project_id=$1`, zmatProjectID); err != nil {
			t.Error(err)
		}
		// Legacy link tables have no project FK/cascade. Remove this test's
		// links before seedZMAT's project cleanup so later fixtures start clean.
		for _, table := range []string{"weave_project_ontology_versions", "weave_project_ontology_versions_archive"} {
			if _, err := pool.Exec(context.Background(), "DELETE FROM "+table+" WHERE project_id=$1", zmatProjectID); err != nil {
				t.Error(err)
			}
		}
	})
	store := ontology.NewPostgresStore(pool)
	defaults, err := store.GetOntologyByPrefix(ctx, "defaults")
	if err != nil || defaults == nil {
		t.Fatalf("defaults fixture: %v", err)
	}
	defaultsVersion := ontology.GenerateVersionID("artifact-tree-defaults", "1")
	if _, err := store.ImportVersion(ctx, ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: defaultsVersion, OntologyID: defaults.ID, VersionString: "artifact-tree-1", RDFContent: "<defaults/>"}}); err != nil {
		t.Fatal(err)
	}
	previousDefaults, err := store.GetActiveVersion(ctx, defaults.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if previousDefaults != nil {
			if err := store.SetActiveVersion(context.Background(), defaults.ID, previousDefaults.ID); err != nil {
				t.Error(err)
			}
		}
	})
	const prefix = "artifact_tree"
	owner, version := ontology.GenerateOntologyID(prefix), ontology.GenerateVersionID(prefix, "1")
	if _, err := store.CreateOntology(ctx, ontology.CreateOntologyInput{ID: owner, Prefix: prefix, Name: "Frozen ontology", Namespace: "https://example.org/frozen/", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	in := ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: version, OntologyID: owner, VersionString: "1", RDFContent: "<original/>", OriginalFilename: "main.rdf"}, SetActive: true,
		Publication:       ontologyrelease.Options{Pins: map[string]string{defaults.ID: defaultsVersion}},
		Classes:           []ontology.ImportClass{{ID: ontology.GenerateClassID(version, "https://example.org/frozen/A"), Prefix: prefix, LocalName: "A", URI: "https://example.org/frozen/A"}},
		CompanionFiles:    []ontology.CompanionFileInput{{Filename: "companion.rdf", Description: "Frozen companion", Content: "<companion/>"}},
		NamespaceBindings: []ontology.NamespaceBinding{{Prefix: prefix, Namespace: "https://example.org/frozen/"}, {Prefix: prefix, Namespace: "https://alias.example/", Weight: 5}},
	}
	if _, err := store.ImportVersion(ctx, in); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{version, defaultsVersion} {
		if _, err := pool.Exec(ctx, `INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,added_at) VALUES ($1,$2,now())`, zmatProjectID, id); err != nil {
			t.Fatal(err)
		}
	}
	svc := newReleaseService(pool)
	if _, err := svc.Create(zmatAuthCtx(ctx), zmatProjectID, release.CreateInput{Version: "7.9.0"}); err != nil {
		t.Fatal(err)
	}
	mat := gitmaterializer.NewMaterializer(pool, t.TempDir(), testLogger()).WithOntologyImporter(ontologyvendor.New(ontology.NewService(store, nil, nil)))
	first := t.TempDir()
	if err := mat.BuildLegacyReleaseTree(ctx, first, zmatProjectID, "7.9.0"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := gitmaterializer.LoadProjectSnapshot(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Vendor.Ontologies) != 2 {
		t.Fatalf("incomplete artifact vendor closure: %d", len(snapshot.Vendor.Ontologies))
	}
	for _, dep := range snapshot.Vendor.Ontologies {
		if dep.Snapshot.Artifact == nil || dep.Snapshot.Manifest.SchemaVersion != 2 {
			t.Fatal("released output used legacy ontology format")
		}
	}
	if err := mat.HydrateVendored(ctx, &gitmaterializer.RestorePlan{ProjectID: zmatProjectID, Snapshot: snapshot}); err != nil {
		t.Fatalf("existing verified IDs should be accepted: %v", err)
	}
	target := testdb.EmptyPool(t)
	targetMat := gitmaterializer.NewMaterializer(target, t.TempDir(), testLogger()).WithOntologyImporter(ontologyvendor.New(ontology.NewService(ontology.NewPostgresStore(target), nil, nil)))
	if err := targetMat.HydrateVendored(ctx, &gitmaterializer.RestorePlan{ProjectID: zmatProjectID, Snapshot: snapshot}); err != nil {
		t.Fatalf("fresh vendor restore: %v", err)
	}
	for _, dep := range snapshot.Vendor.Ontologies {
		doc, err := dep.Snapshot.Artifact.Decode()
		if err != nil {
			t.Fatal(err)
		}
		var json, digest string
		if err := target.QueryRow(ctx, `SELECT artifact,digest FROM weave_ontology_releases WHERE version_id=$1`, doc.VersionID).Scan(&json, &digest); err != nil {
			t.Fatal(err)
		}
		if json != dep.Snapshot.Artifact.JSON || digest != dep.Snapshot.Artifact.Digest {
			t.Fatal("vendor restore changed artifact identity")
		}
	}
	var escaped ontologyrelease.ProjectReader
	// The consumer manifest must identify exactly the artifact being restored.
	req := snapshot.Mod.Require.Ontologies[0]
	snapshot.Mod.Require.Ontologies[0].Digest = strings.Repeat("0", 64)
	if err := mat.HydrateVendored(ctx, &gitmaterializer.RestorePlan{Snapshot: snapshot}); err == nil || !strings.Contains(err.Error(), "differs from") {
		t.Fatalf("consumer digest mismatch accepted: %v", err)
	}
	snapshot.Mod.Require.Ontologies[0].Digest = ""
	if err := mat.HydrateVendored(ctx, &gitmaterializer.RestorePlan{Snapshot: snapshot}); err == nil || !strings.Contains(err.Error(), "no artifact digest") {
		t.Fatalf("published dependency lost its digest: %v", err)
	}
	snapshot.Mod.Require.Ontologies[0] = req
	savedVendors := snapshot.Vendor.Ontologies
	snapshot.Vendor.Ontologies = nil
	if err := mat.HydrateVendored(ctx, &gitmaterializer.RestorePlan{Snapshot: snapshot}); err == nil || !strings.Contains(err.Error(), "no artifact vendor") {
		t.Fatalf("missing published vendor accepted: %v", err)
	}
	snapshot.Vendor.Ontologies = savedVendors
	if err := ontologyrelease.NewFactory(pool).ReadProject(ctx, zmatProjectID, "7.9.0", func(r ontologyrelease.ProjectReader) error {
		escaped = r
		values, err := r.Artifacts(ctx)
		if err == nil && len(values) != 2 {
			t.Fatalf("captured %d artifacts", len(values))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := escaped.Artifacts(ctx); err == nil {
		t.Fatal("project artifact reader escaped its lifetime")
	}
	for _, selection := range [][2]string{{zmatProjectID, ""}, {"wrong-owner", "7.9.0"}, {zmatProjectID, "missing"}} {
		entered := false
		err := ontologyrelease.NewFactory(pool).ReadProject(ctx, selection[0], selection[1], func(ontologyrelease.ProjectReader) error { entered = true; return nil })
		if err == nil || entered {
			t.Fatal("invalid project selection reached reader")
		}
	}
	for _, sql := range []string{`UPDATE weave_release_ontology_dependencies SET digest=repeat('0',64) WHERE project_id=$1`, `DELETE FROM weave_release_ontology_dependencies WHERE project_id=$1`, `UPDATE weave_releases SET ontology_dependencies_version=0 WHERE project_id=$1`} {
		if _, err := pool.Exec(ctx, sql, zmatProjectID); err == nil {
			t.Fatalf("captured inventory mutated: %s", sql)
		}
	}
	for _, sql := range []string{`INSERT INTO weave_release_ontology_dependencies SELECT * FROM weave_release_ontology_dependencies LIMIT 1`, `TRUNCATE weave_release_ontology_dependencies`} {
		if _, err := pool.Exec(ctx, sql); err == nil || !strings.Contains(err.Error(), "release content") {
			t.Fatalf("captured inventory accepted late write: %v", err)
		}
	}
	// An explicit field-path selection survives without a project ontology link.
	if _, err := pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id=$1 AND ontology_version_id=$2`, zmatProjectID, version); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE weave_fields SET path_elements=jsonb_build_array(jsonb_build_object('position',0,'ontology_version_id',$1::text,'prefix',$2::text,'type','Class','local_name','A')) WHERE id='ZMATF1'`, version, prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(zmatAuthCtx(ctx), zmatProjectID, release.CreateInput{Version: "7.9.2"}); err != nil {
		t.Fatal(err)
	}
	// A consumer with no own ontology selections inherits that same closure.
	const child = "ARTIFACT_CHILD"
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects(id,ui_name,owner_id,status,visibility,created_at,updated_at) VALUES ($1,'{"en":"artifact child"}','ZMAT_OWNER','draft','private',now(),now())`, child); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM weave_projects WHERE id=$1`, child); err != nil {
			t.Error(err)
		}
	})
	if _, err := pool.Exec(ctx, `INSERT INTO weave_project_inheritance(project_id,parent_project_id,is_primary,source_mode,source_version) VALUES ($1,$2,true,'release','7.9.2')`, child, zmatProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_models(id,project_id,ui_name,status,created_at,updated_at) VALUES ('ARTIFACT_CHILD_MODEL',$1,'{"en":"child model"}','draft',now(),now())`, child); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(zmatAuthCtx(ctx), child, release.CreateInput{Version: "7.9.3"}); err != nil {
		t.Fatalf("child cut: %#v", err)
	}
	for _, selection := range [][2]string{{zmatProjectID, "7.9.2"}, {child, "7.9.3"}} {
		if err := ontologyrelease.NewFactory(pool).ReadProject(ctx, selection[0], selection[1], func(r ontologyrelease.ProjectReader) error {
			values, err := r.Artifacts(ctx)
			if err != nil {
				return err
			}
			if len(values) != 2 {
				t.Fatalf("explicit/inherited selection captured %d artifacts", len(values))
			}
			for _, value := range values {
				doc, err := value.Artifact.Decode()
				if err != nil || doc.VersionID == version && value.Linked {
					t.Fatalf("implicit pin incorrectly recorded as direct link: %v", err)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	// The release index intentionally lists all cuts. Refresh it after the
	// additional capture tests, before varying ontology serialization inputs.
	if err := mat.BuildLegacyReleaseTree(ctx, first, zmatProjectID, "7.9.0"); err != nil {
		t.Fatal(err)
	}
	// Exercise the actual editor lifecycle after the project release is captured.
	drafts := ontology.NewDraftService(pool, nil)
	draft, err := drafts.StartFromRelease(ctx, ontologybody.OntologyID(owner), ontologybody.VersionID(version))
	if err != nil {
		t.Fatal(err)
	}
	draftRef, err := ontology.NewDraftReference(draft.OntologyID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	draft.Document.Ontology.Name = "Editor descriptor"
	draft.Document.Primary.Content = `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:owl="http://www.w3.org/2002/07/owl#"><owl:Class rdf:about="https://example.org/frozen/A"/></rdf:RDF>`
	draft.Document.Companions = nil
	if err := drafts.Edit(ctx, draftRef, draft.Revision, func(e ontology.DraftEditor) error {
		var err error
		draft, err = e.Save(ctx, draft.Document)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	afterEdit := t.TempDir()
	if err := mat.BuildLegacyReleaseTree(ctx, afterEdit, zmatProjectID, "7.9.0"); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(collectTree(t, first), collectTree(t, afterEdit)); diff != "" {
		t.Fatalf("draft edit changed released tree: %s", diff)
	}
	if _, err := drafts.Publish(ctx, draftRef, draft.Revision, ontology.DraftPublishRequest{Version: "editor-3", SetActive: true}); err != nil {
		t.Fatal(err)
	}
	// Change every formerly live ontology serialization input, including active defaults.
	if _, err := pool.Exec(ctx, `UPDATE weave_ontologies SET name='Changed live descriptor',namespace='https://changed/' WHERE id=$1`, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE weave_namespace_bindings SET namespace='https://changed-alias/' || id || '/' WHERE prefix=$1`, prefix); err != nil {
		t.Fatal(err)
	}
	in.Version.ID = ontology.GenerateVersionID(prefix, "2")
	in.Version.VersionString = "2"
	in.Classes = nil
	in.NamespaceBindings = nil
	if _, err := store.ImportVersion(ctx, in); err != nil {
		t.Fatal(err)
	}
	newDefaults := ontology.GenerateVersionID("artifact-tree-defaults", "2")
	if _, err := store.ImportVersion(ctx, ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: newDefaults, OntologyID: defaults.ID, VersionString: "artifact-tree-2", RDFContent: "<changed-defaults/>"}, SetActive: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id=$1`, zmatProjectID); err != nil {
		t.Fatal(err)
	}
	second := t.TempDir()
	if err := mat.BuildLegacyReleaseTree(ctx, second, zmatProjectID, "7.9.0"); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(collectTree(t, first), collectTree(t, second)); diff != "" {
		t.Fatalf("ontology edits changed released tree (-before +after):\n%s", diff)
	}
	if err := store.DeleteVersion(ctx, version); err == nil {
		t.Fatal("captured ontology was deletable")
	}
	// The loader rejects sources that disagree with their exact artifact before
	// any existing-ID shortcut or database write can run.
	dep := snapshot.Vendor.Ontologies[0]
	sourcePath := filepath.Join(dep.Snapshot.RootDir, filepath.FromSlash(dep.Snapshot.Manifest.Sources.Primary))
	if err := os.WriteFile(sourcePath, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := gitmaterializer.LoadProjectSnapshot(first); err == nil || !strings.Contains(err.Error(), "differs from artifact") {
		t.Fatalf("tampered source accepted: %v", err)
	}
	// An unverified linked version must abort metadata, archive and outbox.
	in.Version.ID = ontology.GenerateVersionID(prefix, "legacy")
	in.Version.VersionString = "legacy"
	in.SetActive = false
	if _, err := store.StageLegacyVersion(ctx, in); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,added_at) VALUES ($1,$2,now())`, zmatProjectID, in.Version.ID); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(zmatAuthCtx(ctx), zmatProjectID, release.CreateInput{Version: "7.9.1"})
	var invalid *release.ErrValidation
	if !errors.As(err, &invalid) || !strings.Contains(strings.Join(invalid.Fields["ontologies"], " "), "not published") {
		t.Fatalf("unverified ontology entered project cut: %v", err)
	}
	var leaked int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM weave_releases WHERE project_id=$1 AND version='7.9.1')+(SELECT count(*) FROM weave_projects_archive WHERE id=$1 AND version_number='7.9.1')+(SELECT count(*) FROM weave_change_set WHERE project_id=$1 AND release_version='7.9.1')`, zmatProjectID).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("failed cut retained %d rows: %v", leaked, err)
	}
}
