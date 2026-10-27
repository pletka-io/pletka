//go:build integration

package database_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

const policyOwner = "POLICY_OWNER"

func policyFixture(t *testing.T) (*pgxpool.Pool, ontology.Store) {
	t.Helper()
	pool := testdb.EmptyPool(t)
	store := ontology.NewPostgresStore(pool)
	if _, err := store.CreateOntology(t.Context(), ontology.CreateOntologyInput{ID: policyOwner, Prefix: "policy", Namespace: "https://policy.example/", Name: "Policy", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"policy-1", "policy-2"} {
		if _, err := store.ImportVersion(t.Context(), ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: id, OntologyID: policyOwner, VersionString: id, RDFContent: "<policy/>"}, SetActive: id == "policy-1"}); err != nil {
			t.Fatal(err)
		}
	}
	return pool, store
}

func policyArtifact(t *testing.T, pool *pgxpool.Pool) ontologyrelease.Artifact {
	t.Helper()
	ref, err := ontologyrelease.NewReference(ontologybody.OntologyID(policyOwner), ontologybody.VersionID("policy-1"))
	if err != nil {
		t.Fatal(err)
	}
	var artifact ontologyrelease.Artifact
	if err := ontologyrelease.NewFactory(pool).Read(t.Context(), ref, func(r ontologyrelease.Reader) error {
		var err error
		artifact, err = r.Artifact(t.Context())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return artifact
}

func requireActive(t *testing.T, store ontology.Store, want string) {
	t.Helper()
	v, err := store.GetActiveVersion(t.Context(), policyOwner)
	if err != nil || v == nil || v.ID != want || !v.IsActive {
		t.Fatalf("active version = %+v, want %s: %v", v, want, err)
	}
}

func TestOntologyPolicyDoesNotWritePublishedContent(t *testing.T) {
	pool, store := policyFixture(t)
	before := policyArtifact(t, pool)
	var bodyBefore, bodyAfter string
	if err := pool.QueryRow(t.Context(), `SELECT to_jsonb(v)::text FROM weave_ontology_versions v WHERE id='policy-1'`).Scan(&bodyBefore); err != nil {
		t.Fatal(err)
	}
	service := ontology.NewService(store, nil, nil)
	if err := service.SetActiveVersion(t.Context(), policyOwner, "policy-2"); err != nil {
		t.Fatal(err)
	}
	requireActive(t, store, "policy-2")
	versions, err := store.ListVersionsByOntology(t.Context(), policyOwner)
	if err != nil || len(versions) != 2 {
		t.Fatalf("list status: %+v, %v", versions, err)
	}
	for _, version := range versions {
		if version.IsActive != (version.ID == "policy-2") {
			t.Fatalf("stale derived active flag: %+v", version)
		}
	}
	if _, err := store.CreateOntology(t.Context(), ontology.CreateOntologyInput{ID: "foreign-owner", Prefix: "foreign", Namespace: "https://foreign.example/", Name: "Foreign", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	for _, selection := range [][2]string{{policyOwner, "missing"}, {"foreign-owner", "policy-1"}} {
		if err := store.SetActiveVersion(t.Context(), selection[0], selection[1]); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("invalid policy target accepted: %v", err)
		}
	}
	for _, statement := range []string{
		`UPDATE weave_ontology_versions SET is_active=true WHERE id='policy-1'`,
		`UPDATE weave_ontology_version_status SET is_active=true WHERE id='policy-1'`,
		`INSERT INTO weave_ontology_active_versions(ontology_id,version_id) VALUES ('foreign-owner','policy-1')`,
		`INSERT INTO weave_ontology_active_versions(ontology_id,version_id) VALUES ('POLICY_OWNER','policy-1')`,
		`UPDATE weave_ontology_active_versions SET version_id='missing' WHERE ontology_id='POLICY_OWNER'`,
	} {
		if _, err := pool.Exec(t.Context(), statement); err == nil {
			t.Fatalf("policy storage bypass succeeded: %s", statement)
		}
	}
	// Import moves policy in its own transaction; publication failure must undo it.
	_, err = store.ImportVersion(t.Context(), ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: "policy-failed", OntologyID: policyOwner, VersionString: "failed", RDFContent: "<failed/>", ImportedOntologies: []string{"urn:unresolved-import"}}, SetActive: true})
	if err == nil {
		t.Fatal("invalid publication succeeded")
	}
	var retained bool
	if err := pool.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM weave_ontology_versions WHERE id='policy-failed')`).Scan(&retained); err != nil || retained {
		t.Fatalf("failed import retained version: %v", err)
	}
	requireActive(t, store, "policy-2")
	if err := pool.QueryRow(t.Context(), `SELECT to_jsonb(v)::text FROM weave_ontology_versions v WHERE id='policy-1'`).Scan(&bodyAfter); err != nil {
		t.Fatal(err)
	}
	if before != policyArtifact(t, pool) || bodyBefore != bodyAfter {
		t.Fatal("selection changed immutable bytes or version metadata")
	}
}

func TestOntologyPolicyInitializationCannotReplaceConcurrentSelection(t *testing.T) {
	pool, store := policyFixture(t)
	if _, err := pool.Exec(t.Context(), `DELETE FROM weave_ontology_active_versions WHERE ontology_id=$1`, policyOwner); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	if found, err := sqlcgen.New(tx).WeaveSelectActiveOntologyVersion(ctx, sqlcgen.WeaveSelectActiveOntologyVersionParams{OntologyID: policyOwner, VersionID: "policy-2"}); err != nil || !found {
		t.Fatalf("select version: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- store.SetActiveVersionIfUnset(ctx, policyOwner, "policy-1") }()
	for {
		var blocked bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::int=ANY(pg_blocking_pids(pid)))`, tx.Conn().PgConn().PID()).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	requireActive(t, store, "policy-2")
	if err := store.SetActiveVersionIfUnset(ctx, policyOwner, "policy-1"); err != nil {
		t.Fatal(err)
	}
	requireActive(t, store, "policy-2")
}

