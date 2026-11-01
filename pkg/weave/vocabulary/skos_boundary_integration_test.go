//go:build integration

package vocabulary_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/service/browserread"
	"github.com/pletka-io/pletka/pkg/service/generatorread"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/skos"
	"github.com/pletka-io/pletka/pkg/weave/publication"
	"github.com/pletka-io/pletka/pkg/weave/release"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

func TestSKOSUsesRetainedHierarchyNamespaceAndSourcePins(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := auth.WithPrincipal(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "SKOWNER"})
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,type,slug,display_name,visibility) VALUES('SKOWNER','organization','sk-owner','SK owner','public');
INSERT INTO weave_projects(id,owner_id,visibility,concept_namespace) VALUES('SKS','SKOWNER','public','https://source.example/'),('SKP','SKOWNER','public','https://consumer.example/');
INSERT INTO weave_models(id,project_id) VALUES('SKS.M','SKS');
INSERT INTO weave_vocabularies(id,project_id,connector_type) VALUES('SKS.V','SKS','local');
INSERT INTO weave_vocabulary_entries(id,vocabulary_id,uri,label,scope_note) VALUES
('SKS.A','SKS.V','pletka:concept/A','{"en":"Crimson"}','{"en":"Recorded note"}'),
('SKS.B','SKS.V','pletka:concept/B','{"en":"Red"}',NULL),
('SKS.OUT','SKS.V','pletka:concept/OUT','{"en":"Not a member"}',NULL);
INSERT INTO weave_concept_lists(id,project_id,semantic_id,ui_name,vocabulary_id) VALUES('SKS.L','SKS','SKS.CL.1','{"en":"Recorded scheme"}','SKS.V');
INSERT INTO weave_concept_list_entries(id,concept_list_id,vocabulary_entry_id,position,custom_label) VALUES('SKS.LA','SKS.L','SKS.A',1,'{"en":"Curated crimson"}'),('SKS.LB','SKS.L','SKS.B',2,NULL);
INSERT INTO weave_concept_broader(id,concept_id,broader_id,scheme_id) VALUES('SKS.EDGE','SKS.A','SKS.B','SKS.L'),('SKS.OUTEDGE','SKS.B','SKS.OUT','SKS.L'),('SKS.GLOBAL','SKS.B','SKS.A',NULL);`)
	cuts := release.NewService(release.NewPostgresStore(pool), pool, nil)
	cut := func(project, version string) {
		t.Helper()
		if _, err := cuts.Create(ctx, project, release.CreateInput{Version: version}); err != nil {
			t.Fatalf("cut %s@%s: %v; fields=%v", project, version, err, apierror.FromError(err).Fields)
		}
	}
	cut("SKS", "1.0.0")
	exec(`INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES('SKP','SKS','release','1.0.0');
INSERT INTO weave_fields(id,project_id,expected_value_type,path_elements) VALUES('SKP.F','SKP','Concept','[]');
INSERT INTO weave_field_overrides(id,project_id,field_id,entity_type,entity_id) VALUES(48001,'SKP','SKP.F','','');
INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id) VALUES(48001,'concept_list','SKS.L','SKS.CL.1');
INSERT INTO weave_concept_lists(id,project_id,semantic_id,ui_name) VALUES('SKP.EMPTY','SKP','SKP.CL.1','{"en":"Empty retained scheme"}');
INSERT INTO weave_concept_lists(id,project_id,semantic_id,vocabulary_id) VALUES('SKP.COPY','SKP','SKP.CL.2','SKS.V');
INSERT INTO weave_concept_list_entries(id,concept_list_id,vocabulary_entry_id) VALUES('SKP.COPY.A','SKP.COPY','SKS.A');`)
	cut("SKP", "1.0.0")
	source, err := generatorread.New(readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	service := generators.NewService(source, nil)
	render := func(scope auth.ReadScope) []byte {
		t.Helper()
		var out bytes.Buffer
		if err := service.GenerateConceptListSKOS(ctx, scope, "SKP", "SKS.L", &out); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	before := render(auth.Release("1.0.0"))
	for _, want := range []string{"<https://source.example/scheme/SKS.CL.1>", `skos:prefLabel "Curated crimson"@en`, `skos:scopeNote "Recorded note"@en`, "skos:broader <https://source.example/concept/B>", "skos:narrower <https://source.example/concept/A>"} {
		if !bytes.Contains(before, []byte(want)) {
			t.Fatalf("missing %s: %s", want, before)
		}
	}
	if bytes.Contains(before, []byte("consumer.example")) || bytes.Contains(before, []byte("concept/OUT")) || bytes.Contains(before, []byte("skos:broader <https://source.example/concept/A>")) {
		t.Fatalf("wrong namespace or scheme membership: %s", before)
	}
	exec(`UPDATE weave_projects SET concept_namespace='https://changed.example/' WHERE id='SKS';
