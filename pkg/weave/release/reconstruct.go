package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Reconstruction fills the seven archive tables added alongside examples,
// vocabularies, collection placements and project credits for releases that
// were cut before those archives existed.
//
// It does NOT snapshot today's draft: on a fleet where most projects have
// been edited since their last release, that would publish in-progress work
// under an existing version number. It reconstructs from row timestamps
// instead, and refuses to guess where the timestamps cannot tell it:
//
//	created_at >  release.created_at                             -> excluded
//	created_at <= release.created_at, updated_at <= release      -> exact
//	created_at <= release.created_at, updated_at >  release      -> ambiguous
//
// An ambiguous row belonged to the release, but its pre-release content is
// gone: only the current content survives, and writing that into the archive
// would fabricate release history. Such rows are reported individually and
// never written.
//
// Two of the seven live tables, weave_project_attributions and
// weave_project_actors, have no updated_at column at all (see
// 001_baseline.sql). For them created-before is the only available signal,
// so a row edited after the release is archived with its current content and
// this method cannot tell. The report prints an explicit caveat rather than
// implying a precision it does not have.
//
// Rows deleted since a release are likewise unrecoverable and invisible here.

// ErrAmbiguousRows is returned when Apply was requested, at least one
// ambiguous row was found, and AcceptAmbiguous was not set. Nothing is
// written in that case: an operator must not be able to fill archives while
// silently skipping rows they were never told about.
var ErrAmbiguousRows = errors.New("ambiguous rows found: re-run with --accept-ambiguous to archive the unambiguous rows and leave these out")

// ErrNoSuchProject and ErrProjectHasNoReleases are returned when an explicit
// ProjectID selects nothing. An explicit project that matches nothing is a
// mistyped argument far more often than it is an empty result, and a silent
// exit 0 on a mistyped instance or project name has already cost this fleet
// one production outage. An omitted ProjectID that finds nothing stays a
// clean, non-error empty run.
var (
	ErrNoSuchProject        = errors.New("no such project")
	ErrProjectHasNoReleases = errors.New("project has no releases")
)

// ReconstructOptions selects what a reconstruction run covers and whether it
// writes. The zero value is a dry run over every project that has releases.
type ReconstructOptions struct {
	// ProjectID limits the run to one project. Empty means every project
	// with at least one release.
	ProjectID string
	// Apply writes the exact rows into the archives. False classifies and
	// reports only.
	Apply bool
	// AcceptAmbiguous allows an Apply run to proceed despite ambiguous rows,
	// archiving the exact rows and leaving the ambiguous ones out.
	AcceptAmbiguous bool
}

