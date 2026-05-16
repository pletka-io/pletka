package weave

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/domain"
	domainweave "github.com/pletka-io/pletka/domain/weave"
	"github.com/pletka-io/pletka/store/dbutil"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

type categoryStore struct {
	store *Store
}

var _ domainweave.CategoryStore = (*categoryStore)(nil)

// Categories returns the store slice for project-scoped category metadata.
func (s *Store) Categories() domainweave.CategoryStore {
	return &categoryStore{store: s}
}

func (s *categoryStore) GetByID(ctx context.Context, projectID, id string) (*domainweave.Category, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	row, err := queries.WeaveGetCategoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get category by id: %w", err)
	}
	if row.ProjectID != projectID {
		return nil, nil
	}
	return categoryFromRow(row), nil
}

func (s *categoryStore) GetByIDVersion(ctx context.Context, projectID, id, version string) (*domainweave.Category, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	row := pool.QueryRow(ctx, `
		SELECT
			id, created_at, updated_at, semantic_id, system_name, ui_name, description,
			status, project_id, canonical_order, deprecated, version_number
		FROM weave_categories_archive
		WHERE id = $1 AND project_id = $2 AND version_number = $3
	`, id, projectID, version)
	category, err := scanArchivedCategory(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archived category by id: %w", err)
	}
	return category, nil
}

func (s *categoryStore) List(ctx context.Context, projectID string) ([]*domainweave.Category, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	rows, err := queries.WeaveListCategories(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	categories := make([]*domainweave.Category, 0, len(rows))
	for _, row := range rows {
		categories = append(categories, categoryFromRow(row))
	}
	return categories, nil
}

func (s *categoryStore) ListVersion(ctx context.Context, projectID, version string) ([]*domainweave.Category, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `
		SELECT
			id, created_at, updated_at, semantic_id, system_name, ui_name, description,
			status, project_id, canonical_order, deprecated, version_number
		FROM weave_categories_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY canonical_order ASC, system_name ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived categories: %w", err)
	}
	defer rows.Close()

	categories := []*domainweave.Category{}
	for rows.Next() {
		category, err := scanArchivedCategory(rows)
		if err != nil {
			return nil, fmt.Errorf("scan archived category: %w", err)
		}
		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived categories: %w", err)
	}
	return categories, nil
}

func (s *categoryStore) queryFacade() (*sqlcgen.Queries, error) {
	if s == nil || s.store == nil || s.store.queries == nil {
		return nil, errors.New("category store is not initialized")
	}
	return s.store.queries, nil
}

func (s *categoryStore) connectionPool() (pgxPool, error) {
	if s == nil || s.store == nil || s.store.pool == nil {
		return nil, errors.New("category store is not initialized")
	}
	return s.store.pool, nil
}

func categoryFromRow(row sqlcgen.WeaveCategory) *domainweave.Category {
	return &domainweave.Category{
		Entity: domain.Entity{
			ID:            row.ID,
			CreatedAt:     row.CreatedAt,
			UpdatedAt:     row.UpdatedAt,
			SemanticID:    dbutil.NilToEmpty(row.SemanticID),
			SystemName:    dbutil.NilToEmpty(row.SystemName),
			UIName:        translationsFromJSON(row.UiName),
			Description:   translationsFromJSON(row.Description),
			Status:        domain.Status(row.Status),
			VersionNumber: row.VersionNumber,
			ProjectID:     row.ProjectID,
			Deprecated:    row.Deprecated,
		},
		Origin:         domain.OriginFromProject(row.ProjectID, semanticProjectID(row.SemanticID)),
		CanonicalOrder: int(row.CanonicalOrder),
	}
}

func scanArchivedCategory(row interface{ Scan(...any) error }) (*domainweave.Category, error) {
	var category sqlcgen.WeaveCategory
	if err := row.Scan(
		&category.ID,
		&category.CreatedAt,
		&category.UpdatedAt,
		&category.SemanticID,
		&category.SystemName,
		&category.UiName,
		&category.Description,
		&category.Status,
		&category.ProjectID,
		&category.CanonicalOrder,
		&category.Deprecated,
		&category.VersionNumber,
	); err != nil {
		return nil, err
	}
	return categoryFromRow(category), nil
}

func semanticProjectID(semanticID *string) string {
	if semanticID == nil {
		return ""
	}
	parts := strings.SplitN(*semanticID, ".", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}
