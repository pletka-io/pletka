package weave

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/domain"
	domainweave "github.com/pletka-io/pletka/domain/weave"
)

type forkStore struct {
	pool *pgxpool.Pool
}

var _ domainweave.ForkStore = (*forkStore)(nil)

// Forks returns the store slice for local divergence provenance.
func (s *Store) Forks() domainweave.ForkStore {
	if s == nil {
		return &forkStore{}
	}
	return &forkStore{pool: s.pool}
}

func (s *forkStore) List(ctx context.Context, opts ...domain.QueryOption) ([]domainweave.EntityFork, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	cfg := domain.ApplyOptions(opts)
	query := buildProvenanceQuery(cfg, map[string]string{
		"entity_type":       "entity_type",
		"fork_entity_id":    "fork_entity_id",
		"source_project_id": "source_project_id",
		"source_entity_id":  "source_entity_id",
	})

	var rows pgx.Rows
	if cfg.Version != "" {
		rows, err = pool.Query(ctx, `
			SELECT
				project_id,
				entity_type,
				fork_entity_id,
				source_project_id,
				source_entity_id,
				COALESCE(source_version, '') AS source_version,
				forked_at,
				created_by_id,
				version_number
			FROM weave_entity_forks_archive
		`+query.where+`
			ORDER BY entity_type ASC, fork_entity_id ASC
		`, query.args...)
	} else {
		rows, err = pool.Query(ctx, `
			SELECT
				id,
				project_id,
				entity_type,
				fork_entity_id,
				source_project_id,
				source_entity_id,
				COALESCE(source_version, '') AS source_version,
				forked_at,
				created_by_id
			FROM weave_entity_forks
		`+query.where+`
			ORDER BY entity_type ASC, fork_entity_id ASC
		`, query.args...)
	}
	if err != nil {
		return nil, fmt.Errorf("list forks: %w", err)
	}
	defer rows.Close()

	forks := []domainweave.EntityFork{}
	for rows.Next() {
		fork, err := scanFork(rows, cfg.Version != "")
		if err != nil {
			return nil, err
		}
		forks = append(forks, fork)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forks: %w", err)
	}
	return forks, nil
}

func (s *forkStore) Create(ctx context.Context, fork *domainweave.EntityFork) error {
	pool, err := s.connectionPool()
	if err != nil {
		return err
	}
	if fork == nil {
		return errors.New("fork is nil")
	}
	return pool.QueryRow(ctx, `
		INSERT INTO weave_entity_forks (
			project_id,
			entity_type,
			fork_entity_id,
			source_project_id,
			source_entity_id,
			source_version,
			forked_at,
			created_by_id
		) VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, NOW()), $8)
		RETURNING id, forked_at
	`,
		fork.ProjectID,
		fork.EntityType,
		fork.ForkEntityID,
		fork.SourceProjectID,
		fork.SourceEntityID,
		strings.TrimSpace(fork.SourceVersion),
		nullableTime(fork.ForkedAt),
		fork.CreatedByID,
	).Scan(&fork.ID, &fork.ForkedAt)
}

func (s *forkStore) connectionPool() (*pgxpool.Pool, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("fork store is not initialized")
	}
	return s.pool, nil
}

func scanFork(rows pgx.Rows, archived bool) (domainweave.EntityFork, error) {
	var fork domainweave.EntityFork
	var sourceVersion string
	if archived {
		if err := rows.Scan(
			&fork.ProjectID,
			&fork.EntityType,
			&fork.ForkEntityID,
			&fork.SourceProjectID,
			&fork.SourceEntityID,
			&sourceVersion,
			&fork.ForkedAt,
			&fork.CreatedByID,
			&fork.VersionNumber,
		); err != nil {
			return fork, fmt.Errorf("scan archived fork: %w", err)
		}
	} else {
		if err := rows.Scan(
			&fork.ID,
			&fork.ProjectID,
			&fork.EntityType,
			&fork.ForkEntityID,
			&fork.SourceProjectID,
			&fork.SourceEntityID,
			&sourceVersion,
			&fork.ForkedAt,
			&fork.CreatedByID,
		); err != nil {
			return fork, fmt.Errorf("scan fork: %w", err)
		}
	}
	fork.SourceVersion = strings.TrimSpace(sourceVersion)
	return fork, nil
}
