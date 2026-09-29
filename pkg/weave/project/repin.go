package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Re-pinning a parent's children onto a newer release.
//
// A child inherits from a parent either by following its draft or by pinning
// one of its releases. A pinned child keeps reading that release until
// something moves it — correctly, since that is what pinning means. So when a
// parent cuts a new release, nothing its children read changes, and content
// added to the parent since the old release stays invisible to them. That is
// Redmine #3597: la-addons was linked to LA seven weeks after LA 0.1.0, so
// the 38 children pinned to 0.1.0 resolve none of its terms.
//
// The remedy is to re-pin, which is a bulk mechanical operation the UI can
// only do one child at a time. This is that operation.

var (
	// ErrNoSuchRelease is returned when the target version is not a release
	// of the parent. Pinning children to a version that does not exist would
	// leave every one of them resolving nothing.
	ErrNoSuchRelease = errors.New("no such release")
	// ErrNoSuchParent is returned when the parent id matches no project, so
	// a typo cannot read as "nothing to do".
	ErrNoSuchParent = errors.New("no such project")
)

// RepinOptions selects which children move and whether the move is written.
type RepinOptions struct {
	// From, when set, limits the move to children currently pinned at that
	// version. Without it every already-pinned child moves, whatever version
	// it sits on.
	From string
	// IncludeDraft also converts children that follow the parent's draft.
	// Off by default: following draft is a deliberate choice (on prod, SI,
	// MRD and SUR follow LA that way), and silently pinning them would take
	// away the live view they asked for.
	IncludeDraft bool
	// Apply writes. Without it the move is reported and nothing changes.
	Apply bool
}

// RepinChild is one child's move, before and after.
type RepinChild struct {
	ProjectID string
	// FromMode is "release" or "draft"; FromVersion is empty for draft.
	FromMode    string
	FromVersion string
	// Skipped is set when the child is already pinned at the target version,
	// or follows draft and IncludeDraft was not given. Reason says which.
	Skipped bool
	Reason  string
}

// RepinReport is one run.
type RepinReport struct {
	ParentID string
	Version  string
	Applied  bool
	Children []RepinChild
}

// Moved counts the children this run moves, or moved.
func (r *RepinReport) Moved() int {
	n := 0
	for _, c := range r.Children {
		if !c.Skipped {
			n++
		}
	}
	return n
}

// RepinChildren points a parent's children at one of its releases.
//
// It refuses a parent or version that does not exist rather than reporting
// zero children, so a mistyped argument fails loudly instead of reading as
// "there was nothing to do" — the shape of failure that took production's API
// keys down once already (a deploy-migrate against a mistyped instance name).
func RepinChildren(ctx context.Context, pool *pgxpool.Pool, parentID, version string, opts RepinOptions) (*RepinReport, error) {
	parentID, version = strings.TrimSpace(parentID), strings.TrimSpace(version)
	if parentID == "" || version == "" {
		return nil, errors.New("parent project id and version are both required")
	}
	if err := checkRepinTarget(ctx, pool, parentID, version); err != nil {
		return nil, err
	}

	children, err := classifyChildren(ctx, pool, parentID, version, opts)
	if err != nil {
		return nil, err
	}
	report := &RepinReport{ParentID: parentID, Version: version, Children: children}

	if !opts.Apply || report.Moved() == 0 {
		return report, nil
	}
	if err := applyRepins(ctx, pool, parentID, version, children); err != nil {
		return report, err
	}
	report.Applied = true
	return report, nil
}

// checkRepinTarget refuses a parent or version that does not exist, so a
// mistyped argument fails loudly instead of reading as "nothing to do".
func checkRepinTarget(ctx context.Context, pool *pgxpool.Pool, parentID, version string) error {
	var parentExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM weave_projects WHERE id = $1)`, parentID).Scan(&parentExists); err != nil {
		return fmt.Errorf("check parent: %w", err)
	}
	if !parentExists {
		return fmt.Errorf("%w: %s — check the spelling", ErrNoSuchParent, parentID)
	}

	var releaseExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM weave_releases WHERE project_id = $1 AND version = $2)`,
		parentID, version).Scan(&releaseExists); err != nil {
		return fmt.Errorf("check release: %w", err)
	}
	if !releaseExists {
		return fmt.Errorf("%w: %s has no release %s — cut it first", ErrNoSuchRelease, parentID, version)
	}
	return nil
}

// classifyChildren reads the parent's children and decides, per child,
// whether this run moves it.
func classifyChildren(ctx context.Context, pool *pgxpool.Pool, parentID, version string, opts RepinOptions) ([]RepinChild, error) {
	rows, err := pool.Query(ctx, `
		SELECT project_id, source_mode, COALESCE(source_version, '')
		FROM weave_project_inheritance
		WHERE parent_project_id = $1
		ORDER BY project_id`, parentID)
	if err != nil {
		return nil, fmt.Errorf("list children: %w", err)
	}
	defer rows.Close()

	var out []RepinChild
	for rows.Next() {
		var c RepinChild
		if err := rows.Scan(&c.ProjectID, &c.FromMode, &c.FromVersion); err != nil {
			return nil, fmt.Errorf("scan child: %w", err)
		}
		switch {
		case c.FromMode == "draft" && !opts.IncludeDraft:
			c.Skipped, c.Reason = true, "follows draft (--include-draft to move it)"
		case c.FromMode == "release" && c.FromVersion == version:
			c.Skipped, c.Reason = true, "already pinned here"
		case opts.From != "" && c.FromVersion != opts.From:
			c.Skipped, c.Reason = true, "not pinned at "+opts.From
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate children: %w", err)
	}
	return out, nil
}

// applyRepins writes every unskipped move in one transaction, so a parent's
// children never end up half moved.
func applyRepins(ctx context.Context, pool *pgxpool.Pool, parentID, version string, children []RepinChild) error {
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
			UPDATE weave_project_inheritance
			SET source_mode = 'release', source_version = $3
			WHERE project_id = $1 AND parent_project_id = $2`,
			c.ProjectID, parentID, version); err != nil {
			return fmt.Errorf("re-pin %s: %w", c.ProjectID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Render writes the run's report: every child named, with what it moves from,
// so an operator can see which children were left alone and why. Built in
// memory and written once, so a short write is reported rather than leaving a
// truncated report that looks complete.
func (r *RepinReport) Render(w io.Writer) error {
	var b strings.Builder

	mode := "dry run — nothing written"
	if r.Applied {
		mode = "applied"
	}
	fmt.Fprintf(&b, "re-pin children of %s onto %s (%s)\n\n", r.ParentID, r.Version, mode)

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
		fmt.Fprintf(&b, "  %-12s %-16s -> release %s\n", c.ProjectID, from, r.Version)
	}

	fmt.Fprintf(&b, "\n%d of %d children move.\n", r.Moved(), len(r.Children))
	if !r.Applied && r.Moved() > 0 {
		fmt.Fprintln(&b, "Re-run with --apply to write.")
	}

	_, err := io.WriteString(w, b.String())
	return err
}