// AmbiguousRow is one live row that belonged to a release but has been edited
// since, so its release-time content no longer exists.
type AmbiguousRow struct {
	// Identity is the live row's key rendered as text: an id for the five
	// tables that have one, the composite key for the two credit tables.
	Identity  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableReport is one live table's classification for one release.
type TableReport struct {
	LiveTable    string
	ArchiveTable string
	// HasUpdatedAt is false for the two credit tables, whose rows can only
	// be classified as exact or excluded.
	HasUpdatedAt bool
	// AlreadyArchived reports that this table already holds rows for this
	// release — the release was cut after the archives shipped, so its
	// content is authoritative and no reconstruction is attempted.
	AlreadyArchived bool
	Exact           int
	Excluded        int
	Ambiguous       []AmbiguousRow
	// Inserted is the number of rows actually written. Zero on a dry run,
	// and zero on a re-run where every exact row is already archived.
	Inserted int64
	// insert is this table's archive INSERT, carried here rather than looked
	// up by position in reconstructTables at apply time. The apply loop used
	// to index the global slice by the report entry's position, which held
	// only while classification appended exactly one report per table in
	// order — a `continue` added to that loop would have silently run one
	// table's insert against another table's report.
	insert string
}

// VersionReport is one release — one (project, version) — reconstructed
// across all seven tables.
type VersionReport struct {
	ProjectID string
	Version   string
	CreatedAt time.Time
	Tables    []TableReport
}

// ReconstructReport is a whole run.
type ReconstructReport struct {
	Applied  bool
	Releases []VersionReport
}

// Totals sums the three classes across every release and table in the run.
func (r *ReconstructReport) Totals() (exact, excluded, ambiguous int) {
	for _, rel := range r.Releases {
		for _, t := range rel.Tables {
			exact += t.Exact
			excluded += t.Excluded
			ambiguous += len(t.Ambiguous)
		}
	}
	return exact, excluded, ambiguous
}

// reconstructTable describes how one live table is scoped to a project and
// copied into its archive.
type reconstructTable struct {
	live    string
	archive string
	// prefix qualifies the timestamp columns in the classification query and
	// in the insert's WHERE clause — "" when the live table is unaliased,
	// "v." / "e." when it is joined to its parent to get a project scope.
	prefix string
	// hasUpdatedAt is false for the two credit tables.
	hasUpdatedAt bool
	// identity renders the live row's key as text for the ambiguous listing.
	identity string
	// from is the FROM clause (live table plus any parent join) used by the
	// classification query.
	from string
	// scope is the project filter over from, with $1 = project id.
	scope string
	// exists reports whether the archive already holds rows for this
	// release, with $1 = project id and $2 = version.
	exists string
	// insert copies the exact rows, with $1 = project id, $2 = version and
	// $3 = the release's created_at. Its column list and ON CONFLICT target
	// mirror the matching entry in snapshotStatements — see
	// TestReconstructInsertsMatchSnapshotStatements, which fails if the two
	// drift apart.
	insert string
}

// exactPredicate is the SQL for "this row's content is still the release's
// content". Both the classification query and the insert's WHERE clause are
// built from it, so the two cannot disagree about what "exact" means.
func exactPredicate(prefix, param string, hasUpdatedAt bool) string {
	if !hasUpdatedAt {
		return fmt.Sprintf("%screated_at <= %s", prefix, param)
	}
	return fmt.Sprintf("%screated_at <= %s AND %supdated_at <= %s", prefix, param, prefix, param)
}

// scopeByProject is the WHERE fragment for the archives whose live table
// carries project_id directly; the joined ones qualify it with an alias.
const scopeByProject = "project_id = $1"

var reconstructTables = []reconstructTable{
	{
		live: "weave_examples", archive: "weave_examples_archive",
		hasUpdatedAt: true, identity: "id",
		from:  "weave_examples",
		scope: scopeByProject,
		exists: `SELECT EXISTS (SELECT 1 FROM weave_examples_archive
			WHERE project_id = $1 AND version_number = $2)`,
		insert: `INSERT INTO weave_examples_archive (
			id, project_id, entity_type, entity_id, title, description, status,
			created_at, updated_at, version_number
		)
		SELECT
			id, project_id, entity_type, entity_id, title, description, status,
			created_at, updated_at, $2
		FROM weave_examples
		WHERE project_id = $1 AND ` + exactPredicate("", "$3", true) + `
		ON CONFLICT (id, version_number) DO NOTHING`,
	},
	{
		live: "weave_example_values", archive: "weave_example_values_archive",
		hasUpdatedAt: true, identity: "v.id::text", prefix: "v.",
		from:  "weave_example_values v JOIN weave_examples e ON e.id = v.example_id",
		scope: "e.project_id = $1",
		exists: `SELECT EXISTS (SELECT 1 FROM weave_example_values_archive v
			JOIN weave_examples e ON e.id = v.example_id
			WHERE e.project_id = $1 AND v.version_number = $2)`,
		insert: `INSERT INTO weave_example_values_archive (
			id, example_id, override_id, field_id, part_of_collection_id,
			occurrence_index, value_kind, value_payload, text_value, number_value,
			date_value, uri_value, concept_uri, linked_example_id, slot_path,
			created_at, updated_at, version_number
		)
		SELECT
			v.id, v.example_id, v.override_id, v.field_id, v.part_of_collection_id,
			v.occurrence_index, v.value_kind, v.value_payload, v.text_value, v.number_value,
			v.date_value, v.uri_value, v.concept_uri, v.linked_example_id, v.slot_path,
			v.created_at, v.updated_at, $2
		FROM weave_example_values v
		JOIN weave_examples e ON e.id = v.example_id
		WHERE e.project_id = $1 AND ` + exactPredicate("v.", "$3", true) + `
		ON CONFLICT (id, version_number) DO NOTHING`,
	},
	{
		live: "weave_vocabularies", archive: "weave_vocabularies_archive",
		hasUpdatedAt: true, identity: "id",
		from:  "weave_vocabularies",
		scope: scopeByProject,
		exists: `SELECT EXISTS (SELECT 1 FROM weave_vocabularies_archive
			WHERE project_id = $1 AND version_number = $2)`,
		insert: `INSERT INTO weave_vocabularies_archive (
			id, project_id, semantic_id, system_name, ui_name, description,
			status, connector_type, base_uri, config, deprecated,
			created_at, updated_at, version_number
		)
		SELECT
			id, project_id, semantic_id, system_name, ui_name, description,
			CASE WHEN status = 'deprecated' THEN status ELSE 'published' END,
			connector_type, base_uri, config, deprecated,
			created_at, updated_at, $2
		FROM weave_vocabularies
		WHERE project_id = $1 AND ` + exactPredicate("", "$3", true) + `
		ON CONFLICT (id, version_number) DO NOTHING`,
	},
	{
		live: "weave_vocabulary_entries", archive: "weave_vocabulary_entries_archive",
		hasUpdatedAt: true, identity: "e.id", prefix: "e.",
		from:  "weave_vocabulary_entries e JOIN weave_vocabularies v ON v.id = e.vocabulary_id",
		scope: "v.project_id = $1",
		exists: `SELECT EXISTS (SELECT 1 FROM weave_vocabulary_entries_archive e
			JOIN weave_vocabularies v ON v.id = e.vocabulary_id
			WHERE v.project_id = $1 AND e.version_number = $2)`,
		insert: `INSERT INTO weave_vocabulary_entries_archive (
			id, vocabulary_id, uri, label, scope_note, broader_uri,
			broader_path, broader_path_items, external_id,
			created_at, updated_at, version_number
		)
		SELECT
			e.id, e.vocabulary_id, e.uri, e.label, e.scope_note, e.broader_uri,
			e.broader_path, e.broader_path_items, e.external_id,
			e.created_at, e.updated_at, $2
		FROM weave_vocabulary_entries e
		JOIN weave_vocabularies v ON v.id = e.vocabulary_id
		WHERE v.project_id = $1 AND ` + exactPredicate("e.", "$3", true) + `
		ON CONFLICT (id, version_number) DO NOTHING`,
	},
	{
		live: "weave_collection_placements", archive: "weave_collection_placements_archive",
		hasUpdatedAt: true, identity: "id::text",
		from:  "weave_collection_placements",
		scope: scopeByProject,
		exists: `SELECT EXISTS (SELECT 1 FROM weave_collection_placements_archive
			WHERE project_id = $1 AND version_number = $2)`,
		insert: `INSERT INTO weave_collection_placements_archive (
			id, project_id, model_id, category_id, collection_id,
			is_required, min_occurs, max_occurs, is_hidden,
			created_at, updated_at, version_number
		)
		SELECT
			id, project_id, model_id, category_id, collection_id,
			is_required, min_occurs, max_occurs, is_hidden,
			created_at, updated_at, $2
		FROM weave_collection_placements
		WHERE project_id = $1 AND ` + exactPredicate("", "$3", true) + `
		ON CONFLICT (id, version_number) DO NOTHING`,
	},
	// The two credit tables have no updated_at, and no id either: their
	// identity is the live primary key, and the archive's ON CONFLICT target
	// is that key in full plus version_number. A narrower target would let a
	// legitimately distinct row collide and vanish.
	{
		live: "weave_project_attributions", archive: "weave_project_attributions_archive",
		hasUpdatedAt: false,
		identity:     `actor_id || '/' || kind || '/' || "position"::text`,
		from:         "weave_project_attributions",
		scope:        scopeByProject,
		exists: `SELECT EXISTS (SELECT 1 FROM weave_project_attributions_archive
			WHERE project_id = $1 AND version_number = $2)`,
		insert: `INSERT INTO weave_project_attributions_archive (
			project_id, actor_id, kind, "position", note, created_at, version_number
		)
		SELECT
			project_id, actor_id, kind, "position", note, created_at, $2
		FROM weave_project_attributions
		WHERE project_id = $1 AND ` + exactPredicate("", "$3", false) + `
		ON CONFLICT (project_id, actor_id, kind, "position", version_number) DO NOTHING`,
	},
	{
		live: "weave_project_actors", archive: "weave_project_actors_archive",
		hasUpdatedAt: false,
		identity:     `actor_id || '/' || role`,
		from:         "weave_project_actors",
		scope:        scopeByProject,
		exists: `SELECT EXISTS (SELECT 1 FROM weave_project_actors_archive
			WHERE project_id = $1 AND version_number = $2)`,
		insert: `INSERT INTO weave_project_actors_archive (
			project_id, actor_id, role, created_at, version_number
		)
		SELECT
			project_id, actor_id, role, created_at, $2
		FROM weave_project_actors
		WHERE project_id = $1 AND ` + exactPredicate("", "$3", false) + `
		ON CONFLICT (project_id, actor_id, role, version_number) DO NOTHING`,
	},
}

// classifySQL builds the classification query for one table: every live row
// in the project, labeled exact / excluded / ambiguous against $2, the
// release's created_at. The exact branch reuses exactPredicate, the same
// expression the insert filters on.
//
// For a table with no updated_at the two branches are exact complements
// (created_at > cutoff, created_at <= cutoff, over a NOT NULL column), which
// is what makes the ambiguous class unreachable there rather than merely
// empty. Break that complement and such rows fall through to 'ambiguous' —
// which is the safe direction (reported, not archived), but it is a symptom
// of a broken predicate, not a real classification.
func (t reconstructTable) classifySQL() string {
	updated := "NULL::timestamptz"
	if t.hasUpdatedAt {
		updated = t.prefix + "updated_at"
	}
	return fmt.Sprintf(`
		SELECT %s AS row_identity, %screated_at, %s,
			CASE
				WHEN %screated_at > $2 THEN 'excluded'
				WHEN %s THEN 'exact'
				ELSE 'ambiguous'
			END AS class
		FROM %s
		WHERE %s
		ORDER BY 1`,
		t.identity, t.prefix, updated,
		t.prefix, exactPredicate(t.prefix, "$2", t.hasUpdatedAt),
		t.from, t.scope)
}

// Reconstruct classifies every live row of the seven archive-backed tables
// against each release's created_at, and — when opts.Apply is set and the
// ambiguity gate passes — writes the exact rows into the archives.
//
// Classification always runs over the whole selection first. Writes happen
// only after the gate, in a single transaction, so a run that is going to be
// refused writes nothing at all.
func Reconstruct(ctx context.Context, pool *pgxpool.Pool, opts ReconstructOptions) (*ReconstructReport, error) {
	releases, err := listReleasesForReconstruct(ctx, pool, opts.ProjectID)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 && opts.ProjectID != "" {
		exists, err := projectExists(ctx, pool, opts.ProjectID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("%w: %s", ErrNoSuchProject, opts.ProjectID)
		}
		return nil, fmt.Errorf("%w: %s", ErrProjectHasNoReleases, opts.ProjectID)
	}

	report := &ReconstructReport{}
	for _, rel := range releases {
		rr := VersionReport{ProjectID: rel.ProjectID, Version: rel.Version, CreatedAt: rel.CreatedAt}
		for _, tbl := range reconstructTables {
			tr, err := classifyTable(ctx, pool, tbl, rel)
			if err != nil {
				return nil, fmt.Errorf("classify %s for %s@%s: %w", tbl.live, rel.ProjectID, rel.Version, err)
			}
			rr.Tables = append(rr.Tables, tr)
		}
		report.Releases = append(report.Releases, rr)
	}

	if !opts.Apply {
		return report, nil
	}
	if _, _, ambiguous := report.Totals(); ambiguous > 0 && !opts.AcceptAmbiguous {
		return report, ErrAmbiguousRows
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return report, fmt.Errorf("begin reconstruct tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for i := range report.Releases {
		rel := &report.Releases[i]
		for j := range rel.Tables {
			if rel.Tables[j].AlreadyArchived {
				continue
			}
			tag, err := tx.Exec(ctx, rel.Tables[j].insert, rel.ProjectID, rel.Version, rel.CreatedAt)
			if err != nil {
				return report, fmt.Errorf("archive %s for %s@%s: %w",
					rel.Tables[j].LiveTable, rel.ProjectID, rel.Version, err)
			}
			rel.Tables[j].Inserted = tag.RowsAffected()
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return report, fmt.Errorf("commit reconstruct tx: %w", err)
	}
	report.Applied = true
	return report, nil
}

type releaseRef struct {
	ProjectID string
	Version   string
	CreatedAt time.Time
}

func listReleasesForReconstruct(ctx context.Context, pool *pgxpool.Pool, projectID string) ([]releaseRef, error) {
	rows, err := pool.Query(ctx, `
		SELECT project_id, version, created_at
		FROM weave_releases
		WHERE $1::text = '' OR project_id = $1::text
		ORDER BY project_id, created_at, version`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}
	defer rows.Close()

	out := []releaseRef{}
	for rows.Next() {
		var r releaseRef
		if err := rows.Scan(&r.ProjectID, &r.Version, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func projectExists(ctx context.Context, pool *pgxpool.Pool, projectID string) (bool, error) {
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM weave_projects WHERE id = $1)`, projectID).Scan(&exists); err != nil {
		return false, fmt.Errorf("look up project %s: %w", projectID, err)
	}
	return exists, nil
}

func classifyTable(ctx context.Context, pool *pgxpool.Pool, tbl reconstructTable, rel releaseRef) (TableReport, error) {
	tr := TableReport{
		LiveTable:    tbl.live,
		ArchiveTable: tbl.archive,
		HasUpdatedAt: tbl.hasUpdatedAt,
		insert:       tbl.insert,
	}

	// The two joined EXISTS queries reach the archive through its LIVE parent
	// (examples, vocabularies), so an archived child whose live parent has
	// since been deleted reads as "not archived" and the table is classified
	// again. That is harmless — the re-run inserts nothing, because the
	// classification walks the same live parent — but it is why this is an
	// optimisation and a report-noise fix, never a correctness guarantee.
	var archived bool
	if err := pool.QueryRow(ctx, tbl.exists, rel.ProjectID, rel.Version).Scan(&archived); err != nil {
		return tr, err
	}
	if archived {
		// The release was cut after this archive shipped, so the archive
		// already holds its real content. Reconstructing over it would at
		// best be a no-op (ON CONFLICT DO NOTHING) and at worst report
		// ambiguity that does not exist.
		tr.AlreadyArchived = true
		return tr, nil
	}

	rows, err := pool.Query(ctx, tbl.classifySQL(), rel.ProjectID, rel.CreatedAt)
	if err != nil {
		return tr, err
	}
	defer rows.Close()

	for rows.Next() {
		var identity, class string
		var created time.Time
		var updated *time.Time
		if err := rows.Scan(&identity, &created, &updated, &class); err != nil {
			return tr, err
		}
		switch class {
		case "exact":
			tr.Exact++
		case "excluded":
			tr.Excluded++
		default:
			amb := AmbiguousRow{Identity: identity, CreatedAt: created}
			if updated != nil {
				amb.UpdatedAt = *updated
			}
			tr.Ambiguous = append(tr.Ambiguous, amb)
		}
	}
	return tr, rows.Err()
}

const reconstructPreamble = `Rows are classified against each release's created_at:
  exact      created before the release and untouched since — its current
             content IS the release's content, so it is archived
  excluded   created after the release — confidently not in it
  ambiguous  created before the release but edited since — it belonged to the
             release, but its pre-release content is gone. Reported, never
             guessed, never archived.

Rows DELETED since a release are unrecoverable and invisible to this method.
`

// Render writes the run's report. Every ambiguous row is listed individually
// with its identity and both timestamps: the counts alone would let an
// operator fill the archives without ever seeing what was left out.
//
// The report is built in memory and written once, so a short write or a
// closed pipe is reported rather than leaving the operator with a truncated
// report that looks complete.
func (r *ReconstructReport) Render(w io.Writer, applyRequested bool) error {
	var b strings.Builder

	mode := "dry run — nothing written"
	if applyRequested {
		mode = "apply"
		if !r.Applied {
			mode = "apply — REFUSED, nothing written"
		}
	}
	fmt.Fprintf(&b, "release content reconstruction (%s)\n\n%s\n", mode, reconstructPreamble)

	if len(r.Releases) == 0 {
		fmt.Fprintln(&b, "No releases matched — nothing to reconstruct.")
		_, err := io.WriteString(w, b.String())
		return err
	}

	for _, rel := range r.Releases {
		fmt.Fprintf(&b, "%s @ %s  (released %s)\n", rel.ProjectID, rel.Version, rel.CreatedAt.UTC().Format(time.RFC3339))
		for _, t := range rel.Tables {
			if t.AlreadyArchived {
				fmt.Fprintf(&b, "  %-32s already archived — skipped\n", t.LiveTable)
				continue
			}
			ambiguous := fmt.Sprintf("%d", len(t.Ambiguous))
			if !t.HasUpdatedAt {
				ambiguous = "n/a"
			}
			fmt.Fprintf(&b, "  %-32s exact %-5d excluded %-5d ambiguous %s", t.LiveTable, t.Exact, t.Excluded, ambiguous)
			if r.Applied {
				fmt.Fprintf(&b, "  (archived %d)", t.Inserted)
			}
			fmt.Fprintln(&b)
			if !t.HasUpdatedAt {
				fmt.Fprintf(&b, "      CAVEAT: %s has no updated_at column. An edit to a row that\n", t.LiveTable)
				fmt.Fprintln(&b, "      already existed at the release is invisible here; created-before is")
				fmt.Fprintln(&b, "      the only signal, so such a row is archived with its CURRENT content.")
			}
			for _, a := range t.Ambiguous {
				fmt.Fprintf(&b, "      ambiguous: %s  created %s  updated %s\n",
					a.Identity, a.CreatedAt.UTC().Format(time.RFC3339), a.UpdatedAt.UTC().Format(time.RFC3339))
			}
		}
		fmt.Fprintln(&b)
	}

	exact, excluded, ambiguous := r.Totals()
	fmt.Fprintf(&b, "totals: %d exact, %d excluded, %d ambiguous\n", exact, excluded, ambiguous)

	_, err := io.WriteString(w, b.String())
	return err
}
