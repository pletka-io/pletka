package category

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// postgresStore is the pgx + sqlc implementation of Store, backed by the
// weave_categories table. Constructed via NewPostgresStore.
type postgresStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

// Compile-time interface check.
var _ Store = (*postgresStore)(nil)

// NewPostgresStore returns a Store backed by the supplied pgx pool.
func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{
		queries: sqlcgen.New(pool),
		pool:    pool,
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

// Create inserts a new category. Generates a ULID if c.ID is empty and
// defaults Status to "draft".
func (s *postgresStore) Create(ctx context.Context, c *domain.Category) error {
	if c.ID == "" {
		c.ID = generateULID()
	}
	if c.Status == "" {
		c.Status = "draft"
	}

	row, err := s.queries.WeaveCreateCategory(ctx, sqlcgen.WeaveCreateCategoryParams{
		ID:             c.ID,
		SemanticID:     dbutil.EmptyToNil(c.SemanticID),
		SystemName:     dbutil.EmptyToNil(c.SystemName),
		UiName:         marshalTranslations(c.UIName),
		Description:    marshalTranslations(c.Description),
		Status:         string(c.Status),
		ProjectID:      c.ProjectID,
		CanonicalOrder: int32(c.CanonicalOrder),
	})
	if err != nil {
		return fmt.Errorf("create category: %w", err)
	}

	c.CreatedAt = row.CreatedAt
	c.UpdatedAt = row.UpdatedAt
	return nil
}

// GetByID retrieves a category by ULID, scoped to projectID. Returns
// (nil, nil) if no row matches OR if the row's project does not match.
func (s *postgresStore) GetByID(ctx context.Context, projectID, id string) (*domain.Category, error) {
	row, err := s.queries.WeaveGetCategoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get category by id: %w", err)
	}

	// Enforce project scope. A category whose project differs is treated as
	// not found rather than leaking existence.
	if row.ProjectID != projectID {
		return nil, nil
	}

	return rowToCategory(row), nil
}

