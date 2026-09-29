package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Giving a parent's children an ontology of their own.
//
// A child pinned to a parent's release resolves the ontologies that release
// archived, and nothing added to the parent since. Usually the answer is to
// re-pin (see RepinChildren), but not when the parent's newer release also
// carries work that is not ready: on prod, LA's draft holds 341 modified and
// 258 new fields, so re-pinning children onto it would hand them all of that
// to get one helper ontology.
//
// This is the other remedy, and the one #3597 listed third: give each child
// the ontology directly. The child then owns it, independent of which release
// it reads, and no release is rewritten to pretend it contained something it
// did not.
//
// A child that owns an ontology AND later inherits the same one resolves it
// once, own winning on overlap, so this does not become wrong if the children
// are re-pinned later. It leaves a redundant row, not a conflict.

// ErrNoSuchOntology is returned when the ontology cannot be resolved, or when
// a prefix matches more than one version and the caller must say which.
var ErrNoSuchOntology = errors.New("no such ontology")

// AddOntologyOptions selects which children are given the ontology.
type AddOntologyOptions struct {
	// PinnedTo, when set, limits the change to children pinned at that
	// version of the parent.
	PinnedTo string
	// IncludeDraft also gives it to children that follow the parent's draft.
	// Off by default: such a child already resolves whatever the parent's
	// draft carries, so a copy of its own buys nothing.
	IncludeDraft bool
	// Note is written to usage_notes, so the row says why it exists.
	Note string
	// Apply writes. Without it the change is reported and nothing happens.
	Apply bool
}

// OntologyRef is a resolved ontology version.
type OntologyRef struct {
	VersionID string
	Prefix    string
	Name      string
	Version   string
}

func (o OntologyRef) String() string {
	return fmt.Sprintf("%s %s (%s)", o.Name, o.Version, o.Prefix)
}

// AddOntologyChild is one child's outcome.
type AddOntologyChild struct {
	ProjectID   string
	FromMode    string
	FromVersion string
	Skipped     bool
	Reason      string
}

// AddOntologyReport is one run.
type AddOntologyReport struct {
	ParentID string
	Ontology OntologyRef
	Applied  bool
	Children []AddOntologyChild
}

// Changed counts the children this run gives the ontology to, or gave it to.
func (r *AddOntologyReport) Changed() int {
	n := 0
	for _, c := range r.Children {
		if !c.Skipped {
			n++
		}
	}
	return n
}

// AddOntologyToChildren gives every selected child of parentID its own link to
// the named ontology.
//
// ontology is either a `weave_ontology_versions.id` or an ontology prefix. A
// prefix matching several versions is an error naming them, rather than a
// silent pick — linking a project to the wrong version of an ontology is not
// something to guess at.
func AddOntologyToChildren(ctx context.Context, pool *pgxpool.Pool, parentID, ontology string, opts AddOntologyOptions) (*AddOntologyReport, error) {
	parentID, ontology = strings.TrimSpace(parentID), strings.TrimSpace(ontology)
	if parentID == "" || ontology == "" {
		return nil, errors.New("parent project id and ontology are both required")
	}

	var parentExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM weave_projects WHERE id = $1)`, parentID).Scan(&parentExists); err != nil {
		return nil, fmt.Errorf("check parent: %w", err)
	}
	if !parentExists {
		return nil, fmt.Errorf("%w: %s — check the spelling", ErrNoSuchParent, parentID)
	}

	ref, err := resolveOntology(ctx, pool, ontology)
	if err != nil {
		return nil, err
	}

	children, err := classifyForOntology(ctx, pool, parentID, ref.VersionID, opts)
	if err != nil {
		return nil, err
	}
	report := &AddOntologyReport{ParentID: parentID, Ontology: ref, Children: children}

	if !opts.Apply || report.Changed() == 0 {
		return report, nil
	}
	if err := applyOntologyLinks(ctx, pool, ref.VersionID, opts.Note, children); err != nil {
		return report, err
	}
	report.Applied = true
	return report, nil
}

// resolveOntology accepts a version id or an ontology prefix.
func resolveOntology(ctx context.Context, pool *pgxpool.Pool, ontology string) (OntologyRef, error) {
	var ref OntologyRef
	err := pool.QueryRow(ctx, `
		SELECT ov.id, o.prefix, o.name, ov.version_string
		FROM weave_ontology_versions ov
		JOIN weave_ontologies o ON o.id = ov.ontology_id
		WHERE ov.id = $1`, ontology).Scan(&ref.VersionID, &ref.Prefix, &ref.Name, &ref.Version)
	if err == nil {
		return ref, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ref, fmt.Errorf("resolve ontology by id: %w", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT ov.id, o.prefix, o.name, ov.version_string
		FROM weave_ontology_versions ov
		JOIN weave_ontologies o ON o.id = ov.ontology_id
		WHERE o.prefix = $1
		ORDER BY ov.version_string`, ontology)
	if err != nil {
		return ref, fmt.Errorf("resolve ontology by prefix: %w", err)
	}
	defer rows.Close()

	var found []OntologyRef
	for rows.Next() {
		var r OntologyRef
		if err := rows.Scan(&r.VersionID, &r.Prefix, &r.Name, &r.Version); err != nil {
			return ref, fmt.Errorf("scan ontology: %w", err)
		}
		found = append(found, r)
	}
	if err := rows.Err(); err != nil {
		return ref, fmt.Errorf("iterate ontologies: %w", err)
	}

	switch len(found) {
	case 0:
		return ref, fmt.Errorf("%w: %s is neither an ontology version id nor a known prefix", ErrNoSuchOntology, ontology)
	case 1:
		return found[0], nil
	default:
		var versions []string
		for _, f := range found {
			versions = append(versions, f.Version+" ("+f.VersionID+")")
		}
		return ref, fmt.Errorf("%w: prefix %s has %d versions — name one by its id: %s",
			ErrNoSuchOntology, ontology, len(found), strings.Join(versions, ", "))
	}
}

