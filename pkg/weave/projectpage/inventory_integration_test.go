//go:build integration

package projectpage

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserhttp"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/service/browserread"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestInventoryAndAdoptionsUseOneSelectedGraph(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := auth.WithPrincipal(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "INVOWNER"})
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,type,slug,display_name,visibility) VALUES('INVOWNER','organization','inventory-owner','Captured adopter','public');
 INSERT INTO weave_projects(id,owner_id,visibility,ui_name) VALUES('INVS','INVOWNER','public','{"en":"Source"}'),('INVP','INVOWNER','public','{"en":"Consumer"}');
 INSERT INTO weave_models(id,project_id,ui_name,model_type) VALUES('INVS.M','INVS','{"en":"Aardvark"}','core'),('INVS.N','INVS','{"en":"Zulu"}','auxiliary'),('INVS.U','INVS','{"en":"Hidden"}','core');
 INSERT INTO weave_categories(id,project_id,ui_name) VALUES('INVS.CAT','INVS','{"en":"Captured category"}');
 INSERT INTO weave_collections(id,project_id,ui_name,default_category_id) VALUES('INVS.C','INVS','{"en":"Box"}','INVS.CAT');
 INSERT INTO weave_fields(id,project_id,ui_name,path_elements) VALUES('INVS.F','INVS','{"en":"Source field"}','[]');
 INSERT INTO weave_field_overrides(id,project_id,entity_type,entity_id,field_id,category_id) VALUES(80001,'INVS','','','INVS.F','INVS.CAT'),(80002,'INVS','model','INVS.M','INVS.F',NULL),(80003,'INVS','model','INVS.N','INVS.F',NULL),(80004,'INVS','collection','INVS.C','INVS.F',NULL);
 INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id,position) VALUES(80002,'resource_model','INVS.N','',0),(80002,'collection_model','INVS.C','',1),(80003,'resource_model','INVS.M','',0);`)
	releases := release.NewService(release.NewPostgresStore(pool), pool, nil)
	if _, err := releases.Create(ctx, "INVS", release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES('INVP','INVS','release','1.0.0');
 INSERT INTO weave_models(id,project_id,ui_name,model_type) VALUES('INVP.M','INVP','{"en":"Bee"}','core'),('INVP.N','INVP','{"en":"Zebra"}','core');
 INSERT INTO weave_fields(id,project_id,ui_name,path_elements) VALUES('INVP.F','INVP','{"en":"Own field"}','[]');
 INSERT INTO weave_field_overrides(id,project_id,entity_type,entity_id,field_id,category_id,part_of_collection_id) VALUES(80005,'INVP','','','INVS.F','INVS.CAT',NULL),(80006,'INVP','model','INVP.M','INVS.F','INVS.CAT','INVS.C'),(80007,'INVP','model','INVP.N','INVP.F',NULL,NULL);
 INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id,position) VALUES(80006,'resource_model','INVS.N','',0);
 INSERT INTO weave_adoptions(project_id,context_entity_type,context_entity_id,entity_type,source_project_id,source_entity_id,source_version,created_by_id) VALUES('INVP','project','INVP','model','INVS','INVS.M','1.0.0','INVOWNER'),('INVP','project','INVP','field','INVS','INVS.F','1.0.0','INVOWNER');`)
	if _, err := releases.Create(ctx, "INVP", release.CreateInput{Version: "3.0.0"}); err != nil {
		t.Fatal(err)
	}
	source, err := browserread.New(readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	query := browserinput.InventoryQuery{Page: 1, PerPage: 2, SortBy: "ui_name"}
	first, err := source.Inventory(ctx, auth.Release("3.0.0"), "INVP", "model", query)
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 4 || len(first.Items) != 2 || first.Items[0].ID != "INVS.M" || first.Items[1].ID != "INVP.M" {
		t.Fatalf("global page: %+v", first)
	}
	if first.Items[0].SemanticID != "INVS.M" || first.Items[0].SourceVersion != "1.0.0" || first.Items[0].FieldCount != 0 || first.Items[1].FieldCount != 1 || first.Items[1].CollectionCount != 1 {
		t.Fatalf("wrong source or consumer counts: %+v", first.Items)
	}
	query.Page = 2
	second, err := source.Inventory(ctx, auth.Release("3.0.0"), "INVP", "model", query)
	if err != nil {
		t.Fatal(err)
	}
	if second.Total != 4 || len(second.Items) != 2 || second.Items[0].ID != "INVP.N" || second.Items[1].ID != "INVS.N" {
		t.Fatalf("second page: %+v", second)
	}
	query.Page = 3
	empty, err := source.Inventory(ctx, auth.Release("3.0.0"), "INVP", "model", query)
	if err != nil || empty.Total != 4 || len(empty.Items) != 0 {
		t.Fatalf("empty page loses total: %+v %v", empty, err)
	}
	query.Page = 1
	query.Search = "Zulu"
	filtered, err := source.Inventory(ctx, auth.Release("3.0.0"), "INVP", "model", query)
	if err != nil || filtered.Total != 1 || filtered.Items[0].OriginKind != "adopted_reference" {
		t.Fatalf("foreign search: %+v %v", filtered, err)
	}
	query.Search = "en"
	filtered, err = source.Inventory(ctx, auth.Release("3.0.0"), "INVP", "model", query)
	if err != nil || filtered.Total != 0 {
		t.Fatalf("search matched language keys: %+v %v", filtered, err)
	}
	query.Search = ""
	query.Descending = true
	filtered, err = source.Inventory(ctx, auth.Release("3.0.0"), "INVP", "model", query)
	if err != nil || len(filtered.Items) != 2 || filtered.Items[0].ID != "INVS.N" || filtered.Items[1].ID != "INVP.N" {
		t.Fatalf("descending union: %+v %v", filtered, err)
	}
	query.Descending = false
	query.Search = ""
	query.Origin = "adopted"
	filtered, err = source.Inventory(ctx, auth.Release("3.0.0"), "INVP", "model", query)
	if err != nil || filtered.Total != 1 || !filtered.Items[0].NotReferenced {
		t.Fatalf("origin filter: %+v %v", filtered, err)
	}
	query.Origin = ""
	query.ModelType = "core"
	filtered, err = source.Inventory(ctx, auth.Release("3.0.0"), "INVP", "model", query)
	if err != nil || filtered.Total != 3 {
		t.Fatalf("type filter: %+v %v", filtered, err)
	}
	query.ModelType = ""
	query.Category = "INVS.CAT"
	fields, err := source.Inventory(ctx, auth.Release("3.0.0"), "INVP", "field", query)
	if err != nil {
		t.Fatal(err)
	}
	if fields.Total != 1 || fields.Items[0].ID != "INVS.F" || fields.Items[0].ModelCount != 1 || fields.Items[0].OriginKind != "adopted" || fields.Items[0].NotReferenced || fields.Categories["INVS.CAT"]["en"] != "Captured category" {
		t.Fatalf("field filter and explicit precedence: %+v", fields)
	}
	adoptions, err := source.Adoptions(ctx, auth.Release("3.0.0"), "INVP")
	if err != nil {
		t.Fatal(err)
	}
	if len(adoptions.Items) != 2 {
		t.Fatalf("receipts: %+v", adoptions)
	}
	for _, a := range adoptions.Items {
		if a.Actor.Label != "Captured adopter" {
			t.Fatalf("actor: %+v", a.Actor)
		}
		if a.Receipt.EntityType == "model" && (len(a.Closure["model"]) != 2 || len(a.Direct["model"]) != 1 || len(a.Closure["collection"]) != 1 || len(a.Closure["field"]) != 1) {
			t.Fatalf("cyclic receipt closure: %+v", a)
		}
		if a.Receipt.EntityType == "field" && (len(a.Closure["field"]) != 1 || len(a.Direct["field"]) != 1) {
			t.Fatalf("field seed: %+v", a)
		}
	}
	store := weave.NewPostgresStore(pool)
	router := chi.NewRouter()
	router.Use(auth.WithProjectVersionContext)
	router.Route("/projects/{projectID}", func(r chi.Router) {
		r.Use(auth.RequireProjectRead(store.Projects()))
		h := browserhttp.Handler{Source: source}
		r.Get("/models", func(w http.ResponseWriter, r *http.Request) { h.List(w, r, "model") })
		r.Get("/fields/filters/categories", func(w http.ResponseWriter, r *http.Request) { h.Filters(w, r, "field", "category") })
		ph := &Handler{logger: slog.Default(), source: source}
		r.Get("/adoptions-tab-schema", ph.ProjectAdoptionsTabSchema)
		r.Get("/adoptions/{sourceProjectID}/{sourceEntityID}/closure", ph.ProjectAdoptionClosure)
	})
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		return w
	}
	modelURL := "/projects/INVP/models?version=3.0.0&per_page=2"
	tabURL := "/projects/INVP/adoptions-tab-schema?version=3.0.0"
	closureURL := "/projects/INVP/adoptions/INVS/INVS.M/closure?kind=models&version=3.0.0"
	if w := get("/projects/INVP/adoptions/INVS/ABSENT/closure?kind=models&version=3.0.0"); w.Code != 404 || !strings.Contains(w.Body.String(), "not_in_release") {
		t.Fatalf("missing released receipt: %d %s", w.Code, w.Body.String())
	}
	before := map[string]string{}
	for _, u := range []string{modelURL, tabURL, closureURL, "/projects/INVP/fields/filters/categories?version=3.0.0"} {
		w := get(u)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", u, w.Code, w.Body.String())
		}
		before[u] = w.Body.String()
	}
	if !strings.Contains(before[tabURL], "version=3.0.0") || !strings.Contains(before[closureURL], "version=1.0.0") {
		t.Fatal("unpinned links")
	}
	exec(`UPDATE weave_models SET ui_name='{"en":"Changed source"}' WHERE id='INVS.M';UPDATE weave_actors SET display_name='Changed adopter' WHERE id='INVOWNER';`)
	if _, err := releases.Create(ctx, "INVS", release.CreateInput{Version: "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE weave_project_inheritance SET source_version='2.0.0' WHERE project_id='INVP';UPDATE weave_adoptions SET source_version='2.0.0' WHERE project_id='INVP';DELETE FROM weave_field_overrides;DELETE FROM weave_models;DELETE FROM weave_fields;DELETE FROM weave_collections;DELETE FROM weave_categories;`)
	for u, want := range before {
		w := get(u)
		if w.Code != 200 || w.Body.String() != want {
			t.Fatalf("live deletion changed %s: %d %s", u, w.Code, w.Body.String())
		}
	}
	draft, err := source.Adoptions(ctx, auth.Draft(), "INVP")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range draft.Items {
		if a.Receipt.EntityType == "model" && (a.Root.UIName["en"] != "Changed source" || a.Receipt.SourceVersion != "2.0.0") {
			t.Fatalf("draft ignored repin: %+v", a)
		}
	}
	exec(`UPDATE weave_projects SET visibility='private' WHERE id='INVS';`)
	if w := get(modelURL); w.Code != 403 {
		t.Fatalf("source authorization: %d %s", w.Code, w.Body.String())
	}
	exec(`UPDATE weave_projects SET visibility='public' WHERE id='INVS';UPDATE weave_projects SET visibility='private' WHERE id='INVP';`)
	if w := get(modelURL); w.Code != 404 {
		t.Fatalf("root authorization: %d %s", w.Code, w.Body.String())
	}
	schema, err := adoptionSchema(adoptions)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	again, err := adoptionSchema(adoptions)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(again)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved, after) {
		t.Fatal("projection depends on database lifetime")
	}
}

