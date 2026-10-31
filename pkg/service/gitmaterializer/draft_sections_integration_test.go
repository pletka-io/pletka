//go:build integration

package gitmaterializer_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/txscope"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestCapturedDraftAllSectionsRepinAndCascadeDeletion(t *testing.T) {
	f := newCapturedFixture(t)
	ctx := t.Context()
	descriptorExec(t, f.pool, `INSERT INTO weave_projects(id,owner_id,status,visibility) VALUES('CGS','CG_OWNER','draft','public'); INSERT INTO weave_fields(id,project_id,path_elements) VALUES('CGS.F','CGS','[]')`)
	authorized := auth.WithPrincipal(auth.WithSnapshot(ctx, &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "CG_OWNER"})
	releases := release.NewService(release.NewPostgresStore(f.pool), f.pool, nil)
	if _, err := releases.Create(authorized, "CGS", release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	ont := ontology.NewPostgresStore(f.pool)
	if _, err := ont.CreateOntology(ctx, ontology.CreateOntologyInput{ID: "CGO", Prefix: "cgo", Name: "Captured ontology", Namespace: "https://cgo.example/", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	if _, err := ont.ImportVersion(ctx, ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: "CGO.1", OntologyID: "CGO", VersionString: "1", RDFContent: "<cgo/>"}, SetActive: true}); err != nil {
		t.Fatal(err)
	}
	f.init(t)
	req := f.request
	req.Message = "Populate all owned sections"
	full, err := f.capture.Mutate(ctx, req, func(bound context.Context) error {
		for _, sql := range []string{
			`UPDATE weave_projects SET system_name='all sections' WHERE id='CG'`,
			`INSERT INTO weave_models(id,project_id) VALUES('CGM','CG')`,
			`INSERT INTO weave_fields(id,project_id,path_elements,ui_name) VALUES('CGF','CG','[]','null')`,
			`INSERT INTO weave_categories(id,project_id) VALUES('CGC','CG')`,
			`INSERT INTO weave_collections(id,project_id,default_category_id) VALUES('CGCOL','CG','CGC')`,
			`INSERT INTO weave_collection_placements(project_id,model_id,collection_id,category_id) VALUES('CG','CGM','CGCOL','CGC')`,
			`INSERT INTO weave_vocabularies(id,project_id,connector_type) VALUES('CGV','CG','local')`,
			`INSERT INTO weave_vocabulary_entries(id,vocabulary_id,uri,label) VALUES('CGE1','CGV','urn:cg:1','{"en":"One"}'),('CGE2','CGV','urn:cg:2','{"en":"Two"}')`,
			`INSERT INTO weave_concept_lists(id,project_id,vocabulary_id) VALUES('CGL','CG','CGV')`,
			`INSERT INTO weave_concept_list_entries(id,concept_list_id,vocabulary_entry_id) VALUES('CGLE','CGL','CGE1')`,
			`INSERT INTO weave_concept_broader(id,concept_id,broader_id,scheme_id) VALUES('CGB','CGE1','CGE2','CGL')`,
			`INSERT INTO weave_field_overrides(field_id,entity_type,entity_id,project_id) VALUES('CGF','model','CGM','CG')`,
			`INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id,position) SELECT id,'concept_list','CGL','CGL',0 FROM weave_field_overrides WHERE project_id='CG'`,
			`INSERT INTO weave_examples(id,project_id,entity_type,entity_id,title,status) VALUES('CGX','CG','model','CGM','{"en":"Example"}','valid')`,
			`INSERT INTO weave_example_values(example_id,field_id,override_id,value_kind,number_value,date_value,value_payload) SELECT 'CGX','CGF',id,'integer',9007199254740993.123456789,'infinity','null' FROM weave_field_overrides WHERE project_id='CG'`,
			`INSERT INTO weave_adoptions(project_id,context_entity_type,context_entity_id,entity_type,source_project_id,source_entity_id,source_version) VALUES('CG','project','CG','field','CGS','CGS.F','1.0.0')`,
			`INSERT INTO weave_entity_forks(project_id,entity_type,fork_entity_id,source_project_id,source_entity_id,source_version) VALUES('CG','field','CGF','CGS','CGS.F','1.0.0')`,
			`INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES('CG','CGS','release','1.0.0')`,
			`INSERT INTO weave_namespace_bindings(id,project_id,prefix,namespace,created_at,updated_at) VALUES('CGNS','CG','cg','https://cg.example/',now(),now())`,
			`INSERT INTO weave_project_actors(project_id,actor_id,role) VALUES('CG','CG_OWNER','editor')`,
			`INSERT INTO weave_project_attributions(project_id,actor_id,kind) VALUES('CG','CG_OWNER','author')`,
			`INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,is_primary,added_by_id,added_at) VALUES('CG','CGO.1',true,'CG_OWNER',now())`,
		} {
			if _, err := txscope.New(f.pool).Exec(bound, sql); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := full.After().Root()
	if err != nil {
		t.Fatal(err)
	}
	content, err := full.After().Content(root)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	var sections map[string][]json.RawMessage
	if err := json.Unmarshal(body, &sections); err != nil {
		t.Fatal(err)
	}
	for _, section := range projectsnapshot.Coverage() {
		if len(sections[section.Name]) == 0 {
			t.Fatalf("test omits section %s", section.Name)
		}
	}
	// Change the source working copy and cut a different immutable release before
	// either consumer operation reaches Git. The first consumer must retain v1.
	descriptorExec(t, f.pool, `UPDATE weave_fields SET system_name='source v2' WHERE id='CGS.F'`)
	if _, err := releases.Create(authorized, "CGS", release.CreateInput{Version: "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	req.Message = "Repin the concrete source"
	repinned, err := f.capture.Mutate(ctx, req, func(bound context.Context) error {
		for _, sql := range []string{
			`UPDATE weave_project_inheritance SET source_version='2.0.0' WHERE project_id='CG'`,
			`UPDATE weave_adoptions SET source_version='2.0.0' WHERE project_id='CG'`,
			`UPDATE weave_entity_forks SET source_version='2.0.0' WHERE project_id='CG'`,
		} {
			if _, err := txscope.New(f.pool).Exec(bound, sql); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.mat.Reconcile(ctx, "CG"); err != nil {
		t.Fatal(err)
	}
	tip := gitOut(t, f.repo, "rev-parse", gitmaterializer.CapturedDraftRef)
	requireCapturedTree(t, f.repo, tip+"^", full.After())
	requireCapturedTree(t, f.repo, tip, repinned.After())
	req.Message = "Cut complete release"
	cut, err := f.capture.Mutate(authorized, req, func(bound context.Context) error {
		_, err := releases.Create(bound, "CG", release.CreateInput{Version: "1.0.0"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.mat.ProcessPending(ctx, 10); err != nil {
		t.Fatal(err)
	}
	gitOut(t, f.repo, "rev-parse", "refs/tags/v1.0.0")
	requireCapturedTree(t, f.repo, gitmaterializer.CapturedDraftRef, cut.After())
	req.Message = "Delete owned children and references"
	deleted, err := f.capture.Mutate(ctx, req, func(bound context.Context) error {
		for _, sql := range []string{
			`DELETE FROM weave_examples WHERE project_id='CG'`,
			`DELETE FROM weave_field_overrides WHERE project_id='CG'`,
			`DELETE FROM weave_collection_placements WHERE project_id='CG'`,
			`DELETE FROM weave_concept_broader WHERE scheme_id='CGL'`,
			`DELETE FROM weave_concept_lists WHERE project_id='CG'`,
			`DELETE FROM weave_vocabularies WHERE project_id='CG'`,
			`DELETE FROM weave_adoptions WHERE project_id='CG'`,
			`DELETE FROM weave_entity_forks WHERE project_id='CG'`,
			`DELETE FROM weave_project_inheritance WHERE project_id='CG'`,
			`DELETE FROM weave_project_ontology_versions WHERE project_id='CG'`,
			`DELETE FROM weave_namespace_bindings WHERE project_id='CG'`,
			`DELETE FROM weave_project_attributions WHERE project_id='CG'`,
			`DELETE FROM weave_project_actors WHERE project_id='CG'`,
			`DELETE FROM weave_collections WHERE project_id='CG'`,
			`DELETE FROM weave_categories WHERE project_id='CG'`,
			`DELETE FROM weave_models WHERE project_id='CG'`,
			`DELETE FROM weave_fields WHERE project_id='CG'`,
			`UPDATE weave_projects SET system_name='children deleted' WHERE id='CG'`,
		} {
			if _, err := txscope.New(f.pool).Exec(bound, sql); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.mat.Reconcile(ctx, "CG"); err != nil {
		t.Fatal(err)
	}
	requireCapturedTree(t, f.repo, gitmaterializer.CapturedDraftRef, deleted.After())
	content, err = deleted.After().Content(root)
	if err != nil {
		t.Fatal(err)
	}
	body, err = json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &sections); err != nil {
		t.Fatal(err)
	}
	for _, section := range projectsnapshot.Coverage() {
		if section.Name != "weave_projects_archive" && len(sections[section.Name]) != 0 {
			t.Fatalf("deleted section retained %s", section.Name)
		}
	}
}
