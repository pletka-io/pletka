package release

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) ListByProject(ctx context.Context, projectID string) ([]Release, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT project_id, version, title, description, created_at, created_by_id, archived_at, archived_message
		FROM weave_releases
		WHERE project_id = $1
		ORDER BY created_at DESC, version DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Release{}
	for rows.Next() {
		var item Release
		if err := rows.Scan(
			&item.ProjectID,
			&item.Version,
			&item.Title,
			&item.Description,
			&item.CreatedAt,
			&item.CreatedByID,
			&item.ArchivedAt,
			&item.ArchivedMessage,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (s *PostgresStore) Get(ctx context.Context, projectID, version string) (*Release, error) {
	var item Release
	err := s.pool.QueryRow(ctx, `
		SELECT project_id, version, title, description, created_at, created_by_id, archived_at, archived_message
		FROM weave_releases
		WHERE project_id = $1 AND version = $2
	`, projectID, version).Scan(
		&item.ProjectID,
		&item.Version,
		&item.Title,
		&item.Description,
		&item.CreatedAt,
		&item.CreatedByID,
		&item.ArchivedAt,
		&item.ArchivedMessage,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}
