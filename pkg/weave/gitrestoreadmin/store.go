package gitrestoreadmin

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/ids"
)

type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

type Job struct {
	ID              string          `json:"id"`
	SnapshotPath    string          `json:"snapshot_path"`
	SourceProjectID string          `json:"source_project_id"`
	TargetProjectID string          `json:"target_project_id"`
	RequestedByID   string          `json:"requested_by_id"`
	Status          JobStatus       `json:"status"`
	CurrentPhase    string          `json:"current_phase"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	Preview         *PreviewResult  `json:"preview,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	FinishedAt      *time.Time      `json:"finished_at,omitempty"`
	PreviewRaw      json.RawMessage `json:"-"`
}

var ErrJobNotFound = errors.New("git restore job not found")

type Store interface {
	Create(ctx context.Context, job Job) (*Job, error)
	Get(ctx context.Context, id string) (*Job, error)
	List(ctx context.Context, limit int) ([]Job, error)
	ProjectExists(ctx context.Context, projectID string) (bool, error)
	UpdateState(ctx context.Context, id string, status JobStatus, currentPhase, errorMessage string, startedAt, finishedAt *time.Time) (*Job, error)
}

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) Create(ctx context.Context, job Job) (*Job, error) {
	if job.ID == "" {
		job.ID = ids.GenerateULID()
	}
	if job.Status == "" {
		job.Status = JobStatusPending
	}
	if len(job.PreviewRaw) == 0 && job.Preview != nil {
		raw, err := json.Marshal(job.Preview)
		if err != nil {
			return nil, err
		}
		job.PreviewRaw = raw
	}

	var out Job
	var previewRaw []byte
	err := s.pool.QueryRow(ctx, `
		INSERT INTO admin_git_restore_jobs (
			id, snapshot_path, source_project_id, target_project_id, requested_by_id,
			status, current_phase, error_message, preview_json
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, snapshot_path, source_project_id, target_project_id, requested_by_id,
		          status, current_phase, error_message, preview_json, created_at, started_at, finished_at
	`,
		job.ID,
		job.SnapshotPath,
		job.SourceProjectID,
		job.TargetProjectID,
		job.RequestedByID,
		string(job.Status),
		job.CurrentPhase,
		job.ErrorMessage,
		job.PreviewRaw,
	).Scan(
		&out.ID,
		&out.SnapshotPath,
		&out.SourceProjectID,
		&out.TargetProjectID,
		&out.RequestedByID,
		&out.Status,
		&out.CurrentPhase,
		&out.ErrorMessage,
		&previewRaw,
		&out.CreatedAt,
		&out.StartedAt,
		&out.FinishedAt,
	)
	if err != nil {
		return nil, err
	}
	out.PreviewRaw = previewRaw
	decodePreview(&out)
	return &out, nil
}

func (s *PostgresStore) Get(ctx context.Context, id string) (*Job, error) {
	var out Job
	var previewRaw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, snapshot_path, source_project_id, target_project_id, requested_by_id,
		       status, current_phase, error_message, preview_json, created_at, started_at, finished_at
		FROM admin_git_restore_jobs
		WHERE id = $1
	`, id).Scan(
		&out.ID,
		&out.SnapshotPath,
		&out.SourceProjectID,
		&out.TargetProjectID,
		&out.RequestedByID,
		&out.Status,
		&out.CurrentPhase,
		&out.ErrorMessage,
		&previewRaw,
		&out.CreatedAt,
		&out.StartedAt,
		&out.FinishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, err
	}
	out.PreviewRaw = previewRaw
	decodePreview(&out)
	return &out, nil
}

func (s *PostgresStore) List(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, snapshot_path, source_project_id, target_project_id, requested_by_id,
		       status, current_phase, error_message, preview_json, created_at, started_at, finished_at
		FROM admin_git_restore_jobs
		ORDER BY created_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Job{}
	for rows.Next() {
		var item Job
		var previewRaw []byte
		if err := rows.Scan(
			&item.ID,
			&item.SnapshotPath,
			&item.SourceProjectID,
			&item.TargetProjectID,
			&item.RequestedByID,
			&item.Status,
			&item.CurrentPhase,
			&item.ErrorMessage,
			&previewRaw,
			&item.CreatedAt,
			&item.StartedAt,
			&item.FinishedAt,
		); err != nil {
			return nil, err
		}
		item.PreviewRaw = previewRaw
		decodePreview(&item)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *PostgresStore) ProjectExists(ctx context.Context, projectID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM weave_projects WHERE id = $1)`, projectID).Scan(&exists)
	return exists, err
}

func (s *PostgresStore) UpdateState(ctx context.Context, id string, status JobStatus, currentPhase, errorMessage string, startedAt, finishedAt *time.Time) (*Job, error) {
	var out Job
	var previewRaw []byte
	err := s.pool.QueryRow(ctx, `
		UPDATE admin_git_restore_jobs
		SET status = $2,
		    current_phase = $3,
		    error_message = $4,
		    started_at = COALESCE($5, started_at),
		    finished_at = $6
		WHERE id = $1
		RETURNING id, snapshot_path, source_project_id, target_project_id, requested_by_id,
		          status, current_phase, error_message, preview_json, created_at, started_at, finished_at
	`, id, string(status), currentPhase, errorMessage, startedAt, finishedAt).Scan(
		&out.ID,
		&out.SnapshotPath,
		&out.SourceProjectID,
		&out.TargetProjectID,
		&out.RequestedByID,
		&out.Status,
		&out.CurrentPhase,
		&out.ErrorMessage,
		&previewRaw,
		&out.CreatedAt,
		&out.StartedAt,
		&out.FinishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, err
	}
	out.PreviewRaw = previewRaw
	decodePreview(&out)
	return &out, nil
}

func decodePreview(job *Job) {
	if job == nil || len(job.PreviewRaw) == 0 {
		return
	}
	var preview PreviewResult
	if err := json.Unmarshal(job.PreviewRaw, &preview); err == nil {
		job.Preview = &preview
	}
}
