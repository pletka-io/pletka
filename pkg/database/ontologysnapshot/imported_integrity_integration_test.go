//go:build integration

package ontologysnapshot_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
	"github.com/pletka-io/pletka/pkg/namespace"
)

func integrityFixture(t *testing.T) ontologysnapshot.Artifact {
	t.Helper()
	p := fixture(t)
	exec(t, p, `INSERT INTO weave_fields_archive(id,project_id,version_number,path_elements) VALUES ('extra-references','legacy-project','1','[{"type":"complete","additional_types":[{"uri":"crm:E1"}],"datatype":"rdf:literal"},{"type":"class","uri":"crm:E1"},{"type":"class","uri":"crm:E1"}]')`)
	capture(t, p, "source")
	return capture(t, p, "legacy-project")
}

// Seed only exact bodies, so a rejected raw import must leave no imported keys
// or ledger entries. This deliberately exercises the SQL boundary without Go's
// Artifact.Decode preflight.
func restoreIntegrityBodies(t *testing.T, p *pgxpool.Pool, a ontologysnapshot.Artifact, whitespace bool) {
	t.Helper()
	tx, err := p.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	for _, b := range decodeSnapshot(t, a).Inventory {
		if whitespace {
			b.BodyJSON += "\n"
			b.Digest = snapshotDigest(b.BodyJSON)
		}
		if _, err := tx.Exec(t.Context(), `SELECT weave_restore_ontology_body($1,$2,$3,$4)`, b.OntologyID, b.VersionID, b.BodyJSON, b.Digest); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func integrityRoot(t *testing.T, a ontologysnapshot.Artifact, project string, conflictingSource bool) ontologysnapshot.Artifact {
	t.Helper()
	d := decodeSnapshot(t, a)
	d.ProjectID = project
	for i := range d.Inventory {
		var selection map[string]any
		if err := json.Unmarshal(d.Inventory[i].Selection, &selection); err != nil {
			t.Fatal(err)
		}
		selection["project_id"] = project
		data, err := json.Marshal(selection)
		if err != nil {
			t.Fatal(err)
		}
		d.Inventory[i].Selection = data
	}
	for i := range d.Dependencies {
		d.Dependencies[i].ProjectID = project
	}
	if conflictingSource {
		var source ontologysnapshot.Document
		if err := json.Unmarshal([]byte(d.Sources[0].JSON), &source); err != nil {
			t.Fatal(err)
		}
		// This alias is unused, so each individual artifact is valid. Only the
		// shared origin's retained interpretation context changes.
		source.Bindings = append(source.Bindings, namespace.Binding{Prefix: "unused", Namespace: "https://other.example/"})
		data, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		d.Sources[0] = ontologysnapshot.Source{JSON: string(data), Digest: snapshotDigest(string(data))}
		d.SourceRefs[0].Digest = d.Sources[0].Digest
	}
	return snapshotValue(t, d)
}

func requireIntegrityError(t *testing.T, err error) {
	t.Helper()
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "23514" || pgerr.ConstraintName != "legacy_ontology_import_conflict" {
		t.Fatalf("want imported evidence integrity error, got %v", err)
	}
}

func rawIntegrityImport(ctx context.Context, q interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, project, payload string) error {
	_, err := q.Exec(ctx, `INSERT INTO weave_imported_legacy_ontology_snapshots(project_id,version_number,document,digest) VALUES ($1,'1',$2,$3)`, project, payload, snapshotDigest(payload))
	return err
}

func TestImportedIntegrityRejectsRawGraphCoverageAndShapesWithoutPoison(t *testing.T) {
	a := integrityFixture(t)
	p := testdb.EmptyPool(t)
	restoreIntegrityBodies(t, p, a, false)
	for _, mode := range []string{
		"missing source", "wrong source digest", "unreferenced source", "duplicate source", "duplicate source ref",
		"missing assessment", "duplicate assessment", "forged assessment", "changed original", "duplicate element",
		"numeric dependency", "numeric source ref", "numeric selection", "source format fraction", "nested additional shape",
		"empty duplicate element", "empty duplicate dependency", "empty dependency overlap",
	} {
		t.Run(mode, func(t *testing.T) {
			var d map[string]any
			if err := json.Unmarshal([]byte(a.JSON), &d); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "missing source":
				d["sources"] = []any{}
			case "wrong source digest":
				d["source_refs"].([]any)[0].(map[string]any)["digest"] = strings.Repeat("0", 64)
			case "unreferenced source":
				d["source_refs"], d["dependencies"] = []any{}, []any{}
			case "duplicate source":
				d["sources"] = append(d["sources"].([]any), d["sources"].([]any)[0])
			case "duplicate source ref":
				d["source_refs"] = append(d["source_refs"].([]any), d["source_refs"].([]any)[0])
			case "missing assessment":
				d["assessments"] = d["assessments"].([]any)[1:]
			case "duplicate assessment":
				d["assessments"] = append(d["assessments"].([]any), d["assessments"].([]any)[0])
			case "forged assessment":
				d["assessments"].([]any)[0].(map[string]any)["status"] = "invented"
			case "changed original":
				d["elements"].([]any)[0].(map[string]any)["original"] = map[string]any{"type": "class", "uri": "https://changed/"}
			case "duplicate element":
				d["elements"] = append(d["elements"].([]any), d["elements"].([]any)[0])
			case "empty duplicate element":
				d["elements"] = append(d["elements"].([]any), map[string]any{"original": nil}, map[string]any{"entity_kind": "", "entity_id": "", "location": "", "original": nil})
			case "empty duplicate dependency":
				d["dependencies"] = append(d["dependencies"].([]any),
					map[string]any{"project_id": "legacy-project", "version_number": "1", "source_project_id": "source", "source_version": "1"},
					map[string]any{"project_id": "legacy-project", "version_number": "1", "source_project_id": "source", "source_version": "1", "entity_type": "", "entity_id": ""})
			case "empty dependency overlap":
				d["elements"] = append(d["elements"].([]any), map[string]any{"entity_kind": "", "entity_id": "", "original": nil})
				d["dependencies"] = append(d["dependencies"].([]any), map[string]any{"project_id": "legacy-project", "version_number": "1", "source_project_id": "source", "source_version": "1"})
			case "numeric dependency":
				d["dependencies"].([]any)[0].(map[string]any)["entity_id"] = 123
			case "numeric source ref":
				d["source_refs"].([]any)[0].(map[string]any)["project_id"] = 123
			case "numeric selection":
				d["inventory"].([]any)[0].(map[string]any)["selection"].(map[string]any)["version_number"] = 1
			case "source format fraction":
				s := d["sources"].([]any)[0].(map[string]any)
				original := s["json"].(string)
				changed := strings.Replace(original, `"format_version": 1`, `"format_version": 1.0`, 1)
				if changed == original {
					t.Fatal("fixture source format not found")
				}
				s["json"], s["digest"] = changed, snapshotDigest(changed)
				d["source_refs"].([]any)[0].(map[string]any)["digest"] = s["digest"]
			case "nested additional shape":
				d["elements"].([]any)[0].(map[string]any)["original"] = map[string]any{"additional_types": []any{map[string]any{"additional_types": "not an array"}}}
			}
			data, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			requireIntegrityError(t, rawIntegrityImport(t.Context(), p, "legacy-project", string(data)))
			requireCount(t, p, `SELECT count(*) FROM weave_imported_legacy_ontology_snapshots`, 0)
			requireCount(t, p, `SELECT count(*) FROM weave_imported_legacy_ontology_contexts`, 0)
		})
	}
	if err := rawIntegrityImport(t.Context(), p, "legacy-project", a.JSON); err != nil {
		t.Fatalf("rejected raw input poisoned the valid origin: %v", err)
	}
	requireCount(t, p, `SELECT count(*) FROM weave_imported_legacy_ontology_contexts`, 2)
	requireCount(t, p, `SELECT (SELECT count(*) FROM weave_releases)+(SELECT count(*) FROM weave_ontology_releases)+(SELECT count(*) FROM weave_ontology_active_versions)`, 0)
}

func TestImportedIntegrityPersistsContextsAcrossRestores(t *testing.T) {
	a := integrityFixture(t)
	p := testdb.EmptyPool(t)
	if err := ontologysnapshot.Restore(t.Context(), p, []ontologysnapshot.Artifact{a}); err != nil {
		t.Fatal(err)
	}
	bad := integrityRoot(t, a, "other-root", true)
	requireRestoreConflict(t, ontologysnapshot.Restore(t.Context(), p, []ontologysnapshot.Artifact{bad}))
	requireCount(t, p, `SELECT count(*) FROM weave_imported_legacy_ontology_snapshots`, 1)
	requireCount(t, p, `SELECT count(*) FROM weave_imported_legacy_ontology_contexts`, 2)
	good := integrityRoot(t, a, "other-root", false)
	if err := ontologysnapshot.Restore(t.Context(), p, []ontologysnapshot.Artifact{good}); err != nil {
		t.Fatal(err)
	}
	if retainedImport(t, p, "legacy-project") != a || retainedImport(t, p, "other-root") != good {
		t.Fatal("ledger normalized original artifact bytes")
	}
	var body string
	if err := p.QueryRow(t.Context(), `SELECT context_document#>>'{inventory,0,body_json}' FROM weave_imported_legacy_ontology_contexts WHERE project_id='legacy-project'`).Scan(&body); err != nil || body != decodeSnapshot(t, a).Inventory[0].BodyJSON {
		t.Fatalf("ledger changed body_json string: %v", err)
	}
	for _, statement := range []string{
		`UPDATE weave_imported_legacy_ontology_contexts SET context_document=context_document`,
		`DELETE FROM weave_imported_legacy_ontology_contexts`,
		`TRUNCATE weave_imported_legacy_ontology_contexts`,
		`INSERT INTO weave_imported_legacy_ontology_contexts(project_id,version_number,context_document,retained_project_id,retained_version) VALUES ('fake','1','{"project_id":"fake","version":"1"}','legacy-project','1')`,
	} {
		_, err := p.Exec(t.Context(), statement)
		requireIntegrityError(t, err)
	}
}

func waitForIntegrityBlock(t *testing.T, p *pgxpool.Pool, pid uint32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := p.QueryRow(t.Context(), `SELECT cardinality(pg_blocking_pids($1::int))>0`, pid).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("concurrent import did not wait on origin fence")
}

func TestImportedIntegritySerializesConcurrentOrigins(t *testing.T) {
	a := integrityFixture(t)
	for _, conflict := range []bool{false, true} {
		name := "identical shared context"
		if conflict {
			name = "conflicting shared context"
		}
		t.Run(name, func(t *testing.T) {
			p := testdb.EmptyPool(t)
			restoreIntegrityBodies(t, p, a, false)
			b := integrityRoot(t, a, "other-root", conflict)
			first, err := p.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
			if err != nil {
				t.Fatal(err)
			}
			defer first.Rollback(context.Background()) //nolint:errcheck
			if err := rawIntegrityImport(t.Context(), first, "legacy-project", a.JSON); err != nil {
				t.Fatal(err)
			}
			second, err := p.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
			if err != nil {
				t.Fatal(err)
			}
			defer second.Rollback(context.Background()) //nolint:errcheck
			var pid uint32
			if err := second.QueryRow(t.Context(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			go func() { result <- rawIntegrityImport(ctx, second, "other-root", b.JSON) }()
			waitForIntegrityBlock(t, p, pid)
			if err := first.Commit(t.Context()); err != nil {
				t.Fatal(err)
			}
			err = <-result
			var pgerr *pgconn.PgError
			if !errors.As(err, &pgerr) || pgerr.Code != "40001" {
				t.Fatalf("stale concurrent snapshot should retry, got %v", err)
			}
			_ = second.Rollback(t.Context())
			requireCount(t, p, `SELECT count(*) FROM weave_imported_legacy_ontology_snapshots`, 1)
			err = ontologysnapshot.Restore(t.Context(), p, []ontologysnapshot.Artifact{b})
			if conflict {
				requireRestoreConflict(t, err)
			} else if err != nil {
				t.Fatalf("identical context retry: %v", err)
			}
		})
	}
}

func TestImportedIntegrityUpgradeAndDowngrade(t *testing.T) {
	a := integrityFixture(t)
	for _, mode := range []string{"empty", "valid existing", "invalid existing", "portable body only"} {
		t.Run(mode, func(t *testing.T) {
			p := testdb.EmptyPool(t)
			db := stdlib.OpenDB(*p.Config().ConnConfig)
			defer db.Close()
			if mode == "portable body only" {
				restoreIntegrityBodies(t, p, a, true)
				requireCount(t, p, `SELECT count(*) FROM weave_imported_legacy_ontology_snapshots`, 0)
				if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "exact portable body context") {
					t.Fatalf("noncanonical body downgrade was allowed: %v", err)
				}
				return
			}
			if err := database.MigrateDown(db); err != nil {
				t.Fatal(err)
			}
			if mode != "empty" {
				restoreIntegrityBodies(t, p, a, false)
				payload := a.JSON
				if mode == "invalid existing" {
					d := decodeSnapshot(t, a)
					d.Assessments = nil
					data, err := json.Marshal(d)
					if err != nil {
						t.Fatal(err)
					}
					payload = string(data)
				}
				if err := rawIntegrityImport(t.Context(), p, "legacy-project", payload); err != nil {
					t.Fatal(err)
				}
			}
			err := database.Migrate(db)
			if mode == "invalid existing" {
				if err == nil || !strings.Contains(err.Error(), "assessments differ") {
					t.Fatalf("invalid import was grandfathered: %v", err)
				}
				var absent bool
				if err := p.QueryRow(t.Context(), `SELECT to_regclass('weave_imported_legacy_ontology_contexts') IS NULL`).Scan(&absent); err != nil || !absent {
					t.Fatalf("failed upgrade left ledger: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if mode == "valid existing" {
				requireCount(t, p, `SELECT count(*) FROM weave_imported_legacy_ontology_contexts`, 2)
			} else {
				requireCount(t, p, `SELECT count(*) FROM weave_imported_legacy_ontology_contexts`, 0)
			}
		})
	}
}
