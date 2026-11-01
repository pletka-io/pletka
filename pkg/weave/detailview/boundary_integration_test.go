//go:build integration

package detailview

import (
	"context"
	"encoding/json"
	"html/template"
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
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/i18n/backend"
	"github.com/pletka-io/pletka/pkg/service/browserread"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/publication"
	"github.com/pletka-io/pletka/pkg/weave/release"
	"github.com/pletka-io/pletka/pkg/weave/templates"
)

type browserAccessOnly struct {
	domain.WeaveStore
	projects domain.WeaveProjectStore
}

func (s browserAccessOnly) Projects() domain.WeaveProjectStore { return s.projects }

func TestDetailBoundaryRetainsContentAndDoesNotReadLiveEntities(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := auth.WithPrincipal(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "BROWSEOWNER"})
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,type,display_name,slug,visibility) VALUES('BROWSEOWNER','organization','Original actor','browse-owner','public');
 INSERT INTO weave_projects(id,owner_id,status,visibility,ui_name) VALUES('BSRC','BROWSEOWNER','draft','public','{"en":"Original source"}'),('BROWSE','BROWSEOWNER','draft','public','{"en":"Original project"}');
 INSERT INTO weave_fields(id,project_id,path_elements,system_name,ui_name) VALUES('BSRC.F','BSRC','[]','foreign','{"en":"Original foreign field"}');
 INSERT INTO weave_models(id,project_id,ui_name) VALUES('BSRC.M','BSRC','{"en":"Inherited model"}');
 INSERT INTO weave_collections(id,project_id,ui_name) VALUES('BSRC.C','BSRC','{"en":"Inherited collection"}');
 INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id,display_name) VALUES('BSRC.F','BSRC','','','{"en":"Source base"}'),('BSRC.F','BSRC','model','BSRC.M','{"en":"Source model field"}'),('BSRC.F','BSRC','collection','BSRC.C','{"en":"Source collection field"}'); UPDATE weave_field_overrides SET set_value='source commitment' WHERE project_id='BSRC' AND entity_type='';`)
	releases := release.NewService(release.NewPostgresStore(pool), pool, nil)
	if _, err := releases.Create(ctx, "BSRC", release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES('BROWSE','BSRC','release','1.0.0');
 INSERT INTO weave_models(id,project_id,ui_name) VALUES('BROWSE.M','BROWSE','{"en":"Original model"}');
 INSERT INTO weave_categories(id,project_id,ui_name,canonical_order) VALUES('BROWSE.CAT','BROWSE','{"en":"Original category"}',1);
 INSERT INTO weave_collections(id,project_id,ui_name) VALUES('BROWSE.C','BROWSE','{"en":"Original collection"}'),('BROWSE.EMPTY','BROWSE','{"en":"Empty placement"}');
 INSERT INTO weave_fields(id,project_id,path_elements,ui_name) VALUES('BROWSE.F','BROWSE','[]','{"en":"Original field"}'),('BROWSE.H','BROWSE','[]','{"en":"Hidden field"}'),('BROWSE.N','BROWSE','[]','{"en":"No base override"}');
 INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id,display_name,category_id,part_of_collection_id,set_value,is_hidden,is_required,max_occurs) VALUES
 ('BROWSE.F','BROWSE','','','{"en":"Base"}','BROWSE.CAT',NULL,'base commitment',false,false,0),
 ('BROWSE.F','BROWSE','model','BROWSE.M','{"en":"Model override"}','BROWSE.CAT','BROWSE.C','context value',false,true,1),
 ('BROWSE.F','BROWSE','collection','BROWSE.C','{"en":"Collection override"}','BROWSE.CAT',NULL,NULL,false,false,0),
 ('BROWSE.H','BROWSE','model','BROWSE.M','{"en":"Hidden"}','BROWSE.CAT','BROWSE.C',NULL,true,false,0),
 ('BSRC.F','BROWSE','model','BROWSE.M','{"en":"Consumer foreign field"}','BROWSE.CAT',NULL,NULL,false,false,0),
 ('BSRC.F','BROWSE','collection','BSRC.C','{"en":"Consumer collection field"}',NULL,NULL,NULL,false,false,0);
 INSERT INTO weave_collection_placements(project_id,model_id,category_id,collection_id,min_occurs,max_occurs) VALUES('BROWSE','BROWSE.M','BROWSE.CAT','BROWSE.C',1,2),('BROWSE','BROWSE.M','BROWSE.CAT','BROWSE.EMPTY',0,NULL);
 INSERT INTO weave_adoptions(project_id,context_entity_type,context_entity_id,entity_type,source_project_id,source_entity_id,source_version,created_by_id) VALUES('BROWSE','model','BROWSE.M','field','BSRC','BSRC.F','1.0.0','BROWSEOWNER'),('BROWSE','project','BROWSE','model','BSRC','BSRC.M','1.0.0','BROWSEOWNER');
 INSERT INTO weave_vocabularies(id,project_id,connector_type,ui_name) VALUES('BROWSE.V','BROWSE','local','{"en":"Original vocabulary"}');
 INSERT INTO weave_vocabulary_entries(id,vocabulary_id,uri,label) VALUES('BROWSE.TERM','BROWSE.V','https://example.test/term','{"en":"Original term"}'),('BROWSE.PARENT','BROWSE.V','https://example.test/parent','{"en":"Original parent"}');
 INSERT INTO weave_concept_lists(id,project_id,ui_name,vocabulary_id,list_type,is_closed) VALUES('BROWSE.L','BROWSE','{"en":"Original list"}','BROWSE.V','BROWSE.PARENT',true),('BROWSE.EMPTYLIST','BROWSE','{"en":"Empty sealed list"}',NULL,NULL,true);
 INSERT INTO weave_concept_list_entries(id,concept_list_id,vocabulary_entry_id,position,custom_label) VALUES('BROWSE.LE','BROWSE.L','BROWSE.TERM',0,'{"en":"Custom term"}');`)
	if _, err := releases.Create(ctx, "BROWSE", release.CreateInput{Version: "9.0.0"}); err != nil {
		t.Fatal(err)
	}
	factory := readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead)
	source, err := browserread.New(factory, projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	i18nMgr, err := i18n.New(i18n.Config{DefaultLanguage: "en", FallbackLanguage: "en", Storage: backend.NewMemoryBackend(), Languages: []i18n.Language{{Code: "en", Name: "English", EnglishName: "English", Direction: "ltr", Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := templates.NewRenderer(func(...string) template.HTML { return "" }, i18nMgr, templates.AnalyticsConfig{})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	if err := Mount(router, Host{Weave: browserAccessOnly{projects: weave.NewPostgresStore(pool).Projects()}, Source: source, I18n: i18nMgr, Renderer: renderer, Publication: publication.NewReader(pool), HasFormat: func(generators.Format) bool { return false }}); err != nil {
		t.Fatal(err)
	}
	request := func(path string, admin bool) *httptest.ResponseRecorder {
		t.Helper()
		c := t.Context()
		if admin {
			c = ctx
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequestWithContext(c, http.MethodGet, path, nil))
		return rr
	}
	paths := []string{
		"/projects/BROWSE/entity-view/model/BROWSE.M",
		"/projects/BROWSE/entity-view/collection/BROWSE.C",
		"/projects/BROWSE/entity-view/field/BROWSE.F",
		"/projects/BROWSE/entity-view/field/BROWSE.N",
		"/projects/BROWSE/entity-view/concept-list/BROWSE.L",
		"/projects/BROWSE/entity-view/model/BSRC.M",
		"/projects/BROWSE/entity-view/collection/BSRC.C",
		"/projects/BROWSE/entity-view/model/BROWSE.M/stats",
		"/projects/BROWSE/entity-view/field/BROWSE.F/stats",
		"/projects/BROWSE/models/BROWSE.M",
		"/projects/BROWSE/entity-view/field/BSRC.F",
		"/projects/BROWSE/entity-view/concept-list/BROWSE.EMPTYLIST",
	}
	before := map[string]string{}
	for _, path := range paths {
		rr := request(path+"?version=9.0.0", true)
		if rr.Code != 200 {
			t.Fatalf("%s: %d %s", path, rr.Code, rr.Body.String())
		}
		before[path] = rr.Body.String()
	}
	var model struct {
		Capabilities ViewCapabilities `json:"capabilities"`
		Adoptions    []AdoptionItem   `json:"adoptions"`
	}
	if err := json.Unmarshal([]byte(before[paths[0]]), &model); err != nil {
		t.Fatal(err)
	}
	if model.Capabilities.Editable || model.Capabilities.ReuseURL != "" || len(model.Adoptions) != 1 || model.Adoptions[0].CreatedBy.Label != "Original actor" || !strings.Contains(model.Adoptions[0].SourceURL, "/projects/BSRC/fields/BSRC.F?version=1.0.0") {
		t.Fatalf("lost selected provenance/capabilities: %+v", model)
	}
	if !strings.Contains(before[paths[0]], "base commitment") || strings.Contains(before[paths[0]], "Hidden field") || !strings.Contains(before[paths[0]], "Empty placement") || !strings.Contains(before[paths[0]], "Consumer foreign field") {
		t.Fatal("composition lost commitments, visibility or empty placements")
	}
	if strings.Contains(before[paths[5]], "Source model field") || !strings.Contains(before[paths[6]], "Consumer collection field") {
		t.Fatal("foreign content silently switched composition context")
	}
	if !strings.Contains(before[paths[4]], "Original term") || !strings.Contains(before[paths[4]], "Custom term") || !strings.Contains(before[paths[4]], "Original parent") || !strings.Contains(before[paths[4]], "Original vocabulary") {
		t.Fatal("concept list lost retained metadata")
	}
	if !strings.Contains(before[paths[9]], "Original project") {
		t.Fatal("page did not use selected project title")
	}
	if !strings.Contains(before[paths[5]], "/projects/BSRC/models/BSRC.M?version=1.0.0") || strings.Contains(before[paths[10]], "source commitment") || !strings.Contains(before[paths[11]], `"is_closed":true`) {
		t.Fatal("source link, missing consumer base or empty closed-list semantics drifted")
	}
	var modelStats domain.ModelViewStats
	if err := json.Unmarshal([]byte(before[paths[7]]), &modelStats); err != nil {
		t.Fatal(err)
	}
	if modelStats.TotalFields != 2 || modelStats.TotalCollections != 2 || modelStats.RequiredFields != 1 {
		t.Fatalf("visible composition stats: %+v", modelStats)
	}
	var counts domain.ModelViewStats
	if err := json.Unmarshal([]byte(before[paths[8]]), &counts); err != nil {
		t.Fatal(err)
	}
	if counts.ModelsUsing != 1 || counts.CollectionsUsing != 1 || counts.FieldOverrideCount != 2 {
		t.Fatalf("field usage: %+v", counts)
	}
	input, err := source.Detail(ctx, auth.Release("9.0.0"), "BROWSE", "model", "BROWSE.M")
	if err != nil {
		t.Fatal(err)
	}
	projected, err := projectDetail(input)
	if err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE weave_actors SET display_name='Changed actor' WHERE id='BROWSEOWNER'; UPDATE weave_projects SET ui_name='{"en":"Changed project"}' WHERE id='BROWSE'; UPDATE weave_fields SET ui_name='{"en":"Changed foreign field"}' WHERE id='BSRC.F';`)
	if _, err := releases.Create(ctx, "BSRC", release.CreateInput{Version: "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE weave_project_inheritance SET source_version='2.0.0' WHERE project_id='BROWSE';
 DELETE FROM weave_adoptions WHERE project_id='BROWSE'; DELETE FROM weave_collection_placements WHERE project_id='BROWSE'; DELETE FROM weave_field_overrides WHERE project_id IN ('BROWSE','BSRC');
 DELETE FROM weave_models WHERE project_id IN ('BROWSE','BSRC'); DELETE FROM weave_collections WHERE project_id IN ('BROWSE','BSRC'); DELETE FROM weave_fields WHERE project_id IN ('BROWSE','BSRC'); DELETE FROM weave_categories WHERE project_id='BROWSE';
 DELETE FROM weave_concept_list_entries WHERE concept_list_id='BROWSE.L'; DELETE FROM weave_concept_lists WHERE project_id='BROWSE'; DELETE FROM weave_vocabulary_entries WHERE vocabulary_id='BROWSE.V'; DELETE FROM weave_vocabularies WHERE project_id='BROWSE';`)
	for _, path := range paths {
		rr := request(path+"?version=9.0.0", true)
		if rr.Code != 200 || rr.Body.String() != before[path] {
			t.Fatalf("%s drifted after live deletion: %d %s", path, rr.Code, rr.Body.String())
		}
	}
	for _, path := range []string{paths[0], paths[2], paths[4]} {
		rr := request(path, false)
		if rr.Code != 200 {
			t.Fatalf("default release: %d %s", rr.Code, rr.Body.String())
		}
	}
	for _, path := range []string{paths[0] + "?version=8.0.0", "/projects/BROWSE/entity-view/field/UNKNOWN?version=9.0.0"} {
		rr := request(path, true)
		if rr.Code != 404 || !strings.Contains(rr.Body.String(), "not_in_release") {
			t.Fatalf("missing content: %d %s", rr.Code, rr.Body.String())
		}
	}
	rr := request(paths[0]+"/reuse?version=9.0.0", true)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("historical global reuse read current state: %d", rr.Code)
	}
	exec(`UPDATE weave_projects SET visibility='private' WHERE id='BSRC';`)
	rr = request(paths[0]+"?version=9.0.0", false)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("private source leaked: %d %s", rr.Code, rr.Body.String())
	}
	pool.Close()
	after, err := projectDetail(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected, after) {
		t.Fatal("projection depends on current data or mutates retained input")
	}
}

func TestBrowserFieldCountsShareTheFragmentSnapshot(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true})
	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors(id,type,display_name,slug,visibility) VALUES('BSCOUNTOWNER','organization','Owner','bs-count-owner','public'); INSERT INTO weave_projects(id,owner_id) VALUES('BSCOUNT','BSCOUNTOWNER'); INSERT INTO weave_fields(id,project_id,path_elements) VALUES('BSCOUNT.F','BSCOUNT','[]'); INSERT INTO weave_models(id,project_id) VALUES('BSCOUNT.M','BSCOUNT');`); err != nil {
		t.Fatal(err)
	}
	a, err := projectread.New(projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	var input browserinput.Detail
	err = readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead).Read(ctx, auth.Draft(), "BSCOUNT", func(s *readsession.Session) error {
		var err error
		input.Fragment, err = a.Entity(ctx, s, "field", "BSCOUNT.F")
		if err != nil {
			return err
		}
		if _, err = pool.Exec(ctx, `INSERT INTO weave_field_overrides(project_id,field_id,entity_type,entity_id) VALUES('BSCOUNT','BSCOUNT.F','model','BSCOUNT.M')`); err != nil {
			return err
		}
		counts, err := s.FieldUsage(ctx, "BSCOUNT.F")
		input.FieldUsage = &counts
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := projectDetail(input)
	if err != nil {
		t.Fatal(err)
	}
	if view.Stats.ModelsUsing != 0 {
		t.Fatal("aggregate observed a different database snapshot")
	}
	source, err := browserread.New(readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	later, err := source.Detail(context.Background(), auth.Draft(), "BSCOUNT", "field", "BSCOUNT.F")
	if err != nil {
		t.Fatal(err)
	}
	if later.FieldUsage.ModelsUsing != 1 {
		t.Fatal("new selection did not see committed usage")
	}
}
