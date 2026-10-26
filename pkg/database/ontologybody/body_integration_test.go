//go:build integration

package ontologybody_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }

type fixture struct {
	pool  *pgxpool.Pool
	store ontology.Store
	input ontology.ImportVersionInput
	ref   ontologybody.Reference
}

func newFixture(t *testing.T, suffix string) fixture {
	t.Helper()
	pool := testdb.Pool(t)
	store := ontology.NewPostgresStore(pool)
	prefix := "body_" + suffix
	owner, version := ontology.GenerateOntologyID(prefix), ontology.GenerateVersionID(prefix, "1")
	_, err := store.CreateOntology(t.Context(), ontology.CreateOntologyInput{ID: owner, Prefix: prefix, Namespace: "https://example.org/" + prefix + "/", Name: "Before", OntologyType: domain.OntologyTypeBase})
	if err != nil {
		t.Fatal(err)
	}
	class, property := ontology.GenerateClassID(version, prefix+":A"), ontology.GeneratePropertyID(version, prefix+":p")
	in := ontology.ImportVersionInput{
		Version:           ontology.CreateVersionInput{ID: version, OntologyID: owner, VersionString: "1", RDFContent: "<rdf/>", OriginalFilename: "main.rdf"},
		SetActive:         true,
		Classes:           []ontology.ImportClass{{ID: class, Prefix: prefix, LocalName: "A", URI: prefix + ":A", Label: domain.Translations{"en": "Class"}}},
		Properties:        []ontology.ImportProperty{{ID: property, Prefix: prefix, LocalName: "p", URI: prefix + ":p"}},
		Relations:         []domain.OntologyRelation{{SourceID: property, SourceKind: "property", RelType: "domain", TargetQname: prefix + ":A"}},
		CompanionFiles:    []ontology.CompanionFileInput{{Filename: "extra.rdf", Content: "<extra/>"}},
		NamespaceBindings: []ontology.NamespaceBinding{{Prefix: prefix, Namespace: "https://example.org/" + prefix + "/"}},
	}
	if _, err := store.ImportVersion(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	ref, err := ontologybody.NewReference(ontologybody.OntologyID(owner), ontologybody.VersionID(version))
	if err != nil {
		t.Fatal(err)
	}
	// Sealed fixture bodies intentionally remain until the package clone drops.
	// Test cleanup must not introduce a production unsealing backdoor.
	return fixture{pool: pool, store: store, input: in, ref: ref}
}

func (f fixture) seal(t *testing.T) {
	t.Helper()
	tx, err := f.pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	if err := ontologybody.Seal(t.Context(), sqlcgen.New(tx), f.ref); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) body(t *testing.T) ontologybody.Body {
	t.Helper()
	var body ontologybody.Body
	err := ontologybody.NewFactory(f.pool).Read(t.Context(), f.ref, func(r ontologybody.Reader) error {
		var err error
		body, err = r.Body(t.Context())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func requireSealed(t *testing.T, err error) {
	t.Helper()
	if err == nil || apierror.FromError(ontologybody.MapError(err)).Code != apierror.CodeConflict {
		t.Fatalf("wanted sealed conflict, got %v", err)
	}
}

func TestBodySealProtectsEveryTableAndSurvivesPolicyChanges(t *testing.T) {
	f := newFixture(t, "guards")
	f.seal(t)
	before := f.body(t)
	v, c, p := f.input.Version.ID, f.input.Classes[0].ID, f.input.Properties[0].ID
	for _, mutation := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE weave_ontology_versions SET rdf_content='changed' WHERE id=$1`, []any{v}},
		{`UPDATE weave_ontology_versions SET body_sealed_at=NULL WHERE id=$1`, []any{v}},
		{`DELETE FROM weave_ontology_versions WHERE id=$1`, []any{v}},
		{`UPDATE weave_ontology_classes SET label='{"en":"changed"}' WHERE id=$1`, []any{c}},
		{`DELETE FROM weave_ontology_classes WHERE id=$1`, []any{c}},
		{`INSERT INTO weave_ontology_classes(id,ontology_version_id,prefix,local_name,uri) VALUES ('seal-extra',$1,'x','B','x:B')`, []any{v}},
		{`UPDATE weave_ontology_properties SET is_transitive=true WHERE id=$1`, []any{p}},
		{`DELETE FROM weave_ontology_properties WHERE id=$1`, []any{p}},
		{`INSERT INTO weave_ontology_properties(id,ontology_version_id,prefix,local_name,uri) VALUES ('seal-extra-property',$1,'x','p','x:p')`, []any{v}},
		{`UPDATE weave_ontology_relations SET position=2 WHERE source_id=$1`, []any{p}},
		{`DELETE FROM weave_ontology_relations WHERE source_id=$1`, []any{p}},
		{`INSERT INTO weave_ontology_relations(source_id,source_kind,rel_type,target_qname) VALUES ($1,'class','subclass_of','x:B')`, []any{c}},
		{`UPDATE weave_ontology_version_companions SET content='changed' WHERE ontology_version_id=$1`, []any{v}},
		{`DELETE FROM weave_ontology_version_companions WHERE ontology_version_id=$1`, []any{v}},
		{`INSERT INTO weave_ontology_version_companions(ontology_version_id,filename,content) VALUES ($1,'new.rdf','new')`, []any{v}},
		{`UPDATE weave_ontology_body_seals SET body='{}' WHERE version_id=$1`, []any{v}},
		{`DELETE FROM weave_ontology_body_seals WHERE version_id=$1`, []any{v}},
		{`TRUNCATE weave_ontology_relations`, nil},
		{`TRUNCATE weave_ontology_classes CASCADE`, nil},
		{`TRUNCATE weave_ontology_properties CASCADE`, nil},
		{`TRUNCATE weave_ontology_version_companions`, nil},
		{`TRUNCATE weave_ontology_body_seals`, nil},
		{`TRUNCATE weave_ontology_versions CASCADE`, nil},
		{`TRUNCATE weave_ontologies CASCADE`, nil},
	} {
		_, err := f.pool.Exec(t.Context(), mutation.sql, mutation.args...)
		requireSealed(t, err)
	}
	_, err := f.store.UpdateVersionMetadata(t.Context(), v, ontology.UpdateVersionMetadataInput{VersionString: "changed"})
	requireSealed(t, err)
	requireSealed(t, f.store.UpdateVersionCounts(t.Context(), v, 9, 9))
	requireSealed(t, f.store.DeleteVersion(t.Context(), v))
	requireSealed(t, f.store.DeleteOntology(t.Context(), f.input.Version.OntologyID))
	changed := f.input
	changed.Version.RDFContent = "replacement"
	_, err = f.store.ImportVersion(t.Context(), changed)
	requireSealed(t, err)
	// Mutable discovery and catalog data cannot affect the sealed reader.
	newVersion := ontology.GenerateVersionID("body_guards", "2")
	_, err = f.store.CreateVersion(t.Context(), ontology.CreateVersionInput{ID: newVersion, OntologyID: f.input.Version.OntologyID, VersionString: "2"})
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range []string{newVersion, v} {
		if err := f.store.SetActiveVersion(t.Context(), f.input.Version.OntologyID, selection); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE weave_ontologies SET name='After', namespace='https://changed.example/' WHERE id=$1`, f.input.Version.OntologyID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE weave_namespace_bindings SET namespace='https://changed.example/' WHERE ontology_id=$1`, f.input.Version.OntologyID); err != nil {
		t.Fatal(err)
	}
	after := f.body(t)
	if before != after {
		t.Fatal("sealed body changed with catalog or active policy")
	}
	if !strings.Contains(after.CanonicalJSON, "<extra/>") || !strings.Contains(after.CanonicalJSON, "Before") {
		t.Fatal("body omitted companion or descriptor")
	}
}

func TestUnsealedReimportRemovesOldRelationsAndValidatesOwners(t *testing.T) {
	f := newFixture(t, "replace")
	f.input.Relations = nil
	if _, err := f.store.ImportVersion(t.Context(), f.input); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM weave_ontology_relations WHERE source_id=$1`, f.input.Properties[0].ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("re-import retained %d old relations", count)
	}
	_, err := f.pool.Exec(t.Context(), `INSERT INTO weave_ontology_relations(source_id,source_kind,rel_type,target_qname,ontology_version_id) VALUES ($1,'class','subclass_of','x:B','wrong-owner')`, f.input.Classes[0].ID)
	if err == nil {
		t.Fatal("accepted relation owned by a different version")
	}
	f.seal(t)
	other := newFixture(t, "moving")
	_, err = f.pool.Exec(t.Context(), `UPDATE weave_ontology_classes SET ontology_version_id=$1 WHERE id=$2`, f.input.Version.ID, other.input.Classes[0].ID)
	requireSealed(t, err)
}

func TestSealRollbackReaderOwnershipLifetimeAndDowngrade(t *testing.T) {
	f := newFixture(t, "reader")
	tx, err := f.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := ontologybody.Seal(t.Context(), sqlcgen.New(tx), f.ref); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	factory := ontologybody.NewFactory(f.pool)
	called := false
	err = factory.Read(t.Context(), f.ref, func(ontologybody.Reader) error { called = true; return nil })
	if apierror.FromError(err).Code != apierror.CodeNotInRelease || called {
		t.Fatalf("rollback left readable body: %v", err)
	}
	f.seal(t)
	wrong, err := ontologybody.NewReference("another-owner", f.ref.VersionID())
	if err != nil {
		t.Fatal(err)
	}
	if err := factory.Read(t.Context(), wrong, func(ontologybody.Reader) error { t.Fatal("foreign owner exposed"); return nil }); apierror.FromError(err).Code != apierror.CodeNotInRelease {
		t.Fatalf("wrong owner: %v", err)
	}
	var escaped ontologybody.Reader
	if err := factory.Read(t.Context(), f.ref, func(r ontologybody.Reader) error { escaped = r; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := escaped.Body(t.Context()); err == nil {
		t.Fatal("reader escaped callback lifetime")
	}
	db := stdlib.OpenDB(*f.pool.Config().ConnConfig)
	t.Cleanup(func() {
		if err := database.Migrate(db); err != nil {
			t.Error(err)
		}
		_ = db.Close()
	})
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version > 23; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "cannot downgrade sealed ontology bodies") {
		t.Fatalf("downgrade discarded seals: %v", err)
	}
}

func waitForBlock(t *testing.T, pool *pgxpool.Pool, pid uint32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := pool.QueryRow(t.Context(), `SELECT cardinality(pg_blocking_pids($1::int)) > 0`, pid).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("writer did not wait on the shared version lock")
}

func TestSealingSerializesWithLateBodyWrites(t *testing.T) {
	for _, isolation := range []pgx.TxIsoLevel{pgx.ReadCommitted, pgx.RepeatableRead} {
		t.Run(string(isolation), func(t *testing.T) {
			f := newFixture(t, strings.ReplaceAll(string(isolation), " ", "_"))
			writer, err := f.pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: isolation})
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Rollback(context.Background()) //nolint:errcheck
			var id string
			if err := writer.QueryRow(t.Context(), `SELECT id FROM weave_ontology_versions WHERE id=$1`, f.input.Version.ID).Scan(&id); err != nil {
				t.Fatal(err)
			}
			sealer, err := f.pool.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer sealer.Rollback(context.Background()) //nolint:errcheck
			if err := ontologybody.Seal(t.Context(), sqlcgen.New(sealer), f.ref); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				_, err := writer.Exec(t.Context(), `INSERT INTO weave_ontology_relations(source_id,source_kind,rel_type,target_qname) VALUES ($1,'class','subclass_of','late:B')`, f.input.Classes[0].ID)
				done <- err
			}()
			waitForBlock(t, f.pool, writer.Conn().PgConn().PID())
			if err := sealer.Commit(t.Context()); err != nil {
				t.Fatal(err)
			}
			err = <-done
			var pgerr *pgconn.PgError
			if !errors.As(err, &pgerr) || (pgerr.Code != "40001" && pgerr.ConstraintName != "ontology_body_sealed") {
				t.Fatalf("late write crossed seal: %v", err)
			}
			if strings.Contains(f.body(t).CanonicalJSON, "late:B") {
				t.Fatal("sealed body included late writer")
			}
		})
	}
}

func TestStaleSealCannotMissCommittedBodyWriter(t *testing.T) {
	f := newFixture(t, "stale_seal")
	sealer, err := f.pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer sealer.Rollback(context.Background()) //nolint:errcheck
	var id string
	if err := sealer.QueryRow(t.Context(), `SELECT id FROM weave_ontology_versions WHERE id=$1`, f.input.Version.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	writer, err := f.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback(context.Background()) //nolint:errcheck
	if _, err := writer.Exec(t.Context(), `INSERT INTO weave_ontology_relations(source_id,source_kind,rel_type,target_qname) VALUES ($1,'class','subclass_of','committed:B')`, f.input.Classes[0].ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	pid := sealer.Conn().PgConn().PID()
	go func() { done <- ontologybody.Seal(t.Context(), sqlcgen.New(sealer), f.ref) }()
	waitForBlock(t, f.pool, pid)
	if err := writer.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	err = <-done
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "40001" {
		t.Fatalf("stale seal did not fail serialization: %v", err)
	}
	if err := sealer.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.seal(t)
	if !strings.Contains(f.body(t).CanonicalJSON, "committed:B") {
		t.Fatal("retry omitted committed relation")
	}
}

func TestBodyReaderCancellationReleasesConnection(t *testing.T) {
	f := newFixture(t, "cancel")
	f.seal(t)
	cfg := f.pool.Config()
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithCancel(t.Context())
	factory := ontologybody.NewFactory(pool)
	err = factory.Read(ctx, f.ref, func(ontologybody.Reader) error { cancel(); return ctx.Err() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error lost: %v", err)
	}
	check, stop := context.WithTimeout(t.Context(), 2*time.Second)
	defer stop()
	if err := pool.Ping(check); err != nil {
		t.Fatalf("canceled body reader retained connection: %v", err)
	}
	if err := factory.Read(check, ontologybody.Reference{}, func(ontologybody.Reader) error { t.Fatal("zero reference accepted"); return nil }); err == nil {
		t.Fatal("zero reference succeeded")
	}
}

func TestSealRejectsBadCountsAndBuildsItsOwnEvidence(t *testing.T) {
	f := newFixture(t, "evidence")
	if _, err := f.pool.Exec(t.Context(), `UPDATE weave_ontology_versions SET body_sealed_at=now() WHERE id=$1`, f.input.Version.ID); err == nil {
		t.Fatal("direct seal marker bypassed capture")
	}
	if err := f.store.UpdateVersionCounts(t.Context(), f.input.Version.ID, 99, 1); err != nil {
		t.Fatal(err)
	}
	err := ontologybody.Seal(t.Context(), sqlcgen.New(f.pool), f.ref)
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.ConstraintName != "ontology_body_counts" {
		t.Fatalf("bad counts were sealed: %v", err)
	}
	var marked bool
	if err := f.pool.QueryRow(t.Context(), `SELECT body_sealed_at IS NOT NULL OR EXISTS (SELECT 1 FROM weave_ontology_body_seals WHERE version_id=$1) FROM weave_ontology_versions WHERE id=$1`, f.input.Version.ID).Scan(&marked); err != nil {
		t.Fatal(err)
	}
	if marked {
		t.Fatal("failed seal retained partial evidence")
	}
	if err := f.store.UpdateVersionCounts(t.Context(), f.input.Version.ID, 1, 1); err != nil {
		t.Fatal(err)
	}
	// Even an unrestricted SQL caller cannot choose the purported body/digest.
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO weave_ontology_body_seals(ontology_id,version_id,format_version,body,digest,sealed_at) VALUES ($1,$2,99,'{}','fabricated',now())`, f.input.Version.OntologyID, f.input.Version.ID); err != nil {
		t.Fatal(err)
	}
	body := f.body(t)
	if body.CanonicalJSON == "{}" || body.FormatVersion != 1 || body.Digest == "fabricated" {
		t.Fatal("caller supplied seal evidence")
	}
}
