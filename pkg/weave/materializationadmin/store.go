// Package materializationadmin is a handler-only admin surface that reports
// the cost and health of async project-save git materialization. It reads the
// materialized_* columns on weave_change_set (recorded by the gitmaterializer)
// plus the live queue-depth / lag gauges. Read-only analytics; no store of its
// own beyond these queries. Mirrors the errortracking slice (GET /admin/errors).
package materializationadmin

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store runs read-only queries over weave_change_set.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store backed by the given pgx pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Run is one materialization run (one change set), newest first.
type Run struct {
	ID          int64      `json:"id"`
	ProjectID   string     `json:"project_id"`
	ActorName   string     `json:"actor_name"`
	StartedAt   time.Time  `json:"started_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
	Outcome     *string    `json:"outcome,omitempty"` // committed | noop | failed | null (pending)
	DurationMS  *int       `json:"duration_ms,omitempty"`
	Files       *int       `json:"files,omitempty"`
	Changed     *int       `json:"changed,omitempty"`
	CommitSHA   *string    `json:"commit_sha,omitempty"`
	Error       *string    `json:"error,omitempty"`
}

// Recent returns the most recent runs, newest first. limit is capped at 500.
func (s *Store) Recent(ctx context.Context, limit int) ([]Run, error) {
	if s == nil || s.pool == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, actor_name, started_at, processed_at,
		       materialized_outcome, materialized_duration_ms,
		       materialized_files, materialized_changed, git_commit_sha,
		       materialized_error
		FROM weave_change_set
		ORDER BY id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(
			&r.ID, &r.ProjectID, &r.ActorName, &r.StartedAt, &r.ProcessedAt,
			&r.Outcome, &r.DurationMS, &r.Files, &r.Changed, &r.CommitSHA, &r.Error,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Summary is the aggregate header: cost percentiles + outcome counts over a
// rolling window, plus the two live gauges.
type Summary struct {
	WindowHours int  `json:"window_hours"`
	Runs        int  `json:"runs"`
	Committed   int  `json:"committed"`
	Noop        int  `json:"noop"`
	Failed      int  `json:"failed"`
	P50MS       *int `json:"p50_ms,omitempty"`
	P95MS       *int `json:"p95_ms,omitempty"`
	MaxMS       *int `json:"max_ms,omitempty"`
	// Live gauges — independent of the window.
	QueueDepth int   `json:"queue_depth"` // closed change sets not yet materialized
	LagSeconds int64 `json:"lag_seconds"` // age of the oldest unprocessed closed set
}

// Summary computes the aggregates over the last windowHours (default 168 = 7d)
// and the live queue gauges.
func (s *Store) Summary(ctx context.Context, windowHours int) (Summary, error) {
	sum := Summary{WindowHours: windowHours}
	if s == nil || s.pool == nil {
		return sum, nil
	}
	if windowHours <= 0 {
		windowHours = 168
	}
	sum.WindowHours = windowHours

	// Percentiles + outcome counts over the window (materialized rows only).
	err := s.pool.QueryRow(ctx, `
		SELECT
		  count(*) FILTER (WHERE materialized_outcome IS NOT NULL),
		  count(*) FILTER (WHERE materialized_outcome = 'committed'),
		  count(*) FILTER (WHERE materialized_outcome = 'noop'),
		  count(*) FILTER (WHERE materialized_outcome = 'failed'),
		  percentile_disc(0.5)  WITHIN GROUP (ORDER BY materialized_duration_ms)
		    FILTER (WHERE materialized_duration_ms IS NOT NULL),
		  percentile_disc(0.95) WITHIN GROUP (ORDER BY materialized_duration_ms)
		    FILTER (WHERE materialized_duration_ms IS NOT NULL),
		  max(materialized_duration_ms)
		FROM weave_change_set
		WHERE started_at > now() - make_interval(hours => $1)
	`, windowHours).Scan(
		&sum.Runs, &sum.Committed, &sum.Noop, &sum.Failed,
		&sum.P50MS, &sum.P95MS, &sum.MaxMS,
	)
	if err != nil {
		return sum, err
	}

	// Live gauges: queue depth = closed but unprocessed; lag = age of the
	// oldest such set. These would surface a stuck materializer immediately.
	err = s.pool.QueryRow(ctx, `
		SELECT
		  count(*) FILTER (WHERE closed_at IS NOT NULL AND processed_at IS NULL),
		  COALESCE(
		    EXTRACT(EPOCH FROM (now() - min(closed_at)
		      FILTER (WHERE closed_at IS NOT NULL AND processed_at IS NULL)))::bigint,
		    0)
		FROM weave_change_set
	`).Scan(&sum.QueueDepth, &sum.LagSeconds)
	if err != nil {
		return sum, err
	}
	return sum, nil
}