func TestOntologyPolicyMigrationPreservesSelectionAndArtifactBytes(t *testing.T) {
	pool, store := policyFixture(t)
	before := policyArtifact(t, pool)
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	downgradeOntologyPolicy(t, db)
	var selected string
	if err := pool.QueryRow(t.Context(), `SELECT id FROM weave_ontology_versions WHERE ontology_id=$1 AND is_active`, policyOwner).Scan(&selected); err != nil || selected != "policy-1" {
		t.Fatalf("downgrade lost selection: %s, %v", selected, err)
	}
	// Legacy evidence must migrate unchanged and remain unpublished.
	if _, err := pool.Exec(t.Context(), `INSERT INTO weave_ontologies(id,prefix,name,namespace) VALUES ('legacy-policy','legacy-policy','Legacy','urn:legacy')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO weave_ontology_versions(id,ontology_id,version_string,is_active) VALUES ('legacy-policy-1','legacy-policy','1',true)`); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	requireActive(t, store, "policy-1")
	var legacy string
	var certified bool
	if err := pool.QueryRow(t.Context(), `SELECT p.version_id,EXISTS(SELECT 1 FROM weave_ontology_releases r WHERE r.version_id=p.version_id) FROM weave_ontology_active_versions p WHERE p.ontology_id='legacy-policy'`).Scan(&legacy, &certified); err != nil || legacy != "legacy-policy-1" || certified {
		t.Fatalf("legacy policy changed or certified: %s, %v, %v", legacy, certified, err)
	}
	if before != policyArtifact(t, pool) {
		t.Fatal("policy migration rewrote publication")
	}
	downgradeOntologyPolicy(t, db)
	if _, err := pool.Exec(t.Context(), `DROP INDEX idx_weave_ontology_versions_active_per_ontology`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE weave_ontology_versions SET is_active=true WHERE id='policy-2'`); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err == nil || !strings.Contains(err.Error(), "ambiguous active ontology selections") {
		t.Fatalf("ambiguous history silently selected: %v", err)
	}
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM weave_ontology_versions WHERE ontology_id=$1 AND is_active`, policyOwner).Scan(&count); err != nil || count != 2 {
		t.Fatalf("migration discarded ambiguous evidence: %d, %v", count, err)
	}
}

func downgradeOntologyPolicy(t *testing.T, db *sql.DB) {
	t.Helper()
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version > 26; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOntologyRestorePreservesExistingActivePolicy(t *testing.T) {
	source, _ := policyFixture(t)
	artifact := policyArtifact(t, source)
	target := testdb.EmptyPool(t)
	store := ontology.NewPostgresStore(target)
	if _, err := store.CreateOntology(t.Context(), ontology.CreateOntologyInput{ID: policyOwner, Prefix: "policy", Namespace: "https://policy.example/", Name: "Destination", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(t.Context(), ontology.CreateVersionInput{ID: "destination-current", OntologyID: policyOwner, VersionString: "current", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateVersionMetadata(t.Context(), "destination-current", ontology.UpdateVersionMetadataInput{VersionString: "current-renamed"})
	if err != nil || updated == nil || !updated.IsActive || updated.VersionString != "current-renamed" {
		t.Fatalf("metadata response lost derived policy: %+v, %v", updated, err)
	}
	if err := ontologyrelease.Restore(t.Context(), target, []ontologyrelease.Artifact{artifact}); err != nil {
		t.Fatal(err)
	}
	requireActive(t, store, "destination-current")
	if artifact != policyArtifact(t, target) {
		t.Fatal("destination policy changed restored artifact")
	}
	if err := store.DeleteVersion(t.Context(), "destination-current"); err != nil {
		t.Fatal(err)
	}
	active, err := store.GetActiveVersion(t.Context(), policyOwner)
	if err != nil || active != nil {
		t.Fatalf("deleting selected legacy content must clear policy without choosing a replacement: %+v, %v", active, err)
	}
}
