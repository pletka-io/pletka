//go:build integration

package release

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestSnapshotIncludesExamples pins that a release carries the examples a
// curator authored. Before this, a release snapshot omitted them entirely, so
// a pinned read showed today's examples under an old tag.
//
// Also pins the empty case, which is the common one: most projects have no
// examples, and the statement must write nothing rather than error.
func TestSnapshotIncludesExamples(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const projectID, version = "SNAPEX", "1.0.0"

	seedProject(t, pool, projectID)
	overrideID := seedExampleField(t, pool, "SNAPEXF.1", "model", "SNAPEXM.1")
	mustExec(t, pool, `INSERT INTO weave_examples (id, project_id, entity_type, entity_id, title, status, version_number)
		VALUES ('ex1', $1, 'model', 'SNAPEXM.1', '{"en":"An example"}'::jsonb, 'valid', '')`, projectID)
	mustExec(t, pool, `INSERT INTO weave_example_values
		(example_id, override_id, field_id, occurrence_index, value_kind, value_payload, text_value, version_number, slot_path)
		VALUES ('ex1', $1, 'SNAPEXF.1', 0, 'string', '{"text":"hello"}'::jsonb, 'hello', '', 'root')`, overrideID)
	// A second example with a different validation status: weave_examples.status
	// is a validation vocabulary (draft/valid/has_issues), not the
	// draft/published entity vocabulary — the snapshot statement must copy it
	// verbatim, not normalize it to a single stamped value.
	mustExec(t, pool, `INSERT INTO weave_examples (id, project_id, entity_type, entity_id, title, status, version_number)
		VALUES ('ex2', $1, 'model', 'SNAPEXM.1', '{"en":"An example with issues"}'::jsonb, 'has_issues', '')`, projectID)

	if err := runSnapshotStatements(ctx, pool, projectID, version); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	var examples, values int
	mustScan(t, pool, `SELECT count(*) FROM weave_examples_archive WHERE project_id=$1 AND version_number=$2`,
		[]any{projectID, version}, &examples)
	mustScan(t, pool, `SELECT count(*) FROM weave_example_values_archive WHERE version_number=$1`,
		[]any{version}, &values)
	if examples != 2 || values != 1 {
		t.Errorf("archived %d examples and %d values, want 2 and 1", examples, values)
	}

	var ex1Status, ex2Status string
	mustScan(t, pool, `SELECT status FROM weave_examples_archive WHERE id='ex1' AND project_id=$1 AND version_number=$2`,
		[]any{projectID, version}, &ex1Status)
	mustScan(t, pool, `SELECT status FROM weave_examples_archive WHERE id='ex2' AND project_id=$1 AND version_number=$2`,
		[]any{projectID, version}, &ex2Status)
	if ex1Status != "valid" || ex2Status != "has_issues" {
		t.Errorf("archived statuses ex1=%q ex2=%q, want ex1=%q ex2=%q — a row-count-only check would pass even if both were stamped 'published'",
			ex1Status, ex2Status, "valid", "has_issues")
	}

	t.Run("a project with no examples snapshots cleanly", func(t *testing.T) {
		const empty = "SNAPEXEMPTY"
		seedProject(t, pool, empty)
		if err := runSnapshotStatements(ctx, pool, empty, version); err != nil {
			t.Fatalf("snapshot of an example-less project must not error: %v", err)
		}
	})

	t.Run("re-running the snapshot is a no-op", func(t *testing.T) {
		if err := runSnapshotStatements(ctx, pool, projectID, version); err != nil {
			t.Fatalf("second snapshot: %v", err)
		}
		var again int
		mustScan(t, pool, `SELECT count(*) FROM weave_examples_archive WHERE project_id=$1 AND version_number=$2`,
			[]any{projectID, version}, &again)
		if again != 2 {
			t.Errorf("archived %d examples after a re-run, want 2 — the statement is not idempotent", again)
		}
	})
}

