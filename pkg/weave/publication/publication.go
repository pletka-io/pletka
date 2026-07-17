// Package publication derives a live entity's publication state — whether it
// matches the project's latest release (published), has unreleased edits
// (modified), or was created after the release (new) — by comparing live rows
// (and their overrides) to the release snapshot. Nothing is stored; the state
// is computed on read. See docs/plans/2026-07-17-publication-state-tier2.md.
package publication

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Attach batch-fetches publication state for ids and writes each into the row
// the setter points at (ids[i] is the id of row i). Best-effort: a nil reader
// or a query error leaves states unset (logged). Callers wrap it in their
// existing len(items)>0 batch block, passing the same ids slice.
func Attach(ctx context.Context, r *Reader, log *slog.Logger, projectID, entityType string, ids []string, set func(i int) *string) {
	if r == nil || len(ids) == 0 {
		return
	}
	st, err := r.BatchState(ctx, projectID, entityType, ids)
	if err != nil {
		if log != nil {
			log.Warn("publication state", "project", projectID, "entity_type", entityType, "err", err)
		}
		return
	}
	for i, id := range ids {
		if s, ok := st[id]; ok {
			*set(i) = string(s)
		}
	}
}

// State is a live entity's publication state relative to the latest release.
type State string

const (
	// StateDraft means the project has no release yet — nothing is published.
	StateDraft State = "draft"
	// StatePublished means the live entity matches the latest release.
	StatePublished State = "published"
	// StateModified means the live entity (or one of its overrides) changed
	// since the latest release.
	StateModified State = "modified"
	// StateNew means the live entity was created after the latest release.
	StateNew State = "new"
)

// Rollup counts a project's live entities that differ from the latest release.
type Rollup struct {
	Modified int `json:"modified"`
	New      int `json:"new"`
}

// Unreleased is the total count of live entities not matching the release.
func (r Rollup) Unreleased() int { return r.Modified + r.New }

// Reader computes publication state from the database. Read-only.
type Reader struct {
	pool *pgxpool.Pool
}

// NewReader returns a Reader over the given pool.
func NewReader(pool *pgxpool.Pool) *Reader { return &Reader{pool: pool} }

// entitySpec maps an entity type to its live table, archive table, and the
// override rows that make up its content.
type entitySpec struct {
	table     string // live + archive share the id/version_number shape
	archive   string
	ovrType   string // weave_field_overrides.entity_type: '' | 'model' | 'collection'
	ovrKeyCol string // the override column that points back at the entity id
}

func specFor(entityType string) (entitySpec, error) {
	switch entityType {
	case "field":
		return entitySpec{"weave_fields", "weave_fields_archive", "", "field_id"}, nil
	case "model":
		return entitySpec{"weave_models", "weave_models_archive", "model", "entity_id"}, nil
	case "collection":
		return entitySpec{"weave_collections", "weave_collections_archive", "collection", "entity_id"}, nil
	default:
		return entitySpec{}, fmt.Errorf("publication: unknown entity type %q", entityType)
	}
}

// latestReleaseCTE selects the highest-semver release for $1, tolerating an
// optional -prerelease / +build suffix by comparing only the numeric core.
const latestReleaseCTE = `
rel AS (
  SELECT version, created_at
  FROM weave_releases
  WHERE project_id = $1
  ORDER BY string_to_array(split_part(split_part(version, '-', 1), '+', 1), '.')::int[] DESC,
           created_at DESC
  LIMIT 1
)`

// stateCase is the shared CASE deriving State from the joined rel/archive/ovr.
const stateCase = `
  CASE
    WHEN rel.version IS NULL THEN 'draft'
    WHEN a.id IS NULL THEN 'new'
    WHEN e.updated_at > rel.created_at OR ovr.mu > rel.created_at THEN 'modified'
    ELSE 'published'
  END`

// ovrCTE builds the "latest override edit per entity" CTE body for a spec.
func ovrCTE(s entitySpec) string {
	return fmt.Sprintf(`
  SELECT %[1]s AS eid, max(updated_at) AS mu
  FROM weave_field_overrides
  WHERE project_id = $1 AND entity_type = '%[2]s'
  GROUP BY %[1]s`, s.ovrKeyCol, s.ovrType)
}

