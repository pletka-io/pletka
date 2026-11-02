//go:build integration

package projectpage

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
	"github.com/pletka-io/pletka/pkg/service/browserread"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestLegacySummarySeparatesLocalAndSourceContexts(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := auth.WithPrincipal(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "CTXOWNER"})
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,type,slug,display_name) VALUES('CTXOWNER','organization','context-owner','Context owner');
 INSERT INTO weave_projects(id,owner_id) VALUES('CTXROOT','CTXOWNER'),('CTXSOURCE','CTXOWNER');
 INSERT INTO weave_releases(project_id,version,created_by_id) VALUES('CTXROOT','1.0.0','CTXOWNER'),('CTXSOURCE','1.0.0','CTXOWNER');
 INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES('CTXROOT','CTXSOURCE','release','1.0.0');
 INSERT INTO weave_fields(id,project_id,path_elements) VALUES
 ('CTXROOT.F','CTXROOT','[{"position":0,"type":"class","prefix":"ctx","local_name":"Local","uri":"https://context.test/Local","ontology_version_id":"CTXONT.1"},{"position":1,"type":"class","prefix":"ctx","local_name":"Local","uri":"https://context.test/Local","ontology_version_id":"CTXONT.1"}]'),
 ('CTXSOURCE.F','CTXSOURCE','[{"position":0,"type":"class","prefix":"ctx","local_name":"Source","uri":"https://context.test/Source","ontology_version_id":"CTXONT.2"}]');`)
	store := ontology.NewPostgresStore(pool)
	if _, err := store.CreateOntology(ctx, ontology.CreateOntologyInput{ID: "CTXONT", Prefix: "ctx", Name: "Retained context", Namespace: "https://context.test/", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	for i, names := range [][]string{{"Local"}, {"Source", "Extra"}} {
		version := fmt.Sprintf("CTXONT.%d", i+1)
		classes := make([]ontology.ImportClass, 0, len(names))
		for _, name := range names {
			classes = append(classes, ontology.ImportClass{ID: version + "." + name, URI: "https://context.test/" + name, Prefix: "ctx", LocalName: name})
		}
		if _, err := store.StageLegacyVersion(ctx, ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: version, OntologyID: "CTXONT", VersionString: fmt.Sprint(i + 1), RDFContent: strings.Repeat("<retained>ä🙂</retained>", 16384)}, Classes: classes}); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,added_at) VALUES('CTXROOT','CTXONT.1',now()),('CTXSOURCE','CTXONT.2',now());`)
	service := release.NewService(release.NewPostgresStore(pool), pool, nil)
	plan, err := service.PlanLegacyRebuild(ctx, []string{"CTXROOT", "CTXSOURCE"})
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
	beforeArtifacts := readLegacyArtifacts(ctx, t, pool)
	var same bool
	var sources int
	if err := pool.QueryRow(ctx, `SELECT context_document::jsonb=(document::jsonb||jsonb_build_object('sources','[]'::jsonb)),jsonb_array_length(document::jsonb->'sources') FROM weave_legacy_ontology_snapshots WHERE project_id='CTXROOT' AND version_number='1.0.0'`).Scan(&same, &sources); err != nil || !same || sources != 1 {
		t.Fatalf("fixture must contain equal local evidence and a separate embedded source: %t %d %v", same, sources, err)
	}
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		t.Run(mode, func(t *testing.T) {
			config := pool.Config()
			config.ConnConfig.RuntimeParams["plan_cache_mode"] = mode
			selected, err := pgxpool.NewWithConfig(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			defer selected.Close()
			checkLocalSummaryContexts(ctx, t, selected)
		})
	}
	source, err := browserread.New(readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	before, err := source.Summary(ctx, auth.Release("1.0.0"), "CTXROOT")
	if err != nil || len(before.Ontologies) != 2 {
		t.Fatalf("combined source summary: %+v %v", before.Ontologies, err)
	}
	for _, o := range before.Ontologies {
		want := 0
		if o.VersionID == "CTXONT.1" {
			want = 1
		}
		if o.ClassesUsed != want || o.UsageUnavailable {
			t.Fatalf("source references leaked into root usage: %+v", o)
		}
	}
	exec(`DELETE FROM weave_fields WHERE project_id IN ('CTXROOT','CTXSOURCE');DELETE FROM weave_project_ontology_versions WHERE project_id IN ('CTXROOT','CTXSOURCE');UPDATE weave_ontologies SET name='Changed live name' WHERE id='CTXONT';`)
	after, err := source.Summary(ctx, auth.Release("1.0.0"), "CTXROOT")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("retained context changed after live deletion: %v", err)
	}
	if !cmp.Equal(beforeArtifacts, readLegacyArtifacts(ctx, t, pool)) {
		t.Fatal("artifact bytes changed after hot mutation/deletion")
	}
}

func readLegacyArtifacts(ctx context.Context, t *testing.T, pool *pgxpool.Pool) map[string]ontologyevidence.Artifact {
	t.Helper()
	factory := readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead)
	artifacts := make(map[string]ontologyevidence.Artifact, 2)
	for _, project := range []string{"CTXROOT", "CTXSOURCE"} {
		var expected ontologyevidence.Artifact
		if err := pool.QueryRow(ctx, `SELECT document,digest FROM weave_legacy_ontology_snapshots WHERE project_id=$1 AND version_number='1.0.0'`, project).Scan(&expected.JSON, &expected.Digest); err != nil {
			t.Fatal(err)
		}
		if len(expected.JSON) <= 256<<10 {
			t.Fatal("fixture must cross transport chunk boundaries")
		}
		var closed *readsession.Session
		err := factory.Read(ctx, auth.Release("1.0.0"), project, func(s *readsession.Session) error {
			closed = s
			got, err := s.LegacyOntology(ctx)
			if err != nil {
				return err
			}
			if got != expected {
				t.Fatal("streamed artifact differs from exact archived bytes")
			}
			artifacts[project] = got
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			if _, err := s.LegacyInterpretation(canceled); !errors.Is(err, context.Canceled) {
				t.Fatalf("cached interpretation ignored cancellation: %v", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := closed.LegacyInterpretation(ctx); err == nil {
			t.Fatal("closed session returned cached interpretation")
		}
	}
	return artifacts
}

func checkLocalSummaryContexts(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	factory := readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead)
	versions := []string{"CTXONT.1", "CTXONT.2"}
	for i, project := range []string{"CTXROOT", "CTXSOURCE"} {
		err := factory.Read(ctx, auth.Release("1.0.0"), project, func(s *readsession.Session) error {
			if _, err := s.ProjectHeader(ctx); err != nil {
				return err
			}
			own, err := s.OwnOntologySummaries(ctx)
			if err != nil || len(own) != 1 || own[0].VersionID != versions[i] || own[0].Classes != i+1 {
				t.Fatalf("wrong local inventory for %s: %+v %v", project, own, err)
			}
			usage, err := s.OntologyUsages(ctx, versions)
			if err != nil || usage[versions[i]].Classes != 1 || usage[versions[1-i]].Classes != 0 || !usage[versions[i]].Known {
				t.Fatalf("wrong local usage for %s: %+v %v", project, usage, err)
			}
			matches, err := s.MatchOntologyReferences(ctx, versions, []byte(`[{"kind":"class","uri":"https://context.test/Local"}]`))
			if err != nil || matches[versions[0]].Classes != 1-i || matches[versions[1]].Classes != 0 {
				t.Fatalf("cross-context reference matching for %s: %+v %v", project, matches, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
