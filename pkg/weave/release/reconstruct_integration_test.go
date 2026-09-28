//go:build integration

package release

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestReconstructClassifiesRowsAgainstReleaseTimestamp pins the reconstruction
// predicate the `release reconstruct` command applies to releases cut before
// the seven archives of Tasks 1-4 existed.
//
// Per table it seeds one row of each class against a release dated in the
// past:
//
//	exact      created_at <= release, updated_at <= release — archived
//	excluded   created_at >  release                        — not archived
//	ambiguous  created_at <= release, updated_at >  release — reported,
//	                                                          never archived
//
// A fourth row per table sits at EXACTLY the release timestamp. The predicate
// is <=, not <, so that row belongs to the release and must be archived — the
// sharpest case in the brief, and the one a later <= -> < slip would break
// while every before/after fixture kept passing.
//
// The column that moves a row between classes is updated_at, which is NOT
// part of any of these archives' primary keys — the three rows of a table
// are separate rows with distinct key tuples, so an assertion here cannot
// pass by a row quietly landing on a different key (the defect the sibling
// two-releases test exists to catch).
//
// weave_project_attributions and weave_project_actors have no updated_at
// column at all (verified in 001_baseline.sql), so only the exact/excluded
// pair is reachable for them; the ambiguous class is unrepresentable rather
// than merely absent, which is exactly what the command's caveat line says.
func TestReconstructClassifiesRowsAgainstReleaseTimestamp(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	const projectID = "RECONSTRUCT1"
	const version = "2.4.0" // unique to this test; the package shares a database
	const actorID = "unite" // pre-seeded fixture actor

	released := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	after := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	const (
		fieldID  = "RECON1F.1"
		exExact  = "RECON1EX_EXACT"
		exExcl   = "RECON1EX_EXCLUDED"
		exAmb    = "RECON1EX_AMBIGUOUS"
		exEq     = "RECON1EX_EQUAL"
		entityID = "RECON1M.1"

		vocExact = "recon1voc_exact"
		vocExcl  = "recon1voc_excluded"
		vocAmb   = "recon1voc_ambiguous"
		vocEq    = "recon1voc_equal"

		entryExact = "recon1voce_exact"
		entryExcl  = "recon1voce_excluded"
		entryAmb   = "recon1voce_ambiguous"
		entryEq    = "recon1voce_equal"

		modelID    = "RECON1PM.1"
		categoryID = "RECON1PCAT.1"
		collExact  = "RECON1PC_EXACT"
		collExcl   = "RECON1PC_EXCLUDED"
		collAmb    = "RECON1PC_AMBIGUOUS"
		collEq     = "RECON1PC_EQUAL"
	)

	seedProject(t, pool, projectID)
	overrideID := seedExampleField(t, pool, fieldID, "model", entityID)

	mustExec(t, pool, `INSERT INTO weave_releases (project_id, version, title, description, created_at, created_by_id)
		VALUES ($1, $2, 'Reconstruct fixture', '', $3, $4)`, projectID, version, released, actorID)

	// --- weave_examples -------------------------------------------------
	for _, row := range []struct {
		id               string
		created, updated time.Time
		status           string
	}{
		{exExact, before, before, "valid"},
		{exExcl, after, after, "valid"},
		{exAmb, before, after, "has_issues"},
		{exEq, released, released, "valid"}, // created_at == release.created_at
	} {
		mustExec(t, pool, `INSERT INTO weave_examples
			(id, project_id, entity_type, entity_id, title, status, created_at, updated_at, version_number)
			VALUES ($1, $2, 'model', $3, '{"en":"An example"}'::jsonb, $4, $5, $6, '')`,
			row.id, projectID, entityID, row.status, row.created, row.updated)
	}

	// --- weave_example_values -------------------------------------------
	// All three hang off the exact example: the values table is classified on
	// its own timestamps, not its parent's, so a shared parent keeps the
	// assertion about the values themselves.
	valExact := insertExampleValue(t, pool, exExact, overrideID, fieldID, 0, "root", "exact", before, before)
	valExcl := insertExampleValue(t, pool, exExact, overrideID, fieldID, 1, "root/excluded", "excluded", after, after)
	valAmb := insertExampleValue(t, pool, exExact, overrideID, fieldID, 2, "root/ambiguous", "ambiguous", before, after)
	valEq := insertExampleValue(t, pool, exExact, overrideID, fieldID, 3, "root/equal", "equal", released, released)

	// --- weave_vocabularies ---------------------------------------------
	for _, row := range []struct {
		id               string
		created, updated time.Time
	}{
		{vocExact, before, before},
		{vocExcl, after, after},
		{vocAmb, before, after},
		{vocEq, released, released},
	} {
		mustExec(t, pool, `INSERT INTO weave_vocabularies
			(id, project_id, system_name, connector_type, status, base_uri, created_at, updated_at)
			VALUES ($1, $2, $1, 'local', 'published', 'https://example.org/recon', $3, $4)`,
			row.id, projectID, row.created, row.updated)
	}

	// --- weave_vocabulary_entries ---------------------------------------
	for _, row := range []struct {
		id               string
		created, updated time.Time
	}{
		{entryExact, before, before},
		{entryExcl, after, after},
		{entryAmb, before, after},
		{entryEq, released, released},
	} {
		mustExec(t, pool, `INSERT INTO weave_vocabulary_entries
			(id, vocabulary_id, uri, label, created_at, updated_at)
			VALUES ($1, $2, 'pletka:concept/' || $1, '{"en":"An entry"}'::jsonb, $3, $4)`,
			row.id, vocExact, row.created, row.updated)
	}

	// --- weave_collection_placements ------------------------------------
	// The placements archive keys on the bigserial id, so the report's
	// identity for an ambiguous placement is that id, not the collection id.
	placementIDs := map[string]int64{}
	for _, row := range []struct {
		collectionID     string
		created, updated time.Time
	}{
		{collExact, before, before},
		{collExcl, after, after},
		{collAmb, before, after},
		{collEq, released, released},
	} {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO weave_collection_placements
			(project_id, model_id, category_id, collection_id, is_required, min_occurs, max_occurs, is_hidden, created_at, updated_at)
			VALUES ($1, $2, $3, $4, true, 1, 3, false, $5, $6) RETURNING id`,
			projectID, modelID, categoryID, row.collectionID, row.created, row.updated).Scan(&id); err != nil {
			t.Fatalf("seed placement %s: %v", row.collectionID, err)
		}
		placementIDs[row.collectionID] = id
	}

	// --- weave_project_attributions / weave_project_actors --------------
	// No updated_at on either table, so only exact and excluded exist here.
	// "position" and role are key columns, which is why the two rows differ
	// in them: without an updated_at there is no non-key column that could
	// separate them.
	mustExec(t, pool, `INSERT INTO weave_project_attributions (project_id, actor_id, kind, "position", note, created_at)
		VALUES ($1, $2, 'author', 0, 'wrote it', $3)`, projectID, actorID, before)
	mustExec(t, pool, `INSERT INTO weave_project_attributions (project_id, actor_id, kind, "position", note, created_at)
		VALUES ($1, $2, 'author', 1, 'joined later', $3)`, projectID, actorID, after)
	mustExec(t, pool, `INSERT INTO weave_project_actors (project_id, actor_id, role, created_at)
		VALUES ($1, $2, 'contributor', $3)`, projectID, actorID, before)
	mustExec(t, pool, `INSERT INTO weave_project_actors (project_id, actor_id, role, created_at)
		VALUES ($1, $2, 'reviewer', $3)`, projectID, actorID, after)
	// The boundary rows: created_at exactly equal to the release timestamp.
	mustExec(t, pool, `INSERT INTO weave_project_attributions (project_id, actor_id, kind, "position", note, created_at)
		VALUES ($1, $2, 'author', 2, 'joined at the release', $3)`, projectID, actorID, released)
	mustExec(t, pool, `INSERT INTO weave_project_actors (project_id, actor_id, role, created_at)
		VALUES ($1, $2, 'boundary', $3)`, projectID, actorID, released)

	t.Cleanup(func() {
		bg := context.Background()
		stmts := []string{
			`DELETE FROM weave_vocabulary_entries_archive WHERE vocabulary_id IN (SELECT id FROM weave_vocabularies WHERE project_id=$1)`,
			`DELETE FROM weave_vocabularies_archive WHERE project_id=$1`,
			`DELETE FROM weave_vocabularies WHERE project_id=$1`,
			`DELETE FROM weave_collection_placements_archive WHERE project_id=$1`,
			`DELETE FROM weave_collection_placements WHERE project_id=$1`,
			`DELETE FROM weave_project_attributions_archive WHERE project_id=$1`,
			`DELETE FROM weave_project_actors_archive WHERE project_id=$1`,
			`DELETE FROM weave_project_attributions WHERE project_id=$1`,
			`DELETE FROM weave_project_actors WHERE project_id=$1`,
			`DELETE FROM weave_releases WHERE project_id=$1`,
		}
		for _, stmt := range stmts {
			if _, err := pool.Exec(bg, stmt, projectID); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
	})

	// --- dry run: classifies, writes nothing ----------------------------
	dry, err := Reconstruct(ctx, pool, ReconstructOptions{ProjectID: projectID})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	assertReconstructClassification(t, dry, projectID, version, map[string]ambiguousExpectation{
		// exact is 2 per table: the created-before row and the boundary row
		// whose created_at equals the release timestamp exactly.
		"weave_examples":              {exact: 2, excluded: 1, ambiguous: []string{exAmb}},
		"weave_example_values":        {exact: 2, excluded: 1, ambiguous: []string{strconv.FormatInt(valAmb, 10)}},
		"weave_vocabularies":          {exact: 2, excluded: 1, ambiguous: []string{vocAmb}},
		"weave_vocabulary_entries":    {exact: 2, excluded: 1, ambiguous: []string{entryAmb}},
		"weave_collection_placements": {exact: 2, excluded: 1, ambiguous: []string{strconv.FormatInt(placementIDs[collAmb], 10)}},
		"weave_project_attributions":  {exact: 2, excluded: 1, ambiguous: nil},
		"weave_project_actors":        {exact: 2, excluded: 1, ambiguous: nil},
	})
	if n := countArchivedForVersion(t, pool, projectID, version, exExact, vocExact, actorID); n != 0 {
		t.Fatalf("dry run wrote %d archive row(s); a dry run must write nothing", n)
	}

	// --- apply without --accept-ambiguous: refused, writes nothing ------
	if _, err := Reconstruct(ctx, pool, ReconstructOptions{ProjectID: projectID, Apply: true}); !errors.Is(err, ErrAmbiguousRows) {
		t.Fatalf("apply without accept-ambiguous: err = %v, want ErrAmbiguousRows", err)
	}
	if n := countArchivedForVersion(t, pool, projectID, version, exExact, vocExact, actorID); n != 0 {
		t.Fatalf("refused apply wrote %d archive row(s); it must write nothing", n)
	}

	// --- apply with --accept-ambiguous ----------------------------------
	applied, err := Reconstruct(ctx, pool, ReconstructOptions{ProjectID: projectID, Apply: true, AcceptAmbiguous: true})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !applied.Applied {
		t.Fatalf("report says Applied=false after a successful --apply")
	}
	// Inserted is what the report tells an operator was written. Assert it
	// against the rows the run actually classified exact, so a report that
	// under- or over-states the write is a failure rather than decoration.
	for _, rel := range applied.Releases {
		for _, tr := range rel.Tables {
			if tr.AlreadyArchived {
				t.Errorf("%s: skipped as already archived on the FIRST apply", tr.LiveTable)
				continue
			}
			if tr.Inserted != int64(tr.Exact) {
				t.Errorf("%s: report says %d inserted but %d exact", tr.LiveTable, tr.Inserted, tr.Exact)
			}
		}
	}

	archived := []struct {
		name  string
		query string
		args  []any
		want  int
	}{
		{"example exact", `SELECT count(*) FROM weave_examples_archive WHERE id=$1 AND version_number=$2`, []any{exExact, version}, 1},
		{"example excluded", `SELECT count(*) FROM weave_examples_archive WHERE id=$1 AND version_number=$2`, []any{exExcl, version}, 0},
		{"example ambiguous", `SELECT count(*) FROM weave_examples_archive WHERE id=$1 AND version_number=$2`, []any{exAmb, version}, 0},
		{"example at the release timestamp", `SELECT count(*) FROM weave_examples_archive WHERE id=$1 AND version_number=$2`, []any{exEq, version}, 1},

		{"example value exact", `SELECT count(*) FROM weave_example_values_archive WHERE id=$1 AND version_number=$2`, []any{valExact, version}, 1},
		{"example value excluded", `SELECT count(*) FROM weave_example_values_archive WHERE id=$1 AND version_number=$2`, []any{valExcl, version}, 0},
		{"example value ambiguous", `SELECT count(*) FROM weave_example_values_archive WHERE id=$1 AND version_number=$2`, []any{valAmb, version}, 0},
		{"example value at the release timestamp", `SELECT count(*) FROM weave_example_values_archive WHERE id=$1 AND version_number=$2`, []any{valEq, version}, 1},

		{"vocabulary exact", `SELECT count(*) FROM weave_vocabularies_archive WHERE id=$1 AND version_number=$2`, []any{vocExact, version}, 1},
		{"vocabulary excluded", `SELECT count(*) FROM weave_vocabularies_archive WHERE id=$1 AND version_number=$2`, []any{vocExcl, version}, 0},
		{"vocabulary ambiguous", `SELECT count(*) FROM weave_vocabularies_archive WHERE id=$1 AND version_number=$2`, []any{vocAmb, version}, 0},
		{"vocabulary at the release timestamp", `SELECT count(*) FROM weave_vocabularies_archive WHERE id=$1 AND version_number=$2`, []any{vocEq, version}, 1},

		{"vocabulary entry exact", `SELECT count(*) FROM weave_vocabulary_entries_archive WHERE id=$1 AND version_number=$2`, []any{entryExact, version}, 1},
		{"vocabulary entry excluded", `SELECT count(*) FROM weave_vocabulary_entries_archive WHERE id=$1 AND version_number=$2`, []any{entryExcl, version}, 0},
		{"vocabulary entry ambiguous", `SELECT count(*) FROM weave_vocabulary_entries_archive WHERE id=$1 AND version_number=$2`, []any{entryAmb, version}, 0},
		{"vocabulary entry at the release timestamp", `SELECT count(*) FROM weave_vocabulary_entries_archive WHERE id=$1 AND version_number=$2`, []any{entryEq, version}, 1},

		{"placement exact", `SELECT count(*) FROM weave_collection_placements_archive WHERE project_id=$1 AND collection_id=$2 AND version_number=$3`, []any{projectID, collExact, version}, 1},
		{"placement excluded", `SELECT count(*) FROM weave_collection_placements_archive WHERE project_id=$1 AND collection_id=$2 AND version_number=$3`, []any{projectID, collExcl, version}, 0},
		{"placement ambiguous", `SELECT count(*) FROM weave_collection_placements_archive WHERE project_id=$1 AND collection_id=$2 AND version_number=$3`, []any{projectID, collAmb, version}, 0},
		{"placement at the release timestamp", `SELECT count(*) FROM weave_collection_placements_archive WHERE project_id=$1 AND collection_id=$2 AND version_number=$3`, []any{projectID, collEq, version}, 1},

		{"attribution exact", `SELECT count(*) FROM weave_project_attributions_archive WHERE project_id=$1 AND actor_id=$2 AND kind='author' AND "position"=0 AND version_number=$3`, []any{projectID, actorID, version}, 1},
		{"attribution excluded", `SELECT count(*) FROM weave_project_attributions_archive WHERE project_id=$1 AND actor_id=$2 AND kind='author' AND "position"=1 AND version_number=$3`, []any{projectID, actorID, version}, 0},
		{"attribution at the release timestamp", `SELECT count(*) FROM weave_project_attributions_archive WHERE project_id=$1 AND actor_id=$2 AND kind='author' AND "position"=2 AND version_number=$3`, []any{projectID, actorID, version}, 1},

		{"project actor exact", `SELECT count(*) FROM weave_project_actors_archive WHERE project_id=$1 AND actor_id=$2 AND role='contributor' AND version_number=$3`, []any{projectID, actorID, version}, 1},
		{"project actor excluded", `SELECT count(*) FROM weave_project_actors_archive WHERE project_id=$1 AND actor_id=$2 AND role='reviewer' AND version_number=$3`, []any{projectID, actorID, version}, 0},
		{"project actor at the release timestamp", `SELECT count(*) FROM weave_project_actors_archive WHERE project_id=$1 AND actor_id=$2 AND role='boundary' AND version_number=$3`, []any{projectID, actorID, version}, 1},
	}
	for _, c := range archived {
		var n int
		mustScan(t, pool, c.query, c.args, &n)
		if n != c.want {
			t.Errorf("%s: %d archive row(s), want %d", c.name, n, c.want)
		}
	}

	// --- re-running --apply changes nothing ------------------------------
	//
	// Note what this does and does not prove. The second run takes the
	// AlreadyArchived skip path for every table, so no reconstruction INSERT
	// executes and the ON CONFLICT clauses are NOT what makes the re-run
	// safe — the skip is. That is the real user-facing property, so it is
	// what gets asserted; claiming "the inserts are idempotent" here would be
	// claiming something this run never exercised.
	second, err := Reconstruct(ctx, pool, ReconstructOptions{ProjectID: projectID, Apply: true, AcceptAmbiguous: true})
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	for _, rel := range second.Releases {
		for _, tr := range rel.Tables {
			if !tr.AlreadyArchived {
				t.Errorf("%s: second apply did not skip an already-archived table", tr.LiveTable)
			}
			if tr.Inserted != 0 {
				t.Errorf("%s: second apply reported %d inserted, want 0", tr.LiveTable, tr.Inserted)
			}
		}
	}
	for _, c := range archived {
		var n int
		mustScan(t, pool, c.query, c.args, &n)
		if n != c.want {
			t.Errorf("after re-run, %s: %d archive row(s), want %d", c.name, n, c.want)
		}
	}
}

type ambiguousExpectation struct {
	exact     int
	excluded  int
	ambiguous []string
}

// assertReconstructClassification checks the per-table counts and the exact
// set of ambiguous row identities the report carries for one release.
func assertReconstructClassification(t *testing.T, rep *ReconstructReport, projectID, version string, want map[string]ambiguousExpectation) {
	t.Helper()
	var rel *VersionReport
	for i := range rep.Releases {
		if rep.Releases[i].ProjectID == projectID && rep.Releases[i].Version == version {
			rel = &rep.Releases[i]
			break
		}
	}
	if rel == nil {
		t.Fatalf("report carries no entry for %s@%s (releases: %d)", projectID, version, len(rep.Releases))
	}
	seen := map[string]bool{}
	for _, tbl := range rel.Tables {
		exp, ok := want[tbl.LiveTable]
		if !ok {
			continue
		}
		seen[tbl.LiveTable] = true
		if tbl.Exact != exp.exact {
			t.Errorf("%s: exact=%d, want %d", tbl.LiveTable, tbl.Exact, exp.exact)
		}
		if tbl.Excluded != exp.excluded {
			t.Errorf("%s: excluded=%d, want %d", tbl.LiveTable, tbl.Excluded, exp.excluded)
		}
		if len(tbl.Ambiguous) != len(exp.ambiguous) {
			t.Errorf("%s: %d ambiguous row(s) %v, want %d %v", tbl.LiveTable, len(tbl.Ambiguous), tbl.Ambiguous, len(exp.ambiguous), exp.ambiguous)
			continue
		}
		for i, id := range exp.ambiguous {
			if tbl.Ambiguous[i].Identity != id {
				t.Errorf("%s: ambiguous[%d].Identity=%q, want %q", tbl.LiveTable, i, tbl.Ambiguous[i].Identity, id)
			}
			if !tbl.Ambiguous[i].UpdatedAt.After(tbl.Ambiguous[i].CreatedAt) {
				t.Errorf("%s: ambiguous[%d] timestamps %s/%s are not created-before/updated-after",
					tbl.LiveTable, i, tbl.Ambiguous[i].CreatedAt, tbl.Ambiguous[i].UpdatedAt)
			}
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("report has no entry for table %s", name)
		}
	}
}

// insertExampleValue seeds one weave_example_values row with explicit
// timestamps and returns its generated bigserial id.
func insertExampleValue(t *testing.T, pool *pgxpool.Pool, exampleID string, overrideID int64, fieldID string, occurrence int, slotPath, text string, created, updated time.Time) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `INSERT INTO weave_example_values
		(example_id, override_id, field_id, occurrence_index, value_kind, value_payload, text_value, version_number, slot_path, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'string', jsonb_build_object('text', $6::text), $6, '', $5, $7, $8)
		RETURNING id`, exampleID, overrideID, fieldID, occurrence, slotPath, text, created, updated).Scan(&id)
	if err != nil {
		t.Fatalf("seed example value %q: %v", text, err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM weave_example_values_archive WHERE id=$1`, id); err != nil {
			t.Errorf("cleanup example value archive: %v", err)
		}
		if _, err := pool.Exec(context.Background(), `DELETE FROM weave_example_values WHERE id=$1`, id); err != nil {
			t.Errorf("cleanup example value: %v", err)
		}
	})
	return id
}

