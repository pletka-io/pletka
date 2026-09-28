//go:build integration

package release

import (
	"context"
	"fmt"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

// conflictArchiveTables are the six weave_*_archive tables whose snapshot
// statement in snapshotStatements historically carried no ON CONFLICT clause.
// They are safe today only because Service.Create guarantees one run per
// (project, version) — the weave_releases primary key rejects a duplicate
// version before the statements execute. That safety lives one call site
// away from the statements themselves, so re-running the list directly (as
// runSnapshotStatements does here, and as every other test in this package
// does) must not depend on it.
var conflictArchiveTables = []string{
	"weave_categories_archive",
	"weave_fields_archive",
	"weave_models_archive",
	"weave_collections_archive",
	"weave_field_overrides_archive",
	"weave_override_refs_archive",
}

// TestSnapshotRerunDoesNotDuplicateArchiveRows pins that running the full
// snapshotStatements list twice for the same (project, version) is a no-op,
// specifically for the six archive tables above. It asserts both halves:
// a re-run changes nothing, AND the first run wrote everything it should
// have — a conflict target naming the wrong columns could silently drop a
// row that should have been written, and DO NOTHING would make that silent,
// so a test that only checked "no error on re-run" would not catch it.
func TestSnapshotRerunDoesNotDuplicateArchiveRows(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const projectID, version = "SNAPCONF", "1.0.0"

	seedProject(t, pool, projectID)

	// Seed one row reachable by each of the six statements under test, all
	// under projectID (unlike the placements/examples fixture-project
	// workaround, which exists precisely because these statements were not
	// yet idempotent — see the comments there).
	mustExec(t, pool, `INSERT INTO weave_categories (id, project_id, ui_name, status)
		VALUES ('SNAPCONFCAT.1', $1, '{"en":"A category"}'::jsonb, 'draft')`, projectID)
	mustExec(t, pool, `INSERT INTO weave_fields (id, project_id, ui_name, status, path_elements)
		VALUES ('SNAPCONFF.1', $1, '{"en":"A field"}'::jsonb, 'draft', '[]'::jsonb)`, projectID)
	mustExec(t, pool, `INSERT INTO weave_models (id, project_id, ui_name, status)
		VALUES ('SNAPCONFM.1', $1, '{"en":"A model"}'::jsonb, 'draft')`, projectID)
	mustExec(t, pool, `INSERT INTO weave_collections (id, project_id, ui_name, status)
		VALUES ('SNAPCONFC.1', $1, '{"en":"A collection"}'::jsonb, 'draft')`, projectID)

	var overrideID int64
	if err := pool.QueryRow(ctx, `INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id)
		VALUES ('SNAPCONFF.1', $1, 'model', 'SNAPCONFM.1') RETURNING id`, projectID).Scan(&overrideID); err != nil {
		t.Fatalf("seed field override: %v", err)
	}
	mustExec(t, pool, `INSERT INTO weave_override_refs (override_id, ref_type, target_id, semantic_id, position)
		VALUES ($1, 'model', 'SNAPCONFM.1', 'snapconf_ref', 0)`, overrideID)

	t.Cleanup(func() {
		bg := context.Background()
		stmts := []string{
			`DELETE FROM weave_override_refs_archive WHERE project_id=$1`,
			`DELETE FROM weave_field_overrides_archive WHERE project_id=$1`,
			`DELETE FROM weave_fields_archive WHERE project_id=$1`,
			`DELETE FROM weave_models_archive WHERE project_id=$1`,
			`DELETE FROM weave_models WHERE project_id=$1`,
			`DELETE FROM weave_collections_archive WHERE project_id=$1`,
			`DELETE FROM weave_collections WHERE project_id=$1`,
			`DELETE FROM weave_categories_archive WHERE project_id=$1`,
			`DELETE FROM weave_categories WHERE project_id=$1`,
		}
		for _, stmt := range stmts {
			if _, err := pool.Exec(bg, stmt, projectID); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
	})

	if err := runSnapshotStatements(ctx, pool, projectID, version); err != nil {
		t.Fatalf("first snapshot: %v", err)
	}

	first := make(map[string]int, len(conflictArchiveTables))
	for _, table := range conflictArchiveTables {
		var n int
		mustScan(t, pool, fmt.Sprintf(`SELECT count(*) FROM %s WHERE project_id=$1 AND version_number=$2`, table),
			[]any{projectID, version}, &n)
		if n == 0 {
			t.Fatalf("%s archived 0 rows after the first snapshot — seed data must exercise every statement, otherwise the re-run check below is vacuous", table)
		}
		first[table] = n
	}

	if err := runSnapshotStatements(ctx, pool, projectID, version); err != nil {
		t.Fatalf("second snapshot for the same (project, version): %v", err)
	}

	for _, table := range conflictArchiveTables {
		var n int
		mustScan(t, pool, fmt.Sprintf(`SELECT count(*) FROM %s WHERE project_id=$1 AND version_number=$2`, table),
			[]any{projectID, version}, &n)
		if n != first[table] {
			t.Errorf("%s archived %d rows after a re-run, want %d (unchanged) — the statement is not idempotent", table, n, first[table])
		}
	}
}