// seedProject creates a minimal owner actor + project so FK-bound rows
// (weave_examples.project_id, weave_projects.owner_id) have somewhere valid
// to point. Registers cleanup for everything this test file writes under
// projectID, including the archive rows the snapshot statements produce
// (archive tables carry no FK to the live rows, so nothing cascades them).
func seedProject(t *testing.T, pool *pgxpool.Pool, projectID string) {
	t.Helper()
	ownerID := projectID + "_OWNER"

	mustExec(t, pool, `INSERT INTO weave_actors (id, type, display_name, slug, visibility)
		VALUES ($1, 'organization', $1, lower($1), 'private')
		ON CONFLICT (id) DO NOTHING`, ownerID)
	mustExec(t, pool, `INSERT INTO weave_projects (id, owner_id, ui_name, status, visibility)
		VALUES ($1, $2, '{"en":"Snapshot examples test"}'::jsonb, 'draft', 'private')
		ON CONFLICT (id) DO NOTHING`, projectID, ownerID)

	t.Cleanup(func() {
		bg := context.Background()
		stmts := []string{
			`DELETE FROM weave_example_values_archive WHERE example_id IN (SELECT id FROM weave_examples WHERE project_id=$1)`,
			`DELETE FROM weave_examples_archive WHERE project_id=$1`,
			`DELETE FROM weave_field_overrides WHERE project_id=$1`,
			`DELETE FROM weave_examples WHERE project_id=$1`,
			`DELETE FROM weave_fields WHERE project_id=$1`,
			`DELETE FROM weave_projects WHERE id=$1`,
		}
		for _, stmt := range stmts {
			if _, err := pool.Exec(bg, stmt, projectID); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
		if _, err := pool.Exec(bg, `DELETE FROM weave_actors WHERE id=$1`, ownerID); err != nil {
			t.Errorf("cleanup owner actor: %v", err)
		}
	})
}

// exampleFieldFixtureProject is deliberately not projectID: weave_fields and
// weave_field_overrides carry no FK to weave_projects, and the existing
// weave_fields_archive / weave_field_overrides_archive snapshot statements
// have no ON CONFLICT clause (they rely on Create only ever archiving a
// given version once — see the sibling statements in service.go). Housing
// the fixture field/override under a project id runSnapshotStatements never
// archives keeps the "re-run is a no-op" subtest below scoped to the two
// example statements this task adds, instead of tripping over that
// pre-existing, unrelated non-idempotency.
const exampleFieldFixtureProject = "SNAPEXFX"

// seedExampleField creates a minimal field and base override so a
// weave_example_values row has a valid field_id/override_id to reference —
// both are hard, non-cascading foreign keys. Returns the generated override
// id. Cleans up after itself; independent of seedProject's cleanup since it
// seeds under exampleFieldFixtureProject, not the caller's project id.
func seedExampleField(t *testing.T, pool *pgxpool.Pool, fieldID, entityType, entityID string) int64 {
	t.Helper()
	ctx := context.Background()
	mustExec(t, pool, `INSERT INTO weave_fields (id, project_id, ui_name, status, path_elements)
		VALUES ($1, $2, '{"en":"Snapshot field"}'::jsonb, 'draft', '[]'::jsonb)
		ON CONFLICT (id) DO NOTHING`, fieldID, exampleFieldFixtureProject)
	var overrideID int64
	if err := pool.QueryRow(ctx, `INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id)
		VALUES ($1, $2, $3, $4) RETURNING id`, fieldID, exampleFieldFixtureProject, entityType, entityID).Scan(&overrideID); err != nil {
		t.Fatalf("seed field override: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		if _, err := pool.Exec(bg, `DELETE FROM weave_field_overrides WHERE id=$1`, overrideID); err != nil {
			t.Errorf("cleanup field override: %v", err)
		}
		if _, err := pool.Exec(bg, `DELETE FROM weave_fields WHERE id=$1 AND project_id=$2`, fieldID, exampleFieldFixtureProject); err != nil {
			t.Errorf("cleanup field: %v", err)
		}
	})
	return overrideID
}

// runSnapshotStatements executes the package's snapshotStatements against
// pool in order, with $1 = projectID and $2 = version — the exact list and
// parameters Service.Create runs in its own transaction, so a statement that
// only works inside Create cannot pass here.
func runSnapshotStatements(ctx context.Context, pool *pgxpool.Pool, projectID, version string) error {
	for _, stmt := range snapshotStatements {
		if _, err := pool.Exec(ctx, stmt, projectID, version); err != nil {
			return err
		}
	}
	return nil
}

// mustExec runs a seed statement and fails the test immediately on error.
func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// mustScan runs a single-row query and scans it into dest, failing the test
// immediately on error.
func mustScan(t *testing.T, pool *pgxpool.Pool, sql string, args []any, dest ...any) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(dest...); err != nil {
		t.Fatalf("scan %q: %v", sql, err)
	}
}
