//go:build integration

package projectpage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/service/browserread"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/example"
	"github.com/pletka-io/pletka/pkg/weave/publication"
	"github.com/pletka-io/pletka/pkg/weave/release"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

func TestCatalogInventoriesAndExampleSchemaRetainRelease(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := auth.WithPrincipal(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "CATOWNER"})
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,type,slug,display_name,visibility) VALUES('CATOWNER','organization','cat-owner','Owner','public');
 INSERT INTO weave_projects(id,owner_id,visibility,ui_name) VALUES('CATS','CATOWNER','public','{"en":"Source"}'),('CATP','CATOWNER','public','{"en":"Project"}');
 INSERT INTO weave_models(id,project_id,ui_name) VALUES('CATS.M','CATS','{"en":"Captured target"}');
 INSERT INTO weave_vocabularies(id,project_id,ui_name,connector_type) VALUES('CATS.V','CATS','{"en":"Captured vocabulary"}','local');
 INSERT INTO weave_vocabulary_entries(id,vocabulary_id,uri,label) VALUES('CATS.T','CATS.V','https://cat.test/term','{"en":"Captured term"}');
 INSERT INTO weave_collections(id,project_id,ui_name) VALUES('CATS.C','CATS','{"en":"Nested collection"}');
 INSERT INTO weave_concept_lists(id,project_id,ui_name,vocabulary_id,is_closed) VALUES('CATS.L','CATS','{"en":"Source list"}','CATS.V',true);
 INSERT INTO weave_concept_list_entries(id,concept_list_id,vocabulary_entry_id,custom_label) VALUES('CATS.LE','CATS.L','CATS.T','{"en":"Pinned membership label"}');
 INSERT INTO weave_fields(id,project_id,ui_name,expected_value_type,path_elements) VALUES('CATS.F','CATS','{"en":"Nested concept"}','Concept','[]');
 INSERT INTO weave_field_overrides(id,project_id,field_id,entity_type,entity_id,display_name) VALUES(91002,'CATS','CATS.F','collection','CATS.C','{"en":"Nested choice"}'),(91004,'CATS','CATS.F','model','CATS.M','{"en":"Owner-only field"}');
 INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id) VALUES(91002,'concept_list','CATS.L','');`)
	releases := release.NewService(release.NewPostgresStore(pool), pool, nil)
	if _, err := releases.Create(ctx, "CATS", release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES('CATP','CATS','release','1.0.0');
 INSERT INTO weave_concept_lists(id,project_id,semantic_id,ui_name,vocabulary_id,list_type,is_closed,status) VALUES('CATP.L','CATP','LIST','{"en":"First list"}','CATS.V','CATS.T',true,'published'),('CATP.EMPTY','CATP','','{"en":"Empty list"}',NULL,NULL,false,'draft');
 INSERT INTO weave_concept_list_entries(id,concept_list_id,vocabulary_entry_id,position) VALUES('CATP.LE','CATP.L','CATS.T',0);
 INSERT INTO weave_fields(id,project_id,ui_name,path_elements) VALUES('CATP.F','CATP','{"en":"Captured field"}','[]');
 INSERT INTO weave_field_overrides(id,project_id,field_id,entity_type,entity_id,display_name) VALUES(91001,'CATP','CATP.F','model','CATS.M','{"en":"Captured slot"}');
 INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id) VALUES(91001,'concept_list','CATP.L','');
 INSERT INTO weave_examples(id,project_id,entity_type,entity_id,title,status) VALUES('CATP.E','CATP','model','CATS.M','{"en":"Worked example"}','valid'),('CATP.ZERO','CATP','model','CATS.M','{"en":"Empty example"}','draft');
 INSERT INTO weave_example_values(example_id,override_id,field_id,occurrence_index,value_kind,value_payload,text_value,slot_path) VALUES('CATP.E',91001,'CATP.F',0,'string','{"kind":"string","string_value":"Recorded content"}','Recorded content','91001:0');
 INSERT INTO weave_example_values(example_id,override_id,field_id,value_kind,value_payload,text_value,slot_path) VALUES('CATP.ZERO',999999,'REMOVED.FIELD','string','{}','Legacy scalar','999999:0');
 INSERT INTO weave_example_values(example_id,override_id,field_id,occurrence_index,value_kind,value_payload,linked_example_id,slot_path) VALUES('CATP.E',91001,'CATP.F',1,'example_ref','{"kind":"example_ref","example_id":"CATP.ZERO"}','CATP.ZERO','91001:1');
 INSERT INTO weave_fields(id,project_id,ui_name,expected_value_type,path_elements) VALUES('CATP.N','CATP','{"en":"Nested container"}','Collection','[]');
 INSERT INTO weave_field_overrides(id,project_id,field_id,entity_type,entity_id,display_name,position) VALUES(91003,'CATP','CATP.N','model','CATS.M','{"en":"Container"}',1);
 INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id) VALUES(91003,'collection_model','CATS.C','');
 INSERT INTO weave_example_values(example_id,override_id,field_id,value_kind,value_payload,concept_uri,slot_path) VALUES('CATP.E',91002,'CATS.F','concept','{"kind":"concept","concept_uri":"https://cat.test/term"}','https://cat.test/term','91003:0/91002:0');`)
	if _, err := releases.Create(ctx, "CATP", release.CreateInput{Version: "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	source, err := browserread.New(readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	query := browserinput.InventoryQuery{Page: 1, PerPage: 1, SortBy: "entries", Descending: true}
	list, err := source.Inventory(ctx, auth.Release("2.0.0"), "CATP", "concept_list", query)
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 2 || len(list.Items) != 1 || list.Items[0].ID != "CATP.L" || list.Items[0].EntryCount != 1 || list.Items[0].BoundFieldCount != 1 || list.Items[0].VocabularyLabel != "Captured vocabulary" || list.Items[0].ListTypeLabel != "Captured term" {
		t.Fatalf("list: %+v", list)
	}
	query.Page = 2
	second, err := source.Inventory(ctx, auth.Release("2.0.0"), "CATP", "concept_list", query)
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != "CATP.EMPTY" || second.Items[0].BoundFieldCount != 0 {
		t.Fatalf("empty semantic ID matched refs: %+v %v", second, err)
	}
	query.Page = 3
	empty, err := source.Inventory(ctx, auth.Release("2.0.0"), "CATP", "concept_list", query)
	if err != nil || len(empty.Items) != 0 || empty.Total != 2 {
		t.Fatalf("empty page: %+v %v", empty, err)
	}
	query = browserinput.InventoryQuery{Page: 1, PerPage: 20, Search: "captured vocabulary", VocabularyID: "CATS.V", Status: "published"}
	filtered, err := source.Inventory(ctx, auth.Release("2.0.0"), "CATP", "concept_list", query)
	if err != nil || filtered.Total != 1 {
		t.Fatalf("label search/filters: %+v %v", filtered, err)
	}
	query.Search = "en"
	query.Status = ""
	query.VocabularyID = ""
	filtered, err = source.Inventory(ctx, auth.Release("2.0.0"), "CATP", "concept_list", query)
	if err != nil || filtered.Total != 0 {
		t.Fatalf("language keys matched search: %+v %v", filtered, err)
	}
	query = browserinput.InventoryQuery{Page: 1, PerPage: 20, EntityType: "model", EntityID: "CATS.M", Status: "valid"}
	examples, err := source.Inventory(ctx, auth.Release("2.0.0"), "CATP", "example", query)
	if err != nil || examples.Total != 1 || examples.Items[0].EntityName != "Captured target" || examples.Items[0].Status != "valid" {
		t.Fatalf("examples: %+v %v", examples, err)
	}
	detail, err := source.Detail(ctx, auth.Release("2.0.0"), "CATP", "example", "CATP.E")
	if err != nil {
		t.Fatal(err)
	}
	record, schema, err := example.ProjectRead(ctx, detail, 2, "en", nil)
	if err != nil {
		t.Fatal(err)
	}
	if schema.Target.Name["en"] != "Captured target" || len(record.Values) != 3 || schema.Sections[0].Groups[0].Fields[0].Label["en"] != "Captured slot" || *record.Values[0].ValuePayload.StringValue != "Recorded content" {
		t.Fatalf("schema: %+v record: %+v", schema, record)
	}
	if schema.Endpoint != nil || len(schema.LinkedExamples) != 1 || schema.LinkedExamples[0].URL != "/projects/CATP/examples/CATP.ZERO?version=2.0.0" || len(schema.UnplacedValues) != 1 {
		t.Fatalf("capabilities or recorded links/values: %+v", schema)
	}
	nested := schema.Sections[0].Groups[0].Fields[1].Nested
	if nested == nil || len(nested.Fields) != 1 || nested.Fields[0].Label["en"] != "Nested choice" {
		t.Fatalf("nested projection: %+v", nested)
	}
	choices := nested.Fields[0].ConceptSources
	if len(choices) != 1 || choices[0].SearchURL != "" || len(choices[0].Entries) != 1 || choices[0].Entries[0].Label["en"] != "Pinned membership label" {
		t.Fatalf("selected concept choices: %+v", choices)
	}
	// A foreign model's owner composition must not leak into an empty consumer.
	exec(`INSERT INTO weave_projects(id,owner_id,visibility) VALUES('CATE','CATOWNER','public');INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES('CATE','CATS','release','1.0.0');INSERT INTO weave_adoptions(project_id,context_entity_type,context_entity_id,entity_type,source_project_id,source_entity_id,source_version) VALUES('CATE','project','CATE','model','CATS','CATS.M','1.0.0');`)
	emptyModel, err := source.Detail(ctx, auth.Draft(), "CATE", "model", "CATS.M")
	if err != nil {
		t.Fatal(err)
	}
	_, emptyForm, err := example.ProjectRead(ctx, emptyModel, 2, "en", nil)
	if err != nil || len(emptyForm.Sections) != 0 {
		t.Fatalf("borrowed owner composition: %+v %v", emptyForm, err)
	}
	stale, err := source.Detail(ctx, auth.Release("2.0.0"), "CATP", "example", "CATP.ZERO")
	if err != nil {
		t.Fatal(err)
	}
	staleRecord, staleSchema, err := example.ProjectRead(ctx, stale, 2, "en", nil)
	if err != nil || len(staleSchema.UnplacedValues) != 1 || *staleRecord.Values[0].ValuePayload.StringValue != "Legacy scalar" {
		t.Fatalf("stale saved value: %+v %v", staleSchema, err)
	}
	store := weave.NewPostgresStore(pool)
	latest := publication.NewReader(pool)
	router := chi.NewRouter()
	// Both schema reads and mutation validation use this same source.
	mutationBoundary, err := example.NewPostgresMutations(pool, source)
	if err != nil {
		t.Fatal(err)
	}
	vocabulary.Mount(router, vocabulary.Host{Service: vocabulary.NewService(pool, nil, store), Projects: store.Projects(), Inventory: source, LatestRelease: latest})
	router.Route("/projects/{projectID}/examples", func(r chi.Router) {
		r.Use(auth.RequireProjectRead(store.Projects()), auth.WithProjectVersionContext, auth.ResolveContentVersion(latest))
		example.Mount(r, example.Host{Service: example.NewService(mutationBoundary), Projects: store.Projects(), Browser: source})
	})
	// A valid draft read can be read-only independently of release selection.
	router.Route("/draft/projects/{projectID}/examples", func(r chi.Router) {
		r.Use(auth.RequireProjectRead(store.Projects()), auth.WithProjectVersionContext)
		example.Mount(r, example.Host{Service: example.NewService(mutationBoundary), Projects: store.Projects(), Browser: source})
	})
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		return w
	}
	paths := []string{"/api/v2/projects/CATP/concept-lists?version=2.0.0", "/api/v2/projects/CATP/concept-lists/filters/vocabularies?version=2.0.0", "/projects/CATP/examples?version=2.0.0", "/projects/CATP/examples/form-schema?mode=edit&example_id=CATP.E&version=2.0.0", "/projects/CATP/examples/CATP.E?version=2.0.0"}
	before := map[string]string{}
	for _, path := range paths {
		w := get(path)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		before[path] = w.Body.String()
	}
	if !strings.Contains(before[paths[2]], `"status":"valid"`) {
		t.Fatal("example authored status was rewritten")
	}
	type schemaRead struct {
		Kind     string                       `json:"kind"`
		Example  *domain.Example              `json:"example"`
		Endpoint *formschema.SchemaEndpoint   `json:"endpoint"`
		Sections []example.ExampleFormSection `json:"sections"`
	}
	var releasedSchema schemaRead
	if err := json.Unmarshal([]byte(before[paths[3]]), &releasedSchema); err != nil {
		t.Fatal(err)
	}
	if releasedSchema.Endpoint != nil || releasedSchema.Example.Status != "valid" {
		t.Fatalf("release grants mutation or rewrites status: %+v", releasedSchema)
	}
	// The same shape serves authorized editing, explicit viewing and public draft reads.
	for _, tc := range []struct {
		mode, version string
		editable      bool
	}{
		{"edit", "", true}, {"view", "", false}, {"edit", "2.0.0", false},
	} {
		path := "/projects/CATP/examples/form-schema?mode=" + tc.mode + "&example_id=CATP.E"
		if tc.version != "" {
			path += "&version=" + tc.version
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil))
		if w.Code != 200 {
			t.Fatalf("form %s: %d %s", path, w.Code, w.Body.String())
		}
		var form schemaRead
		if err := json.Unmarshal(w.Body.Bytes(), &form); err != nil {
			t.Fatal(err)
		}
		if (form.Endpoint != nil) != tc.editable {
			t.Fatalf("capability %s: %+v", path, form.Endpoint)
		}
		if tc.editable && (form.Endpoint.Method != "PUT" || form.Endpoint.URL != "/projects/CATP/examples/CATP.E") {
			t.Fatalf("write endpoint: %+v", form.Endpoint)
		}
		form.Endpoint = nil
		form.Example = nil
		releasedCopy := releasedSchema
		releasedCopy.Example = nil
		if !reflect.DeepEqual(form, releasedCopy) && tc.version != "" {
			t.Fatalf("released owner shape changed")
		}
	}
	for _, tc := range []struct {
		path   string
		owner  bool
		status int
		method string
	}{
		{"/draft/projects/CATP/examples/form-schema?mode=edit&example_id=CATP.E", false, 200, ""},
		{"/projects/CATP/examples/form-schema?mode=create&target_type=model&target_id=CATS.M", true, 200, "POST"},
		{"/projects/CATP/examples/form-schema?mode=create&target_type=model&target_id=CATS.M&version=2.0.0", true, 403, ""},
		{"/draft/projects/CATP/examples/form-schema?mode=create&target_type=model&target_id=CATS.M", false, 403, ""},
	} {
		requestCtx := t.Context()
		if tc.owner {
			requestCtx = ctx
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequestWithContext(requestCtx, http.MethodGet, tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		if tc.status != 200 {
			continue
		}
		var form schemaRead
		if err := json.Unmarshal(w.Body.Bytes(), &form); err != nil {
			t.Fatal(err)
		}
		if tc.method == "" {
			if form.Endpoint != nil {
				t.Fatalf("read-only draft grants mutation: %+v", form.Endpoint)
			}
		} else if form.Endpoint == nil || form.Endpoint.Method != tc.method {
			t.Fatalf("create endpoint: %+v", form.Endpoint)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequestWithContext(ctx, method, "/projects/CATP/examples/CATP.E?version=2.0.0", nil))
		if w.Code != 405 {
			t.Fatalf("release mutation %s: %d %s", method, w.Code, w.Body.String())
		}
	}
	for _, accept := range []string{"text/html,application/xhtml+xml,*/*;q=0.8", "application/json", "*/*", ""} {
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, paths[4], nil)
		req.Header.Set("Accept", accept)
		router.ServeHTTP(w, req)
		if strings.HasPrefix(accept, "text/html") {
			if w.Code != 302 || w.Header().Get("Location") != "/projects/CATP?version=2.0.0#tab=examples&item=CATP.E" {
				t.Fatalf("browser redirect: %d %s", w.Code, w.Header().Get("Location"))
			}
		} else {
			var body example.ExampleRecord
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Example == nil || body.Example.ID != "CATP.E" {
				t.Fatalf("JSON contract: %s %v", w.Body.String(), err)
			}
		}
	}
	for _, path := range []string{"/api/v2/projects/CATP/concept-lists?version=9.0.0", "/projects/CATP/examples/MISSING?version=2.0.0"} {
		w := get(path)
		if w.Code != 404 || !strings.Contains(w.Body.String(), "not_in_release") {
			t.Fatalf("missing: %d %s", w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/api/v2/projects/CATP/concept-lists?per_page=0", "/projects/CATP/examples?page=1000000", "/projects/CATP/examples?sort_by=untrusted"} {
		if w := get(path); w.Code != 400 {
			t.Fatalf("invalid query: %d %s", w.Code, w.Body.String())
		}
	}
	runExampleWorkspaceBrowser(t, router)
	exec(`UPDATE weave_models SET ui_name='{"en":"Changed target"}' WHERE id='CATS.M';UPDATE weave_vocabularies SET ui_name='{"en":"Changed vocabulary"}' WHERE id='CATS.V';UPDATE weave_vocabulary_entries SET label='{"en":"Changed term"}' WHERE id='CATS.T';`)
	if _, err := releases.Create(ctx, "CATS", release.CreateInput{Version: "1.1.0"}); err != nil {
		t.Fatal(err)
	}
	draft, err := source.Inventory(ctx, auth.Draft(), "CATP", "concept_list", browserinput.InventoryQuery{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range draft.Items {
		if row.ID == "CATP.L" && row.VocabularyLabel != "Captured vocabulary" {
			t.Fatalf("draft ignored pin: %+v", row)
		}
		if strings.Contains(row.CurrentURL, "version=draft") {
			t.Fatal("draft URL names a nonexistent release")
		}
	}
	exec(`DELETE FROM weave_example_values WHERE example_id IN ('CATP.E','CATP.ZERO');DELETE FROM weave_examples WHERE project_id='CATP';DELETE FROM weave_override_refs WHERE override_id IN(91001,91003);DELETE FROM weave_field_overrides WHERE project_id='CATP';DELETE FROM weave_fields WHERE project_id='CATP';DELETE FROM weave_concept_list_entries WHERE concept_list_id='CATP.L';DELETE FROM weave_concept_lists WHERE project_id='CATP';`)
	for _, path := range paths {
		w := get(path)
		if w.Code != 200 || w.Body.String() != before[path] {
			t.Fatalf("release changed after hot mutation %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	// Public default release uses the same middleware as explicit reads, including API-v2 vocabulary routes.
	for _, pair := range [][2]string{{paths[0], "/api/v2/projects/CATP/concept-lists"}, {paths[2], "/projects/CATP/examples"}, {paths[3], "/projects/CATP/examples/form-schema?mode=edit&example_id=CATP.E"}} {
		if w := get(pair[1]); w.Code != 200 || w.Body.String() != before[pair[0]] {
			t.Fatalf("default selection: %d %s", w.Code, w.Body.String())
		}
	}
	again, err := source.Detail(ctx, auth.Release("2.0.0"), "CATP", "example", "CATP.E")
	if err != nil {
		t.Fatal(err)
	}
	after, afterSchema, err := example.ProjectRead(ctx, again, 2, "en", nil)
	if err != nil || !reflect.DeepEqual(record, after) || !reflect.DeepEqual(schema, afterSchema) {
		t.Fatalf("projection changed: %+v %v", after, err)
	}
	// Current source access remains mandatory even when labels are captured.
	exec(`UPDATE weave_projects SET visibility='private' WHERE id='CATS';`)
	if w := get(paths[0]); w.Code != 403 {
		t.Fatalf("source authorization: %d %s", w.Code, w.Body.String())
	}
	denied := get(paths[3])
	if denied.Code != 403 {
		t.Fatalf("view source authorization: %d %s", denied.Code, denied.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(denied.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	pool.Close()
	detached, detachedSchema, err := example.ProjectRead(ctx, again, 2, "en", nil)
	if err != nil || !reflect.DeepEqual(record, detached) || !reflect.DeepEqual(schema, detachedSchema) {
		t.Fatalf("detached projection: %+v %v", detached, err)
	}
}

func TestCatalogDraftSnapshotAndFacetBounds(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := t.Context()
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,slug,display_name) VALUES('CATBOWNER','cat-b-owner','Owner');INSERT INTO weave_projects(id,owner_id,visibility) VALUES('CATB','CATBOWNER','public');
 INSERT INTO weave_vocabularies(id,project_id,ui_name,connector_type) VALUES('CATB.V','CATB','{"en":"Before"}','local');
 INSERT INTO weave_concept_lists(id,project_id,ui_name,vocabulary_id) VALUES('CATB.L','CATB','{"en":"Before list"}','CATB.V');`)
	factory := readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead)
	if err := factory.Read(ctx, auth.Draft(), "CATB", func(s *readsession.Session) error {
		refs, err := s.CatalogReferences(ctx, "concept_list")
		if err != nil {
			return err
		}
		if len(refs) != 1 {
			t.Fatalf("references: %+v", refs)
		}
		exec(`UPDATE weave_vocabularies SET ui_name='{"en":"After"}' WHERE id='CATB.V';UPDATE weave_concept_lists SET ui_name='{"en":"After list"}' WHERE id='CATB.L';`)
		labels, err := s.CatalogLabels(ctx, "vocabulary", []string{"CATB.V"})
		if err != nil {
			return err
		}
		if labels["CATB.V"].Name != "Before" {
			t.Fatalf("mixed snapshot labels: %+v", labels)
		}
		page, err := s.CatalogPage(ctx, "concept_list", map[string]readsession.CatalogLabel{"vocabulary:CATB.V": labels["CATB.V"]}, browserinput.InventoryQuery{Page: 1, PerPage: 10})
		if err != nil {
			return err
		}
		if len(page.Items) != 1 || page.Items[0].UIName["en"] != "Before list" || page.Items[0].VocabularyLabel != "Before" {
			t.Fatalf("mixed snapshot rows: %+v", page)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	source, err := browserread.New(factory, projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Inventory(ctx, auth.ReadScope{}, "CATB", "concept_list", browserinput.InventoryQuery{Page: 1, PerPage: 1}); err == nil {
		t.Fatal("zero scope accepted")
	}
	exec(`INSERT INTO weave_vocabularies(id,project_id,connector_type) SELECT 'CATB.V'||n,'CATB','local' FROM generate_series(1,1000) n;INSERT INTO weave_concept_lists(id,project_id,vocabulary_id) SELECT 'CATB.L'||n,'CATB','CATB.V'||n FROM generate_series(1,1000) n;`)
	value, err := source.Inventory(ctx, auth.Draft(), "CATB", "concept_list", browserinput.InventoryQuery{Page: 1, PerPage: 1})
	if err == nil || len(value.Items) > 0 || value.Total != 0 {
		t.Fatalf("oversized facets returned partial data: %+v %v", value, err)
	}
}
