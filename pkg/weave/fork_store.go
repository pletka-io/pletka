package weave

import (
	"context"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type forkStore struct {
	pool *pgxpool.Pool
}

var _ domain.ForkStore = (*forkStore)(nil)

func (s *forkStore) List(ctx context.Context, opts ...domain.QueryOption) ([]domain.EntityFork, error) {
	cfg := domain.ApplyOptions(opts)
	args := []any{}
	filters := []string{}
	add := func(cond string, arg any) {
		args = append(args, arg)
		filters = append(filters, fmt.Sprintf(cond, len(args)))
	}

	if cfg.ProjectID != "" {
		add("project_id = $%d", cfg.ProjectID)
	}
	if v, ok := cfg.Filters["entity_type"].(string); ok && strings.TrimSpace(v) != "" {
		add("entity_type = $%d", strings.TrimSpace(v))
	}
	if v, ok := cfg.Filters["fork_entity_id"].(string); ok && strings.TrimSpace(v) != "" {
		add("fork_entity_id = $%d", strings.TrimSpace(v))
	}
	if v, ok := cfg.Filters["source_project_id"].(string); ok && strings.TrimSpace(v) != "" {
		add("source_project_id = $%d", strings.TrimSpace(v))
	}
	if v, ok := cfg.Filters["source_entity_id"].(string); ok && strings.TrimSpace(v) != "" {
		add("source_entity_id = $%d", strings.TrimSpace(v))
	}

	where := ""
	if len(filters) > 0 {
		where = " WHERE " + strings.Join(filters, " AND ")
	}

	var (
		rows pgx.Rows
		err  error
	)
	if cfg.Version != "" {
		args = append(args, cfg.Version)
		versionPos := len(args)
		versionWhere := fmt.Sprintf("version_number = $%d", versionPos)
		if where == "" {
			where = " WHERE " + versionWhere
		} else {
			where += " AND " + versionWhere
		}
		rows, err = s.pool.Query(ctx, `
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
		`+where+`
			ORDER BY entity_type ASC, fork_entity_id ASC
		`, args...)
	} else {
		rows, err = s.pool.Query(ctx, `
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
		`+where+`
			ORDER BY entity_type ASC, fork_entity_id ASC
		`, args...)
	}
	if err != nil {
		return nil, fmt.Errorf("list forks: %w", err)
	}
	defer rows.Close()

	out := make([]domain.EntityFork, 0)
	for rows.Next() {
		var row domain.EntityFork
		var sourceVersion string
		if cfg.Version != "" {
			if err := rows.Scan(
				&row.ProjectID,
				&row.EntityType,
				&row.ForkEntityID,
				&row.SourceProjectID,
				&row.SourceEntityID,
				&sourceVersion,
				&row.ForkedAt,
				&row.CreatedByID,
				&row.VersionNumber,
			); err != nil {
				return nil, fmt.Errorf("scan archived fork: %w", err)
			}
		} else {
			if err := rows.Scan(
				&row.ID,
				&row.ProjectID,
				&row.EntityType,
				&row.ForkEntityID,
				&row.SourceProjectID,
				&row.SourceEntityID,
				&sourceVersion,
				&row.ForkedAt,
				&row.CreatedByID,
			); err != nil {
				return nil, fmt.Errorf("scan fork: %w", err)
			}
		}
		row.SourceVersion = strings.TrimSpace(sourceVersion)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forks: %w", err)
	}
	return out, nil
}

func (s *forkStore) Create(ctx context.Context, fork *domain.EntityFork) error {
	if fork == nil {
		return fmt.Errorf("fork is nil")
	}
	return s.pool.QueryRow(ctx, `
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