func (s *postgresStore) GetByIDVersion(ctx context.Context, projectID, id, version string) (*domain.Category, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT
			id, created_at, updated_at, semantic_id, system_name, ui_name, description,
			status, project_id, canonical_order, deprecated, version_number
		FROM weave_categories_archive
		WHERE id = $1 AND project_id = $2 AND version_number = $3
	`, id, projectID, version)
	cat, err := scanArchivedCategory(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archived category by id: %w", err)
	}
	return cat, nil
}

// GetByIdentifier finds a category by semantic_id (or system_name fallback)
// within the given project. Returns (nil, nil) if not found.
func (s *postgresStore) GetByIdentifier(ctx context.Context, projectID, identifier string) (*domain.Category, error) {
	row, err := s.queries.WeaveGetCategoryByIdentifier(ctx, sqlcgen.WeaveGetCategoryByIdentifierParams{
		SemanticID: dbutil.EmptyToNil(identifier),
		ProjectID:  projectID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get category by identifier: %w", err)
	}

	return rowToCategory(row), nil
}

// Update writes the full row. Status defaults to "draft" if empty.
func (s *postgresStore) Update(ctx context.Context, c *domain.Category) error {
	if c.Status == "" {
		c.Status = "draft"
	}

	row, err := s.queries.WeaveUpdateCategory(ctx, sqlcgen.WeaveUpdateCategoryParams{
		ID:             c.ID,
		UiName:         marshalTranslations(c.UIName),
		Description:    marshalTranslations(c.Description),
		SystemName:     dbutil.EmptyToNil(c.SystemName),
		Status:         string(c.Status),
		CanonicalOrder: int32(c.CanonicalOrder),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("update category %s: not found", c.ID)
		}
		return fmt.Errorf("update category: %w", err)
	}

	c.UpdatedAt = row.UpdatedAt
	return nil
}

// UpdateFields applies a partial update inside a transaction. Supported keys:
//   - "ui_name"          domain.Translations
//   - "description"      domain.Translations
//   - "system_name"      string
//   - "canonical_order"  int / int32 / float64
//
// Unknown keys are silently ignored. Returns "not found" if the row is gone.
func (s *postgresStore) UpdateFields(ctx context.Context, id string, fields map[string]any) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := s.queries.WithTx(tx)

	row, err := q.WeaveGetCategoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("update category fields %s: not found", id)
		}
		return fmt.Errorf("update category fields: %w", err)
	}

	cat := rowToCategory(row)

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

	if _, err := q.WeaveUpdateCategory(ctx, sqlcgen.WeaveUpdateCategoryParams{
		ID:             cat.ID,
		UiName:         marshalTranslations(cat.UIName),
		Description:    marshalTranslations(cat.Description),
		SystemName:     dbutil.EmptyToNil(cat.SystemName),
		Status:         row.Status,
		CanonicalOrder: int32(cat.CanonicalOrder),
	}); err != nil {
		return fmt.Errorf("update category fields: %w", err)
	}

	return tx.Commit(ctx)
}

// Delete removes a category. Caller is responsible for any cascade —
// see DeleteWithReassignment for fields/overrides cleanup.
func (s *postgresStore) Delete(ctx context.Context, projectID, id string) error {
	if err := s.queries.WeaveDeleteCategory(ctx, id); err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	return nil
}

// DeleteWithReassignment performs a transactional cascade:
//  1. Reassign fields whose category_id == id to reassignTo (skip if "")
//  2. Clear field overrides referencing the deleted category
//  3. Delete the category row
//
// semanticID is currently unused — kept on the interface for future symmetric
// override cleanup. Override rows are matched via category_id alone.
func (s *postgresStore) DeleteWithReassignment(ctx context.Context, projectID, id, reassignTo, semanticID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := s.queries.WithTx(tx)

	cat, catErr := q.WeaveGetCategoryByID(ctx, id)
	if catErr != nil {
		if errors.Is(catErr, pgx.ErrNoRows) {
			return fmt.Errorf("delete with reassignment: category not found")
		}
		return fmt.Errorf("delete with reassignment: %w", catErr)
	}
	if cat.ProjectID != projectID {
		return fmt.Errorf("delete with reassignment: category %s does not belong to project %s", id, projectID)
	}

	if reassignTo != "" {
		if err := q.WeaveReassignFieldsToCategory(ctx, sqlcgen.WeaveReassignFieldsToCategoryParams{
			CategoryID:   &id,
			CategoryID_2: dbutil.EmptyToNil(reassignTo),
			ProjectID:    cat.ProjectID,
		}); err != nil {
			return fmt.Errorf("reassign fields: %w", err)
		}
	}

	if err := q.WeaveClearFieldOverrideCategory(ctx, &id); err != nil {
		return fmt.Errorf("clear field overrides: %w", err)
	}

	if err := q.WeaveDeleteCategory(ctx, id); err != nil {
		return fmt.Errorf("delete category: %w", err)
	}

	_ = semanticID

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

// List returns categories for a project, ordered by canonical_order.
// Optional QueryOption arguments are accepted for forward-compatibility but
// the underlying sqlc query only takes projectID — filter/sort/pagination
// will plug in as the query gains parameters.
func (s *postgresStore) List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Category, error) {
	_ = domain.ApplyOptions(opts) // reserved for future filter/limit/offset

	rows, err := s.queries.WeaveListCategories(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}

	out := make([]*domain.Category, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToCategory(row))
	}
	return out, nil
}

// Count returns the number of categories in a project.
func (s *postgresStore) Count(ctx context.Context, projectID string, opts ...domain.QueryOption) (int64, error) {
	_ = domain.ApplyOptions(opts)

	n, err := s.queries.WeaveCountCategories(ctx, projectID)
	if err != nil {
		return 0, fmt.Errorf("count categories: %w", err)
	}
	return n, nil
}

// ListWithCounts returns each category in the project with its field /
// override usage counts via a single joined query.
func (s *postgresStore) ListWithCounts(ctx context.Context, projectID string) ([]WithCounts, error) {
	rows, err := s.queries.WeaveListCategoriesWithCounts(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list categories with counts: %w", err)
	}

	out := make([]WithCounts, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToCategoryWithCounts(row))
	}
	return out, nil
}

func (s *postgresStore) ListWithCountsVersion(ctx context.Context, projectID, version string) ([]WithCounts, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			c.id, c.created_at, c.updated_at, c.semantic_id, c.system_name, c.ui_name,
			c.description, c.status, c.project_id, c.canonical_order, c.deprecated, c.version_number,
			(SELECT COUNT(*) FROM weave_field_overrides_archive
			 WHERE entity_type = ''
			   AND category_id = c.id
			   AND project_id = $1
			   AND version_number = $2) AS field_count,
			(SELECT COUNT(*) FROM weave_field_overrides_archive
			 WHERE entity_type = 'model'
			   AND category_id = c.id
			   AND project_id = $1
			   AND version_number = $2) AS model_field_count,
			(SELECT COUNT(*) FROM weave_field_overrides_archive
			 WHERE entity_type = 'collection'
			   AND category_id = c.id
			   AND project_id = $1
			   AND version_number = $2) AS collection_field_count,
			EXISTS (
				SELECT 1 FROM weave_field_overrides_archive fo
				WHERE fo.category_id != ''
				  AND fo.category_id IN (c.id, c.semantic_id)
				  AND fo.project_id = $1
				  AND fo.version_number = $2
			) AS in_use
		FROM weave_categories_archive c
		WHERE c.project_id = $1 AND c.version_number = $2
		ORDER BY c.canonical_order ASC, c.system_name ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived categories with counts: %w", err)
	}
	defer rows.Close()

	out := []WithCounts{}
	for rows.Next() {
		item, err := scanArchivedCategoryWithCounts(rows)
		if err != nil {
			return nil, fmt.Errorf("scan archived categories with counts: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

// Reorder rewrites canonical_order on each ID to its 1-based position in
// orderedIDs, transactionally. IDs not belonging to projectID are rejected.
func (s *postgresStore) Reorder(ctx context.Context, projectID string, orderedIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := s.queries.WithTx(tx)

	rows, err := q.WeaveListCategories(ctx, projectID)
	if err != nil {
		return fmt.Errorf("reorder: list categories: %w", err)
	}

	owned := make(map[string]bool, len(rows))
	for _, row := range rows {
		owned[row.ID] = true
	}

	for i, catID := range orderedIDs {
		if !owned[catID] {
			return fmt.Errorf("reorder: category %s not in project %s", catID, projectID)
		}
		if err := q.WeaveUpdateCategoryOrder(ctx, sqlcgen.WeaveUpdateCategoryOrderParams{
			ID:             catID,
			CanonicalOrder: int32(i + 1),
		}); err != nil {
			return fmt.Errorf("reorder: update %s: %w", catID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Cross-entity reads
// ---------------------------------------------------------------------------

// ModelFieldOverrides returns model-field rows referencing categoryID.
func (s *postgresStore) ModelFieldOverrides(ctx context.Context, projectID, categoryID string) ([]domain.OverrideEntry, error) {
	rows, err := s.queries.WeaveCategoryModelFieldOverrides(ctx, sqlcgen.WeaveCategoryModelFieldOverridesParams{
		CategoryID: dbutil.EmptyToNil(categoryID),
		ProjectID:  projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("model field overrides: %w", err)
	}

	out := make([]domain.OverrideEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.OverrideEntry{
			ID:         strconv.FormatInt(r.OverrideID, 10),
			ParentName: r.ModelName,
			FieldName:  r.FieldName,
		})
	}
	return out, nil
}

func (s *postgresStore) ModelFieldOverridesVersion(ctx context.Context, projectID, categoryID, version string) ([]domain.OverrideEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			fo.id,
			COALESCE(m.ui_name ->> 'en', m.system_name, '') AS model_name,
			COALESCE(f.ui_name ->> 'en', f.system_name, '') AS field_name
		FROM weave_field_overrides_archive fo
		JOIN weave_models_archive m
		  ON fo.entity_id = m.id AND m.version_number = $3
		JOIN weave_fields_archive f
		  ON fo.field_id = f.id AND f.version_number = $3
		WHERE fo.entity_type = 'model'
		  AND fo.category_id = $1
		  AND m.project_id = $2
		  AND fo.version_number = $3
	`, categoryID, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("archived model field overrides: %w", err)
	}
	defer rows.Close()

	out := []domain.OverrideEntry{}
	for rows.Next() {
		var entry domain.OverrideEntry
		var overrideID int64
		if err := rows.Scan(&overrideID, &entry.ParentName, &entry.FieldName); err != nil {
			return nil, err
		}
		entry.ID = strconv.FormatInt(overrideID, 10)
		out = append(out, entry)
	}
	return out, rows.Err()
}

