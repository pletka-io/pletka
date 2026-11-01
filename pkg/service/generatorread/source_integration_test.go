//go:build integration

package generatorread_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/service/generatorread"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/csv"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }

func TestGeneratorReleaseAndBulkInputRemainDetached(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := t.Context()
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,type,display_name,slug,visibility) VALUES('GENOWNER','organization','Generator owner','generator-owner','public');
 INSERT INTO weave_projects(id,owner_id,status,visibility) VALUES('GENSRC','GENOWNER','draft','public'),('GEN','GENOWNER','draft','public');
 INSERT INTO weave_fields(id,project_id,path_elements,system_name) VALUES('GENSRC.F','GENSRC','[]','source original');
 INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id,display_name) VALUES('GENSRC.F','GENSRC','','','{"en":"Source default"}');`)
	authorized := auth.WithPrincipal(auth.WithSnapshot(ctx, &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "GENOWNER"})
	releases := release.NewService(release.NewPostgresStore(pool), pool, nil)
	if _, err := releases.Create(authorized, "GENSRC", release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES('GEN','GENSRC','release','1.0.0');
 INSERT INTO weave_models(id,project_id,system_name,ui_name) VALUES('GEN.M','GEN','original_model','{"en":"Original model"}');
 INSERT INTO weave_categories(id,project_id) VALUES('GEN.CAT','GEN');
 INSERT INTO weave_collections(id,project_id) VALUES('GEN.COL','GEN');
 INSERT INTO weave_fields(id,project_id,path_elements,system_name) VALUES('GEN.F','GEN','[]','original field');
 INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id,display_name,is_required,max_occurs) VALUES
 ('GEN.F','GEN','','','{"en":"Base"}',false,0),
 ('GEN.F','GEN','collection','GEN.COL','{"en":"Collection field"}',false,0),
 ('GEN.F','GEN','model','GEN.M','{"en":"Model field"}',false,0),
 ('GENSRC.F','GEN','model','GEN.M','{"en":"Consumer field"}',true,1);
 INSERT INTO weave_collection_placements(project_id,model_id,category_id,collection_id,is_required,min_occurs,max_occurs) VALUES('GEN','GEN.M','GEN.CAT','GEN.COL',true,1,2);
 INSERT INTO weave_namespace_bindings(id,project_id,prefix,namespace,created_at,updated_at) VALUES('GEN.NS','GEN','gen','https://old.example/',now(),now());`)
	source, err := generatorread.New(readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generators.NewRegistry(csv.NewRenderer())
	if err != nil {
		t.Fatal(err)
	}
	service := generators.NewService(source, registry)
	// Each selection uses bounded entity assembly; the bulk path is explicit.
	for _, kind := range []string{"model", "collection", "field"} {
		id := map[string]string{"model": "GEN.M", "collection": "GEN.COL", "field": "GEN.F"}[kind]
		fragment, err := source.Entity(authorized, auth.Draft(), "GEN", kind, id)
		if err != nil {
			t.Fatal(err)
		}
		snap, err := generators.BuildSnapshot(fragment, generators.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if snap.Selection.Kind != kind || snap.Selection.ProjectID != "GEN" {
			t.Fatal("wrong selected identity")
		}
	}
	if _, err := releases.Create(authorized, "GEN", release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	first, err := service.SnapshotForModel(authorized, auth.Release("1.0.0"), "GEN", "GEN.M", generators.Options{})
	if err != nil {
		t.Fatal(err)
	}
	before, err := generators.EncodeInput(first)
	if err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE weave_fields SET system_name='source changed' WHERE id='GENSRC.F'; UPDATE weave_models SET ui_name='{"en":"New model"}',system_name='new_model' WHERE id='GEN.M'; UPDATE weave_field_overrides SET display_name='{"en":"Changed"}',is_required=true,max_occurs=9 WHERE field_id='GEN.F'; UPDATE weave_namespace_bindings SET namespace='https://new.example/' WHERE id='GEN.NS';`)
	second, err := service.SnapshotForModel(authorized, auth.Release("1.0.0"), "GEN", "GEN.M", generators.Options{})
	if err != nil {
		t.Fatal(err)
	}
	after, err := generators.EncodeInput(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("released generator input changed after hot edits")
	}
	found := false
	for _, f := range second.Fields {
		if f.Field.ID == "GENSRC.F" {
			found = true
			if f.Field.SystemName != "source original" || f.Field.SourceVersion != "1.0.0" || f.Field.DisplayName["en"] != "Consumer field" {
				t.Fatalf("source/body/context drift: %+v", f.Field)
			}
		}
	}
	if !found {
		t.Fatal("source field disappeared")
	}
	bound, value, err := service.BindProject(authorized, auth.Release("1.0.0"), "GEN")
	if err != nil {
		t.Fatal(err)
	}
	if id, err := generators.ResolveIdentifier(value, "model", "original_model"); err != nil || id != "GEN.M" {
		t.Fatalf("retained identifier: %s %v", id, err)
	}
	if _, err := generators.ResolveIdentifier(value, "model", "new_model"); err == nil {
		t.Fatal("hot identifier accepted")
	}
	if _, err := service.SnapshotForModel(authorized, auth.ReadScope{}, "GEN", "GEN.M", generators.Options{}); err == nil {
		t.Fatal("missing scope accepted")
	}
	if _, err := service.SnapshotForModel(authorized, auth.Release("2.0.0"), "GEN", "GEN.M", generators.Options{}); err == nil {
		t.Fatal("missing release fell back")
	}
	denied, err := generatorread.New(readsession.NewFactory(pool).WithSourceAuthorization(func(context.Context, auth.Resource) error { return context.Canceled }), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := denied.Entity(authorized, auth.Release("1.0.0"), "GEN", "model", "GEN.M"); err == nil {
		t.Fatal("source authorization was bypassed")
	}
	if pool.Stat().AcquiredConns() != 0 {
		t.Fatal("generator retained database connection")
	}
	pool.Close()
	snap, err := bound.SnapshotForModel(authorized, auth.Release("1.0.0"), "GEN", "GEN.M", generators.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var csv bytes.Buffer
	if err := bound.RenderSnapshot(authorized, generators.FormatCSV, snap, &csv); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(csv.String(), "Consumer field") {
		t.Fatal(csv.String())
	}
	encoded, err := generators.EncodeInput(snap)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(encoded, &data); err != nil {
		t.Fatal(err)
	}
	if data["schema_version"] != float64(1) {
		t.Fatal("missing script input schema")
	}
}
