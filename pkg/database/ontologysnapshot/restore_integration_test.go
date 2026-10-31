//go:build integration

package ontologysnapshot_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

func snapshotDigest(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

func snapshotValue(t *testing.T, d ontologysnapshot.Document) ontologysnapshot.Artifact {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	a := ontologysnapshot.Artifact{JSON: string(b), Digest: snapshotDigest(string(b))}
	if _, err := a.Decode(); err != nil {
		t.Fatal(err)
	}
	return a
}

func decodeSnapshot(t *testing.T, a ontologysnapshot.Artifact) ontologysnapshot.Document {
	t.Helper()
	d, err := a.Decode()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func retainedImport(t *testing.T, p *pgxpool.Pool, project string) ontologysnapshot.Artifact {
	t.Helper()
	var a ontologysnapshot.Artifact
	if err := p.QueryRow(t.Context(), `SELECT document,digest FROM weave_imported_legacy_ontology_snapshots WHERE project_id=$1 AND version_number='1'`, project).Scan(&a.JSON, &a.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Decode(); err != nil {
		t.Fatal(err)
	}
	return a
}

func requireCount(t *testing.T, p *pgxpool.Pool, query string, want int) {
	t.Helper()
	var got int
	if err := p.QueryRow(t.Context(), query).Scan(&got); err != nil || got != want {
		t.Fatalf("count: got %d want %d: %v (%s)", got, want, err, query)
	}
}

func requireRestoreConflict(t *testing.T, err error) {
	t.Helper()
	if err == nil || apierror.FromError(err).Code != apierror.CodeConflict {
		t.Fatalf("want restore conflict, got %v", err)
	}
}

// A different JSON rendering is the same indexed body but a different exact
// body identity. All candidate evidence must continue to bind those exact bytes.
func reformatSnapshotBody(d *ontologysnapshot.Document, version string) {
	for i := range d.Inventory {
		b := &d.Inventory[i]
		if b.VersionID != version {
			continue
		}
		b.BodyJSON += "\n"
		b.Digest = snapshotDigest(b.BodyJSON)
		for j := range d.Assessments {
			for k := range d.Assessments[j].Candidates {
				c := &d.Assessments[j].Candidates[k]
				if c.VersionID == version {
					c.Digest = b.Digest
				}
			}
		}
	}
}

func TestRestoreLegacyEvidencePreservesBodiesSourcesAndPolicy(t *testing.T) {
	sourceDB := fixture(t)
	exec(t, sourceDB, `INSERT INTO weave_ontology_properties(id,ontology_version_id,prefix,local_name,uri,is_transitive,source_module) VALUES ('restore-property','crm-6.2','crm','P1','https://example.org/crm/P1',true,'companion.rdf')`)
	exec(t, sourceDB, `INSERT INTO weave_ontology_relations(ontology_version_id,source_id,source_kind,rel_type,target_qname,position) VALUES ('crm-6.2','restore-property','property','domain','crm:E1',0)`)
	exec(t, sourceDB, `INSERT INTO weave_ontology_version_companions(ontology_version_id,filename,content,position) VALUES ('crm-6.2','companion.rdf','opaque original source bytes',0)`)
	exec(t, sourceDB, `UPDATE weave_ontology_versions SET property_count=1 WHERE id='crm-6.2'`)
	source := capture(t, sourceDB, "source")
	root := capture(t, sourceDB, "legacy-project")
	d := decodeSnapshot(t, root)
	reformatSnapshotBody(&d, "crm-6.2")
	root = snapshotValue(t, d)
	target := testdb.EmptyPool(t)
	exec(t, target, `INSERT INTO weave_namespace_bindings(id,prefix,namespace,weight,source,created_at,updated_at) VALUES ('destination','crm','https://destination.example/',99,'user',now(),now())`)
	if err := ontologysnapshot.Restore(t.Context(), target, []ontologysnapshot.Artifact{source}); err != nil {
		t.Fatal(err)
	}
	requireCount(t, target, `SELECT count(*) FROM weave_ontology_active_versions`, 0)
	exec(t, target, `SELECT weave_set_ontology_active_version('snapshot-crm','crm-7.1.3',false)`)
	var policy string
	if err := target.QueryRow(t.Context(), `SELECT row_to_json(p)::text FROM weave_ontology_active_versions p`).Scan(&policy); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := ontologysnapshot.Restore(t.Context(), target, []ontologysnapshot.Artifact{root, source, root}); err != nil {
			t.Fatal(err)
		}
	}
	if got := retainedImport(t, target, "legacy-project"); got != root {
		t.Fatal("changed imported document bytes")
	}
	if got := retainedImport(t, target, "source"); got != source {
		t.Fatal("changed source document bytes")
	}
	var after string
	if err := target.QueryRow(t.Context(), `SELECT row_to_json(p)::text FROM weave_ontology_active_versions p`).Scan(&after); err != nil || after != policy {
		t.Fatalf("changed existing policy: %v", err)
	}
	requireCount(t, target, `SELECT count(*) FROM weave_ontology_body_seals`, 2)
	requireCount(t, target, `SELECT count(*) FROM weave_imported_legacy_ontology_snapshots`, 2)
	requireCount(t, target, `SELECT (SELECT count(*) FROM weave_releases)+(SELECT count(*) FROM weave_legacy_ontology_snapshots)+(SELECT count(*) FROM weave_ontology_releases)`, 0)
	requireCount(t, target, `SELECT count(*) FROM weave_namespace_bindings WHERE id='destination' AND namespace='https://destination.example/'`, 1)
	requireCount(t, target, `SELECT count(*) FROM weave_ontology_properties WHERE id='restore-property' AND is_transitive AND source_module='companion.rdf'`, 1)
	requireCount(t, target, `SELECT count(*) FROM weave_ontology_relations WHERE source_id='restore-property' AND target_qname='crm:E1'`, 1)
	requireCount(t, target, `SELECT count(*) FROM weave_ontology_version_companions WHERE content='opaque original source bytes'`, 1)
	for _, b := range d.Inventory {
		ref, err := ontologybody.NewReference(ontologybody.OntologyID(b.OntologyID), ontologybody.VersionID(b.VersionID))
		if err != nil {
			t.Fatal(err)
		}
		if err := ontologybody.NewFactory(target).Read(t.Context(), ref, func(r ontologybody.Reader) error {
			got, err := r.Body(t.Context())
			if err == nil && (got.CanonicalJSON != b.BodyJSON || got.Digest != b.Digest) {
				return fmt.Errorf("changed restored body bytes")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	ref, err := root.Reference()
	if err != nil {
		t.Fatal(err)
	}
	if err := ontologysnapshot.NewFactory(target).Read(t.Context(), ref, func(ontologysnapshot.Reader) error { return nil }); err == nil {
		t.Fatal("import became a local capture")
	}
	if _, err := retainedImport(t, target, "legacy-project").Resolve("field", "source-ref", "/path_elements/0"); err != nil {
		t.Fatalf("lost imported source interpretation: %v", err)
	}
	// A later, explicitly created local capture also reads transported body_text.
	exec(t, target, `INSERT INTO weave_actors(id,type,slug,display_name) VALUES ('recapture-owner','organization','recapture-owner','Owner')`)
	exec(t, target, `INSERT INTO weave_projects(id,owner_id) VALUES ('recapture','recapture-owner')`)
	exec(t, target, `INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('recapture','1','recapture-owner')`)
	exec(t, target, `INSERT INTO weave_project_ontology_versions_archive(project_id,version_number,ontology_version_id,added_at,is_primary) VALUES ('recapture','1','crm-6.2',now(),true)`)
	got := decodeSnapshot(t, capture(t, target, "recapture"))
	if got.Inventory[0].BodyJSON != d.Inventory[0].BodyJSON || got.Inventory[0].Digest != d.Inventory[0].Digest {
		t.Fatal("local capture normalized the restored body")
	}
}

func TestRestoreLegacyReusesLocalPublishedSealWithoutPromotion(t *testing.T) {
	p := fixture(t)
	a := capture(t, p, "source")
	b := decodeSnapshot(t, a).Inventory[0]
	doc := ontologyrelease.Document{FormatVersion: 1, OntologyID: b.OntologyID, VersionID: b.VersionID,
		BodyJSON: b.BodyJSON, BodyDigest: b.Digest, Pins: []ontologyrelease.Pin{},
		Requirements: []ontologyrelease.Requirement{{Kind: "defaults", Disposition: "absent"}}}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := p.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	if _, err := tx.Exec(t.Context(), `INSERT INTO weave_ontology_releases(ontology_id,version_id,artifact,digest) VALUES ($1,$2,$3,$4)`, b.OntologyID, b.VersionID, string(data), snapshotDigest(string(data))); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := p.QueryRow(t.Context(), `SELECT row_to_json(s)::text FROM weave_ontology_body_seals s WHERE version_id='crm-7.1.3'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := ontologysnapshot.Restore(t.Context(), p, []ontologysnapshot.Artifact{a}); err != nil {
		t.Fatal(err)
	}
	var after string
	if err := p.QueryRow(t.Context(), `SELECT row_to_json(s)::text FROM weave_ontology_body_seals s WHERE version_id='crm-7.1.3'`).Scan(&after); err != nil || before != after {
		t.Fatalf("reused seal changed: %v", err)
	}
	requireCount(t, p, `SELECT count(*) FROM weave_ontology_releases`, 1)
	requireCount(t, p, `SELECT count(*) FROM weave_ontology_active_versions`, 0)
}

func TestRestoreLegacyCollisionAndInvalidBundleAreAtomic(t *testing.T) {
	p := fixture(t)
	source := capture(t, p, "source")
	root := capture(t, p, "legacy-project")
	t.Run("unsealed late collision", func(t *testing.T) {
		target := testdb.EmptyPool(t)
		exec(t, target, `INSERT INTO weave_ontologies(id,prefix,namespace,name) VALUES ('snapshot-crm','destination','https://destination/','existing')`)
		exec(t, target, `INSERT INTO weave_ontology_versions(id,ontology_id,version_string) VALUES ('crm-7.1.3','snapshot-crm','7.1.3')`)
		requireRestoreConflict(t, ontologysnapshot.Restore(t.Context(), target, []ontologysnapshot.Artifact{root}))
		requireCount(t, target, `SELECT count(*) FROM weave_ontology_versions`, 1)
		requireCount(t, target, `SELECT (SELECT count(*) FROM weave_ontology_body_seals)+(SELECT count(*) FROM weave_imported_legacy_ontology_snapshots)`, 0)
	})
	t.Run("sealed conflict", func(t *testing.T) {
		target := testdb.EmptyPool(t)
		if err := ontologysnapshot.Restore(t.Context(), target, []ontologysnapshot.Artifact{source}); err != nil {
			t.Fatal(err)
		}
		d := decodeSnapshot(t, source)
		reformatSnapshotBody(&d, "crm-7.1.3")
		requireRestoreConflict(t, ontologysnapshot.Restore(t.Context(), target, []ontologysnapshot.Artifact{snapshotValue(t, d)}))
		if retainedImport(t, target, "source") != source {
			t.Fatal("overwrote retained source")
		}
	})
	t.Run("same origin different document", func(t *testing.T) {
		target := testdb.EmptyPool(t)
		if err := ontologysnapshot.Restore(t.Context(), target, []ontologysnapshot.Artifact{root}); err != nil {
			t.Fatal(err)
		}
		other := ontologysnapshot.Artifact{JSON: root.JSON + "\n", Digest: snapshotDigest(root.JSON + "\n")}
		requireRestoreConflict(t, ontologysnapshot.Restore(t.Context(), target, []ontologysnapshot.Artifact{source, other}))
		requireCount(t, target, `SELECT count(*) FROM weave_imported_legacy_ontology_snapshots`, 1)
		if retainedImport(t, target, "legacy-project") != root {
			t.Fatal("overwrote retained root")
		}
	})
	t.Run("invalid last artifact", func(t *testing.T) {
		target := testdb.EmptyPool(t)
		bad := root
		bad.JSON += "\n"
		if err := ontologysnapshot.Restore(t.Context(), target, []ontologysnapshot.Artifact{source, bad}); err == nil {
			t.Fatal("accepted invalid digest")
		}
		requireCount(t, target, `SELECT (SELECT count(*) FROM weave_ontologies)+(SELECT count(*) FROM weave_imported_legacy_ontology_snapshots)`, 0)
	})
}

func TestRestoreLegacyStorageGuards(t *testing.T) {
	p := fixture(t)
	a := capture(t, p, "source")
	if err := ontologysnapshot.Restore(t.Context(), p, []ontologysnapshot.Artifact{a}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE weave_imported_legacy_ontology_snapshots SET document=document`,
		`DELETE FROM weave_imported_legacy_ontology_snapshots`,
		`TRUNCATE weave_imported_legacy_ontology_snapshots CASCADE`,
	} {
		_, err := p.Exec(t.Context(), statement)
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != "23514" || pgerr.ConstraintName != "legacy_ontology_import_conflict" {
			t.Fatalf("missing immutable guard for %s: %v", statement, err)
		}
	}
	_, err := p.Exec(t.Context(), `INSERT INTO weave_imported_legacy_ontology_snapshots(project_id,version_number,document,digest) VALUES ('elsewhere','1',$1,$2)`, a.JSON, a.Digest)
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.ConstraintName != "legacy_ontology_import_conflict" {
		t.Fatalf("raw writer forged origin: %v", err)
	}
	empty := testdb.EmptyPool(t)
	_, err = empty.Exec(t.Context(), `INSERT INTO weave_imported_legacy_ontology_snapshots(project_id,version_number,document,digest) VALUES ('source','1',$1,$2)`, a.JSON, a.Digest)
	if !errors.As(err, &pgerr) || pgerr.ConstraintName != "legacy_ontology_import_conflict" {
		t.Fatalf("raw writer referenced missing body: %v", err)
	}
	db := stdlib.OpenDB(*p.Config().ConnConfig)
	defer db.Close()
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version > 39; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "cannot discard imported legacy ontology evidence") {
		t.Fatalf("unsafe imported-evidence downgrade: %v", err)
	}
}

func TestImportedReaderIsSeparateBoundAndExact(t *testing.T) {
	p := fixture(t)
	a := capture(t, p, "source")
	ref, err := a.Reference()
	if err != nil {
		t.Fatal(err)
	}
	factory := ontologysnapshot.NewFactory(p)
	if err := factory.ReadImported(t.Context(), ref, func(ontologysnapshot.Reader) error { return nil }); err == nil {
		t.Fatal("imported reader fell back to local capture")
	}
	if err := ontologysnapshot.Restore(t.Context(), p, []ontologysnapshot.Artifact{a}); err != nil {
		t.Fatal(err)
	}
	var escaped ontologysnapshot.Reader
	if err := factory.ReadImported(t.Context(), ref, func(r ontologysnapshot.Reader) error {
		escaped = r
		got, err := r.Snapshot(t.Context())
		if err == nil && got != a {
			return fmt.Errorf("imported reader changed artifact")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := escaped.Snapshot(t.Context()); err == nil {
		t.Fatal("escaped imported reader remained usable")
	}
	for _, identity := range [][3]string{{"elsewhere", "1", a.Digest}, {"source", "2", a.Digest}, {"source", "1", strings.Repeat("0", 64)}} {
		wrong, err := ontologysnapshot.NewReference(identity[0], identity[1], identity[2])
		if err != nil {
			t.Fatal(err)
		}
		if err := factory.ReadImported(t.Context(), wrong, func(ontologysnapshot.Reader) error { return nil }); err == nil {
			t.Fatal("imported reader accepted wrong binding")
		}
	}
}

func TestRestoreLegacyBodyValidationIsInDatabase(t *testing.T) {
	p := fixture(t)
	a := capture(t, p, "source")
	b := decodeSnapshot(t, a).Inventory[0]
	for _, mode := range []string{"wrong owner", "wrong digest", "foreign class", "foreign companion", "bad counts", "missing array"} {
		t.Run(mode, func(t *testing.T) {
			target := testdb.EmptyPool(t)
			var body map[string]any
			if err := json.Unmarshal([]byte(b.BodyJSON), &body); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "wrong owner":
				body["ontology"].(map[string]any)["id"] = "elsewhere"
			case "foreign class":
				body["classes"].([]any)[0].(map[string]any)["ontology_version_id"] = "elsewhere"
			case "foreign companion":
				body["companions"] = []any{map[string]any{"ontology_version_id": "elsewhere                 ", "filename": "foreign.rdf", "content": "original", "position": 0}}
			case "bad counts":
				body["version"].(map[string]any)["class_count"] = 2
			case "missing array":
				delete(body, "companions")
			}
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			digest := snapshotDigest(string(data))
			if mode == "wrong digest" {
				digest = strings.Repeat("0", 64)
			}
			tx, err := target.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(t.Context(), `SELECT weave_restore_ontology_body($1,$2,$3,$4)`, b.OntologyID, b.VersionID, string(data), digest)
			_ = tx.Rollback(context.Background())
			var pgerr *pgconn.PgError
			if !errors.As(err, &pgerr) || pgerr.Code != "23514" {
				t.Fatalf("raw body validation %s: %v", mode, err)
			}
			requireCount(t, target, `SELECT count(*) FROM weave_ontology_versions`, 0)
		})
	}
}
