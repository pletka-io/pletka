package weave

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domainweave "github.com/pletka-io/pletka/domain/weave"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

type releaseStore struct {
	pool *pgxpool.Pool
}

var _ domainweave.ReleaseStore = (*releaseStore)(nil)

// Releases returns the store slice for release metadata.
func (s *Store) Releases() domainweave.ReleaseStore {
	if s == nil {
		return &releaseStore{}
	}
	return &releaseStore{pool: s.pool}
}

func (s *releaseStore) ListByProject(ctx context.Context, projectID string) ([]domainweave.Release, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `
		SELECT project_id, version, title, description, created_at, created_by_id
		FROM weave_releases
		WHERE project_id = $1
		ORDER BY created_at DESC, version DESC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}
	defer rows.Close()

	releases := []domainweave.Release{}
	for rows.Next() {
		var release domainweave.Release
		if err := rows.Scan(
			&release.ProjectID,
			&release.Version,
			&release.Title,
			&release.Description,
			&release.CreatedAt,
			&release.CreatedByID,
		); err != nil {
			return nil, fmt.Errorf("scan release: %w", err)
		}
		releases = append(releases, release)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate releases: %w", err)
	}
	return releases, nil
}

func (s *releaseStore) Get(ctx context.Context, projectID, version string) (*domainweave.Release, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	var release domainweave.Release
	err = pool.QueryRow(ctx, `
		SELECT project_id, version, title, description, created_at, created_by_id
		FROM weave_releases
		WHERE project_id = $1 AND version = $2
	`, projectID, version).Scan(
		&release.ProjectID,
		&release.Version,
		&release.Title,
		&release.Description,
		&release.CreatedAt,
		&release.CreatedByID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domainweave.ErrReleaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get release: %w", err)
	}
	return &release, nil
}

func (s *releaseStore) connectionPool() (*pgxpool.Pool, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("release store is not initialized")
	}
	return s.pool, nil
}

func releaseFromRow(row sqlcgen.WeaveRelease) domainweave.Release {
	return domainweave.Release{
		ProjectID:   row.ProjectID,
		Version:     row.Version,
		Title:       row.Title,
		Description: row.Description,
		CreatedAt:   row.CreatedAt,
		CreatedByID: row.CreatedByID,
	}
}