// typedStateSelect builds the per-type SELECT of (id, state) over live rows.
// idFilter, when true, restricts to e.id = ANY($2).
func typedStateSelect(s entitySpec, idFilter bool) string {
	where := "e.project_id = $1 AND e.version_number = ''"
	if idFilter {
		where += " AND e.id = ANY($2)"
	}
	return fmt.Sprintf(`
SELECT e.id, %[3]s AS state
FROM %[1]s e
LEFT JOIN rel ON true
LEFT JOIN %[2]s a ON a.id = e.id AND a.version_number = rel.version
LEFT JOIN ovr ON ovr.eid = e.id
WHERE %[4]s`, s.table, s.archive, stateCase, where)
}

// BatchState returns publication state for the given live entity ids of one
// type ("field"|"model"|"collection") in a project. Ids absent from the result
// (e.g. wrong project, non-live) are simply omitted.
func (r *Reader) BatchState(ctx context.Context, projectID, entityType string, ids []string) (map[string]State, error) {
	out := make(map[string]State, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	spec, err := specFor(entityType)
	if err != nil {
		return nil, err
	}
	q := "WITH " + latestReleaseCTE + ", ovr AS (" + ovrCTE(spec) + ")" + typedStateSelect(spec, true)
	rows, err := r.pool.Query(ctx, q, projectID, ids)
	if err != nil {
		return nil, fmt.Errorf("publication batch state (%s): %w", entityType, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, state string
		if err := rows.Scan(&id, &state); err != nil {
			return nil, fmt.Errorf("scan publication state: %w", err)
		}
		out[id] = State(state)
	}
	return out, rows.Err()
}

// ProjectRollup counts the project's live fields+models+collections that are
// new or modified relative to the latest release.
func (r *Reader) ProjectRollup(ctx context.Context, projectID string) (Rollup, error) {
	var rollup Rollup
	fSpec, _ := specFor("field")
	mSpec, _ := specFor("model")
	cSpec, _ := specFor("collection")
	// One query: shared rel CTE + per-type ovr CTEs, union the state selects,
	// count the new/modified buckets.
	q := fmt.Sprintf(`
WITH %[1]s,
ovr_f AS (%[2]s),
ovr_m AS (%[3]s),
ovr_c AS (%[4]s)
SELECT state, count(*) FROM (
  %[5]s
  UNION ALL
  %[6]s
  UNION ALL
  %[7]s
) s
WHERE state IN ('new','modified')
GROUP BY state`,
		latestReleaseCTE,
		ovrCTE(fSpec), ovrCTE(mSpec), ovrCTE(cSpec),
		typedStateSelectAliased(fSpec, "ovr_f"),
		typedStateSelectAliased(mSpec, "ovr_m"),
		typedStateSelectAliased(cSpec, "ovr_c"),
	)
	rows, err := r.pool.Query(ctx, q, projectID)
	if err != nil {
		return rollup, fmt.Errorf("publication rollup: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var n int
		if err := rows.Scan(&state, &n); err != nil {
			return rollup, fmt.Errorf("scan rollup: %w", err)
		}
		switch State(state) {
		case StateModified:
			rollup.Modified = n
		case StateNew:
			rollup.New = n
		}
	}
	return rollup, rows.Err()
}

// typedStateSelectAliased is typedStateSelect with a named ovr CTE (for the
// rollup union, which has one ovr CTE per type).
func typedStateSelectAliased(s entitySpec, ovrAlias string) string {
	return fmt.Sprintf(`
SELECT %[3]s AS state
FROM %[1]s e
LEFT JOIN rel ON true
LEFT JOIN %[2]s a ON a.id = e.id AND a.version_number = rel.version
LEFT JOIN %[4]s ovr ON ovr.eid = e.id
WHERE e.project_id = $1 AND e.version_number = ''`, s.table, s.archive, stateCase, ovrAlias)
}
