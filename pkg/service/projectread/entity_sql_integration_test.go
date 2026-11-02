//go:build integration

package projectread_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }

func discoveryFixture(t *testing.T, legacy bool) (*pgxpool.Pool, context.Context) {
	t.Helper()
	pool := testdb.EmptyPool(t)
	ctx := auth.WithPrincipal(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "DISCOWNER"})
	_, err := pool.Exec(ctx, `INSERT INTO weave_actors(id,type,slug,display_name) VALUES('DISCOWNER','organization','discovery-owner','Discovery owner');
INSERT INTO weave_projects(id,owner_id) VALUES('DISC','DISCOWNER'),('FOREIGN','DISCOWNER');
INSERT INTO weave_categories(id,project_id) VALUES('DISC.G','DISC');
INSERT INTO weave_models(id,project_id) VALUES('DISC.M','DISC'),('DISC.M2','DISC'),('FOREIGN.M','FOREIGN');
INSERT INTO weave_collections(id,project_id,default_category_id) VALUES('DISC.C','DISC','DISC.G'),('DISC.C2','DISC','DISC.G');
INSERT INTO weave_fields(id,project_id,path_elements) VALUES('DISC.F','DISC','[]'),('DISC.F2','DISC','[]');
INSERT INTO weave_vocabularies(id,project_id,connector_type) VALUES('DISC.V','DISC','local');
INSERT INTO weave_vocabulary_entries(id,vocabulary_id,uri,label) VALUES('DISC.E1','DISC.V','urn:one','{"en":"One"}'),('DISC.E2','DISC.V','urn:two','{"en":"Two"}');
INSERT INTO weave_concept_lists(id,project_id,vocabulary_id,list_type) VALUES('DISC.L','DISC','DISC.V','DISC.E1');
INSERT INTO weave_concept_list_entries(id,concept_list_id,vocabulary_entry_id,position) VALUES(900001,'DISC.L','DISC.E1',0),(900002,'DISC.L','DISC.E2',1);
INSERT INTO weave_concept_broader(id,concept_id,broader_id,scheme_id) VALUES('DISC.B','DISC.E1','DISC.E2','DISC.L');
INSERT INTO weave_collection_placements(project_id,model_id,category_id,collection_id) VALUES('DISC','DISC.M','DISC.G','DISC.C');
INSERT INTO weave_field_overrides(id,field_id,project_id,entity_type,entity_id,category_id,part_of_collection_id,set_value_entry_id) VALUES
(900001,'DISC.F','DISC','','','DISC.G','DISC.C2','DISC.E1'),
(900002,'DISC.F','DISC','model','DISC.M','DISC.G',NULL,NULL),
(900003,'DISC.F','DISC','collection','DISC.C',NULL,NULL,NULL),
(900004,'DISC.F2','DISC','field','DISC.F',NULL,NULL,NULL);
INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id,position) VALUES
(900001,'resource_model','DISC.M2','ignored',0),
(900001,'concept_list','DISC.L','ignored',0),
(900003,'collection_model','','DISC.C2',0),
(900004,'collection_model','DISC.C','ignored',0);
INSERT INTO weave_examples(id,project_id,entity_type,entity_id) VALUES('DISC.X','DISC','model','DISC.M'),('DISC.X2','DISC','collection','DISC.C2');
INSERT INTO weave_example_values(id,example_id,override_id,field_id,part_of_collection_id,value_kind,value_payload,linked_example_id,slot_path) VALUES
(900001,'DISC.X',900002,'DISC.F','DISC.C','example_ref','{}','DISC.X2','live'),
(900002,'DISC.X',999999,'missing-field',NULL,'string','{"value":"stale"}',NULL,'stale');`)
	if err != nil {
		t.Fatal(err)
	}
	service := release.NewService(release.NewPostgresStore(pool), pool, nil)
	if !legacy {
		if _, err := service.Create(ctx, "DISC", release.CreateInput{Version: "1.0.0"}); err != nil {
			t.Fatal(err)
		}
		return pool, ctx
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_releases(project_id,version,created_by_id) VALUES('DISC','1.0.0','DISCOWNER')`); err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanLegacyRebuild(ctx, []string{"DISC"})
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
	return pool, ctx
}

func TestSQLDiscoveryMatchesRowOracle(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy=%t", legacy), func(t *testing.T) {
			pool, ctx := discoveryFixture(t, legacy)
			roots := []struct{ kind, id string }{{"model", "DISC.M"}, {"collection", "DISC.C"}, {"field", "DISC.F"}, {"override", "900001"}, {"category", "DISC.G"}, {"concept_list", "DISC.L"}, {"vocabulary", "DISC.V"}, {"vocabulary_entry", "DISC.E1"}, {"example", "DISC.X"}}
			for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
				config := pool.Config()
				config.ConnConfig.RuntimeParams["plan_cache_mode"] = mode
				boundPool, err := pgxpool.NewWithConfig(ctx, config)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(boundPool.Close)
				factory := readsession.NewFactory(boundPool).WithSourceAuthorization(auth.CheckProjectRead)
				options := projectread.DefaultOptions()
				options.Limits.PageSize = 2
				assembler, err := projectread.New(options)
				if err != nil {
					t.Fatal(err)
				}
				for _, scope := range []auth.ReadScope{auth.Draft(), auth.Release("1.0.0")} {
					err := factory.Read(ctx, scope, "DISC", func(s *readsession.Session) error {
						for _, root := range roots {
							selection := projectsnapshot.Selection{root.kind: {root.id}}
							checkDiscoveryPageOracle(ctx, t, s, selection)
							want, err := projectread.EntityUsingRowsForTest(ctx, assembler, s, root.kind, root.id)
							if err != nil {
								return fmt.Errorf("row oracle %s: %w", root.kind, err)
							}
							got, err := assembler.Entity(ctx, s, root.kind, root.id)
							if err != nil {
								return fmt.Errorf("SQL discovery %s: %w", root.kind, err)
							}
							compareFragments(t, want, got)
						}
						checkDiscoveryPageOracle(ctx, t, s, projectsnapshot.Selection{})
						large := projectsnapshot.Selection{"override": {"900001", "900001", "900004"}}
						for i := range 2048 {
							large["override"] = append(large["override"], fmt.Sprint(-i-1))
						}
						checkDiscoveryPageOracle(ctx, t, s, large)
						checkDiscoveryPageOracle(ctx, t, s, projectsnapshot.Selection{"model": {"FOREIGN.M", "absent", "DISC.M", "DISC.M"}, "field": {"DISC.F"}, "override": {"900001", "900003"}, "vocabulary": {"DISC.V"}, "vocabulary_entry": {"DISC.E1"}})
						return nil
					})
					if err != nil {
						t.Fatalf("%s %s: %v", mode, scope.Version(), err)
					}
				}
			}
		})
	}
}

func checkDiscoveryPageOracle(ctx context.Context, t *testing.T, s *readsession.Session, selection projectsnapshot.Selection) {
	t.Helper()
	want, present, err := projectread.DiscoveryRowsForTest(ctx, s, selection)
	if err != nil {
		t.Fatal(err)
	}
	got, found := projectsnapshot.Selection{}, projectsnapshot.Selection{}
	for offset := int64(0); ; offset += 2 {
		rows, err := s.DiscoverIdentities(ctx, readsession.Page{Limit: 2, Offset: offset, Selection: selection})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if added, err := got.Include(row.Kind, row.ID); err != nil || !added {
				t.Fatalf("invalid or repeated identity: %+v: %v", row, err)
			}
			if row.Present {
				if _, err := found.Include(row.Kind, row.ID); err != nil {
					t.Fatal(err)
				}
			}
		}
		if len(rows) < 2 {
			break
		}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("reference parity: %s", diff)
	}
	if diff := cmp.Diff(present, found); diff != "" {
		t.Fatalf("body presence parity: %s", diff)
	}
}

func compareFragments(t *testing.T, want, got projectsnapshot.EntityFragment) {
	t.Helper()
	wantRoot, _ := want.Root()
	gotRoot, err := got.Root()
	if err != nil || wantRoot != gotRoot {
		t.Fatalf("fragment root changed: %v", err)
	}
	wantProjects, _ := want.Projects()
	gotProjects, err := got.Projects()
	if err != nil || !reflect.DeepEqual(wantProjects, gotProjects) {
		t.Fatalf("fragment source pins changed: %v", err)
	}
	for _, source := range wantProjects {
		wantContent, _ := want.Content(source)
		gotContent, err := got.Content(source)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(wantContent, gotContent); diff != "" {
			t.Fatal(diff)
		}
	}
}
