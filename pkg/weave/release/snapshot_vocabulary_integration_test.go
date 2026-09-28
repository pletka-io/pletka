//go:build integration

package release

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestSnapshotIncludesVocabularies pins that a release carries a project's
// vocabularies and their cached entries. Before this, a release snapshot
// archived the concept lists that reference vocabulary entries but not the
// entries themselves, so a pinned read of an old release could show terms
// that had since changed underneath it.
func TestSnapshotIncludesVocabularies(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const projectID, version = "SNAPVOC", "1.0.0"

	seedProject(t, pool, projectID)
	seedVocabulary(t, pool, projectID)

	mustExec(t, pool, `INSERT INTO weave_vocabularies (id, project_id, system_name, connector_type, status, deprecated)
		VALUES ('snapvoc_dep', $1, 'retired', 'local', 'published', true)`, projectID)

	if err := runSnapshotStatements(ctx, pool, projectID, version); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	var archived, entries int
	mustScan(t, pool, `SELECT count(*) FROM weave_vocabularies_archive WHERE project_id=$1 AND version_number=$2`,
		[]any{projectID, version}, &archived)
	// Scoped by vocabulary_id, not just version_number: weave_vocabulary_entries_archive
	// carries no project_id of its own, and other tests in this package also
	// archive at version "1.0.0", so an unscoped count would pick up their rows too.
	mustScan(t, pool, `SELECT count(*) FROM weave_vocabulary_entries_archive WHERE vocabulary_id='snapvoc_1' AND version_number=$1`,
		[]any{version}, &entries)
	if archived != 2 {
		t.Errorf("archived %d vocabularies, want 2 — a deprecated vocabulary is retired, not absent", archived)
	}
	if entries != 1 {
		t.Errorf("archived %d vocabulary entries, want 1", entries)
	}

	var deprecated bool
	mustScan(t, pool, `SELECT deprecated FROM weave_vocabularies_archive WHERE id='snapvoc_dep' AND project_id=$1 AND version_number=$2`,
		[]any{projectID, version}, &deprecated)
	if !deprecated {
		t.Errorf("archived vocabulary snapvoc_dep has deprecated=false, want true — deprecated must be copied verbatim")
	}

	t.Run("a project with no vocabularies snapshots cleanly", func(t *testing.T) {
		const empty = "SNAPVOCEMPTY"
		seedProject(t, pool, empty)
		if err := runSnapshotStatements(ctx, pool, empty, version); err != nil {
			t.Fatalf("snapshot of a vocabulary-less project must not error: %v", err)
		}
	})

	t.Run("re-running the snapshot is a no-op", func(t *testing.T) {
		if err := runSnapshotStatements(ctx, pool, projectID, version); err != nil {
			t.Fatalf("second snapshot: %v", err)
		}
		var again int
		mustScan(t, pool, `SELECT count(*) FROM weave_vocabularies_archive WHERE project_id=$1 AND version_number=$2`,
			[]any{projectID, version}, &again)
		if again != 2 {
			t.Errorf("archived %d vocabularies after a re-run, want 2 — the statement is not idempotent", again)
		}
	})
}

// seedVocabulary creates a project-owned vocabulary with one entry. Cleans up
// after itself, deleting archive rows first — the archive tables carry no FK
// to the live rows, so nothing cascades them — then the live rows, before
// seedProject's own cleanup (registered earlier) removes the project.
func seedVocabulary(t *testing.T, pool *pgxpool.Pool, projectID string) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO weave_vocabularies (id, project_id, system_name, connector_type, status)
		VALUES ('snapvoc_1', $1, 'snapvoc', 'local', 'published')`, projectID)
	mustExec(t, pool, `INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label)
		VALUES ('snapvoce_1', 'snapvoc_1', 'pletka:concept/snapvoce_1', '{"en":"An entry"}'::jsonb)`)

	t.Cleanup(func() {
		bg := context.Background()
		stmts := []string{
			`DELETE FROM weave_vocabulary_entries_archive WHERE vocabulary_id IN (SELECT id FROM weave_vocabularies WHERE project_id=$1)`,
			`DELETE FROM weave_vocabularies_archive WHERE project_id=$1`,
			`DELETE FROM weave_vocabulary_entries WHERE vocabulary_id IN (SELECT id FROM weave_vocabularies WHERE project_id=$1)`,
			`DELETE FROM weave_vocabularies WHERE project_id=$1`,
		}
		for _, stmt := range stmts {
			if _, err := pool.Exec(bg, stmt, projectID); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
	})
}