UPDATE weave_vocabulary_entries SET label='{"en":"Changed label"}',scope_note='{"en":"Changed note"}' WHERE id='SKS.A';
UPDATE weave_concept_list_entries SET custom_label='{"en":"Changed custom"}' WHERE id='SKS.LA';
DELETE FROM weave_concept_broader WHERE scheme_id='SKS.L';`)
	cut("SKS", "2.0.0")
	exec(`UPDATE weave_project_inheritance SET source_version='2.0.0' WHERE project_id='SKP';
INSERT INTO weave_concept_lists(id,project_id,semantic_id) VALUES('SKP.NEW','SKP','SKP.CL.NEW');`)
	if !bytes.Equal(before, render(auth.Release("1.0.0"))) {
		t.Fatal("release export followed source repin or hot edits")
	}
	if draft := render(auth.Draft()); !bytes.Contains(draft, []byte("Changed custom")) || !bytes.Contains(draft, []byte("changed.example")) || bytes.Contains(draft, []byte("skos:broader")) {
		t.Fatalf("draft did not use its selected source release: %s", draft)
	}
	// Remove the hot identity altogether. Released reads still resolve it.
	exec(`DELETE FROM weave_concept_lists WHERE id='SKS.L';`)
	if !bytes.Equal(before, render(auth.Release("1.0.0"))) {
		t.Fatal("release export required a hot identity")
	}
	store := weave.NewPostgresStore(pool)
	router := chi.NewRouter()
	vocabulary.Mount(router, vocabulary.Host{SKOS: service, Inventory: &browserread.Source{}, LatestRelease: publication.NewReader(pool), Service: vocabulary.NewService(pool, nil), Projects: store.Projects()})
	get := func(c context.Context, path string) *httptest.ResponseRecorder {
		t.Helper()
		out := httptest.NewRecorder()
		router.ServeHTTP(out, httptest.NewRequestWithContext(c, http.MethodGet, path, nil))
		return out
	}
	base := "/api/v2/projects/SKP/concept-lists/"
	for _, path := range []string{base + "SKS.L/skos?version=1.0.0", base + "SKS.L/skos"} {
		out := get(t.Context(), path)
		if out.Code != 200 || !bytes.Equal(out.Body.Bytes(), before) || !strings.HasPrefix(out.Header().Get("Content-Type"), "text/turtle") || !strings.Contains(out.Header().Get("Content-Disposition"), "SKS.L.ttl") {
			t.Fatalf("HTTP %s: %d %s", path, out.Code, out.Body.String())
		}
	}
	if out := get(t.Context(), base+"SKP.EMPTY/skos?version=1.0.0"); out.Code != 200 || !strings.Contains(out.Body.String(), "Empty retained scheme") || strings.Contains(out.Body.String(), " a skos:Concept ;") {
		t.Fatalf("empty list: %d %s", out.Code, out.Body.String())
	}
	// An owned list can select a vocabulary entry from a different pinned source.
	// Its own labels/memberships and namespace remain distinct from that source's list.
	if out := get(t.Context(), base+"SKP.COPY/skos?version=1.0.0"); out.Code != 200 || !strings.Contains(out.Body.String(), `skos:prefLabel "Crimson"@en`) || !strings.Contains(out.Body.String(), "https://consumer.example/concept/A") || strings.Contains(out.Body.String(), "Changed") || strings.Contains(out.Body.String(), "Curated") {
		t.Fatalf("selected foreign member: %d %s", out.Code, out.Body.String())
	}
	if out := get(t.Context(), base+"SKP.NEW/skos?version=1.0.0"); out.Code != 404 || !strings.Contains(out.Body.String(), `"code":"not_in_release"`) || strings.Contains(out.Body.String(), "@prefix") {
		t.Fatalf("missing release member: %d %s", out.Code, out.Body.String())
	}
	fragment, err := source.Entity(ctx, auth.Release("1.0.0"), "SKP", "concept_list", "SKS.L")
	if err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE weave_projects SET visibility='private' WHERE id='SKS';`)
	if out := get(t.Context(), base+"SKS.L/skos?version=1.0.0"); out.Code != 403 || strings.Contains(out.Body.String(), "@prefix") {
		t.Fatalf("source access: %d %s", out.Code, out.Body.String())
	}
	exec(`UPDATE weave_projects SET visibility='private' WHERE id='SKP';`)
	if out := get(t.Context(), base+"SKP.EMPTY/skos?version=1.0.0"); out.Code != 404 {
		t.Fatalf("root access: %d %s", out.Code, out.Body.String())
	}
	pool.Close()
	var detached bytes.Buffer
	if err := skos.RenderFragment(&detached, fragment); err != nil || !bytes.Equal(before, detached.Bytes()) {
		t.Fatalf("render after pool close: %v", err)
	}
}
