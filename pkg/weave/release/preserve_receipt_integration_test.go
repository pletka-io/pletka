//go:build integration

package release

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
)

// The preserve path writes the import ledger before finalization, so it must
// compute the exact receipt migration 041's finalizer will compute from the
// same archive rows. These functions are that computation, read-only.
func TestLegacyArchiveReceiptMatchesFinalizerShape(t *testing.T) {
	p := testdb.EmptyPool(t)
	ctx := t.Context()
	for _, sql := range []string{
		`INSERT INTO weave_actors(id,type,slug,display_name) VALUES ('rcpt-owner','organization','rcpt-owner','Owner')`,
		`INSERT INTO weave_projects(id,owner_id) VALUES ('rcpt','rcpt-owner')`,
		`INSERT INTO weave_models(id,project_id) VALUES ('rcpt-m1','rcpt'),('rcpt-m2','rcpt')`,
		`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('rcpt','0.1.0','rcpt-owner')`,
	} {
		mustExec(t, p, sql)
	}
	testdb.ArchiveDraft(t, p, "rcpt", "0.1.0")
	var raw []byte
	mustScan(t, p, `SELECT weave_legacy_archive_sections('rcpt','0.1.0')`, nil, &raw)
	var sections map[string]struct {
		Rows   int    `json:"rows"`
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(raw, &sections); err != nil {
		t.Fatal(err)
	}
	var registry int
	mustScan(t, p, `SELECT count(*) FROM weave_content_sections_v1()`, nil, &registry)
	if len(sections) != registry {
		t.Fatalf("sections %d, registry %d", len(sections), registry)
	}
	if sections["weave_models_archive"].Rows != 2 || sections["weave_projects_archive"].Rows != 1 || sections["weave_fields_archive"].Rows != 0 {
		t.Fatalf("wrong row counts: %+v", sections)
	}
	for table, s := range sections {
		if len(s.SHA256) != 64 {
			t.Fatalf("%s digest %q", table, s.SHA256)
		}
	}
	var again []byte
	mustScan(t, p, `SELECT weave_legacy_archive_sections('rcpt','0.1.0')`, nil, &again)
	if string(again) != string(raw) {
		t.Fatal("sections are not stable")
	}
	if _, err := p.Exec(ctx, `SELECT weave_legacy_archive_receipt('rcpt','0.1.0')`); err == nil {
		t.Fatal("receipt without an ontology snapshot")
	}
	// A snapshot row cannot be inserted outside a capture (migration 035
	// trigger), so the receipt's full shape is asserted in the preserve test
	// (Task 4), where it must equal the finalized content_capture_receipt.
}

// Archived project rows from before migrations 031/032 carry NULL concept
// policy, which publication refuses. Migration 046 backfills the only values
// such a cut could have had; a row sealed later is never touched (it has no
// NULLs), so re-running the migration is a no-op.
func TestMigration046BackfillsArchivedConceptPolicy(t *testing.T) {
	p := testdb.EmptyPool(t)
	for _, sql := range []string{
		`INSERT INTO weave_actors(id,type,slug,display_name) VALUES ('bf-owner','organization','bf-owner','Owner')`,
		`INSERT INTO weave_projects(id,owner_id) VALUES ('bf','bf-owner')`,
		`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('bf','0.1.0','bf-owner')`,
		`INSERT INTO weave_projects_archive(id,owner_id,version_number) VALUES ('bf','bf-owner','0.1.0')`,
	} {
		mustExec(t, p, sql)
	}
	db := stdlib.OpenDB(*p.Config().ConnConfig)
	defer db.Close()
	if err := database.MigrateDown(db); err != nil { // 046 down
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil { // 046 up: backfill runs
		t.Fatal(err)
	}
	var effective string
	var enforce bool
	mustScan(t, p, `SELECT effective_concept_namespace, enforce_concept_lists FROM weave_projects_archive WHERE id='bf' AND version_number='0.1.0'`, nil, &effective, &enforce)
	if effective != "https://vocab.pletka.io/" || enforce {
		t.Fatalf("backfill wrong: %q %v", effective, enforce)
	}
}
