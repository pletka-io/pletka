package weave

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

type adoptionStore struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

var _ domain.AdoptionStore = (*adoptionStore)(nil)

func (s *adoptionStore) List(ctx context.Context, opts ...domain.QueryOption) ([]domain.Adoption, error) {
	cfg := domain.ApplyOptions(opts)
	var (
		rows pgx.Rows
		err  error
	)

	args := []any{}
	filters := []string{}
	add := func(cond string, arg any) {
		args = append(args, arg)
		filters = append(filters, fmt.Sprintf(cond, len(args)))
	}

	if cfg.ProjectID != "" {
		add("project_id = $%d", cfg.ProjectID)
	}
	if v, ok := cfg.Filters["context_entity_type"].(string); ok && strings.TrimSpace(v) != "" {
		add("context_entity_type = $%d", strings.TrimSpace(v))
	}
	if v, ok := cfg.Filters["context_entity_id"].(string); ok && strings.TrimSpace(v) != "" {
		add("context_entity_id = $%d", strings.TrimSpace(v))
	}
	if v, ok := cfg.Filters["entity_type"].(string); ok && strings.TrimSpace(v) != "" {
		add("entity_type = $%d", strings.TrimSpace(v))
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
		`+where+`
			ORDER BY context_entity_type ASC, context_entity_id ASC, entity_type ASC, source_entity_id ASC
		`, args...)
	} else {
		rows, err = s.pool.Query(ctx, `
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
		`+where+`
			ORDER BY context_entity_type ASC, context_entity_id ASC, entity_type ASC, source_entity_id ASC
		`, args...)
	}
	if err != nil {
		return nil, fmt.Errorf("list adoptions: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Adoption, 0)
	for rows.Next() {
		var (
			row       domain.Adoption
			sourceVer string
			createdBy *string
		)
		if cfg.Version != "" {
			if err := rows.Scan(
				&row.ProjectID,
				&row.ContextEntityType,
				&row.ContextEntityID,
				&row.EntityType,
				&row.SourceProjectID,
				&row.SourceEntityID,
				&sourceVer,
				&row.AdoptedAt,
				&createdBy,
				&row.VersionNumber,
			); err != nil {
				return nil, fmt.Errorf("scan archived adoption: %w", err)
			}
		} else {
			if err := rows.Scan(
				&row.ID,
				&row.ProjectID,
				&row.ContextEntityType,
				&row.ContextEntityID,
				&row.EntityType,
				&row.SourceProjectID,
				&row.SourceEntityID,
				&sourceVer,
				&row.AdoptedAt,
				&createdBy,
			); err != nil {
				return nil, fmt.Errorf("scan adoption: %w", err)
			}
		}
		row.SourceVersion = strings.TrimSpace(sourceVer)
		row.CreatedByID = createdBy
		row.Origin = domain.Origin{
			Kind:            domain.OriginAdopted,
			SourceProjectID: row.SourceProjectID,
			SourceEntityID:  row.SourceEntityID,
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate adoptions: %w", err)
	}
	return out, nil
}

func (s *adoptionStore) ReplaceForContext(ctx context.Context, projectID, contextEntityType, contextEntityID string, adoptions []domain.Adoption) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin adoption replace tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		DELETE FROM weave_adoptions
		WHERE project_id = $1 AND context_entity_type = $2 AND context_entity_id = $3
	`, projectID, contextEntityType, contextEntityID); err != nil {
		return fmt.Errorf("delete adoptions for context: %w", err)
	}

	dedup := make(map[string]domain.Adoption, len(adoptions))
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

	q := s.queries.WithTx(tx)
	for key, adoption := range dedup {
		if adoption.SourceVersion != "" {
			continue
		}
		version, err := sourceVersionFor(ctx, q, projectID, adoption)
		if err != nil {
			return err
		}
		adoption.SourceVersion = version
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
		return fmt.Errorf("commit adoption replace tx: %w", err)
	}
	return nil
}

func adoptionKey(a domain.Adoption) string {
	return strings.Join([]string{
		a.ProjectID,
		a.ContextEntityType,
		a.ContextEntityID,
		a.EntityType,
		a.SourceProjectID,
		a.SourceEntityID,
		a.SourceVersion,
	}, "|")
}

// sourceVersionFor names the release a receipt was taken from when the writer
// gave none: the consumer's pinned release of that source when it holds the
// entity, else the newest live release that does. A consumer that follows the
// source's draft on purpose, or an entity no release holds, keeps an empty
// version; a release cut refuses while one remains.
func sourceVersionFor(ctx context.Context, q *sqlcgen.Queries, projectID string, a domain.Adoption) (string, error) {
	pin, err := q.WeaveAdoptionSourcePin(ctx, sqlcgen.WeaveAdoptionSourcePinParams{ProjectID: projectID, ParentProjectID: a.SourceProjectID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return "", fmt.Errorf("read parent link for adoption: %w", err)
	case pin.SourceMode == string(domain.DependencySourceDraft):
		return "", nil
	case pin.SourceVersion != "":
		ok, err := q.WeaveReleaseContainsEntity(ctx, sqlcgen.WeaveReleaseContainsEntityParams{SourceProjectID: a.SourceProjectID, Version: pin.SourceVersion, EntityType: a.EntityType, EntityID: a.SourceEntityID})
		if err != nil {
			return "", fmt.Errorf("check pinned release for adoption: %w", err)
		}
		if ok {
			return pin.SourceVersion, nil
		}
	}
	version, err := q.WeaveLatestReleaseContaining(ctx, sqlcgen.WeaveLatestReleaseContainingParams{SourceProjectID: a.SourceProjectID, EntityType: a.EntityType, EntityID: a.SourceEntityID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find release holding adopted entity: %w", err)
	}
	return version, nil
}
