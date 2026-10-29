//go:build integration

package ontologysnapshot_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }

func exec(t *testing.T, q interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, statement string) {
	t.Helper()
	if _, err := q.Exec(t.Context(), statement); err != nil {
		t.Fatal(err)
	}
}

// Synthetic fixture reproduces the reviewed two intentional CRM selections without
// embedding customer content. Both versions define the same URI differently.
func fixture(t *testing.T) *pgxpool.Pool {
	t.Helper()
	p := testdb.EmptyPool(t)
	store := ontology.NewPostgresStore(p)
	_, err := store.CreateOntology(t.Context(), ontology.CreateOntologyInput{ID: "snapshot-crm", Prefix: "snapshot_crm", Namespace: "https://example.org/crm/", Name: "CRM", OntologyType: domain.OntologyTypeBase})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"6.2", "7.1.3", "8"} {
		_, err := store.StageLegacyVersion(t.Context(), ontology.ImportVersionInput{
			Version:           ontology.CreateVersionInput{ID: "crm-" + v, OntologyID: "snapshot-crm", VersionString: v, RDFContent: "<rdf version='" + v + "'/>"},
			Classes:           []ontology.ImportClass{{ID: "class-" + v, Prefix: "crm", LocalName: "E1", URI: "https://example.org/crm/E1", Label: domain.Translations{"en": "definition " + v}}},
			NamespaceBindings: []ontology.NamespaceBinding{{Prefix: "crm", Namespace: "https://example.org/crm/"}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{
		`INSERT INTO weave_actors(id,type,slug,display_name) VALUES ('snapshot-owner','organization','snapshot-owner','Owner')`,
		`INSERT INTO weave_projects(id,owner_id) VALUES ('legacy-project','snapshot-owner'),('source','snapshot-owner'),('empty','snapshot-owner')`,
		`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('legacy-project','1','snapshot-owner'),('source','1','snapshot-owner'),('empty','1','snapshot-owner')`,
		`INSERT INTO weave_project_ontology_versions_archive(project_id,version_number,ontology_version_id,added_at,is_primary) VALUES ('legacy-project','1','crm-6.2',now(),true),('legacy-project','1','crm-7.1.3',now(),false),('source','1','crm-7.1.3',now(),true)`,
		`INSERT INTO weave_fields_archive(id,project_id,version_number,path_elements) VALUES ('her-ref','legacy-project','1','[{"type":"class","uri":"https://example.org/crm/E1"},{"type":"class","prefix":"crm","local_name":"E1","ontology_version_id":"crm-6.2"},{"type":"class","uri":"https://example.org/crm/E1","ontology_version_id":"crm-8"},{"type":"literal","uri":"rdfs:Literal","datatype":"rdf:literal"},{"type":"invalid","uri":"crm:E1"},{"type":"class","uri":"source_alias:E1"}]'),('source-ref','source','1','[{"type":"class","prefix":"source_alias","local_name":"E1"}]')`,
		`INSERT INTO weave_release_namespace_bindings_archive(release_project_id,version_number,id,created_at,updated_at,prefix,namespace,weight,source) VALUES ('source','1','source-alias',now(),now(),'source_alias','https://example.org/crm/',80,'manifest'),('legacy-project','1','her-alias',now(),now(),'source_alias','https://elsewhere.example/',80,'manifest')`,
		`INSERT INTO weave_release_project_dependencies VALUES ('legacy-project','1','source','1')`,
		`INSERT INTO weave_release_dependency_entities VALUES ('legacy-project','1','field','source-ref','source','1')`,
	} {
		exec(t, p, s)
	}
	return p
}

func origin(t *testing.T, project string) ontologysnapshot.Origin {
	t.Helper()
	o, err := ontologysnapshot.NewOrigin(project, "1")
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func capture(t *testing.T, p *pgxpool.Pool, project string) ontologysnapshot.Artifact {
	t.Helper()
	tx, err := p.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	a, err := ontologysnapshot.Capture(t.Context(), sqlcgen.New(tx), origin(t, project))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestMixedLegacySelectionsKeepSourceContextAndSurviveLaterChanges(t *testing.T) {
	p := fixture(t)
	source := capture(t, p, "source")
	a := capture(t, p, "legacy-project")
	d, err := a.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Inventory) != 2 || len(d.Sources) != 1 || (d.Sources[0].JSON != source.JSON || d.Sources[0].Digest != source.Digest) || len(d.Dependencies) != 1 {
		t.Fatalf("lost inventory or origin: %+v", d)
	}
	want := []ontologysnapshot.Status{ontologysnapshot.Ambiguous, ontologysnapshot.Resolved, ontologysnapshot.Invalid, ontologysnapshot.Missing, ontologysnapshot.Missing, ontologysnapshot.Invalid, ontologysnapshot.Missing}
	if len(d.Assessments) != len(want) {
		t.Fatalf("assessments: %+v", d.Assessments)
	}
	for i, status := range want {
		if d.Assessments[i].Status != status {
			t.Errorf("assessment %d: %s want %s", i, d.Assessments[i].Status, status)
		}
	}
	foundLiteral := false
	for _, e := range d.Elements {
		if strings.Contains(string(e.Original), `"datatype": "rdf:literal"`) {
			foundLiteral = true
		}
	}
	if !foundLiteral {
		t.Fatal("original literal changed")
	}
	_, err = a.Resolve("field", "her-ref", "/path_elements/0")
	var unresolved *ontologysnapshot.UnresolvedError
	if !errors.As(err, &unresolved) || unresolved.Status != ontologysnapshot.Ambiguous {
		t.Fatalf("ambiguous lookup: %v", err)
	}
	definition, err := a.Resolve("field", "her-ref", "/path_elements/1")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := definition.Evidence()
	if err != nil || c.VersionID != "crm-6.2" {
		t.Fatalf("explicit lookup: %+v %v", c, err)
	}
	definition, err = source.Resolve("field", "source-ref", "/path_elements/0")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err = definition.Evidence()
	if err != nil || c.VersionID != "crm-7.1.3" {
		t.Fatalf("source lookup: %+v %v", c, err)
	}
	definition, err = a.Resolve("field", "source-ref", "/path_elements/0")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err = definition.Evidence()
	if err != nil || c.VersionID != "crm-7.1.3" {
		t.Fatalf("importing reader did not route through source context: %+v %v", c, err)
	}
	ref, err := a.Reference()
	if err != nil {
		t.Fatal(err)
	}
	var escaped ontologysnapshot.Reader
	read := func() {
		t.Helper()
		err := ontologysnapshot.NewFactory(p).Read(t.Context(), ref, func(r ontologysnapshot.Reader) error {
			escaped = r
			got, err := r.Snapshot(t.Context())
			if err == nil && got != a {
				t.Error("snapshot changed")
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	read()
	if _, err := escaped.Snapshot(t.Context()); err == nil {
		t.Fatal("reader escaped callback")
	}
	exec(t, p, `UPDATE weave_ontologies SET name='changed' WHERE id='snapshot-crm'`)
	exec(t, p, `UPDATE weave_namespace_bindings SET namespace='https://changed.example/' WHERE prefix='crm'`)
	exec(t, p, `UPDATE weave_ontology_classes SET label='{"en":"new draft"}' WHERE ontology_version_id='crm-8'`)
	if err := ontology.NewPostgresStore(p).SetActiveVersion(t.Context(), "snapshot-crm", "crm-8"); err != nil {
		t.Fatal(err)
	}
	exec(t, p, `UPDATE weave_project_ontology_versions_archive SET is_primary=NOT is_primary WHERE project_id='legacy-project'`)
	exec(t, p, `UPDATE weave_fields_archive SET path_elements='[]' WHERE id='her-ref'`)
	read()
	var publications, markers int
	if err := p.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM weave_ontology_releases),(SELECT count(*) FROM weave_releases WHERE ontology_dependencies_version<>0 OR content_capture_version<>0)`).Scan(&publications, &markers); err != nil {
		t.Fatal(err)
	}
	if publications != 0 || markers != 0 {
		t.Fatal("legacy snapshot promoted verified evidence")
	}
}

func TestRawWritersCannotForgeOrRemoveSnapshot(t *testing.T) {
	p := fixture(t)
	capture(t, p, "source")
	tx, err := p.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	var a ontologysnapshot.Artifact
	err = tx.QueryRow(t.Context(), `INSERT INTO weave_legacy_ontology_snapshots(project_id,version_number,document,digest) VALUES ('legacy-project','1','{}','forged') RETURNING document,digest`).Scan(&a.JSON, &a.Digest)
	if err != nil {
		t.Fatal(err)
	}
	d, err := a.Decode()
	if err != nil || len(d.Assessments) != 7 {
		t.Fatalf("raw writer forged evidence: %+v %v", d, err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`UPDATE weave_legacy_ontology_snapshots SET document='{}'`,
		`DELETE FROM weave_legacy_ontology_snapshots`,
		`TRUNCATE weave_legacy_ontology_snapshots`,
		`DELETE FROM weave_releases WHERE project_id='legacy-project'`,
		`TRUNCATE weave_releases CASCADE`,
		`UPDATE weave_ontology_classes SET uri='changed' WHERE ontology_version_id='crm-6.2'`,
		`DELETE FROM weave_ontology_body_seals WHERE version_id='crm-6.2'`,
	} {
		_, err := p.Exec(t.Context(), s)
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || (pgerr.Code != "23514" && pgerr.Code != "23503" && pgerr.Code != "23001") {
			t.Errorf("mutation must fail an integrity guard: %s: %v", s, err)
		}
	}
	db := stdlib.OpenDB(*p.Config().ConnConfig)
	defer db.Close()
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version > 35; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "cannot discard retained legacy") {
		t.Fatalf("downgrade: %v", err)
	}
}

func TestCaptureFailureRollbackAndReaderBindings(t *testing.T) {
	p := fixture(t)
	if _, err := ontologysnapshot.Capture(t.Context(), sqlcgen.New(p), origin(t, "source")); err == nil {
		t.Fatal("read-committed capture accepted")
	}
	tx, err := p.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ontologysnapshot.Capture(t.Context(), sqlcgen.New(tx), origin(t, "legacy-project")); err == nil {
		t.Fatal("uncaptured source accepted")
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	tx, err = p.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ontologysnapshot.Capture(t.Context(), sqlcgen.New(tx), origin(t, "source")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := p.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM weave_legacy_ontology_snapshots)+(SELECT count(*) FROM weave_ontology_body_seals)`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback leaked evidence: %d %v", count, err)
	}
	a := capture(t, p, "source")
	ref, err := a.Reference()
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := ontologysnapshot.NewReference("legacy-project", "1", a.Digest)
	if err != nil {
		t.Fatal(err)
	}
	factory := ontologysnapshot.NewFactory(p)
	for _, bad := range []ontologysnapshot.Reference{{}, wrong} {
		if err := factory.Read(t.Context(), bad, func(ontologysnapshot.Reader) error { t.Error("invalid reference entered callback"); return nil }); err == nil {
			t.Fatal("invalid binding accepted")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := factory.Read(ctx, ref, func(ontologysnapshot.Reader) error { return nil }); err == nil {
		t.Fatal("canceled reader accepted")
	}
	if err := factory.Read(t.Context(), ref, func(ontologysnapshot.Reader) error { return nil }); err != nil {
		t.Fatal(err)
	}
	// Existing publication capture still requires verified source evidence.
	tx, err = p.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	if err := ontologyrelease.CaptureProject(t.Context(), sqlcgen.New(tx), "legacy-project", "1"); err == nil {
		t.Fatal("ordinary publication admitted legacy mixture")
	}
}

func TestEmptyMigrationRoundTrip(t *testing.T) {
	p := testdb.EmptyPool(t)
	db := stdlib.OpenDB(*p.Config().ConnConfig)
	defer db.Close()
	if err := database.MigrateDown(db); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := p.QueryRow(t.Context(), `SELECT to_regclass('weave_legacy_ontology_snapshots') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("migration: %v %v", exists, err)
	}
}

func TestStaleCaptureCannotMixConcurrentArchiveWrites(t *testing.T) {
	p := fixture(t)
	tx, err := p.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	// Establish the old transaction snapshot, then commit an archive write whose
	// existing trigger updates the per-release fence.
	var count int
	if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM weave_fields_archive`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	exec(t, p, `UPDATE weave_fields_archive SET path_elements='[]' WHERE id='source-ref'`)
	_, err = ontologysnapshot.Capture(t.Context(), sqlcgen.New(tx), origin(t, "source"))
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "40001" {
		t.Fatalf("stale capture should serialize: %v", err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	a := capture(t, p, "source")
	d, err := a.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Assessments) != 0 {
		t.Fatal("new cut retained stale references")
	}
}

func TestMissingSelectedBodyRollsBackEarlierSeals(t *testing.T) {
	p := fixture(t)
	exec(t, p, `INSERT INTO weave_project_ontology_versions_archive(project_id,version_number,ontology_version_id,added_at) VALUES ('empty','1','crm-6.2',now()),('empty','1','zzz-missing',now())`)
	tx, err := p.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	_, err = ontologysnapshot.Capture(t.Context(), sqlcgen.New(tx), origin(t, "empty"))
	if err == nil || !strings.Contains(err.Error(), "selected ontology body is missing") {
		t.Fatalf("missing body: %v", err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := p.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM weave_ontology_body_seals)+(SELECT count(*) FROM weave_legacy_ontology_snapshots)`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed cut leaked seals: %d %v", count, err)
	}
}

func TestTransitiveSourceContextsAreFlatAndDeduplicated(t *testing.T) {
	p := fixture(t)
	capture(t, p, "source")
	capture(t, p, "legacy-project")
	exec(t, p, `INSERT INTO weave_release_project_dependencies VALUES ('empty','1','legacy-project','1'),('empty','1','source','1')`)
	exec(t, p, `INSERT INTO weave_release_dependency_entities VALUES ('empty','1','field','source-ref','legacy-project','1')`)
	a := capture(t, p, "empty")
	d, err := a.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Sources) != 2 {
		t.Fatalf("transitive context duplicated: %d", len(d.Sources))
	}
	for _, source := range d.Sources {
		var local struct {
			Sources []json.RawMessage `json:"sources"`
		}
		if err := json.Unmarshal([]byte(source.JSON), &local); err != nil {
			t.Fatal(err)
		}
		if len(local.Sources) != 0 {
			t.Fatal("nested source snapshot")
		}
	}
	definition, err := a.Resolve("field", "source-ref", "/path_elements/0")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := definition.Evidence()
	if err != nil || c.VersionID != "crm-7.1.3" {
		t.Fatalf("transitive source: %+v %v", c, err)
	}
}
