//go:build integration

package csvexport_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/service/generatorread"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/project/csvexport"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }

func TestWholeProjectCSVReleaseSurvivesLiveEditsAndDeletes(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := auth.WithPrincipal(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "CSVOWNER"})
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,type,display_name,slug,visibility) VALUES('CSVOWNER','organization','CSV owner','csv-owner','public');
 INSERT INTO weave_projects(id,owner_id,status,visibility,system_name) VALUES('CSVSRC','CSVOWNER','draft','public','source'),('CSV','CSVOWNER','draft','public','captured');
 INSERT INTO weave_fields(id,semantic_id,project_id,path_elements,system_name) VALUES('CSVSRC.F','SOURCE.ORIGINAL','CSVSRC','[]','source field');
 INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id) VALUES('CSVSRC.F','CSVSRC','','');`)
	releases := release.NewService(release.NewPostgresStore(pool), pool, nil)
	if _, err := releases.Create(ctx, "CSVSRC", release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES('CSV','CSVSRC','release','1.0.0');
 INSERT INTO weave_models(id,project_id,system_name) VALUES('CSV.M','CSV','original_model');
 INSERT INTO weave_collections(id,project_id,system_name) VALUES('CSV.C','CSV','original_collection');
 INSERT INTO weave_categories(id,semantic_id,project_id,system_name) VALUES('CSV.CAT','CSV.CAT','CSV','original_category');
 INSERT INTO weave_fields(id,semantic_id,project_id,path_elements,system_name) VALUES('CSV.F','CSV.FIELD','CSV','[]','original_field');
 INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id,display_name,is_required,max_occurs,set_value) VALUES
 ('CSV.F','CSV','','','{"en":"Base"}',false,0,'base fixed'),
 ('CSV.F','CSV','collection','CSV.C','{"en":"Collection"}',false,0,'collection fixed'),
 ('CSV.F','CSV','model','CSV.M','{"en":"Model"}',true,1,'model fixed'),
 ('CSVSRC.F','CSV','model','CSV.M','{"en":"Foreign"}',false,0,'foreign fixed');
 INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id,position)
 SELECT id,'resource_model','CSV.M','CSV.M',1 FROM weave_field_overrides WHERE project_id='CSV' AND field_id='CSV.F' AND entity_type='';`)
	store := ontology.NewPostgresStore(pool)
	if _, err := store.CreateOntology(ctx, ontology.CreateOntologyInput{ID: "CSVONT", Prefix: "csvont", Name: "Captured ontology", Namespace: "https://csv.example/", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"CSVONT.1", "CSVONT.2"} {
		if _, err := store.ImportVersion(ctx, ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: id, OntologyID: "CSVONT", VersionString: id, RDFContent: "<ontology/>", OntologyURI: "https://csv.example/", OntologyLabel: domain.Translations{"en": "Captured ontology label"}}, SetActive: id == "CSVONT.1"}); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,is_primary,added_by_id,added_at,usage_notes) VALUES('CSV','CSVONT.1',true,'CSVOWNER','2020-01-01T00:00:00.123456Z','captured notes');`)
	if _, err := releases.Create(ctx, "CSV", release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	source, err := generatorread.New(readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	first, err := source.Project(ctx, auth.Release("1.0.0"), "CSV")
	if err != nil {
		t.Fatal(err)
	}
	projected, err := csvexport.Build(first)
	if err != nil {
		t.Fatal(err)
	}
	before, err := projected.ZIP()
	if err != nil {
		t.Fatal(err)
	}
	overrides, err := projected.CSV("model-field-overrides")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(overrides), "SOURCE.ORIGINAL") {
		t.Fatal("foreign field semantic ID omitted")
	}
	ontologies, err := projected.CSV("ontologies")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ontologies), "CSVONT.1") || !strings.Contains(string(ontologies), ".123456Z") {
		t.Fatal("retained ontology metadata omitted")
	}
	if err := store.SetActiveVersion(ctx, "CSVONT", "CSVONT.2"); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE weave_fields SET semantic_id='SOURCE.CURRENT' WHERE id='CSVSRC.F';
 UPDATE weave_projects SET system_name='current' WHERE id='CSV';
 UPDATE weave_project_ontology_versions SET usage_notes='current notes' WHERE project_id='CSV';
 UPDATE weave_field_overrides SET set_value='changed',display_name='{"en":"Changed"}' WHERE project_id='CSV';
 DELETE FROM weave_models WHERE id='CSV.M';
 DELETE FROM weave_collections WHERE id='CSV.C';
 DELETE FROM weave_fields WHERE id='CSV.F';
 DELETE FROM weave_categories WHERE id='CSV.CAT';`)
	second, err := source.Project(ctx, auth.Release("1.0.0"), "CSV")
	if err != nil {
		t.Fatal(err)
	}
	if pool.Stat().AcquiredConns() != 0 {
		t.Fatal("capture retained a connection")
	}
	pool.Close()
	// Projection and complete ZIP output remain usable after all I/O is unavailable.
	afterProjection, err := csvexport.Build(second)
	if err != nil {
		t.Fatal(err)
	}
	after, err := afterProjection.ZIP()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("release CSV ZIP changed with live state")
	}
	for _, kind := range csvexport.ExportTypes() {
		data, err := afterProjection.CSV(kind)
		if err != nil {
			t.Fatal(err)
		}
		if len(bytes.Split(bytes.TrimSpace(data), []byte("\n"))) < 2 {
			t.Fatalf("%s lost released rows", kind)
		}
	}
}
