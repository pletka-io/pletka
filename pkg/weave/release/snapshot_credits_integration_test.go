//go:build integration

package release

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestSnapshotIncludesProjectCredits pins that a release carries a project's
// credits: who is credited (weave_project_attributions) and which actors are
// linked to the project (weave_project_actors). A release is citable, so who
// was credited at that release is part of what is cited.
//
// Neither live table has an id column — weave_project_attributions is keyed
// (project_id, actor_id, kind, position) and weave_project_actors
// (project_id, actor_id, role) — so their archives carry those composites
// (position and role aside — see the plan) plus version_number, deliberately,
// rather than inventing a surrogate id. The case that composite protects: the
// same actor credited under two different kinds must both survive a
// snapshot, not collapse into one row. A row count alone could pass even if
// one kind were silently dropped by a too-narrow conflict target, so this
// test asserts the distinct kinds themselves.
func TestSnapshotIncludesProjectCredits(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const projectID, version = "SNAPCREDIT", "1.0.0"

	seedProject(t, pool, projectID)

	// unite is a pre-seeded fixture organization actor (see
	// internal/testdb/fixture_identities.go) — reused here per the task brief
	// rather than inventing a new actor row.
	const actorID = "unite"

	mustExec(t, pool, `INSERT INTO weave_project_attributions (project_id, actor_id, kind, position, note)
		VALUES ($1, $2, 'author', 0, 'wrote it')`, projectID, actorID)
	mustExec(t, pool, `INSERT INTO weave_project_attributions (project_id, actor_id, kind, position, note)
		VALUES ($1, $2, 'funder', 0, 'paid for it')`, projectID, actorID)

	mustExec(t, pool, `INSERT INTO weave_project_actors (project_id, actor_id, role)
		VALUES ($1, $2, 'owner')`, projectID, actorID)

	t.Cleanup(func() {
		bg := context.Background()
		stmts := []string{
			`DELETE FROM weave_project_attributions_archive WHERE project_id=$1`,
			`DELETE FROM weave_project_actors_archive WHERE project_id=$1`,
			`DELETE FROM weave_project_attributions WHERE project_id=$1`,
			`DELETE FROM weave_project_actors WHERE project_id=$1`,
		}
		for _, stmt := range stmts {
			if _, err := pool.Exec(bg, stmt, projectID); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
	})

	if err := runSnapshotStatements(ctx, pool, projectID, version); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	var attributions int
	mustScan(t, pool, `SELECT count(*) FROM weave_project_attributions_archive WHERE project_id=$1 AND version_number=$2`,
		[]any{projectID, version}, &attributions)
	if attributions != 2 {
		t.Fatalf("archived %d attributions, want 2 — a naive archive keyed on (project_id, actor_id, version_number) would silently keep only one of the two kinds", attributions)
	}

	rows, err := pool.Query(ctx, `SELECT kind FROM weave_project_attributions_archive
		WHERE project_id=$1 AND actor_id=$2 AND version_number=$3 ORDER BY kind`,
		projectID, actorID, version)
	if err != nil {
		t.Fatalf("query archived kinds: %v", err)
	}
	var kinds []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatalf("scan kind: %v", err)
		}
		kinds = append(kinds, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate archived kinds: %v", err)
	}
	if len(kinds) != 2 || kinds[0] != "author" || kinds[1] != "funder" {
		t.Fatalf("archived kinds for actor %s = %v, want [author funder]", actorID, kinds)
	}

	var actors int
	mustScan(t, pool, `SELECT count(*) FROM weave_project_actors_archive WHERE project_id=$1 AND version_number=$2`,
		[]any{projectID, version}, &actors)
	if actors != 1 {
		t.Fatalf("archived %d project actors, want 1", actors)
	}

	var role string
	mustScan(t, pool, `SELECT role FROM weave_project_actors_archive WHERE project_id=$1 AND actor_id=$2 AND version_number=$3`,
		[]any{projectID, actorID, version}, &role)
	if role != "owner" {
		t.Errorf("archived project actor role = %q, want owner", role)
	}

	t.Run("a project with no credits snapshots cleanly", func(t *testing.T) {
		const empty = "SNAPCREDITEMPTY"
		seedProject(t, pool, empty)
		if err := runSnapshotStatements(ctx, pool, empty, version); err != nil {
			t.Fatalf("snapshot of a credit-less project must not error: %v", err)
		}
	})

	t.Run("re-running the snapshot is a no-op", func(t *testing.T) {
		if err := runSnapshotStatements(ctx, pool, projectID, version); err != nil {
			t.Fatalf("second snapshot: %v", err)
		}
		var again int
		mustScan(t, pool, `SELECT count(*) FROM weave_project_attributions_archive WHERE project_id=$1 AND version_number=$2`,
			[]any{projectID, version}, &again)
		if again != 2 {
			t.Errorf("archived %d attributions after a re-run, want 2 — the statement is not idempotent", again)
		}
		var againActors int
		mustScan(t, pool, `SELECT count(*) FROM weave_project_actors_archive WHERE project_id=$1 AND version_number=$2`,
			[]any{projectID, version}, &againActors)
		if againActors != 1 {
			t.Errorf("archived %d project actors after a re-run, want 1 — the statement is not idempotent", againActors)
		}
	})
}
