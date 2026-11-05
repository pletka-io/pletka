//go:build integration

package projectpage

import (
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/i18n/backend"
	"github.com/pletka-io/pletka/pkg/service/browserread"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/pages"
	"github.com/pletka-io/pletka/pkg/weave/publication"
	"github.com/pletka-io/pletka/pkg/weave/release"
	"github.com/pletka-io/pletka/pkg/weave/templates"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }

type summaryAccessOnly struct {
	domain.WeaveStore
	projects domain.WeaveProjectStore
}

func (s summaryAccessOnly) Projects() domain.WeaveProjectStore { return s.projects }

type summaryProjectAccess struct {
	domain.WeaveProjectStore
	current domain.WeaveProjectStore
}

func (s summaryProjectAccess) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	return s.current.GetByID(ctx, id)
}

func TestProjectSummaryRetainsReleasedContentAndPinnedOntology(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := auth.WithPrincipal(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "SUMOWNER"})
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,type,slug,display_name,visibility) VALUES('SUMOWNER','organization','sum-owner','Captured credit actor','public');
 INSERT INTO weave_projects(id,owner_id,visibility,ui_name,description,namespace) VALUES('SUMSRC','SUMOWNER','public','{"en":"Source"}','{}','https://sum.test/'),('SUM','SUMOWNER','public','{"en":"Captured project"}','{"en":"Captured description"}','https://sum.test/project/');`)
	onto := ontology.NewPostgresStore(pool)
	if _, err := onto.CreateOntology(ctx, ontology.CreateOntologyInput{ID: "SUMONT", Prefix: "sum", Name: "Captured ontology", Namespace: "https://sum.test/", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"1", "2"} {
		if _, err := onto.ImportVersion(ctx, ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: "SUMONT." + v, OntologyID: "SUMONT", VersionString: v, RDFContent: "<ontology/>", OntologyURI: "https://sum.test/"}, Classes: []ontology.ImportClass{{ID: "SUMCLASS." + v, URI: "https://sum.test/Thing", Prefix: "sum", LocalName: "Thing", Label: domain.Translations{"en": "Thing"}}}, NamespaceBindings: []ontology.NamespaceBinding{{Prefix: "sum", Namespace: "https://sum.test/"}}, SetActive: v == "1"}); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,is_primary,added_at) VALUES('SUMSRC','SUMONT.1',true,now());INSERT INTO weave_models(id,project_id) VALUES('SUMSRC.M','SUMSRC');INSERT INTO weave_fields(id,project_id,path_elements) VALUES('SUMSRC.F','SUMSRC','[]');`)
	releases := release.NewService(release.NewPostgresStore(pool), pool, nil)
	if _, err := releases.Create(ctx, "SUMSRC", release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES('SUM','SUMSRC','release','1.0.0');
 INSERT INTO weave_models(id,project_id,ui_name,ontology_scope) VALUES('SUM.M','SUM','{"en":"Captured model"}','{"prefix":"sum","local_name":"Thing","uri":"https://sum.test/Thing"}');
 INSERT INTO weave_collections(id,project_id) VALUES('SUM.C','SUM');
 INSERT INTO weave_categories(id,project_id) VALUES('SUM.CAT','SUM');
 INSERT INTO weave_fields(id,project_id,path_elements) VALUES('SUM.F','SUM','[]');
 INSERT INTO weave_concept_lists(id,project_id,ui_name) VALUES('SUM.L','SUM','{"en":"Captured list"}');
 INSERT INTO weave_project_attributions(project_id,actor_id,kind,position,note) VALUES('SUM','SUMOWNER','author',0,'Captured credit note');
INSERT INTO weave_examples(id,project_id,entity_type,entity_id) VALUES('SUM.E','SUM','model','SUM.M');
INSERT INTO weave_adoptions(project_id,context_entity_type,context_entity_id,entity_type,source_project_id,source_entity_id,source_version) VALUES('SUM','project','SUM','model','SUMSRC','SUMSRC.M','1.0.0'),('SUM','model','SUM.M','field','SUMSRC','SUMSRC.F','1.0.0');`)
	if _, err := releases.Create(ctx, "SUM", release.CreateInput{Version: "3.0.0"}); err != nil {
		t.Fatal(err)
	}
	factory := readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead)
	source, err := browserread.New(factory, projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	captured, err := source.Summary(ctx, auth.Release("3.0.0"), "SUM")
	if err != nil {
		t.Fatal(err)
	}
	if c := captured.Counts; c.Models != 1 || c.Fields != 1 || c.Collections != 1 || c.Categories != 1 || c.ConceptLists != 1 || c.Examples != 1 || c.Adoptions != 1 {
		t.Fatalf("wrong captured counts: %+v", c)
	}
	if len(captured.Ontologies) != 1 || captured.Ontologies[0].Version != "1" || captured.Ontologies[0].Classes != 1 || captured.Ontologies[0].ClassesUsed != 1 || captured.Ontologies[0].SourceVersion != "1.0.0" {
		t.Fatalf("wrong ontology summary: %+v", captured.Ontologies)
	}
	if _, err := source.Summary(ctx, auth.Draft(), "SUM"); err != nil {
		t.Fatal(err)
	}
	mgr, err := i18n.New(i18n.Config{DefaultLanguage: "en", FallbackLanguage: "en", Storage: backend.NewMemoryBackend(), Languages: []i18n.Language{{Code: "en", Name: "English", EnglishName: "English", Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := templates.NewRenderer(func(...string) template.HTML { return "" }, mgr, templates.AnalyticsConfig{})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	store := summaryAccessOnly{projects: summaryProjectAccess{current: weave.NewPostgresStore(pool).Projects()}}
	pub := publication.NewReader(pool)
	Mount(router, Host{Logger: slog.Default(), Weave: store, Source: source, Releases: releases, I18n: mgr, Publication: pub})
	if err := pages.Mount(router, pages.Host{Weave: store, Source: source, Renderer: renderer, I18n: mgr, LatestRelease: pub}); err != nil {
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
	paths := []string{"/projects/SUM/page-schema", "/projects/SUM/overview-schema", "/projects/SUM"}
	before := map[string]string{}
	for _, p := range paths {
		r := request(p+"?version=3.0.0", true)
		if r.Code != 200 {
			t.Fatalf("%s: %d %s", p, r.Code, r.Body.String())
		}
		before[p] = r.Body.String()
	}
	for _, needle := range []string{"Captured credit actor", "Captured credit note", "Captured ontology", "/projects/SUMSRC?version=1.0.0"} {
		if !strings.Contains(before[paths[1]], needle) {
			t.Fatalf("overview missing %s: %s", needle, before[paths[1]])
		}
	}
	if !strings.Contains(before[paths[2]], "SUM Captured project") {
		t.Fatal("shell did not use selected title")
	}
	exec(`UPDATE weave_projects SET ui_name='{"en":"Changed live project"}',description='{"en":"Changed description"}',namespace='https://changed.test/' WHERE id='SUM';
 UPDATE weave_actors SET display_name='Changed actor' WHERE id='SUMOWNER';
 DELETE FROM weave_project_attributions WHERE project_id='SUM';
 DELETE FROM weave_examples WHERE project_id='SUM';DELETE FROM weave_models WHERE project_id='SUM';DELETE FROM weave_fields WHERE project_id='SUM';DELETE FROM weave_collections WHERE project_id='SUM';DELETE FROM weave_categories WHERE project_id='SUM';DELETE FROM weave_concept_lists WHERE project_id='SUM';
 UPDATE weave_ontologies SET name='Changed live ontology' WHERE id='SUMONT';
 UPDATE weave_project_ontology_versions SET ontology_version_id='SUMONT.2' WHERE project_id='SUMSRC';`)
	if _, err := releases.Create(ctx, "SUMSRC", release.CreateInput{Version: "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE weave_project_inheritance SET source_version='2.0.0' WHERE project_id='SUM';UPDATE weave_adoptions SET source_version='2.0.0' WHERE project_id='SUM';`)
	for _, p := range paths {
		r := request(p+"?version=3.0.0", true)
		if r.Code != 200 || r.Body.String() != before[p] {
			t.Fatalf("released %s changed: %d %s", p, r.Code, r.Body.String())
		}
	}
	draft, err := source.Summary(ctx, auth.Draft(), "SUM")
	if err != nil {
		t.Fatal(err)
	}
	if draft.Counts.Models != 0 || len(draft.Credits) != 0 || draft.Ontologies[0].Version != "2" {
		t.Fatalf("working summary did not advance: %+v", draft)
	}
	for _, p := range paths {
		r := request(p, false)
		if r.Code != 200 || strings.Contains(r.Body.String(), "Changed live project") {
			t.Fatalf("public default did not retain release: %s %d", p, r.Code)
		}
	}
	r := request(paths[1]+"?version=missing", true)
	if r.Code != 404 || !strings.Contains(r.Body.String(), "not_in_release") {
		t.Fatalf("missing release: %d %s", r.Code, r.Body.String())
	}
	exec(`UPDATE weave_projects SET visibility='private' WHERE id='SUMSRC';`)
	if r := request(paths[1]+"?version=3.0.0", false); r.Code != 403 {
		t.Fatalf("private source: %d %s", r.Code, r.Body.String())
	}
	exec(`UPDATE weave_projects SET visibility='private' WHERE id='SUM';`)
	for _, p := range paths {
		if r := request(p+"?version=3.0.0", false); r.Code != 404 {
			t.Fatalf("private root: %s %d", p, r.Code)
		}
	}
	first, err := projectOverview(captured, "en", nil)
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	after, err := projectOverview(captured, "en", nil)
	if err != nil || !reflect.DeepEqual(first, after) {
		t.Fatalf("projection needs live database: %v", err)
	}
}

