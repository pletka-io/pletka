package weave

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// weaveCategoryStore implements domain.WeaveCategoryStore backed by
// the weave_categories table via sqlc queries.
type weaveCategoryStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

// Compile-time interface check.
var _ domain.WeaveCategoryStore = (*weaveCategoryStore)(nil)

// ---------------------------------------------------------------------------
// Row converters
// ---------------------------------------------------------------------------

// weaveRowToCategory converts a sqlcgen.WeaveCategory row to a *domain.Category.
func weaveRowToCategory(row sqlcgen.WeaveCategory) *domain.Category {
	return &domain.Category{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  dbutil.NilToEmpty(row.SemanticID),
			SystemName:  dbutil.NilToEmpty(row.SystemName),
			UIName:      unmarshalDomainTranslations(row.UiName),
			Description: unmarshalDomainTranslations(row.Description),
			Status:      domain.Status(row.Status),
			ProjectID:   row.ProjectID,
			Deprecated:  row.Deprecated,
		},
		CanonicalOrder: int(row.CanonicalOrder),
	}
}

// weaveRowFromCountsToCategory converts a WeaveListCategoriesWithCountsRow
// into a WeaveCategoryWithCounts using domain.Category.
func weaveRowFromCountsToCategory(row sqlcgen.WeaveListCategoriesWithCountsRow) domain.WeaveCategoryWithCounts {
	cat := domain.Category{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  dbutil.NilToEmpty(row.SemanticID),
			SystemName:  dbutil.NilToEmpty(row.SystemName),
			UIName:      unmarshalDomainTranslations(row.UiName),
			Description: unmarshalDomainTranslations(row.Description),
			Status:      domain.Status(row.Status),
			ProjectID:   row.ProjectID,
			Deprecated:  row.Deprecated,
		},
		CanonicalOrder: int(row.CanonicalOrder),
	}

	return domain.WeaveCategoryWithCounts{
		Category:             cat,
		FieldCount:           row.FieldCount,
		ModelFieldCount:      row.ModelFieldCount,
		CollectionFieldCount: row.CollectionFieldCount,
	}
}

// ---------------------------------------------------------------------------
// WeaveCategoryStore implementation
// ---------------------------------------------------------------------------

// Create inserts a new category into the weave_categories table.
func (s *weaveCategoryStore) Create(ctx context.Context, category *domain.Category) error {
	if category.ID == "" {
		category.ID = ids.GenerateULID()
	}

	if category.Status == "" {
		category.Status = "draft"
	}

	row, err := s.queries.WeaveCreateCategory(ctx, sqlcgen.WeaveCreateCategoryParams{
		ID:             category.ID,
		SemanticID:     dbutil.EmptyToNil(category.SemanticID),
		SystemName:     dbutil.EmptyToNil(category.SystemName),
		UiName:         marshalDomainTranslations(category.UIName),
		Description:    marshalDomainTranslations(category.Description),
		Status:         string(category.Status),
		ProjectID:      category.ProjectID,
		CanonicalOrder: int32(category.CanonicalOrder),
	})
	if err != nil {
		return fmt.Errorf("create weave category: %w", err)
	}

	category.CreatedAt = row.CreatedAt
	category.UpdatedAt = row.UpdatedAt

	return nil
}

// GetByID retrieves a category by its ULID. Returns nil, nil if not found.
func (s *weaveCategoryStore) GetByID(ctx context.Context, id string) (*domain.Category, error) {
	row, err := s.queries.WeaveGetCategoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get weave category by id: %w", err)
	}

	return weaveRowToCategory(row), nil
}

// Update modifies an existing category in the weave_categories table.
func (s *weaveCategoryStore) Update(ctx context.Context, category *domain.Category) error {
	if category.Status == "" {
		category.Status = "draft"
	}

	row, err := s.queries.WeaveUpdateCategory(ctx, sqlcgen.WeaveUpdateCategoryParams{
		ID:             category.ID,
		UiName:         marshalDomainTranslations(category.UIName),
		Description:    marshalDomainTranslations(category.Description),
		SystemName:     dbutil.EmptyToNil(category.SystemName),
		Status:         string(category.Status),
		CanonicalOrder: int32(category.CanonicalOrder),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("update weave category %s: not found", category.ID)
		}
		return fmt.Errorf("update weave category: %w", err)
	}

	category.UpdatedAt = row.UpdatedAt

	return nil
}

