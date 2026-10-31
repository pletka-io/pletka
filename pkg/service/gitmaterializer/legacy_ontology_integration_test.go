//go:build integration

package gitmaterializer_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestLegacyOntologyTransportAndRepointRecovery(t *testing.T) {
	skipIfNoGit(t)
	pool := testdb.EmptyPool(t)
	ctx := zmatAuthCtx(seedZMAT(t, pool))
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	store := ontology.NewPostgresStore(pool)
	if _, err := store.CreateOntology(ctx, ontology.CreateOntologyInput{ID: "transport-crm", Prefix: "crm", Name: "CRM", Namespace: "https://example.org/crm/", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"6.2", "7.1.3"} {
		id := "transport-crm-" + version
		if _, err := store.StageLegacyVersion(ctx, ontology.ImportVersionInput{
			Version:           ontology.CreateVersionInput{ID: id, OntologyID: "transport-crm", VersionString: version, RDFContent: "<original version='" + version + "'/>", OriginalFilename: "crm.rdf"},
			Classes:           []ontology.ImportClass{{ID: ontology.GenerateClassID(id, "https://example.org/crm/E1"), Prefix: "crm", LocalName: "E1", URI: "https://example.org/crm/E1", Label: domain.Translations{"en": version}}},
			CompanionFiles:    []ontology.CompanionFileInput{{Filename: "notes.rdf", Content: "<companion/>", Description: "Preserved"}},
			NamespaceBindings: []ontology.NamespaceBinding{{Prefix: "crm", Namespace: "https://example.org/crm/"}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,added_at,is_primary) VALUES ('ZMAT','transport-crm-6.2',now(),true),('ZMAT','transport-crm-7.1.3',now(),false)`)
	// The captured registry can contain tied aliases; it is not an effective map.
	exec(`INSERT INTO weave_namespace_bindings(id,prefix,namespace,weight,source,created_at,updated_at) VALUES
		('transport-alias-old','crmarchaeo','https://old.example/crmarchaeo/',40,'version-derived',now(),now()),
		('transport-alias-new','crmarchaeo','https://new.example/crmarchaeo/',40,'version-derived',now(),now())`)
	exec(`UPDATE weave_fields SET path_elements='[{"position":0,"type":"class","prefix":"crm","local_name":"E1","uri":"https://example.org/crm/E1"},{"position":1,"type":"class","prefix":"crm","local_name":"E1","ontology_version_id":"transport-crm-6.2"},{"position":2,"type":"literal","prefix":"rdfs","local_name":"Literal","uri":"rdfs:Literal","datatype":"rdf:literal"}]' WHERE id='ZMATF1'`)
	exec(`INSERT INTO weave_projects(id,owner_id) VALUES ('transport-source','ZMAT_OWNER')`)
	exec(`INSERT INTO weave_fields(id,semantic_id,project_id,ontology_scope) VALUES ('transport-source-field','TRANSPORTF.1','transport-source','{"prefix":"crm","local_name":"E1"}')`)
	exec(`INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,added_at,is_primary) VALUES ('transport-source','transport-crm-7.1.3',now(),true)`)
	exec(`INSERT INTO weave_project_inheritance(project_id,parent_project_id,is_primary,source_mode,source_version) VALUES ('ZMAT','transport-source',true,'release','7.0.1')`)
	exec(`INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id) VALUES ('transport-source-field','ZMAT','model','ZMATM.1')`)
	exec(`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('ZMAT','7.0.1','ZMAT_OWNER'),('transport-source','7.0.1','ZMAT_OWNER')`)
	svc := newReleaseService(pool)
	plan, err := svc.PlanLegacyRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	m := gitmaterializer.NewMaterializer(pool, root, testLogger())
	refs := make([]gitmaterializer.ReleaseRefState, 0, len(plan.Projects))
	for _, p := range plan.Projects {
		r, err := m.InspectReleaseRefs(ctx, p.ProjectID, p.Version)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, r)
	}
	check := func(ctx context.Context, id string, resume bool) error {
		for _, r := range refs {
			if err := m.CheckReleaseRepoint(ctx, id, r, resume); err != nil {
				return err
			}
		}
		return nil
	}
	fail := true
	var prepared []gitmaterializer.PreparedRelease
	publish := func(ctx context.Context, op release.RebuildOperation) (map[string]string, error) {
		prepared = nil
		for _, r := range refs {
			p, err := m.PrepareLegacyReleaseRepoint(ctx, op.ID, r)
			if err != nil {
				return nil, err
			}
			prepared = append(prepared, p)
		}
		commits := map[string]string{}
		for i, p := range prepared {
			if err := m.PublishReleaseRepoint(ctx, op.ID, p); err != nil {
				return nil, err
			}
			commits[p.Expected.ProjectID] = p.Commit
			if fail && i == 0 {
				return nil, errors.New("interrupted after first ref")
			}
		}
		return commits, nil
	}
	request := release.LegacyRebuildRequest{Plan: plan}
	op, err := svc.RunLegacyRebuild(ctx, request, check, publish)
	if err == nil || !strings.Contains(err.Error(), "interrupted after first ref") {
		t.Fatalf("expected publication interruption, got %v", err)
	}
	firstPrepared := append([]gitmaterializer.PreparedRelease(nil), prepared...)
	if _, err := m.PrepareReleaseRepoint(ctx, op.ID, refs[0]); err == nil {
		t.Fatal("verified publisher accepted legacy content")
	}
	first := t.TempDir()
	if err := m.BuildLegacyReleaseTree(ctx, first, zmatProjectID, "7.0.1"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := gitmaterializer.LoadProjectSnapshot(first)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Manifest.SchemaVersion != 2 || snapshot.Mod.SchemaVersion != 2 || snapshot.LegacyOntology == nil || len(snapshot.Vendor.Ontologies) != 0 {
		t.Fatal("legacy evidence transport missing or reinterpreted")
	}
	a := *snapshot.LegacyOntology
	d, err := a.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Inventory) != 2 || len(d.Sources) != 1 {
		t.Fatal("lost versions or source context")
	}
	assertLegacyTransportAliases(t, d)
	if snapshot.Manifest.Namespaces != nil {
		t.Fatal("schema 2 emitted a collapsed namespace map")
	}
	for _, vendor := range snapshot.Vendor.Projects {
		if vendor.Snapshot.Manifest.Namespaces != nil {
			t.Fatal("schema 2 source emitted a collapsed namespace map")
		}
	}
	if _, err := a.Resolve("field", "ZMATF1", "/path_elements/0"); err == nil {
		t.Fatal("ambiguous definition silently resolved")
	}
	if _, err := a.Resolve("field", "ZMATF1", "/path_elements/1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Resolve("field", "transport-source-field", "/ontology_scope"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitmaterializer.PrepareRestorePlan(first, "different-project"); err == nil {
		t.Fatal("retargeted retained meaning without provenance")
	}

	// Restore exact bodies and evidence into an empty catalog, without an RDF
	// importer or active selection. The raw RDF deliberately differs from rows.
	target := testdb.EmptyPool(t)
	// Project shell hydration requires the owner identity to exist at the destination.
	if _, err := target.Exec(ctx, `INSERT INTO weave_actors(id,type,slug,display_name) VALUES ('ZMAT_OWNER','organization','zmat-owner','Owner')`); err != nil {
		t.Fatal(err)
	}
	targetMat := gitmaterializer.NewMaterializer(target, t.TempDir(), testLogger())
	restore := &gitmaterializer.RestorePlan{ProjectID: zmatProjectID, Snapshot: snapshot}
	if err := targetMat.HydrateVendored(ctx, restore); err != nil {
		t.Fatal(err)
	}
	if err := targetMat.HydrateVendored(ctx, restore); err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	ref, err := a.Reference()
	if err != nil {
		t.Fatal(err)
	}
	if err := ontologysnapshot.NewFactory(target).ReadImported(ctx, ref, func(reader ontologysnapshot.Reader) error {
		got, err := reader.Snapshot(ctx)
		if err == nil && got != a {
			t.Error("restore changed exact evidence, including root/source alias registries")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Exec(ctx, `INSERT INTO weave_projects(id,owner_id) VALUES ('ZMAT','ZMAT_OWNER');
		INSERT INTO weave_namespace_bindings(id,project_id,prefix,namespace,weight,source,created_at,updated_at)
		VALUES ('destination-policy','ZMAT','crmarchaeo','https://destination.example/',99,'user',now(),now())`); err != nil {
		t.Fatal(err)
	}
	var policyBefore, policyAfter string
	policySQL := `SELECT COALESCE(jsonb_agg(to_jsonb(b) ORDER BY id),'[]'::jsonb)::text FROM weave_namespace_bindings b`
	if err := target.QueryRow(ctx, policySQL).Scan(&policyBefore); err != nil {
		t.Fatal(err)
	}
	if err := targetMat.HydrateProjectShell(ctx, restore); err != nil {
		t.Fatal(err)
	}
	if err := target.QueryRow(ctx, policySQL).Scan(&policyAfter); err != nil {
		t.Fatal(err)
	}
	if policyAfter != policyBefore {
		t.Fatal("schema 2 hydration changed current namespace policy")
	}
	for _, b := range d.Inventory {
		var body, digest string
		if err := target.QueryRow(ctx, `SELECT COALESCE(body_text,body::text),digest FROM weave_ontology_body_seals WHERE version_id=$1`, b.VersionID).Scan(&body, &digest); err != nil {
			t.Fatal(err)
		}
		if body != b.BodyJSON || digest != b.Digest {
			t.Fatal("restore changed body bytes")
		}
	}
	var unexpected int
	if err := target.QueryRow(ctx, `SELECT (SELECT count(*) FROM weave_ontology_releases)+(SELECT count(*) FROM weave_ontology_active_versions)+(SELECT count(*) FROM weave_releases)`).Scan(&unexpected); err != nil || unexpected != 0 {
		t.Fatalf("restore manufactured publication/policy/cut: %d %v", unexpected, err)
	}
	for name, mutate := range map[string]func(){
		"body digest":       func() { snapshot.Mod.Require.Ontologies[0].BodyDigest = strings.Repeat("0", 64) },
		"missing selection": func() { snapshot.Manifest.Ontologies.LinkedVersions = snapshot.Manifest.Ontologies.LinkedVersions[:1] },
		"wrong version":     func() { snapshot.Mod.LegacyOntology.Version = "9.0.0" },
	} {
		t.Run(name, func(t *testing.T) {
			var err error
			snapshot, err = gitmaterializer.LoadProjectSnapshot(first)
			if err != nil {
				t.Fatal(err)
			}
			mutate()
			if err := targetMat.HydrateVendored(ctx, &gitmaterializer.RestorePlan{Snapshot: snapshot}); err == nil {
				t.Fatal("tampered consumer evidence accepted")
			}
		})
	}
	bytes, err := os.ReadFile(filepath.Join(first, "ontology-snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, "ontology-snapshot.json"), append(bytes, ' '), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := gitmaterializer.LoadProjectSnapshot(first); err == nil {
		t.Fatal("changed snapshot bytes accepted")
	}
	if err := os.WriteFile(filepath.Join(first, "ontology-snapshot.json"), bytes, 0644); err != nil {
		t.Fatal(err)
	}

	exec(`UPDATE weave_fields SET path_elements='[]' WHERE id='ZMATF1'`)
	exec(`UPDATE weave_ontologies SET name='Later catalog',namespace='https://changed.example/' WHERE id='transport-crm'`)
	exec(`UPDATE weave_namespace_bindings SET namespace='https://changed.example/' WHERE prefix='crm'`)
	fail = false
	completed, err := svc.RunLegacyRebuild(ctx, request, check, publish)
	if err != nil || !completed.Complete {
		t.Fatalf("resume: %v", err)
	}
	if diff := cmp.Diff(firstPrepared, prepared); diff != "" {
		t.Fatalf("resume changed prepared objects: %s", diff)
	}
	second := t.TempDir()
	if err := m.BuildLegacyReleaseTree(ctx, second, zmatProjectID, "7.0.1"); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(collectTree(t, first), collectTree(t, second)); diff != "" {
		t.Fatalf("released tree followed current data: %s", diff)
	}
}

func assertLegacyTransportAliases(t *testing.T, root ontologysnapshot.Document) {
	t.Helper()
	contexts := make([]ontologysnapshot.Document, 0, 1+len(root.Sources))
	contexts = append(contexts, root)
	for _, source := range root.Sources {
		var d ontologysnapshot.Document
		if err := json.Unmarshal([]byte(source.JSON), &d); err != nil {
			t.Fatal(err)
		}
		contexts = append(contexts, d)
	}
	for _, d := range contexts {
		aliases := map[string]bool{}
		for _, binding := range d.Bindings {
			if binding.Prefix == "crmarchaeo" {
				if binding.Weight != 40 || binding.Source != "version-derived" {
					t.Fatalf("changed captured alias: %+v", binding)
				}
				aliases[binding.Namespace] = true
			}
		}
		want := map[string]bool{"https://old.example/crmarchaeo/": true, "https://new.example/crmarchaeo/": true}
		if diff := cmp.Diff(want, aliases); diff != "" {
			t.Fatalf("%s lost tied aliases: %s", d.ProjectID, diff)
		}
	}
}