// classifyForOntology reads the parent's children and decides, per child,
// whether this run gives it the ontology.
func classifyForOntology(ctx context.Context, pool *pgxpool.Pool, parentID, versionID string, opts AddOntologyOptions) ([]AddOntologyChild, error) {
	rows, err := pool.Query(ctx, `
		SELECT i.project_id, i.source_mode, COALESCE(i.source_version, ''),
		       EXISTS (SELECT 1 FROM weave_project_ontology_versions o
		               WHERE o.project_id = i.project_id AND o.ontology_version_id = $2)
		FROM weave_project_inheritance i
		WHERE i.parent_project_id = $1
		ORDER BY i.project_id`, parentID, versionID)
	if err != nil {
		return nil, fmt.Errorf("list children: %w", err)
	}
	defer rows.Close()

	var out []AddOntologyChild
	for rows.Next() {
		var c AddOntologyChild
		var alreadyLinked bool
		if err := rows.Scan(&c.ProjectID, &c.FromMode, &c.FromVersion, &alreadyLinked); err != nil {
			return nil, fmt.Errorf("scan child: %w", err)
		}
		switch {
		case alreadyLinked:
			c.Skipped, c.Reason = true, "already has it"
		case c.FromMode == "draft" && !opts.IncludeDraft:
			c.Skipped, c.Reason = true, "follows the parent's draft, so already resolves it"
		case opts.PinnedTo != "" && c.FromVersion != opts.PinnedTo:
			c.Skipped, c.Reason = true, "not pinned at "+opts.PinnedTo
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate children: %w", err)
	}
	return out, nil
}

// applyOntologyLinks writes every unskipped link in one transaction.
//
// is_primary is false: this is a helper ontology a child is being given, never
// the ontology its paths are built on. The insert is idempotent on the live
// table's (project_id, ontology_version_id) primary key.
func applyOntologyLinks(ctx context.Context, pool *pgxpool.Pool, versionID, note string, children []AddOntologyChild) error {
	if strings.TrimSpace(note) == "" {
		note = "added to children by pletkactl project add-ontology"
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, c := range children {
		if c.Skipped {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO weave_project_ontology_versions
				(project_id, ontology_version_id, added_at, is_primary, usage_notes)
			VALUES ($1, $2, NOW(), false, $3)
			ON CONFLICT (project_id, ontology_version_id) DO NOTHING`,
			c.ProjectID, versionID, note); err != nil {
			return fmt.Errorf("link %s: %w", c.ProjectID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Render writes the run's report, naming every child and why it was skipped.
func (r *AddOntologyReport) Render(w io.Writer) error {
	var b strings.Builder

	mode := "dry run — nothing written"
	if r.Applied {
		mode = "applied"
	}
	fmt.Fprintf(&b, "give %s to the children of %s (%s)\n\n", r.Ontology, r.ParentID, mode)

	if len(r.Children) == 0 {
		fmt.Fprintf(&b, "%s has no children.\n", r.ParentID)
		_, err := io.WriteString(w, b.String())
		return err
	}

	for _, c := range r.Children {
		from := c.FromMode
		if c.FromVersion != "" {
			from += " " + c.FromVersion
		}
		if c.Skipped {
			fmt.Fprintf(&b, "  %-12s %-16s skipped — %s\n", c.ProjectID, from, c.Reason)
			continue
		}
		fmt.Fprintf(&b, "  %-12s %-16s -> gains %s\n", c.ProjectID, from, r.Ontology.Prefix)
	}

	fmt.Fprintf(&b, "\n%d of %d children gain it.\n", r.Changed(), len(r.Children))
	if !r.Applied && r.Changed() > 0 {
		fmt.Fprintln(&b, "Re-run with --apply to write.")
	}

	_, err := io.WriteString(w, b.String())
	return err
}
