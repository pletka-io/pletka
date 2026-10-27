//go:build integration

package gitmaterializer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestProjectSemanticReferencesCaptureAndIsolation(t *testing.T) {
	pool := testPool(t)
	ctx := seedZMAT(t, pool)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, sql := range []string{
			`DELETE FROM weave_project_ontology_versions WHERE project_id='ZMAT'`,
			`DELETE FROM weave_project_ontology_versions_archive WHERE project_id='ZMAT'`,
			`DELETE FROM weave_collections WHERE project_id='ZMAT'`,
			`DELETE FROM weave_collections_archive WHERE project_id='ZMAT'`,
			`DELETE FROM weave_namespace_bindings WHERE project_id='ZMAT'`,
		} {
			if _, err := pool.Exec(context.Background(), sql); err != nil {
				t.Error(err)
			}
		}
	})
	store := ontology.NewPostgresStore(pool)
	defaults, err := store.GetOntologyByPrefix(ctx, "defaults")
	if err != nil || defaults == nil {
		t.Fatalf("defaults: %v", err)
	}
	oldDefaults, err := store.GetActiveVersion(ctx, defaults.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if oldDefaults != nil {
			if err := store.SetActiveVersion(context.Background(), defaults.ID, oldDefaults.ID); err != nil {
				t.Error(err)
			}
		}
	})
	defaultVersion := "semantic-defaults-1"
	if _, err := store.ImportVersion(ctx, ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: defaultVersion, OntologyID: defaults.ID, VersionString: defaultVersion, RDFContent: "<defaults/>"}, SetActive: true,
		Classes: []ontology.ImportClass{{ID: "semantic-string", Prefix: "xsd", LocalName: "string", URI: "http://www.w3.org/2001/XMLSchema#string"}}}); err != nil {
		t.Fatal(err)
	}
	owner, version := "semantic-owner", "semantic-version-1"
	if _, err := store.CreateOntology(ctx, ontology.CreateOntologyInput{ID: owner, Prefix: "sem", Name: "Semantic test", Namespace: "https://canonical.example/", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	input := ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: version, OntologyID: owner, VersionString: "1", RDFContent: "<terms/>"}, SetActive: true,
		Classes: []ontology.ImportClass{
			{ID: "semantic-A", Prefix: "sem", LocalName: "A", URI: "https://old.example/A"},
			{ID: "semantic-B", Prefix: "sem", LocalName: "B", URI: "https://old.example/B"},
		},
		Properties:        []ontology.ImportProperty{{ID: "semantic-P", Prefix: "sem", LocalName: "P", URI: "https://old.example/P"}},
		NamespaceBindings: []ontology.NamespaceBinding{{Prefix: "sem", Namespace: "https://canonical.example/", Weight: 80}, {Prefix: "alias", Namespace: "https://old.example/", Weight: 20}},
	}
	if _, err := store.ImportVersion(ctx, input); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,added_at) VALUES ('ZMAT',$1,now())`, version)
	path := `[{"type":"class","uri":"sem:A","additional_types":[{"prefix":"alias","local_name":"B"}]},{"type":"property","uri":"https://old.example/P"},{"type":"literal","uri":"xsd:string","datatype":"http://www.w3.org/2001/XMLSchema#string"}]`
	exec(`UPDATE weave_fields SET ontology_scope='{"type":"class","prefix":"sem","local_name":"A"}',path_elements=$1::jsonb,
		subfield_paths='[{"path_elements":[{"type":"class","uri":"alias:B"}],"scope":"legacy-display-only","source":"https://provenance.example/"}]',
		expected_value_type='Reference Model',examples='[{"value":"https://instance.example/not-an-ontology-term"}]'
		WHERE id='ZMATF1'`, semanticPath(t, path))
	exec(`UPDATE weave_models SET ontology_scope='{"type":"class","uri":"sem:A"}' WHERE id='ZMATM.1'`)
	exec(`INSERT INTO weave_collections(id,project_id,system_name,status,ontology_scope) VALUES ('SEM-C','ZMAT','semantic-collection','draft','{"type":"class","uri":"alias:B"}')`)
	svc := newReleaseService(pool)
	if _, err := svc.Create(zmatAuthCtx(ctx), zmatProjectID, release.CreateInput{Version: "7.11.0"}); err != nil {
		t.Fatal(err)
	}
	read := func() []ontologyrelease.TermResolution {
		t.Helper()
		var refs []ontologyrelease.TermResolution
		if err := ontologyrelease.NewFactory(pool).ReadProject(ctx, zmatProjectID, "7.11.0", func(r ontologyrelease.ProjectReader) error { var err error; refs, err = r.References(ctx); return err }); err != nil {
			t.Fatal(err)
		}
		return refs
	}
	before := read()
	archived, err := field.NewPostgresStore(pool).GetByIDVersion(ctx, zmatProjectID, "ZMATF1", "7.11.0")
	if err != nil || archived == nil || len(archived.SubfieldPaths) != 1 {
		t.Fatalf("archived subfield paths missing: %+v, %v", archived, err)
	}
	if len(before) != 9 {
		t.Fatalf("inventory = %d, want 9: %+v", len(before), before)
	}
	for _, r := range before {
		if r.VersionID == "" || r.Digest == "" || r.Reference.VersionID != "" {
			t.Fatalf("unversioned input was not resolved: %+v", r)
		}
		if strings.HasPrefix(r.URI, "https://canonical.example/") {
			t.Fatal("rewrote alias to canonical namespace")
		}
	}
	mat := gitmaterializer.NewMaterializer(pool, t.TempDir(), testLogger())
	first := t.TempDir()
	if err := mat.BuildReleaseTree(ctx, first, zmatProjectID, "7.11.0"); err != nil {
		t.Fatal(err)
	}
	var foundSubfield bool
	for _, content := range collectTree(t, first) {
		foundSubfield = foundSubfield || strings.Contains(content, "legacy-display-only")
	}
	if !foundSubfield {
		t.Fatal("released serialization discarded archived subfield paths")
	}
	for _, tc := range []struct{ name, path string }{
		{"unknown term", `[{"type":"class","uri":"sem:Missing"}]`},
		{"wrong type", `[{"type":"property","uri":"sem:A"}]`},
		{"contradictory identity", `[{"type":"class","uri":"https://old.example/A","prefix":"sem","local_name":"B"}]`},
		{"unknown additional type", `[{"type":"class","uri":"sem:A","additional_types":[{"uri":"unknown:X"}]}]`},
		{"unknown datatype", `[{"type":"literal","uri":"xsd:string","datatype":"xsd:missing"}]`},
		{"incomplete element", `[{"type":"class"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec(`UPDATE weave_fields SET path_elements=$1::jsonb WHERE id='ZMATF1'`, semanticPath(t, tc.path))
			_, err := svc.Create(zmatAuthCtx(ctx), zmatProjectID, release.CreateInput{Version: "7.11.1"})
			if apierror.FromError(err).Code != apierror.CodeValidation {
				t.Fatalf("wanted validation, got %v", err)
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM weave_releases WHERE project_id='ZMAT' AND version='7.11.1')+(SELECT count(*) FROM weave_fields_archive WHERE project_id='ZMAT' AND version_number='7.11.1')+(SELECT count(*) FROM weave_change_set WHERE project_id='ZMAT' AND release_version='7.11.1')`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("partial failed release survived: %d", count)
			}
		})
	}
	exec(`UPDATE weave_fields SET path_elements=$1::jsonb WHERE id='ZMATF1'`, semanticPath(t, path))
	for _, sql := range []string{
		`UPDATE weave_release_ontology_references SET resolutions='[]' WHERE project_id='ZMAT'`,
		`DELETE FROM weave_release_ontology_references WHERE project_id='ZMAT'`,
		`TRUNCATE weave_release_ontology_references`,
	} {
		if _, err := pool.Exec(ctx, sql); err == nil {
			t.Fatalf("immutable inventory accepted %s", sql)
		}
	}
	// Verify the database checks the same coverage and identity as the Go cut,
	// even when a caller bypasses CaptureProject.
	for _, tc := range []string{"omit", "duplicate", "wrong term", "valid"} {
		t.Run("raw "+tc, func(t *testing.T) {
			tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx) // test-only transaction always rolls back
			rawExec := func(sql string, args ...any) {
				t.Helper()
				if _, err := tx.Exec(ctx, sql, args...); err != nil {
					t.Fatal(err)
				}
			}
			rawExec(`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('ZMAT','7.11.9','ZMAT_OWNER')`)
			for _, table := range []string{"weave_fields_archive", "weave_models_archive", "weave_collections_archive"} {
				rawExec(fmt.Sprintf(`INSERT INTO %s SELECT (jsonb_populate_record(NULL::%s,to_jsonb(f)||'{"version_number":"7.11.9"}'::jsonb)).* FROM %s f WHERE project_id='ZMAT' AND version_number='7.11.0'`, table, table, table))
			}
			rawExec(`INSERT INTO weave_release_ontology_dependencies SELECT project_id,'7.11.9',ontology_id,ontology_version_id,digest FROM weave_release_ontology_dependencies WHERE project_id='ZMAT' AND version_number='7.11.0'`)
			rawExec(`UPDATE weave_releases SET ontology_dependencies_version=1 WHERE project_id='ZMAT' AND version='7.11.9'`)
			refs := append([]ontologyrelease.TermResolution{}, before...)
			switch tc {
			case "omit":
				refs = refs[1:]
			case "duplicate":
				refs[0] = refs[1]
			case "wrong term":
				refs[0].URI = "https://old.example/A"
			}
			data, err := json.Marshal(refs)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, `INSERT INTO weave_release_ontology_references VALUES ('ZMAT','7.11.9',$1)`, data)
			if (err == nil) != (tc == "valid") {
				t.Fatalf("storage check %s: %v", tc, err)
			}
		})
	}
	exec(`UPDATE weave_fields SET path_elements='[]',ontology_scope='{}',subfield_paths='[]' WHERE id='ZMATF1'`)
	exec(`UPDATE weave_namespace_bindings SET namespace='https://changed.example/' WHERE prefix='alias'`)
	input.Version.ID, input.Version.VersionString = "semantic-version-2", "2"
	input.Classes, input.Properties = nil, nil
	input.NamespaceBindings = nil
	if _, err := store.ImportVersion(ctx, input); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(before, read()); diff != "" {
		t.Fatalf("live edits changed inventory: %s", diff)
	}
	second := t.TempDir()
	if err := mat.BuildReleaseTree(ctx, second, zmatProjectID, "7.11.0"); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(collectTree(t, first), collectTree(t, second)); diff != "" {
		t.Fatalf("released tree changed: %s", diff)
	}
	// The consumer deliberately assigns the alias a different meaning. Its
	// adopted field still belongs to the source release, including subfields.
	exec(`INSERT INTO weave_projects(id,owner_id) VALUES ('SEM-CHILD','ZMAT_OWNER')`)
	t.Cleanup(func() {
		for _, sql := range []string{
			`DELETE FROM weave_adoptions WHERE project_id='SEM-CHILD'`,
			`DELETE FROM weave_adoptions_archive WHERE project_id='SEM-CHILD'`,
			`DELETE FROM weave_releases WHERE project_id='SEM-CHILD'`,
			`DELETE FROM weave_namespace_bindings WHERE project_id='SEM-CHILD'`,
			`DELETE FROM weave_change_set WHERE project_id='SEM-CHILD'`,
			`DELETE FROM weave_projects_archive WHERE id='SEM-CHILD'`,
			`DELETE FROM weave_projects WHERE id='SEM-CHILD'`,
		} {
			if _, err := pool.Exec(context.Background(), sql); err != nil {
				t.Error(err)
			}
		}
	})
	exec(`INSERT INTO weave_adoptions(project_id,context_entity_type,context_entity_id,entity_type,source_project_id,source_entity_id,source_version) VALUES ('SEM-CHILD','project','SEM-CHILD','field','ZMAT','ZMATF1','7.11.0')`)
	exec(`INSERT INTO weave_namespace_bindings(id,project_id,prefix,namespace,weight,source,created_at,updated_at) VALUES ('semantic-child-alias','SEM-CHILD','alias','https://consumer.example/',1000,'manifest',now(),now())`)
	if _, err := svc.Create(zmatAuthCtx(ctx), "SEM-CHILD", release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if err := ontologyrelease.NewFactory(pool).ReadProject(ctx, "SEM-CHILD", "1.0.0", func(r ontologyrelease.ProjectReader) error {
		refs, err := r.References(ctx)
		if err != nil {
			return err
		}
		if len(refs) != 0 {
			t.Fatalf("consumer re-resolved source references: %+v", refs)
		}
		artifacts, err := r.Artifacts(ctx)
		if err != nil {
			return err
		}
		if len(artifacts) != 2 {
			t.Fatalf("missing source ontology closure: %d", len(artifacts))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	childTree := t.TempDir()
	if err := mat.BuildReleaseTree(ctx, childTree, "SEM-CHILD", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	foundSubfield = false
	for _, content := range collectTree(t, childTree) {
		foundSubfield = foundSubfield || strings.Contains(content, "legacy-display-only")
	}
	if !foundSubfield {
		t.Fatal("adopted archived field lost subfield paths")
	}
	exec(`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('ZMAT','7.11.8','ZMAT_OWNER')`)
	if err := ontologyrelease.NewFactory(pool).ReadProject(ctx, zmatProjectID, "7.11.8", func(ontologyrelease.ProjectReader) error { t.Fatal("unverified reader escaped"); return nil }); apierror.FromError(err).Code != apierror.CodeNotInRelease {
		t.Fatalf("historical inventory: %v", err)
	}
	// Standard datatypes alone capture defaults; other unlinked ontologies must
	// never be discovered by a matching prefix or current active policy.
	exec(`DELETE FROM weave_project_ontology_versions WHERE project_id='ZMAT'`)
	exec(`UPDATE weave_models SET ontology_scope='{}' WHERE project_id='ZMAT'`)
	exec(`UPDATE weave_collections SET ontology_scope='{}' WHERE project_id='ZMAT'`)
	exec(`UPDATE weave_fields SET path_elements=$1::jsonb WHERE id='ZMATF1'`, semanticPath(t, `[{"type":"literal","uri":"xsd:string"}]`))
	if _, err := svc.Create(zmatAuthCtx(ctx), zmatProjectID, release.CreateInput{Version: "7.11.2"}); err != nil {
		t.Fatal(err)
	}
	if err := ontologyrelease.NewFactory(pool).ReadProject(ctx, zmatProjectID, "7.11.2", func(r ontologyrelease.ProjectReader) error {
		refs, err := r.References(ctx)
		if err != nil {
			return err
		}
		if len(refs) != 1 || refs[0].VersionID != defaultVersion {
			t.Fatalf("defaults-only inventory: %+v", refs)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE weave_fields SET path_elements=$1::jsonb WHERE id='ZMATF1'`, semanticPath(t, `[{"type":"class","uri":"sem:A"}]`))
	if _, err := svc.Create(zmatAuthCtx(ctx), zmatProjectID, release.CreateInput{Version: "7.11.3"}); apierror.FromError(err).Code != apierror.CodeValidation {
		t.Fatalf("unlinked ontology was inferred: %v", err)
	}
}

func semanticPath(t *testing.T, raw string) []byte {
	t.Helper()
	var path []domain.PathElement
	if err := json.Unmarshal([]byte(raw), &path); err != nil {
		t.Fatal(err)
	}
	for i := range path {
		path[i].Position = i
	}
	data, err := json.Marshal(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
