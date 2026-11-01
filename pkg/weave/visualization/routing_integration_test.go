//go:build integration

package visualization_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/service/generatorread"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/mermaid"
	"github.com/pletka-io/pletka/pkg/weave/publication"
	"github.com/pletka-io/pletka/pkg/weave/release"
	"github.com/pletka-io/pletka/pkg/weave/visualization"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }

// Only authorization can reach the current project store. Any attempted live
// entity lookup panics, including before deletion while such rows still exist.
type currentProjectAccess struct {
	domain.WeaveStore
	projects domain.WeaveProjectStore
}

func (s currentProjectAccess) Projects() domain.WeaveProjectStore { return s.projects }

func TestReleaseRoutesSurviveDeletedRootsAndPreserveConsumerPins(t *testing.T) {
	pool := testdb.EmptyPool(t)
	admin := auth.WithPrincipal(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "ROUTEOWNER"})
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(admin, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,type,display_name,slug,visibility) VALUES('ROUTEOWNER','organization','Route owner','route-owner','public');
 INSERT INTO weave_projects(id,owner_id,status,visibility) VALUES('ROUTESRC','ROUTEOWNER','draft','public'),('ROUTE','ROUTEOWNER','draft','public'),('OUTSIDE','ROUTEOWNER','draft','public');
 INSERT INTO weave_fields(id,project_id,path_elements,system_name,ui_name) VALUES('ROUTESRC.F','ROUTESRC','[]','original_source','{"en":"Source original"}');
 INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id,display_name) VALUES('ROUTESRC.F','ROUTESRC','','','{"en":"Source default"}');`)
	releases := release.NewService(release.NewPostgresStore(pool), pool, nil)
	if _, err := releases.Create(admin, "ROUTESRC", release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES('ROUTE','ROUTESRC','release','1.0.0');
 INSERT INTO weave_models(id,project_id,system_name,ui_name) VALUES('ROUTE.M','ROUTE','original_model','{"en":"Original model"}'),('OUTSIDE.M','OUTSIDE','outside','{"en":"Outside"}');
 INSERT INTO weave_collections(id,project_id,system_name,ui_name) VALUES('ROUTE.C','ROUTE','original_collection','{"en":"Original collection"}');
 INSERT INTO weave_fields(id,project_id,path_elements,system_name,ui_name) VALUES('ROUTE.F','ROUTE','[]','original_field','{"en":"Original field"}');
 INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id,display_name) VALUES
 ('ROUTE.F','ROUTE','','','{"en":"Base"}'),
 ('ROUTE.F','ROUTE','collection','ROUTE.C','{"en":"Collection field"}'),
 ('ROUTE.F','ROUTE','model','ROUTE.M','{"en":"Model field"}'),
 ('ROUTESRC.F','ROUTE','','','{"en":"Consumer base"}'),
 ('ROUTESRC.F','ROUTE','model','ROUTE.M','{"en":"Consumer field"}');`)
	if _, err := releases.Create(admin, "ROUTE", release.CreateInput{Version: "9.0.0"}); err != nil {
		t.Fatal(err)
	}
	source, err := generatorread.New(readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generators.NewRegistry(mermaid.NewRenderer())
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	visualization.Mount(router, visualization.Host{
		Weave:         currentProjectAccess{projects: weave.NewPostgresStore(pool).Projects()},
		Generators:    generators.NewService(source, registry),
		LatestRelease: publication.NewReader(pool),
	})
	request := func(path string, privileged bool) *httptest.ResponseRecorder {
		t.Helper()
		ctx := t.Context()
		if privileged {
			ctx = admin
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil))
		return rr
	}
	paths := []string{
		"/projects/ROUTE/gen/models/ROUTE.M/diagram",
		"/projects/ROUTE/gen/collections/ROUTE.C/diagram",
		"/projects/ROUTE/gen/fields/ROUTE.F/diagram",
		"/projects/ROUTE/gen/fields/ROUTESRC.F/diagram",
		"/projects/ROUTE/gen/models/ROUTE.M/snapshot",
	}
	before := map[string]string{}
	for _, path := range paths {
		rr := request(path+"?version=9.0.0", true)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, rr.Code, rr.Body.String())
		}
		before[path] = rr.Body.String()
	}
	var snapshot generators.Snapshot
	if err := json.Unmarshal([]byte(before[paths[len(paths)-1]]), &snapshot); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, field := range snapshot.Fields {
		if field.Field.ID == "ROUTESRC.F" {
			found = field.Field.SourceVersion == "1.0.0" && field.Field.DisplayName["en"] == "Consumer field"
		}
	}
	if !found {
		t.Fatal("consumer composition lost captured source or override")
	}
	exec(`UPDATE weave_fields SET ui_name='{"en":"Changed source"}' WHERE id='ROUTESRC.F';`)
	if _, err := releases.Create(admin, "ROUTESRC", release.CreateInput{Version: "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE weave_project_inheritance SET source_version='2.0.0' WHERE project_id='ROUTE';
 DELETE FROM weave_field_overrides WHERE project_id IN ('ROUTE','ROUTESRC');
 DELETE FROM weave_models WHERE id='ROUTE.M';
 DELETE FROM weave_collections WHERE id='ROUTE.C';
 DELETE FROM weave_fields WHERE id IN ('ROUTE.F','ROUTESRC.F');
 INSERT INTO weave_models(id,project_id,system_name) VALUES('ROUTE.NEW','ROUTE','unreleased');`)
	for _, path := range paths {
		rr := request(path+"?version=9.0.0", true)
		if rr.Code != http.StatusOK || rr.Body.String() != before[path] {
			t.Fatalf("%s changed after deleting live roots/repinning source: %d %s", path, rr.Code, rr.Body.String())
		}
		// Public readers without an explicit version still default to the
		// consumer's latest release. Snapshot diagnostics are admin-only.
		if !strings.HasSuffix(path, "/snapshot") {
			rr = request(path, false)
			if rr.Code != http.StatusOK || rr.Body.String() != before[path] {
				t.Fatalf("default selection: %s: %d %s", path, rr.Code, rr.Body.String())
			}
		}
	}
	for _, path := range []string{
		"/projects/ROUTE/gen/models/ROUTE.NEW/diagram?version=9.0.0",
		"/projects/ROUTE/gen/models/OUTSIDE.M/diagram?version=9.0.0",
		"/projects/ROUTE/gen/models/ROUTE.M/diagram?version=8.0.0",
	} {
		rr := request(path, true)
		if rr.Code != http.StatusNotFound || !strings.Contains(rr.Body.String(), "not_in_release") {
			t.Fatalf("unexpected membership failure: %d %s", rr.Code, rr.Body.String())
		}
	}
	// Archived public visibility is not an access grant, for either the
	// selected consumer or its captured source project.
	exec(`UPDATE weave_projects SET visibility='private' WHERE id='ROUTESRC';`)
	rr := request(paths[3]+"?version=9.0.0", false)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("private source leaked: %d %s", rr.Code, rr.Body.String())
	}
	exec(`UPDATE weave_projects SET visibility='public' WHERE id='ROUTESRC'; UPDATE weave_projects SET visibility='private' WHERE id='ROUTE';`)
	rr = request(paths[0]+"?version=9.0.0", false)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("private consumer leaked: %d %s", rr.Code, rr.Body.String())
	}
}
