//go:build integration

package gitmaterializer_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/weave/apierror"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/pkg/database"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/database/releasedescriptor"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func descriptorExec(t *testing.T, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func descriptorContext(t *testing.T) context.Context {
	return auth.WithPrincipal(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "descriptor-publisher"})
}

func descriptorCut(t *testing.T, pool *pgxpool.Pool, project, version, host string) {
	t.Helper()
	_, err := release.NewService(release.NewPostgresStore(pool), pool, nil, release.WithModuleHosts(host, host+"-ontologies")).Create(descriptorContext(t), project, release.CreateInput{Version: version})
	if err != nil {
		t.Fatal(err)
	}
}

func descriptorTree(t *testing.T, pool *pgxpool.Pool, project, version, host string) map[string]string {
	t.Helper()
	dir := t.TempDir()
	mat := gitmaterializer.NewMaterializer(pool, t.TempDir(), nil).WithModuleHosts(host, host+"-ontologies")
	if err := mat.BuildReleaseTree(t.Context(), dir, project, version); err != nil {
		t.Fatal(err)
	}
	if _, err := gitmaterializer.LoadProjectSnapshot(dir); err != nil {
		t.Fatalf("load serialized release: %v", err)
	}
	out := map[string]string{}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func descriptorFixture(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.EmptyPool(t)
	for _, id := range []string{"owner", "creator", "publisher", "adopter", "forker", "credit"} {
		descriptorExec(t, pool, `INSERT INTO weave_actors(id,type,slug,display_name) VALUES ($1,'organization',$2,$2)`, "descriptor-"+id, id)
	}
	for _, project := range []string{"descriptor-parent", "descriptor-child"} {
		descriptorExec(t, pool, `INSERT INTO weave_projects(id,owner_id,created_by_id) VALUES ($1,'descriptor-owner','descriptor-creator')`, project)
		descriptorExec(t, pool, `INSERT INTO weave_fields(id,project_id,system_name,status) VALUES ($1||'-field',$1,'field','draft')`, project)
		descriptorExec(t, pool, `INSERT INTO weave_collections(id,project_id,system_name,status) VALUES ($1||'-collection',$1,'collection','draft')`, project)
	}
	return pool
}

func TestReleaseDescriptorsFreezeActorAndModuleOutput(t *testing.T) {
	pool := descriptorFixture(t)
	store := ontology.NewPostgresStore(pool)
	owner := ontology.GenerateOntologyID("descriptor")
	version := ontology.GenerateVersionID("descriptor", "1")
	if _, err := store.CreateOntology(t.Context(), ontology.CreateOntologyInput{ID: owner, Prefix: "descriptor", Name: "Frozen Ontology", Namespace: "https://descriptor.example/", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ImportVersion(t.Context(), ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: version, OntologyID: owner, VersionString: "1", RDFContent: "<source/>"}}); err != nil {
		t.Fatal(err)
	}
	descriptorExec(t, pool, `INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,added_at) VALUES ('descriptor-parent',$1,now())`, version)
	descriptorCut(t, pool, "descriptor-parent", "1.0.0", "source.example")
	descriptorExec(t, pool, `INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES ('descriptor-child','descriptor-parent','release','1.0.0')`)
	descriptorExec(t, pool, `INSERT INTO weave_adoptions(project_id,context_entity_type,context_entity_id,entity_type,source_project_id,source_entity_id,source_version,created_by_id) VALUES ('descriptor-child','collection','descriptor-child-collection','field','descriptor-parent','descriptor-parent-field','1.0.0','descriptor-adopter')`)
	descriptorExec(t, pool, `INSERT INTO weave_entity_forks(project_id,entity_type,fork_entity_id,source_project_id,source_entity_id,source_version,created_by_id) VALUES ('descriptor-child','field','descriptor-child-field','descriptor-parent','descriptor-parent-field','1.0.0','descriptor-forker')`)
	descriptorExec(t, pool, `INSERT INTO weave_project_attributions(project_id,actor_id,kind) VALUES ('descriptor-child','descriptor-credit','author')`)
	descriptorExec(t, pool, `INSERT INTO weave_project_actors(project_id,actor_id,role) VALUES ('descriptor-child','descriptor-credit','contributor')`)
	descriptorCut(t, pool, "descriptor-child", "1.0.0", "child.example")
	before := descriptorTree(t, pool, "descriptor-child", "1.0.0", "unrelated.example")
	if !strings.Contains(before["pletka.mod"], "child.example/orgs/owner/projects/descriptor-child") || !strings.Contains(before["pletka.mod"], "source.example/orgs/owner/projects/descriptor-parent") {
		t.Fatal("module identities do not come from their respective cuts")
	}
	if !strings.Contains(before["pletka.mod"], "source.example-ontologies/frozen-ontology") {
		t.Fatal("source ontology module identity was not retained")
	}
	descriptorExec(t, pool, `UPDATE weave_actors SET slug=slug||'-changed',display_name='Changed',type='person' WHERE id LIKE 'descriptor-%'`)
	after := descriptorTree(t, pool, "descriptor-child", "1.0.0", "changed.example")
	if !reflect.DeepEqual(before, after) {
		for file, data := range before {
			if after[file] != data {
				t.Errorf("released file changed: %s", file)
			}
		}
		t.Fatal("release tree changed after actor/host mutations")
	}
	var escaped *readsession.Session
	err := readsession.NewFactory(pool).Read(t.Context(), auth.Release("1.0.0"), "descriptor-child", func(s *readsession.Session) error {
		escaped = s
		d, err := s.Descriptors(t.Context())
		if err != nil {
			return err
		}
		for _, suffix := range []string{"owner", "creator", "publisher", "adopter", "forker", "credit"} {
			id := "descriptor-" + suffix
			a, err := d.Actor(&id)
			if err != nil || a.DisplayName != suffix {
				t.Fatalf("captured %s: %+v %v", id, a, err)
			}
		}
		if a, err := d.Actor(nil); err != nil || a != nil {
			t.Fatal("optional actor became a value")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := escaped.Descriptors(t.Context()); err == nil {
		t.Fatal("closed reader remained usable")
	}
	for _, query := range []string{
		`UPDATE weave_release_descriptors SET document=document WHERE project_id='descriptor-child'`,
		`DELETE FROM weave_release_descriptors WHERE project_id='descriptor-child'`,
		`TRUNCATE weave_release_descriptors`,
	} {
		if _, err := pool.Exec(t.Context(), query); err == nil {
			t.Fatalf("raw mutation succeeded: %s", query)
		}
	}
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version > 30; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "cannot downgrade captured semantic references") {
		t.Fatalf("downgrade discarded descriptors: %v", err)
	}
	descriptorExec(t, pool, `DELETE FROM weave_releases WHERE project_id='descriptor-child'`)
	var remaining int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM weave_release_descriptors WHERE project_id='descriptor-child'`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("release cleanup: %d %v", remaining, err)
	}
}

func descriptorPending(t *testing.T, pool *pgxpool.Pool, project, version string) (pgx.Tx, *sqlcgen.Queries) {
	t.Helper()
	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.WithoutCancel(t.Context())) })
	if _, err := tx.Exec(t.Context(), `INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ($1,$2,'descriptor-publisher')`, project, version); err != nil {
		t.Fatal(err)
	}
	q := sqlcgen.New(tx)
	if _, err := q.WeaveSnapshotOwnedProject(t.Context(), sqlcgen.WeaveSnapshotOwnedProjectParams{ProjectID: project, Version: version}); err != nil {
		t.Fatal(err)
	}
	return tx, q
}

func descriptorFinish(t *testing.T, tx pgx.Tx, q *sqlcgen.Queries, project, version string) {
	t.Helper()
	if err := q.WeaveMarkReleaseProjectDependencies(t.Context(), sqlcgen.WeaveMarkReleaseProjectDependenciesParams{ProjectID: project, Version: version}); err != nil {
		t.Fatal(err)
	}
	if err := ontologyrelease.CaptureProject(t.Context(), q, project, version); err != nil {
		t.Fatal(err)
	}
	if err := q.WeaveMarkReleaseArchiveOwnership(t.Context(), sqlcgen.WeaveMarkReleaseArchiveOwnershipParams{ProjectID: project, Version: version}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseDescriptorsRejectUncapturedHistoryAndRollback(t *testing.T) {
	pool := descriptorFixture(t)
	const project = "descriptor-parent"
	// Reproduce a historical cut with verified bodies/pins but no descriptors.
	tx, q := descriptorPending(t, pool, project, "0.9.0")
	descriptorFinish(t, tx, q, project, "0.9.0")
	factory := readsession.NewFactory(pool)
	err := factory.Read(t.Context(), auth.Release("0.9.0"), project, func(s *readsession.Session) error { _, err := s.Descriptors(t.Context()); return err })
	var missing *releasedescriptor.UnavailableError
	if !errors.As(err, &missing) || apierror.FromError(err).Code != apierror.CodeNotInRelease {
		t.Fatalf("historical descriptor error: %v", err)
	}
	err = gitmaterializer.NewMaterializer(pool, t.TempDir(), nil).BuildReleaseTree(t.Context(), t.TempDir(), project, "0.9.0")
	if !errors.As(err, &missing) {
		t.Fatalf("historical tree guessed descriptors: %v", err)
	}
	err = factory.Read(t.Context(), auth.Draft(), project, func(s *readsession.Session) error { _, err := s.Descriptors(t.Context()); return err })
	if err == nil {
		t.Fatal("draft session read release descriptors")
	}
	tx, err = pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	if err := releasedescriptor.Capture(t.Context(), sqlcgen.New(tx), project, "0.9.0", releasedescriptor.Hosts{}); err == nil {
		t.Error("historical descriptors were backfilled")
	}
	_ = tx.Rollback(t.Context())
	descriptorExec(t, pool, `INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES ('descriptor-child','descriptor-parent','release','0.9.0')`)
	_, err = release.NewService(release.NewPostgresStore(pool), pool, nil).Create(descriptorContext(t), "descriptor-child", release.CreateInput{Version: "1.0.0"})
	var invalid *release.ErrValidation
	if !errors.As(err, &invalid) || len(invalid.Fields["descriptors"]) == 0 {
		t.Fatalf("uncaptured source was accepted: %v", err)
	}
	for _, query := range []string{
		`SELECT count(*) FROM weave_releases WHERE project_id='descriptor-child'`,
		`SELECT count(*) FROM weave_projects_archive WHERE id='descriptor-child'`,
		`SELECT count(*) FROM weave_fields_archive WHERE project_id='descriptor-child'`,
		`SELECT count(*) FROM weave_change_set WHERE project_id='descriptor-child'`,
		`SELECT count(*) FROM weave_release_project_dependencies WHERE project_id='descriptor-child'`,
	} {
		var count int
		if err := pool.QueryRow(t.Context(), query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("failed capture left state behind (%d): %s %v", count, query, err)
		}
	}
}

func TestReleaseDescriptorsCaptureOneSnapshotAndValidateStorage(t *testing.T) {
	pool := descriptorFixture(t)
	const project = "descriptor-parent"
	tx, q := descriptorPending(t, pool, project, "1.0.0")
	// The transaction's snapshot already exists. A concurrent actor edit must
	// not mix its new name with the older archived project/actor selection.
	descriptorExec(t, pool, `UPDATE weave_actors SET slug='later',display_name='Later' WHERE id='descriptor-owner'`)
	if _, err := tx.Exec(t.Context(), "SAVEPOINT invalid_descriptor"); err != nil {
		t.Fatal(err)
	}
	err := q.WeaveInsertReleaseDescriptors(t.Context(), sqlcgen.WeaveInsertReleaseDescriptorsParams{ProjectID: project, Version: "1.0.0", Document: []byte(`{"format":1,"project_id":"descriptor-parent","version":"1.0.0","actors":{},"project_modules":{"descriptor-parent":"fake"},"ontology_modules":{}}`)})
	if err == nil {
		t.Fatal("storage accepted omitted required actors")
	}
	if _, err := tx.Exec(t.Context(), "ROLLBACK TO SAVEPOINT invalid_descriptor"); err != nil {
		t.Fatal(err)
	}
	if err := releasedescriptor.Capture(t.Context(), q, project, "1.0.0", releasedescriptor.Hosts{Project: "captured.example"}); err != nil {
		t.Fatal(err)
	}
	descriptorFinish(t, tx, q, project, "1.0.0")
	err = readsession.NewFactory(pool).Read(t.Context(), auth.Release("1.0.0"), project, func(s *readsession.Session) error {
		d, err := s.Descriptors(t.Context())
		if err != nil {
			return err
		}
		id := "descriptor-owner"
		a, err := d.Actor(&id)
		if err != nil {
			return err
		}
		if a.DisplayName != "owner" || !strings.Contains(d.Module(), "/orgs/owner/") {
			t.Fatalf("mixed transaction snapshots: %+v %s", a, d.Module())
		}
		a.DisplayName = "caller mutation"
		again, err := d.Actor(&id)
		if err != nil {
			return err
		}
		if again.DisplayName != "owner" {
			t.Fatal("snapshot leaked mutable actor state")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReleaseDescriptorsRejectConflictingSourceOntologyModules(t *testing.T) {
	pool := descriptorFixture(t)
	store := ontology.NewPostgresStore(pool)
	owner, version := ontology.GenerateOntologyID("identity-conflict"), ontology.GenerateVersionID("identity-conflict", "1")
	if _, err := store.CreateOntology(t.Context(), ontology.CreateOntologyInput{ID: owner, Prefix: "identity", Name: "Shared ontology", Namespace: "https://identity.example/", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ImportVersion(t.Context(), ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: version, OntologyID: owner, VersionString: "1", RDFContent: "<source/>"}}); err != nil {
		t.Fatal(err)
	}
	for _, project := range []string{"descriptor-parent", "descriptor-child"} {
		descriptorExec(t, pool, `INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,added_at) VALUES ($1,$2,now())`, project, version)
		descriptorCut(t, pool, project, "1.0.0", project+".example")
	}
	const consumer = "descriptor-consumer"
	descriptorExec(t, pool, `INSERT INTO weave_projects(id,owner_id) VALUES ($1,'descriptor-owner')`, consumer)
	descriptorExec(t, pool, `INSERT INTO weave_fields(id,project_id,status) VALUES ($1||'-field',$1,'draft')`, consumer)
	for _, parent := range []string{"descriptor-parent", "descriptor-child"} {
		descriptorExec(t, pool, `INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES ($1,$2,'release','1.0.0')`, consumer, parent)
	}
	_, err := release.NewService(release.NewPostgresStore(pool), pool, nil).Create(descriptorContext(t), consumer, release.CreateInput{Version: "1.0.0"})
	var invalid *release.ErrValidation
	if !errors.As(err, &invalid) || len(invalid.Fields["descriptors"]) == 0 || !strings.Contains(invalid.Fields["descriptors"][0], "conflicting captured ontology module") {
		t.Fatalf("inconsistent source modules were accepted: %v", err)
	}
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM weave_releases WHERE project_id=$1`, consumer).Scan(&count); err != nil || count != 0 {
		t.Fatalf("conflict did not roll back: %d %v", count, err)
	}

	// The same rule holds below the service: a raw document cannot silently
	// relocate a source ontology's captured module path.
	tx, q := descriptorPending(t, pool, consumer, "1.0.0")
	if err := q.WeaveInsertReleaseProjectDependency(t.Context(), sqlcgen.WeaveInsertReleaseProjectDependencyParams{ProjectID: consumer, Version: "1.0.0", SourceProjectID: "descriptor-parent", SourceVersion: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	var data []byte
	if err := pool.QueryRow(t.Context(), `SELECT document FROM weave_release_descriptors WHERE project_id='descriptor-parent' AND version_number='1.0.0'`).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc["project_id"] = consumer
	doc["project_modules"].(map[string]any)[consumer] = "consumer.example/orgs/owner/projects/descriptor-consumer"
	doc["ontology_modules"].(map[string]any)[version] = "changed.example/shared-ontology"
	data, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	err = q.WeaveInsertReleaseDescriptors(t.Context(), sqlcgen.WeaveInsertReleaseDescriptorsParams{ProjectID: consumer, Version: "1.0.0", Document: data})
	if err == nil || !strings.Contains(err.Error(), "source ontology module") {
		t.Fatalf("raw source module change: %v", err)
	}
	_ = tx.Rollback(t.Context())
}