func TestProjectSummaryFacetsShareOneSnapshot(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors(id,type,display_name,slug) VALUES('RR','organization','Before','rr');INSERT INTO weave_projects(id,owner_id,ui_name) VALUES('RR','RR','{"en":"Before"}');INSERT INTO weave_project_attributions(project_id,actor_id,kind,position) VALUES('RR','RR','author',0)`); err != nil {
		t.Fatal(err)
	}
	err := readsession.NewFactory(pool).Read(ctx, auth.Draft(), "RR", func(s *readsession.Session) error {
		h, err := s.ProjectHeader(ctx)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `INSERT INTO weave_models(id,project_id) VALUES('RR.M','RR');UPDATE weave_actors SET display_name='After' WHERE id='RR'`); err != nil {
			return err
		}
		counts, err := s.ProjectCounts(ctx)
		if err != nil {
			return err
		}
		credits, err := s.ProjectCredits(ctx)
		if err != nil {
			return err
		}
		if counts.Models != 0 || len(credits) != 1 || credits[0].ActorName != "Before" || !strings.Contains(string(h.Record.UIName), "Before") {
			t.Fatalf("mixed snapshots: %+v %+v", counts, credits)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := browserread.New(readsession.NewFactory(pool), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := source.Summary(ctx, auth.Draft(), "RR")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Counts.Models != 1 || fresh.Credits[0].ActorName != "After" {
		t.Fatal("new snapshot did not advance")
	}
	encoded, err := json.Marshal(fresh)
	if err != nil || len(encoded) == 0 {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_project_attributions(project_id,actor_id,kind,position) SELECT 'RR','RR','funder',n FROM generate_series(1,1001)n`); err != nil {
		t.Fatal(err)
	}
	partial, err := source.Summary(ctx, auth.Draft(), "RR")
	if err == nil || partial.Header.Record.ID != "" {
		t.Fatal("oversized credits returned a partial successful summary")
	}
}

