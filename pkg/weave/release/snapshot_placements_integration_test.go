//go:build integration

package release

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestSnapshotIncludesCollectionPlacements pins that a release carries a
// project's collection placements. The live table's unique key is
// (model_id, category_id, collection_id) — that composite is what allows more
// than one collection in the same category on the same model, which is
// curator-authored structure a pinned view must not lose. An archive keyed or
// conflicted on (model_id, version_number) would silently keep only one of
// two same-category placements, and a test that merely counted rows for the
// project could still pass — so this test asserts the distinct collections
// and their distinct constraint values, not a count alone.
func TestSnapshotIncludesCollectionPlacements(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const projectID, version = "SNAPPLACE", "1.0.0"

	seedProject(t, pool, projectID)
	// The model/category/collections referenced by the placements live under
	// a separate fixture project id, not projectID, so the "re-run is a
	// no-op" subtest below stays scoped to the placement statement this test
	// covers. Originally this was load-bearing — weave_models_archive,
	// weave_categories_archive and weave_collections_archive had no ON
	// CONFLICT clause — but 3640a35 gave all six a conflict target, so it is
	// now only the narrower, clearer scope. weave_collection_placements
	// carries no FK to weave_models/weave_categories/weave_collections, so
	// the mismatch is harmless.
	mustExec(t, pool, `INSERT INTO weave_models (id, project_id, ui_name, status)
		VALUES ('SNAPPLACEM.1', $1, '{"en":"A model"}'::jsonb, 'draft')`, placementFixtureProject)
	mustExec(t, pool, `INSERT INTO weave_collections (id, project_id, ui_name, status)
		VALUES ('SNAPPLACEC.1', $1, '{"en":"Collection one"}'::jsonb, 'draft')`, placementFixtureProject)
	mustExec(t, pool, `INSERT INTO weave_collections (id, project_id, ui_name, status)
		VALUES ('SNAPPLACEC.2', $1, '{"en":"Collection two"}'::jsonb, 'draft')`, placementFixtureProject)
	mustExec(t, pool, `INSERT INTO weave_categories (id, project_id, ui_name, status)
		VALUES ('SNAPPLACECAT.1', $1, '{"en":"A category"}'::jsonb, 'draft')`, placementFixtureProject)
	t.Cleanup(func() {
		bg := context.Background()
		stmts := []string{
			`DELETE FROM weave_categories WHERE project_id=$1`,
			`DELETE FROM weave_collections WHERE project_id=$1`,
			`DELETE FROM weave_models WHERE project_id=$1`,
		}
		for _, stmt := range stmts {
			if _, err := pool.Exec(bg, stmt, placementFixtureProject); err != nil {
				t.Errorf("cleanup fixture %q: %v", stmt, err)
			}
		}
	})

	// Two placements of DIFFERENT collections in the SAME category on the
	// SAME model — the case the composite key exists for.
	mustExec(t, pool, `INSERT INTO weave_collection_placements
		(project_id, model_id, category_id, collection_id, is_required, min_occurs, max_occurs, is_hidden)
		VALUES ($1, 'SNAPPLACEM.1', 'SNAPPLACECAT.1', 'SNAPPLACEC.1', true, 1, 3, false)`, projectID)
	mustExec(t, pool, `INSERT INTO weave_collection_placements
		(project_id, model_id, category_id, collection_id, is_required, min_occurs, max_occurs, is_hidden)
		VALUES ($1, 'SNAPPLACEM.1', 'SNAPPLACECAT.1', 'SNAPPLACEC.2', false, 0, NULL, true)`, projectID)

	if err := runSnapshotStatements(ctx, pool, projectID, version); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	var archived int
	mustScan(t, pool, `SELECT count(*) FROM weave_collection_placements_archive WHERE project_id=$1 AND version_number=$2`,
		[]any{projectID, version}, &archived)
	if archived != 2 {
		t.Fatalf("archived %d placements, want 2 — a naive archive keyed on (model_id, version_number) would silently keep only one", archived)
	}

	var required1 bool
	var minOccurs1 int
	mustScan(t, pool, `SELECT is_required, min_occurs FROM weave_collection_placements_archive
		WHERE project_id=$1 AND version_number=$2 AND collection_id='SNAPPLACEC.1'`,
		[]any{projectID, version}, &required1, &minOccurs1)
	if !required1 || minOccurs1 != 1 {
		t.Errorf("archived placement for SNAPPLACEC.1: is_required=%v min_occurs=%d, want true 1", required1, minOccurs1)
	}

	var required2 bool
	var minOccurs2 int
	mustScan(t, pool, `SELECT is_required, min_occurs FROM weave_collection_placements_archive
		WHERE project_id=$1 AND version_number=$2 AND collection_id='SNAPPLACEC.2'`,
		[]any{projectID, version}, &required2, &minOccurs2)
	if required2 || minOccurs2 != 0 {
		t.Errorf("archived placement for SNAPPLACEC.2: is_required=%v min_occurs=%d, want false 0", required2, minOccurs2)
	}

	t.Run("a project with no placements snapshots cleanly", func(t *testing.T) {
		const empty = "SNAPPLACEEMPTY"
		seedProject(t, pool, empty)
		if err := runSnapshotStatements(ctx, pool, empty, version); err != nil {
			t.Fatalf("snapshot of a placement-less project must not error: %v", err)
		}
	})

	t.Run("re-running the snapshot is a no-op", func(t *testing.T) {
		if err := runSnapshotStatements(ctx, pool, projectID, version); err != nil {
			t.Fatalf("second snapshot: %v", err)
		}
		var again int
		mustScan(t, pool, `SELECT count(*) FROM weave_collection_placements_archive WHERE project_id=$1 AND version_number=$2`,
			[]any{projectID, version}, &again)
		if again != 2 {
			t.Errorf("archived %d placements after a re-run, want 2 — the statement is not idempotent", again)
		}
	})

	t.Cleanup(func() {
		bg := context.Background()
		stmts := []string{
			`DELETE FROM weave_collection_placements_archive WHERE project_id=$1`,
			`DELETE FROM weave_collection_placements WHERE project_id=$1`,
		}
		for _, stmt := range stmts {
			if _, err := pool.Exec(bg, stmt, projectID); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
	})
}

// placementFixtureProject is deliberately not the test's own projectID — see
// the comment above its seeding in TestSnapshotIncludesCollectionPlacements.
const placementFixtureProject = "SNAPPLACEFX"