// countArchivedForVersion counts the archive rows this test's fixture would
// produce, across all seven tables, scoped by project/parent id and version —
// never by version_number alone, since the package shares one database.
func countArchivedForVersion(t *testing.T, pool *pgxpool.Pool, projectID, version, exampleID, vocabID, actorID string) int {
	t.Helper()
	var n int
	mustScan(t, pool, `SELECT
		(SELECT count(*) FROM weave_examples_archive WHERE project_id=$1 AND version_number=$2) +
		(SELECT count(*) FROM weave_example_values_archive WHERE example_id=$3 AND version_number=$2) +
		(SELECT count(*) FROM weave_vocabularies_archive WHERE project_id=$1 AND version_number=$2) +
		(SELECT count(*) FROM weave_vocabulary_entries_archive WHERE vocabulary_id=$4 AND version_number=$2) +
		(SELECT count(*) FROM weave_collection_placements_archive WHERE project_id=$1 AND version_number=$2) +
		(SELECT count(*) FROM weave_project_attributions_archive WHERE project_id=$1 AND actor_id=$5 AND version_number=$2) +
		(SELECT count(*) FROM weave_project_actors_archive WHERE project_id=$1 AND actor_id=$5 AND version_number=$2)`,
		[]any{projectID, version, exampleID, vocabID, actorID}, &n)
	return n
}

// TestReconstructExplicitProjectMustMatch pins that an explicit --project
// selecting nothing is an error, not a quiet success. This fleet has already
// lost production API keys to a command that no-opped silently on a mistyped
// instance name; a reconstruction that prints "nothing to reconstruct" and
// exits 0 because the project id was misspelled is the same shape of failure.
//
// The two cases are distinguished, because the operator's next move differs:
// a misspelling needs a different id, an empty project needs a release first.
// An OMITTED project that finds nothing stays a clean, non-error empty run.
func TestReconstructExplicitProjectMustMatch(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	const missingID = "RECONSTRUCT_NO_SUCH_PROJECT"
	const emptyID = "RECONSTRUCT_NO_RELEASES"

	if _, err := Reconstruct(ctx, pool, ReconstructOptions{ProjectID: missingID}); !errors.Is(err, ErrNoSuchProject) {
		t.Errorf("unknown project: err = %v, want ErrNoSuchProject", err)
	}

	seedProject(t, pool, emptyID)
	if _, err := Reconstruct(ctx, pool, ReconstructOptions{ProjectID: emptyID}); !errors.Is(err, ErrProjectHasNoReleases) {
		t.Errorf("project without releases: err = %v, want ErrProjectHasNoReleases", err)
	}
}