func TestInventorySnapshotAndExplicitBudget(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := t.Context()
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,type,slug,display_name,visibility) VALUES('INVBOWNER','organization','inventory-budget','Budget owner','public');INSERT INTO weave_projects(id,owner_id,visibility) VALUES('INVB','INVBOWNER','public');INSERT INTO weave_fields(id,project_id,ui_name,path_elements) VALUES('INVB.F','INVB','{"en":"Before"}','[]');`)
	exec(`UPDATE weave_fields SET ontology_scope='{"local_name":"E33_E41_Appellation","class_code":"Explicit","additional_types":[{"prefix":"custom","local_name":"Other"}]}' WHERE id='INVB.F';`)
	factory := readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead)
	if err := factory.Read(ctx, auth.Draft(), "INVB", func(session *readsession.Session) error {
		index, err := session.InventoryIndex(ctx, "field", nil, nil, nil, browserinput.InventoryQuery{SortBy: "ui_name"}, 10)
		if err != nil {
			return err
		}
		if index.Total != 1 || len(index.Items) != 1 || len(index.Scopes) != 1 || index.Scopes[0] != "E33_E41_Appellation/custom:Other" {
			t.Fatalf("index: %+v", index)
		}
		exec(`DELETE FROM weave_fields WHERE project_id='INVB';INSERT INTO weave_fields(id,project_id,ui_name,path_elements) VALUES('INVB.NEW','INVB','{"en":"After"}','[]');`)
		rows, err := session.InventoryRows(ctx, "field", []string{"INVB.F"})
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0].UIName["en"] != "Before" || rows[0].OntologyScope != "E33_E41_Appellation/custom:Other" || rows[0].ScopeClassCode != "Explicit" {
			t.Fatalf("page hydration changed snapshots: %+v", rows)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO weave_fields(id,project_id,path_elements,ontology_scope) SELECT 'INVB.'||n::text,'INVB','[]',jsonb_build_object('prefix','unknown','local_name','Term'||n::text) FROM generate_series(1,1001) n;`)
	source, err := browserread.New(factory, projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	value, err := source.Inventory(ctx, auth.Draft(), "INVB", "field", browserinput.InventoryQuery{Page: 1, PerPage: 1})
	if err == nil || len(value.Items) > 0 || value.Total != 0 {
		t.Fatalf("oversized facets returned partial value: %+v %v", value, err)
	}
}
