//go:build integration

package release

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

func TestProjectAssemblerOntologySelectionSharesSnapshotWithContent(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx, owner, project := seedZREL(t, pool)
	ctx = releaseAuthCtx(ctx, owner)
	store := ontology.NewPostgresStore(pool)
	for _, entry := range []struct{ id, prefix string }{{"assembly-defaults", "defaults"}, {"assembly-ontology", "assembly"}} {
		if _, err := store.CreateOntology(ctx, ontology.CreateOntologyInput{ID: entry.id, Prefix: entry.prefix, Name: entry.prefix, Namespace: "https://" + entry.prefix + ".example/", OntologyType: domain.OntologyTypeBase}); err != nil {
			t.Fatal(err)
		}
	}
	for _, version := range []string{"assembly-defaults-1", "assembly-defaults-2"} {
		if _, err := store.ImportVersion(ctx, ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: version, OntologyID: "assembly-defaults", VersionString: version, RDFContent: "<defaults/>"}, Classes: []ontology.ImportClass{{ID: version + "-class", URI: "https://defaults.example/E1", Prefix: "defaults", LocalName: "E1"}}, SetActive: version == "assembly-defaults-1"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ImportVersion(ctx, ontology.ImportVersionInput{
		Version:     ontology.CreateVersionInput{ID: "assembly-ontology-1", OntologyID: "assembly-ontology", VersionString: "1", RDFContent: "<ontology/>"},
		Publication: ontologyrelease.Options{Pins: map[string]string{"assembly-defaults": "assembly-defaults-1"}},
		Classes:     []ontology.ImportClass{{ID: "assembly-class", URI: "https://assembly.example/E1", Prefix: "assembly", LocalName: "E1"}},
		SetActive:   true,
	}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, pool, `INSERT INTO weave_actors(id,type,slug,display_name) VALUES ('link-author','person','link-author','Link author')`)
	mustExec(t, pool, `INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,is_primary,added_by_id,added_at) VALUES ('ZREL','assembly-ontology-1',true,'link-author',now())`)
	mustExec(t, pool, `UPDATE weave_fields SET ontology_scope='{"prefix":"assembly","local_name":"E1"}' WHERE project_id='ZREL'`)
	if _, err := NewService(NewPostgresStore(pool), pool, nil).Create(ctx, project, CreateInput{Version: "9.0.1"}); err != nil {
		t.Fatal(err)
	}
	opts := projectread.DefaultOptions()
	released, err := assembleFixture(ctx, t, pool, project, auth.Release("9.0.1"), opts)
	if err != nil {
		t.Fatal(err)
	}
	tr := &assemblyTrace{pids: map[uint32]bool{}}
	tr.mutate = func() {
		if err := store.SetActiveVersion(ctx, "assembly-defaults", "assembly-defaults-2"); err != nil {
			t.Fatal(err)
		}
	}
	cfg := pool.Config()
	cfg.MaxConns, cfg.MinConns = 1, 0
	cfg.ConnConfig.Tracer = tr
	one, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	value, err := assembleFixture(ctx, t, one, project, auth.Draft(), opts)
	if err != nil {
		t.Fatal(err)
	}
	id, err := value.Root()
	if err != nil {
		t.Fatal(err)
	}
	e, err := value.Evidence(id)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := e.References()
	if err != nil || len(refs) != 1 || refs[0].URI != "https://assembly.example/E1" {
		t.Fatalf("semantic input not resolved: %v", err)
	}
	pubs, err := e.Publications()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range pubs {
		d, err := a.Artifact.Decode()
		if err != nil {
			t.Fatal(err)
		}
		if d.OntologyID == "assembly-defaults" {
			found = true
			if d.VersionID != "assembly-defaults-1" {
				t.Fatal("ontology selection escaped snapshot")
			}
		}
	}
	if !tr.triggered || !found || len(tr.pids) != 1 {
		t.Fatal("did not exercise same-session ontology planning")
	}
	// Current policy now conflicts with the linked publication's fixed closure.
	if _, err := assembleFixture(ctx, t, pool, project, auth.Draft(), opts); err == nil {
		t.Fatal("draft silently resolved a conflicting defaults choice")
	}
	stable, err := assembleFixture(ctx, t, pool, project, auth.Release("9.0.1"), opts)
	if err != nil {
		t.Fatal(err)
	}
	rid, err := released.Root()
	if err != nil {
		t.Fatal(err)
	}
	historical, err := stable.Evidence(rid)
	if err != nil {
		t.Fatal(err)
	}
	actorID := "link-author"
	actor, err := historical.Descriptors().Actor(&actorID)
	if err != nil || actor.DisplayName != "Link author" {
		t.Fatalf("link provenance actor omitted: %v", err)
	}
	// A reference-free entity still inherits the whole project's defaults choice.
	mustExec(t, pool, `DELETE FROM weave_project_ontology_versions WHERE project_id='ZREL'`)
	mustExec(t, pool, `UPDATE weave_fields SET ontology_scope='{"prefix":"defaults","local_name":"E1"}' WHERE project_id='ZREL'`)
	mustExec(t, pool, `INSERT INTO weave_models(id,project_id) VALUES ('blank-model','ZREL')`)
	fragment, err := assembleEntityFixture(ctx, t, pool, project, auth.Draft(), "model", "blank-model", opts)
	if err != nil {
		t.Fatal(err)
	}
	consumer, _ := fragment.Consumer()
	content, _ := fragment.Content(consumer)
	if len(content.Fields) != 0 {
		t.Fatal("fragment loaded unrelated semantic fields")
	}
	evidence, err := fragment.Evidence(consumer)
	if err != nil {
		t.Fatal(err)
	}
	publications, err := evidence.Publications()
	if err != nil || len(publications) != 1 {
		t.Fatalf("fragment defaults policy lost: %v", err)
	}
	doc, err := publications[0].Artifact.Decode()
	if err != nil || doc.VersionID != "assembly-defaults-2" {
		t.Fatalf("fragment selected defaults from its subset: %v", err)
	}

}

func TestProjectOntologyLinkActorMigrationDoesNotInventHistory(t *testing.T) {
	ctx, pool, svc := seedLegacyRebuild(t, false)
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	if err := database.MigrateDown(db); err != nil {
		t.Fatal(err)
	}
	mustExec(t, pool, `INSERT INTO weave_actors(id,type,slug,display_name) VALUES ('legacy-link-author','person','author','Historical author')`)
	mustExec(t, pool, `UPDATE weave_project_ontology_versions SET added_by_id='legacy-link-author'`)
	plan, err := svc.PlanLegacyRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunLegacyRebuild(ctx, LegacyRebuildRequest{Plan: plan}, checkRebuild, finishRebuild); err != nil {
		t.Fatal(err)
	}
	var before, after string
	mustScan(t, pool, `SELECT document::text FROM weave_release_descriptors WHERE project_id=$1 AND version_number=$2`, []any{legacyProject, legacyVersion}, &before)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	mustScan(t, pool, `SELECT document::text FROM weave_release_descriptors WHERE project_id=$1 AND version_number=$2`, []any{legacyProject, legacyVersion}, &after)
	if before != after {
		t.Fatal("migration changed retained descriptor bytes")
	}
	value, err := assembleFixture(ctx, t, pool, legacyProject, auth.Release(legacyVersion), projectread.DefaultOptions())
	var unavailable interface{ IsNotInRelease() bool }
	if !errors.As(err, &unavailable) || !unavailable.IsNotInRelease() {
		t.Fatalf("historical actor absence did not fail closed: %v", err)
	}
	if _, err := value.Root(); err == nil {
		t.Fatal("missing historical actor produced complete value")
	}
}
