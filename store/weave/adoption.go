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

type adoptionStore struct {
	pool *pgxpool.Pool
}

var _ domainweave.AdoptionStore = (*adoptionStore)(nil)

// Adoptions returns the store slice for project-local reuse receipts.
func (s *Store) Adoptions() domainweave.AdoptionStore {
	if s == nil {
		return &adoptionStore{}
	}
	return &adoptionStore{pool: s.pool}
}

func (s *adoptionStore) List(ctx context.Context, opts ...domain.QueryOption) ([]domainweave.Adoption, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	cfg := domain.ApplyOptions(opts)
	query := buildProvenanceQuery(cfg, map[string]string{
		"context_entity_type": "context_entity_type",
		"context_entity_id":   "context_entity_id",
		"entity_type":         "entity_type",
		"source_project_id":   "source_project_id",
		"source_entity_id":    "source_entity_id",
	})

	var rows pgx.Rows
	if cfg.Version != "" {
		rows, err = pool.Query(ctx, `
			SELECT
				project_id,
				context_entity_type,
				context_entity_id,
				entity_type,
				source_project_id,
				source_entity_id,
				COALESCE(source_version, '') AS source_version,
				adopted_at,
				created_by_id,
				version_number
			FROM weave_adoptions_archive
		`+query.where+`
			ORDER BY context_entity_type ASC, context_entity_id ASC, entity_type ASC, source_entity_id ASC
		`, query.args...)
	} else {
		rows, err = pool.Query(ctx, `
			SELECT
				id,
				project_id,
				context_entity_type,
				context_entity_id,
				entity_type,
				source_project_id,
				source_entity_id,
				COALESCE(source_version, '') AS source_version,
				adopted_at,
				created_by_id
			FROM weave_adoptions
		`+query.where+`
			ORDER BY context_entity_type ASC, context_entity_id ASC, entity_type ASC, source_entity_id ASC
		`, query.args...)
	}
	if err != nil {
		return nil, fmt.Errorf("list adoptions: %w", err)
	}
	defer rows.Close()

	adoptions := []domainweave.Adoption{}
	for rows.Next() {
		adoption, err := scanAdoption(rows, cfg.Version != "")
		if err != nil {
			return nil, err
		}
		adoptions = append(adoptions, adoption)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate adoptions: %w", err)
	}
	return adoptions, nil
}

func (s *adoptionStore) ReplaceForContext(ctx context.Context, projectID, contextEntityType, contextEntityID string, adoptions []domainweave.Adoption) error {
	pool, err := s.connectionPool()
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin adoption replace transaction: %w", err)
	}
	defer rollback(ctx, tx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM weave_adoptions
		WHERE project_id = $1 AND context_entity_type = $2 AND context_entity_id = $3
	`, projectID, contextEntityType, contextEntityID); err != nil {
		return fmt.Errorf("delete adoptions for context: %w", err)
	}

	dedup := make(map[string]domainweave.Adoption, len(adoptions))
	for _, adoption := range adoptions {
		adoption.ProjectID = projectID
		adoption.ContextEntityType = contextEntityType
		adoption.ContextEntityID = contextEntityID
		adoption.SourceVersion = strings.TrimSpace(adoption.SourceVersion)
		key := adoptionKey(adoption)
		if _, ok := dedup[key]; ok {
			continue
		}
		dedup[key] = adoption
	}

	for _, adoption := range dedup {
		if _, err := tx.Exec(ctx, `
			INSERT INTO weave_adoptions (
				project_id,
				context_entity_type,
				context_entity_id,
				entity_type,
				source_project_id,
				source_entity_id,
				source_version,
				adopted_at,
				created_by_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE($8, NOW()), $9)
		`,
			adoption.ProjectID,
			adoption.ContextEntityType,
			adoption.ContextEntityID,
			adoption.EntityType,
			adoption.SourceProjectID,
			adoption.SourceEntityID,
			adoption.SourceVersion,
			nullableTime(adoption.AdoptedAt),
			adoption.CreatedByID,
		); err != nil {
			return fmt.Errorf("insert adoption: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit adoption replace transaction: %w", err)
	}
	return nil
}

func (s *adoptionStore) connectionPool() (*pgxpool.Pool, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("adoption store is not initialized")
	}
	return s.pool, nil
}

func scanAdoption(rows pgx.Rows, archived bool) (domainweave.Adoption, error) {
	var adoption domainweave.Adoption
	var sourceVersion string
	if archived {
		if err := rows.Scan(
			&adoption.ProjectID,
			&adoption.ContextEntityType,
			&adoption.ContextEntityID,
			&adoption.EntityType,
			&adoption.SourceProjectID,
			&adoption.SourceEntityID,
			&sourceVersion,
			&adoption.AdoptedAt,
			&adoption.CreatedByID,
			&adoption.VersionNumber,
		); err != nil {
			return adoption, fmt.Errorf("scan archived adoption: %w", err)
		}
	} else {
		if err := rows.Scan(
			&adoption.ID,
			&adoption.ProjectID,
			&adoption.ContextEntityType,
			&adoption.ContextEntityID,
			&adoption.EntityType,
			&adoption.SourceProjectID,
			&adoption.SourceEntityID,
			&sourceVersion,
			&adoption.AdoptedAt,
			&adoption.CreatedByID,
		); err != nil {
			return adoption, fmt.Errorf("scan adoption: %w", err)
		}
	}
	adoption.SourceVersion = strings.TrimSpace(sourceVersion)
	adoption.Origin = domain.AdoptedOrigin(adoption.SourceProjectID, adoption.SourceEntityID)
	return adoption, nil
}

func adoptionKey(adoption domainweave.Adoption) string {
	return strings.Join([]string{
		adoption.ProjectID,
		adoption.ContextEntityType,
		adoption.ContextEntityID,
		adoption.EntityType,
		adoption.SourceProjectID,
		adoption.SourceEntityID,
		adoption.SourceVersion,
	}, "|")
}
