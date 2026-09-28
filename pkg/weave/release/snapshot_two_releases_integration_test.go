//go:build integration

package release

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestSnapshotSecondReleaseIsIndependent is the gate none of the four
// single-version snapshot tests (examples, vocabularies, collection
// placements, project credits) can be: that a SECOND release at a different
// version snapshots each of the seven archive tables Tasks 1-4 added
// independently of the first.
//
// The failure this catches: a PRIMARY KEY or ON CONFLICT target that omits
// version_number, which makes the second release silently keep the first
// release's content and look like it worked. A row-count-only assertion
// would still pass in that failure mode (two rows total, just both from the
// first release plus a no-op second insert would even undercount to one) —
// so this test also asserts that the 1.0.0 rows still carry the pre-edit
// value and the 1.1.0 rows carry the post-edit one, for all seven tables.
func TestSnapshotSecondReleaseIsIndependent(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const projectID = "SNAPTWO"
	const v1, v2 = "1.0.0", "1.1.0"
	const actorID = "unite" // pre-seeded fixture actor, see internal/testdb/fixture_identities.go

	const (
		fieldID    = "SNAPTWOF.1"
		exampleID  = "SNAPTWOEX1"
		entityID   = "SNAPTWOM.1"
		vocabID    = "snaptwovoc_1"
		entryID    = "snaptwovoce_1"
		modelID    = "SNAPTWOPM.1"
		categoryID = "SNAPTWOPCAT.1"
		collID     = "SNAPTWOPC.1"
	)

	seedProject(t, pool, projectID)
	overrideID := seedExampleField(t, pool, fieldID, "model", entityID)

	mustExec(t, pool, `INSERT INTO weave_examples (id, project_id, entity_type, entity_id, title, status, version_number)
		VALUES ($1, $2, 'model', $3, '{"en":"An example"}'::jsonb, 'valid', '')`, exampleID, projectID, entityID)
	mustExec(t, pool, `INSERT INTO weave_example_values
		(example_id, override_id, field_id, occurrence_index, value_kind, value_payload, text_value, version_number, slot_path)
		VALUES ($1, $2, $3, 0, 'string', '{"text":"hello"}'::jsonb, 'hello', '', 'root')`, exampleID, overrideID, fieldID)

	// weave_vocabularies.project_id and weave_collection_placements carry no
	// FK to weave_projects (confirmed in 001_baseline.sql), so no separate
	// fixture project is needed here the way the placements test needs one
	// for its model/category/collection rows — those columns on this table
	// have no FK either.
	mustExec(t, pool, `INSERT INTO weave_vocabularies (id, project_id, system_name, connector_type, status, base_uri)
		VALUES ($1, $2, 'snaptwovoc', 'local', 'published', 'https://example.org/v1')`, vocabID, projectID)
	mustExec(t, pool, `INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label)
		VALUES ($1, $2, 'pletka:concept/snaptwovoce_1', '{"en":"An entry"}'::jsonb)`, entryID, vocabID)

	mustExec(t, pool, `INSERT INTO weave_collection_placements
		(project_id, model_id, category_id, collection_id, is_required, min_occurs, max_occurs, is_hidden)
		VALUES ($1, $2, $3, $4, true, 1, 3, false)`, projectID, modelID, categoryID, collID)

	mustExec(t, pool, `INSERT INTO weave_project_attributions (project_id, actor_id, kind, "position", note)
		VALUES ($1, $2, 'author', 0, 'wrote it')`, projectID, actorID)
	mustExec(t, pool, `INSERT INTO weave_project_actors (project_id, actor_id, role)
		VALUES ($1, $2, 'contributor')`, projectID, actorID)

	// weave_vocabulary_entries has an FK to weave_vocabularies with ON DELETE
	// CASCADE, so only the vocabulary's own live row needs an explicit
	// delete; its archive counterpart does not cascade (archive tables carry
	// no FK to live rows, matching every other test in this package).
	t.Cleanup(func() {
		bg := context.Background()
		stmts := []string{
			`DELETE FROM weave_vocabulary_entries_archive WHERE vocabulary_id=$1`,
			`DELETE FROM weave_vocabularies_archive WHERE id=$1`,
			`DELETE FROM weave_vocabularies WHERE id=$1`,
		}
		for _, stmt := range stmts {
			if _, err := pool.Exec(bg, stmt, vocabID); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
	})
	t.Cleanup(func() {
		bg := context.Background()
		stmts := []string{
			`DELETE FROM weave_collection_placements_archive WHERE project_id=$1`,
			`DELETE FROM weave_collection_placements WHERE project_id=$1`,
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

	if err := runSnapshotStatements(ctx, pool, projectID, v1); err != nil {
		t.Fatalf("first snapshot (%s): %v", v1, err)
	}

	// Edit one row of each of the seven tables before the second snapshot,
	// on a column each snapshot statement actually copies (see
	// snapshotStatements in service.go) so the assertions below are not
	// vacuous.
	mustExec(t, pool, `UPDATE weave_examples SET status='has_issues' WHERE id=$1`, exampleID)
	mustExec(t, pool, `UPDATE weave_example_values SET text_value='world' WHERE example_id=$1`, exampleID)
	mustExec(t, pool, `UPDATE weave_vocabularies SET base_uri='https://example.org/v2' WHERE id=$1`, vocabID)
	mustExec(t, pool, `UPDATE weave_vocabulary_entries SET label='{"en":"An edited entry"}'::jsonb WHERE id=$1`, entryID)
	mustExec(t, pool, `UPDATE weave_collection_placements SET min_occurs=5
		WHERE project_id=$1 AND model_id=$2 AND category_id=$3 AND collection_id=$4`,
		projectID, modelID, categoryID, collID)
	mustExec(t, pool, `UPDATE weave_project_attributions SET note='rewrote it'
		WHERE project_id=$1 AND actor_id=$2 AND kind='author' AND "position"=0`, projectID, actorID)
	mustExec(t, pool, `UPDATE weave_project_actors SET role='lead'
		WHERE project_id=$1 AND actor_id=$2 AND role='contributor'`, projectID, actorID)

	if err := runSnapshotStatements(ctx, pool, projectID, v2); err != nil {
		t.Fatalf("second snapshot (%s): %v", v2, err)
	}

	// Two rows per live row, one per version. Every query below is scoped by
	// an id/parent id (or the full composite key), never by version_number
	// alone — this package shares one fixture database across tests, and
	// several of them archive at version "1.0.0" too.
	counts := []struct {
		name  string
		query string
		args  []any
		want  int
	}{
		{"examples", `SELECT count(*) FROM weave_examples_archive WHERE id=$1`, []any{exampleID}, 2},
		{"example values", `SELECT count(*) FROM weave_example_values_archive WHERE example_id=$1`, []any{exampleID}, 2},
		{"vocabularies", `SELECT count(*) FROM weave_vocabularies_archive WHERE id=$1`, []any{vocabID}, 2},
		{"vocabulary entries", `SELECT count(*) FROM weave_vocabulary_entries_archive WHERE vocabulary_id=$1`, []any{vocabID}, 2},
		{"collection placements", `SELECT count(*) FROM weave_collection_placements_archive
			WHERE project_id=$1 AND model_id=$2 AND category_id=$3 AND collection_id=$4`,
			[]any{projectID, modelID, categoryID, collID}, 2},
		{"project attributions", `SELECT count(*) FROM weave_project_attributions_archive
			WHERE project_id=$1 AND actor_id=$2 AND kind='author' AND "position"=0`, []any{projectID, actorID}, 2},
		{"project actors", `SELECT count(*) FROM weave_project_actors_archive WHERE project_id=$1 AND actor_id=$2`,
			[]any{projectID, actorID}, 2},
	}
	for _, c := range counts {
		var n int
		mustScan(t, pool, c.query, c.args, &n)
		if n != c.want {
			t.Errorf("%s: archived %d rows across the two versions, want %d — a key or ON CONFLICT target omitting version_number would collapse the second release into the first",
				c.name, n, c.want)
		}
	}

	// The 1.0.0 rows still carry the pre-edit value; the 1.1.0 rows carry
	// the post-edit value.
	var exStatus1, exStatus2 string
	mustScan(t, pool, `SELECT status FROM weave_examples_archive WHERE id=$1 AND version_number=$2`, []any{exampleID, v1}, &exStatus1)
	mustScan(t, pool, `SELECT status FROM weave_examples_archive WHERE id=$1 AND version_number=$2`, []any{exampleID, v2}, &exStatus2)
	if exStatus1 != "valid" || exStatus2 != "has_issues" {
		t.Errorf("example status: %s=%q %s=%q, want %q and %q — the second release kept the first release's content",
			v1, exStatus1, v2, exStatus2, "valid", "has_issues")
	}

	var valText1, valText2 string
	mustScan(t, pool, `SELECT text_value FROM weave_example_values_archive WHERE example_id=$1 AND version_number=$2`, []any{exampleID, v1}, &valText1)
	mustScan(t, pool, `SELECT text_value FROM weave_example_values_archive WHERE example_id=$1 AND version_number=$2`, []any{exampleID, v2}, &valText2)
	if valText1 != "hello" || valText2 != "world" {
		t.Errorf("example value text: %s=%q %s=%q, want %q and %q", v1, valText1, v2, valText2, "hello", "world")
	}

	var baseURI1, baseURI2 string
	mustScan(t, pool, `SELECT base_uri FROM weave_vocabularies_archive WHERE id=$1 AND version_number=$2`, []any{vocabID, v1}, &baseURI1)
	mustScan(t, pool, `SELECT base_uri FROM weave_vocabularies_archive WHERE id=$1 AND version_number=$2`, []any{vocabID, v2}, &baseURI2)
	if baseURI1 != "https://example.org/v1" || baseURI2 != "https://example.org/v2" {
		t.Errorf("vocabulary base_uri: %s=%q %s=%q, want %q and %q", v1, baseURI1, v2, baseURI2, "https://example.org/v1", "https://example.org/v2")
	}

	var label1, label2 string
	mustScan(t, pool, `SELECT label->>'en' FROM weave_vocabulary_entries_archive WHERE id=$1 AND version_number=$2`, []any{entryID, v1}, &label1)
	mustScan(t, pool, `SELECT label->>'en' FROM weave_vocabulary_entries_archive WHERE id=$1 AND version_number=$2`, []any{entryID, v2}, &label2)
	if label1 != "An entry" || label2 != "An edited entry" {
		t.Errorf("vocabulary entry label: %s=%q %s=%q, want %q and %q", v1, label1, v2, label2, "An entry", "An edited entry")
	}

	var minOccurs1, minOccurs2 int
	mustScan(t, pool, `SELECT min_occurs FROM weave_collection_placements_archive
		WHERE project_id=$1 AND collection_id=$2 AND version_number=$3`, []any{projectID, collID, v1}, &minOccurs1)
	mustScan(t, pool, `SELECT min_occurs FROM weave_collection_placements_archive
		WHERE project_id=$1 AND collection_id=$2 AND version_number=$3`, []any{projectID, collID, v2}, &minOccurs2)
	if minOccurs1 != 1 || minOccurs2 != 5 {
		t.Errorf("placement min_occurs: %s=%d %s=%d, want %d and %d", v1, minOccurs1, v2, minOccurs2, 1, 5)
	}

	var note1, note2 string
	mustScan(t, pool, `SELECT note FROM weave_project_attributions_archive
		WHERE project_id=$1 AND actor_id=$2 AND kind='author' AND "position"=0 AND version_number=$3`, []any{projectID, actorID, v1}, &note1)
	mustScan(t, pool, `SELECT note FROM weave_project_attributions_archive
		WHERE project_id=$1 AND actor_id=$2 AND kind='author' AND "position"=0 AND version_number=$3`, []any{projectID, actorID, v2}, &note2)
	if note1 != "wrote it" || note2 != "rewrote it" {
		t.Errorf("attribution note: %s=%q %s=%q, want %q and %q", v1, note1, v2, note2, "wrote it", "rewrote it")
	}

	var role1, role2 string
	mustScan(t, pool, `SELECT role FROM weave_project_actors_archive WHERE project_id=$1 AND actor_id=$2 AND version_number=$3`,
		[]any{projectID, actorID, v1}, &role1)
	mustScan(t, pool, `SELECT role FROM weave_project_actors_archive WHERE project_id=$1 AND actor_id=$2 AND version_number=$3`,
		[]any{projectID, actorID, v2}, &role2)
	if role1 != "contributor" || role2 != "lead" {
		t.Errorf("project actor role: %s=%q %s=%q, want %q and %q", v1, role1, v2, role2, "contributor", "lead")
	}
}