// Delete removes a category from the weave_categories table.
func (s *weaveCategoryStore) Delete(ctx context.Context, id string) error {
	if err := s.queries.WeaveDeleteCategory(ctx, id); err != nil {
		return fmt.Errorf("delete weave category: %w", err)
	}
	return nil
}

// List returns all categories for a project, ordered by canonical_order.
func (s *weaveCategoryStore) List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Category, error) {
	cfg := domain.ApplyOptions(opts)

	rows, err := s.queries.WeaveListCategories(ctx, cfg.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("list weave categories: %w", err)
	}

	categories := make([]*domain.Category, 0, len(rows))
	for _, row := range rows {
		categories = append(categories, weaveRowToCategory(row))
	}

	return categories, nil
}

// Reorder updates canonical_order for a list of category IDs (1-indexed).
func (s *weaveCategoryStore) Reorder(ctx context.Context, projectID string, categoryIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	// Fetch all categories for this project, then validate in-memory.
	rows, err := qtx.WeaveListCategories(ctx, projectID)
	if err != nil {
		return fmt.Errorf("reorder: list weave categories: %w", err)
	}

	owned := make(map[string]bool, len(rows))
	for _, row := range rows {
		owned[row.ID] = true
	}

	for i, catID := range categoryIDs {
		if !owned[catID] {
			return fmt.Errorf("reorder: category %s not found or does not belong to project %s", catID, projectID)
		}

		if err := qtx.WeaveUpdateCategoryOrder(ctx, sqlcgen.WeaveUpdateCategoryOrderParams{
			ID:             catID,
			CanonicalOrder: int32(i + 1),
		}); err != nil {
			return fmt.Errorf("reorder: update category %s: %w", catID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// UpdateFields performs a partial update, applying only the keys present in fields.
// Supported keys: "ui_name", "description", "system_name", "canonical_order".
// Unknown keys are silently ignored.
func (s *weaveCategoryStore) UpdateFields(ctx context.Context, id string, fields map[string]any) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	row, err := qtx.WeaveGetCategoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("update weave category fields %s: not found", id)
		}
		return fmt.Errorf("update weave category fields: %w", err)
	}

	cat := weaveRowToCategory(row)

	for key, val := range fields {
		switch key {
		case "ui_name":
			if v, ok := val.(domain.Translations); ok {
				cat.UIName = v
			}
		case "description":
			if v, ok := val.(domain.Translations); ok {
				cat.Description = v
			}
		case "system_name":
			if v, ok := val.(string); ok {
				cat.SystemName = v
			}
		case "canonical_order":
			switch v := val.(type) {
			case int:
				cat.CanonicalOrder = v
			case int32:
				cat.CanonicalOrder = int(v)
			case float64:
				cat.CanonicalOrder = int(v)
			}
		}
	}

	if _, err := qtx.WeaveUpdateCategory(ctx, sqlcgen.WeaveUpdateCategoryParams{
		ID:             cat.ID,
		UiName:         marshalDomainTranslations(cat.UIName),
		Description:    marshalDomainTranslations(cat.Description),
		SystemName:     dbutil.EmptyToNil(cat.SystemName),
		Status:         row.Status,
		CanonicalOrder: int32(cat.CanonicalOrder),
	}); err != nil {
		return fmt.Errorf("update weave category fields: %w", err)
	}

	return tx.Commit(ctx)
}

// Count returns the number of categories matching the filter.
func (s *weaveCategoryStore) Count(ctx context.Context, opts ...domain.QueryOption) (int64, error) {
	cfg := domain.ApplyOptions(opts)

	count, err := s.queries.WeaveCountCategories(ctx, cfg.ProjectID)
	if err != nil {
		return 0, fmt.Errorf("count weave categories: %w", err)
	}

	return count, nil
}

// GetByIdentifier finds a category by semantic_id within a project.
// Returns nil, nil if not found.
func (s *weaveCategoryStore) GetByIdentifier(ctx context.Context, identifier string, projectID string) (*domain.Category, error) {
	row, err := s.queries.WeaveGetCategoryByIdentifier(ctx, sqlcgen.WeaveGetCategoryByIdentifierParams{
		SemanticID: dbutil.EmptyToNil(identifier),
		ProjectID:  projectID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get weave category by identifier: %w", err)
	}

	return weaveRowToCategory(row), nil
}

// ListWithCounts returns all categories for a project along with their field/override usage counts.
func (s *weaveCategoryStore) ListWithCounts(ctx context.Context, projectID string) ([]domain.WeaveCategoryWithCounts, error) {
	rows, err := s.queries.WeaveListCategoriesWithCounts(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list weave categories with counts: %w", err)
	}

	out := make([]domain.WeaveCategoryWithCounts, 0, len(rows))
	for _, row := range rows {
		out = append(out, weaveRowFromCountsToCategory(row))
	}

	return out, nil
}

// DeleteWithReassignment reassigns fields from the target category to a replacement,
// clears field overrides referencing it, then deletes the category itself — all in
// one transaction.
func (s *weaveCategoryStore) DeleteWithReassignment(ctx context.Context, id string, reassignTo string, semanticID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	cat, catErr := qtx.WeaveGetCategoryByID(ctx, id)
	if catErr != nil {
		if errors.Is(catErr, pgx.ErrNoRows) {
			return fmt.Errorf("delete with reassignment: category not found")
		}
		return fmt.Errorf("delete with reassignment: %w", catErr)
	}

	// 1. Reassign fields from the deleted category to the target category.
	//    Only run when a target is provided — avoids setting category_id to NULL.
	if reassignTo != "" {
		if err := qtx.WeaveReassignFieldsToCategory(ctx, sqlcgen.WeaveReassignFieldsToCategoryParams{
			CategoryID:   &id,
			CategoryID_2: dbutil.EmptyToNil(reassignTo),
			ProjectID:    cat.ProjectID,
		}); err != nil {
			return fmt.Errorf("reassign fields: %w", err)
		}
	}

	// 2. Clear field overrides (model + collection) that reference the deleted category.
	if err := qtx.WeaveClearFieldOverrideCategory(ctx, &id); err != nil {
		return fmt.Errorf("clear field overrides: %w", err)
	}

	// 3. Delete the category itself.
	if err := qtx.WeaveDeleteCategory(ctx, id); err != nil {
		return fmt.Errorf("delete category: %w", err)
	}

	_ = semanticID // unused — weave unifies override cleanup via category_id above.

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// ModelFieldOverrides returns model_field rows whose category_id matches, for the usage modal.
func (s *weaveCategoryStore) ModelFieldOverrides(ctx context.Context, categoryID string, projectID string) ([]domain.OverrideEntry, error) {
	rows, err := s.queries.WeaveCategoryModelFieldOverrides(ctx, sqlcgen.WeaveCategoryModelFieldOverridesParams{
		CategoryID: dbutil.EmptyToNil(categoryID),
		ProjectID:  projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("model field overrides: %w", err)
	}

	entries := make([]domain.OverrideEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, domain.OverrideEntry{
			ID:         strconv.FormatInt(r.OverrideID, 10),
			ParentName: r.ModelName,
			FieldName:  r.FieldName,
		})
	}

	return entries, nil
}

// CollectionFieldOverrides returns collection_field rows whose category_id matches, for the usage modal.
func (s *weaveCategoryStore) CollectionFieldOverrides(ctx context.Context, semanticID string, projectID string) ([]domain.OverrideEntry, error) {
	rows, err := s.queries.WeaveCategoryCollectionFieldOverrides(ctx, sqlcgen.WeaveCategoryCollectionFieldOverridesParams{
		CategoryID: dbutil.EmptyToNil(semanticID),
		ProjectID:  projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("collection field overrides: %w", err)
	}

	entries := make([]domain.OverrideEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, domain.OverrideEntry{
			ID:         strconv.FormatInt(r.OverrideID, 10),
			ParentName: r.CollectionName,
			FieldName:  r.FieldName,
		})
	}

	return entries, nil
}