func TestLegacySummaryPreservesEvidenceWithoutInventingUsage(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := auth.WithPrincipal(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "LEGOWNER"})
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,type,slug,display_name) VALUES('LEGOWNER','organization','legacy','Legacy actor');
 INSERT INTO weave_projects(id,owner_id,ui_name) VALUES('LEG','LEGOWNER','{"en":"Legacy project"}');
 INSERT INTO weave_fields(id,project_id,path_elements) VALUES('LEG.F','LEG','[{"position":0,"type":"class","uri":"https://legacy.test/Thing","prefix":"leg","local_name":"Thing"}]');
 INSERT INTO weave_releases(project_id,version,created_by_id) VALUES('LEG','1.0.0','LEGOWNER');`)
	onto := ontology.NewPostgresStore(pool)
	if _, err := onto.CreateOntology(ctx, ontology.CreateOntologyInput{ID: "LEGONT", Prefix: "leg", Name: "Legacy ontology", Namespace: "https://legacy.test/", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"1", "2"} {
		if _, err := onto.StageLegacyVersion(ctx, ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: "LEGONT." + v, OntologyID: "LEGONT", VersionString: v, RDFContent: "<legacy/>"}, Classes: []ontology.ImportClass{{ID: "LEGCLASS." + v, URI: "https://legacy.test/Thing", Prefix: "leg", LocalName: "Thing", Label: domain.Translations{"en": "Thing"}}}, NamespaceBindings: []ontology.NamespaceBinding{{Prefix: "leg", Namespace: "https://legacy.test/"}}}); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,is_primary,added_at) VALUES('LEG','LEGONT.1',true,now()),('LEG','LEGONT.2',false,now());`)
	testdb.ArchiveDraft(t, pool, "LEG", "1.0.0")
	service := release.NewService(release.NewPostgresStore(pool), pool, nil)
	plan, err := service.PlanLegacyRebuild(ctx, []string{"LEG"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RunLegacyRebuild(ctx, release.LegacyRebuildRequest{Plan: plan}, func(context.Context, string, bool) error { return nil }, func(_ context.Context, op release.RebuildOperation) (map[string]string, error) {
		commits := map[string]string{}
		for _, r := range op.Releases {
			commits[r.ProjectID] = strings.Repeat("a", 40)
		}
		return commits, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := browserread.New(readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	value, err := source.Summary(ctx, auth.Release("1.0.0"), "LEG")
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Ontologies) != 2 || !value.Ontologies[0].UsageUnavailable || !value.Ontologies[1].UsageUnavailable {
		t.Fatalf("legacy ambiguity became known usage: %+v", value.Ontologies)
	}
	exec(`INSERT INTO weave_projects(id,owner_id) VALUES('LEGCHILD','LEGOWNER');
 INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES('LEGCHILD','LEG','release','1.0.0');
 INSERT INTO weave_fields(id,project_id,path_elements) VALUES('LEGCHILD.F','LEGCHILD','[{"position":0,"type":"class","uri":"https://legacy.test/Thing","prefix":"leg","local_name":"Thing","ontology_version_id":"LEGONT.1"}]');`)
	child, err := source.Summary(ctx, auth.Draft(), "LEGCHILD")
	if err != nil {
		t.Fatal(err)
	}
	if len(child.Ontologies) != 2 || child.Ontologies[0].SourceVersion != "1.0.0" || child.Ontologies[0].ClassesUsed != 1 || child.Ontologies[1].ClassesUsed != 0 {
		t.Fatalf("draft usage lost exact legacy source/version: %+v", child.Ontologies)
	}
	exec(`DELETE FROM weave_fields WHERE project_id='LEG';DELETE FROM weave_project_ontology_versions WHERE project_id='LEG';UPDATE weave_ontologies SET name='Changed legacy catalog' WHERE id='LEGONT';`)
	after, err := source.Summary(ctx, auth.Release("1.0.0"), "LEG")
	if err != nil || !reflect.DeepEqual(value, after) {
		t.Fatalf("legacy summary drift: %v", err)
	}
	exec(`INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,is_primary,added_at) VALUES('LEG','LEGONT.1',true,now());`)
	draft, err := source.Summary(ctx, auth.Draft(), "LEG")
	if err != nil || len(draft.Ontologies) != 1 || draft.Ontologies[0].Name != "Legacy ontology" {
		t.Fatalf("draft legacy body rejected: %+v %v", draft, err)
	}
}