// CollectionFieldOverrides returns collection-field rows referencing
// semanticID. Collection overrides key off semantic_id (legacy).
func (s *postgresStore) CollectionFieldOverrides(ctx context.Context, projectID, semanticID string) ([]domain.OverrideEntry, error) {
	rows, err := s.queries.WeaveCategoryCollectionFieldOverrides(ctx, sqlcgen.WeaveCategoryCollectionFieldOverridesParams{
		CategoryID: dbutil.EmptyToNil(semanticID),
		ProjectID:  projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("collection field overrides: %w", err)
	}

	out := make([]domain.OverrideEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.OverrideEntry{
			ID:         strconv.FormatInt(r.OverrideID, 10),
			ParentName: r.CollectionName,
			FieldName:  r.FieldName,
		})
	}
	return out, nil
}

func (s *postgresStore) CollectionFieldOverridesVersion(ctx context.Context, projectID, semanticID, version string) ([]domain.OverrideEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			fo.id,
			COALESCE(col.ui_name ->> 'en', col.system_name, '') AS collection_name,
			COALESCE(f.ui_name ->> 'en', f.system_name, '') AS field_name
		FROM weave_field_overrides_archive fo
		JOIN weave_collections_archive col
		  ON fo.entity_id = col.id AND col.version_number = $3
		JOIN weave_fields_archive f
		  ON fo.field_id = f.id AND f.version_number = $3
		WHERE fo.entity_type = 'collection'
		  AND fo.category_id = $1
		  AND col.project_id = $2
		  AND fo.version_number = $3
	`, semanticID, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("archived collection field overrides: %w", err)
	}
	defer rows.Close()

	out := []domain.OverrideEntry{}
	for rows.Next() {
		var entry domain.OverrideEntry
		var overrideID int64
		if err := rows.Scan(&overrideID, &entry.ParentName, &entry.FieldName); err != nil {
			return nil, err
		}
		entry.ID = strconv.FormatInt(overrideID, 10)
		out = append(out, entry)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Deprecate flips deprecated=true. The category is hidden from new-connection
// pickers but existing references are preserved.
func (s *postgresStore) Deprecate(ctx context.Context, projectID, id string) error {
	if err := s.queries.WeaveDeprecateCategory(ctx, id); err != nil {
		return fmt.Errorf("deprecate category: %w", err)
	}
	return nil
}

// Activate flips deprecated=false. Reverses Deprecate.
func (s *postgresStore) Activate(ctx context.Context, projectID, id string) error {
	if err := s.queries.WeaveActivateCategory(ctx, id); err != nil {
		return fmt.Errorf("activate category: %w", err)
	}
	return nil
}

// IsInUse runs the single-EXISTS query against weave_field_overrides matching
// either the category's ULID or its semantic_id. semanticID may be empty —
// pass empty string when the category has no semantic_id.
func (s *postgresStore) IsInUse(ctx context.Context, projectID, id, semanticID string) (bool, error) {
	inUse, err := s.queries.WeaveCategoryIsInUse(ctx, sqlcgen.WeaveCategoryIsInUseParams{
		CategoryUlid:       id,
		CategorySemanticID: semanticID,
	})
	if err != nil {
		return false, fmt.Errorf("category is in use: %w", err)
	}
	return inUse, nil
}

// ---------------------------------------------------------------------------
// Row converters
// ---------------------------------------------------------------------------

// rowToCategory converts a sqlcgen.WeaveCategory row to a *domain.Category.
func rowToCategory(row sqlcgen.WeaveCategory) *domain.Category {
	return &domain.Category{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  derefStr(row.SemanticID),
			SystemName:  derefStr(row.SystemName),
			UIName:      unmarshalTranslations(row.UiName),
			Description: unmarshalTranslations(row.Description),
			Status:      domain.Status(row.Status),
			ProjectID:   row.ProjectID,
			Deprecated:  row.Deprecated,
		},
		CanonicalOrder: int(row.CanonicalOrder),
	}
}

func scanArchivedCategory(scanner interface{ Scan(...any) error }) (*domain.Category, error) {
	var (
		id             string
		createdAt      time.Time
		updatedAt      time.Time
		semanticID     *string
		systemName     *string
		uiName         []byte
		description    []byte
		status         string
		projectID      string
		canonicalOrder int32
		deprecated     bool
		versionNumber  string
	)
	if err := scanner.Scan(
		&id,
		&createdAt,
		&updatedAt,
		&semanticID,
		&systemName,
		&uiName,
		&description,
		&status,
		&projectID,
		&canonicalOrder,
		&deprecated,
		&versionNumber,
	); err != nil {
		return nil, err
	}
	return &domain.Category{
		Entity: domain.Entity{
			ID:            id,
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
			SemanticID:    derefStr(semanticID),
			SystemName:    derefStr(systemName),
			UIName:        unmarshalTranslations(uiName),
			Description:   unmarshalTranslations(description),
			Status:        domain.Status(status),
			ProjectID:     projectID,
			Deprecated:    deprecated,
			VersionNumber: versionNumber,
		},
		CanonicalOrder: int(canonicalOrder),
	}, nil
}

// rowToCategoryWithCounts converts a list-with-counts row to WithCounts.
func rowToCategoryWithCounts(row sqlcgen.WeaveListCategoriesWithCountsRow) WithCounts {
	return WithCounts{
		Category: domain.Category{
			Entity: domain.Entity{
				ID:          row.ID,
				CreatedAt:   row.CreatedAt,
				UpdatedAt:   row.UpdatedAt,
				SemanticID:  derefStr(row.SemanticID),
				SystemName:  derefStr(row.SystemName),
				UIName:      unmarshalTranslations(row.UiName),
				Description: unmarshalTranslations(row.Description),
				Status:      domain.Status(row.Status),
				ProjectID:   row.ProjectID,
				Deprecated:  row.Deprecated,
			},
			CanonicalOrder: int(row.CanonicalOrder),
		},
		FieldCount:           row.FieldCount,
		ModelFieldCount:      row.ModelFieldCount,
		CollectionFieldCount: row.CollectionFieldCount,
		InUse:                row.InUse,
	}
}

func scanArchivedCategoryWithCounts(scanner interface{ Scan(...any) error }) (WithCounts, error) {
	var (
		id                   string
		createdAt            time.Time
		updatedAt            time.Time
		semanticID           *string
		systemName           *string
		uiName               []byte
		description          []byte
		status               string
		projectID            string
		canonicalOrder       int32
		deprecated           bool
		versionNumber        string
		fieldCount           int64
		modelFieldCount      int64
		collectionFieldCount int64
		inUse                bool
	)
	if err := scanner.Scan(
		&id,
		&createdAt,
		&updatedAt,
		&semanticID,
		&systemName,
		&uiName,
		&description,
		&status,
		&projectID,
		&canonicalOrder,
		&deprecated,
		&versionNumber,
		&fieldCount,
		&modelFieldCount,
		&collectionFieldCount,
		&inUse,
	); err != nil {
		return WithCounts{}, err
	}
	return WithCounts{
		Category: domain.Category{
			Entity: domain.Entity{
				ID:            id,
				CreatedAt:     createdAt,
				UpdatedAt:     updatedAt,
				SemanticID:    derefStr(semanticID),
				SystemName:    derefStr(systemName),
				UIName:        unmarshalTranslations(uiName),
				Description:   unmarshalTranslations(description),
				Status:        domain.Status(status),
				ProjectID:     projectID,
				Deprecated:    deprecated,
				VersionNumber: versionNumber,
			},
			CanonicalOrder: int(canonicalOrder),
		},
		FieldCount:           fieldCount,
		ModelFieldCount:      modelFieldCount,
		CollectionFieldCount: collectionFieldCount,
		InUse:                inUse,
	}, nil
}

// ---------------------------------------------------------------------------
// Local helpers (private to this slice)
// ---------------------------------------------------------------------------


// derefStr returns *p, or "" if p is nil.
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// marshalTranslations converts a domain.Translations to JSON bytes for
// storage. Returns nil for nil/empty input or marshalling errors.
func marshalTranslations(t domain.Translations) []byte {
	if t == nil {
		return nil
	}
	b, err := json.Marshal(t)
	if err != nil {
		return nil
	}
	return b
}

// unmarshalTranslations converts JSONB bytes back to a domain.Translations.
// Returns nil for nil/empty input or unmarshalling errors.
func unmarshalTranslations(b []byte) domain.Translations {
	if len(b) == 0 {
		return nil
	}
	var t domain.Translations
	if err := json.Unmarshal(b, &t); err != nil {
		return nil
	}
	return t
}

// generateULID returns a new monotonic ULID as a string. Inlined here to
// keep the slice self-contained — extract to a shared helpers package once
// a second slice needs the same function.
func generateULID() string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
