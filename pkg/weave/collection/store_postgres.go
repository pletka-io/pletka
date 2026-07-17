package collection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

type postgresStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

var _ Store = (*postgresStore)(nil)

func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{queries: sqlcgen.New(pool), pool: pool}
}

func (s *postgresStore) Create(ctx context.Context, c *domain.Collection) error {
	if c.Status == "" {
		c.Status = domain.StatusDraft
	}
	row, err := s.queries.WeaveCreateCollection(ctx, sqlcgen.WeaveCreateCollectionParams{
		ID:                       c.ID,
		SystemName:               dbutil.EmptyToNil(c.SystemName),
		UiName:                   marshalTranslations(c.UIName),
		Description:              marshalTranslations(c.Description),
		Status:                   string(c.Status),
		ProjectID:                c.ProjectID,
		OntologyScope:            marshalJSON(c.OntologyScope),
		CollectionNumber:         int32Ptr(c.CollectionNumber),
		CanonicalCollectionOrder: int32Ptr(c.CanonicalCollectionOrder),
		DefaultCategoryID:        c.DefaultCategoryID,
		StagingID:                c.StagingID,
	})
	if err != nil {
		return fmt.Errorf("create weave collection: %w", err)
	}
	c.CreatedAt = row.CreatedAt
	c.UpdatedAt = row.UpdatedAt
	return nil
}

func (s *postgresStore) GetByID(ctx context.Context, id string) (*domain.Collection, error) {
	row, err := s.queries.WeaveGetCollectionByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get weave collection: %w", err)
	}
	return rowToCollection(row), nil
}

// ListReferenceAdopted returns every collection from another project
// that the current project's field overrides reference via
// part_of_collection_id — task 3b reference-adopted slice.
func (s *postgresStore) ListReferenceAdopted(ctx context.Context, projectID string) ([]*domain.Collection, error) {
	rows, err := s.queries.WeaveListReferenceAdoptedCollections(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list reference-adopted collections: %w", err)
	}
	out := make([]*domain.Collection, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToCollection(r))
	}
	return out, nil
}

// ListReceiptCollectionDirect — see Store doc.
func (s *postgresStore) ListReceiptCollectionDirect(ctx context.Context, seedID, seedKind string) ([]string, error) {
	if seedID == "" {
		return nil, nil
	}
	ids, err := s.queries.WeaveListReceiptCollectionDirect(ctx, sqlcgen.WeaveListReceiptCollectionDirectParams{
		Column1: seedID,
		Column2: seedKind,
	})
	if err != nil {
		return nil, fmt.Errorf("list receipt collection direct: %w", err)
	}
	return ids, nil
}

// ListReceiptCollectionClosure returns collections in the transitive
// closure rooted at a single seed entity. Used by the Adoptions tab's
// per-receipt bill-of-materials.
func (s *postgresStore) ListReceiptCollectionClosure(ctx context.Context, seedID, seedKind string) ([]string, error) {
	if seedID == "" {
		return nil, nil
	}
	ids, err := s.queries.WeaveListReceiptCollectionClosure(ctx, sqlcgen.WeaveListReceiptCollectionClosureParams{
		Column1: seedID,
		Column2: seedKind,
	})
	if err != nil {
		return nil, fmt.Errorf("list receipt collection closure: %w", err)
	}
	return ids, nil
}

func (s *postgresStore) GetByIDVersion(ctx context.Context, projectID, id, version string) (*domain.Collection, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT
			id, created_at, updated_at, system_name, ui_name, description,
			status, project_id, ontology_scope, collection_number,
			canonical_collection_order, staging_id, deprecated, default_category_id, version_number
		FROM weave_collections_archive
		WHERE id = $1 AND project_id = $2 AND version_number = $3
	`, id, projectID, version)
	collection, err := scanArchivedCollection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archived weave collection: %w", err)
	}
	return collection, nil
}

func (s *postgresStore) GetByIdentifier(ctx context.Context, projectID, identifier string) (*domain.Collection, error) {
	row, err := s.queries.WeaveGetCollectionByIdentifier(ctx, sqlcgen.WeaveGetCollectionByIdentifierParams{
		ProjectID:  projectID,
		Identifier: identifier,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get weave collection by identifier: %w", err)
	}
	return rowToCollection(row), nil
}

func (s *postgresStore) Update(ctx context.Context, c *domain.Collection) error {
	if c.Status == "" {
		c.Status = domain.StatusDraft
	}
	row, err := s.queries.WeaveUpdateCollection(ctx, sqlcgen.WeaveUpdateCollectionParams{
		ID:                       c.ID,
		UiName:                   marshalTranslations(c.UIName),
		Description:              marshalTranslations(c.Description),
		SystemName:               dbutil.EmptyToNil(c.SystemName),
		Status:                   string(c.Status),
		OntologyScope:            marshalJSON(c.OntologyScope),
		CollectionNumber:         int32Ptr(c.CollectionNumber),
		CanonicalCollectionOrder: int32Ptr(c.CanonicalCollectionOrder),
		DefaultCategoryID:        c.DefaultCategoryID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("update weave collection %s: not found", c.ID)
		}
		return fmt.Errorf("update weave collection: %w", err)
	}
	c.UpdatedAt = row.UpdatedAt
	return nil
}

func (s *postgresStore) Delete(ctx context.Context, id string) error {
	if err := s.queries.WeaveDeleteCollection(ctx, id); err != nil {
		return fmt.Errorf("delete weave collection: %w", err)
	}
	return nil
}

func (s *postgresStore) List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Collection, int64, error) {
	cfg := domain.ApplyOptions(opts)
	limit := int32(cfg.Limit)
	if limit <= 0 {
		limit = 100
	}
	offset := int32(cfg.Offset)
	if offset < 0 {
		offset = 0
	}
	scopeClass, _ := cfg.Filters["scope_class"].(string)
	categoryID, _ := cfg.Filters["category_id"].(string)
	rows, err := s.queries.WeaveListCollections(ctx, sqlcgen.WeaveListCollectionsParams{
		ProjectID:    cfg.ProjectID,
		Search:       cfg.Search,
		ScopeClass:   scopeClass,
		CategoryID:   categoryID,
		SortBy:       cfg.OrderBy,
		SortDesc:     cfg.OrderDesc,
		ResultLimit:  limit,
		ResultOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list weave collections: %w", err)
	}
	total, err := s.queries.WeaveCountCollections(ctx, sqlcgen.WeaveCountCollectionsParams{
		ProjectID:  cfg.ProjectID,
		Search:     cfg.Search,
		ScopeClass: scopeClass,
		CategoryID: categoryID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count weave collections: %w", err)
	}
	out := make([]*domain.Collection, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToCollectionList(row))
	}
	return out, total, nil
}

// CategoriesUsed returns the distinct default_category_id values
// assigned to collections in this project.
func (s *postgresStore) CategoriesUsed(ctx context.Context, projectID string) ([]CollectionCategoryOption, error) {
	rows, err := s.queries.WeaveListCollectionCategoriesForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list collection categories: %w", err)
	}
	out := make([]CollectionCategoryOption, 0, len(rows))
	for _, r := range rows {
		out = append(out, CollectionCategoryOption{
			ID:             r.ID,
			UIName:         unmarshalTranslations(r.UiName),
			CanonicalOrder: int(r.CanonicalOrder),
		})
	}
	return out, nil
}

// ScopeClasses returns distinct ontology scope classes
// (prefix:local_name) used by collections in this project. Drives the
// scope_class filter dropdown.
func (s *postgresStore) ScopeClasses(ctx context.Context, projectID string) ([]string, error) {
	rows, err := s.queries.WeaveListCollectionScopeClasses(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list collection scope classes: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r != "" {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *postgresStore) ListVersion(ctx context.Context, projectID, version string, opts ...domain.QueryOption) ([]*domain.Collection, int64, error) {
	cfg := domain.ApplyOptions(opts)
	limit := int32(cfg.Limit)
	if limit <= 0 {
		limit = 100
	}
	offset := int32(cfg.Offset)
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `
		SELECT
			id, created_at, updated_at, system_name, ui_name, description,
			status, project_id, ontology_scope, collection_number,
			canonical_collection_order, staging_id, deprecated, default_category_id, version_number
		FROM weave_collections_archive
		WHERE project_id = $1
		  AND version_number = $2
		  AND ($3 = ''
		       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || $3 || '%')
		       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || $3 || '%')
		       OR system_name ILIKE '%' || $3 || '%')
		ORDER BY
			CASE WHEN $4 = 'ui_name' AND NOT $5::boolean THEN ui_name->>'en' END ASC NULLS LAST,
			CASE WHEN $4 = 'ui_name' AND $5::boolean THEN ui_name->>'en' END DESC NULLS LAST,
			CASE WHEN $4 = 'name' AND NOT $5::boolean THEN system_name END ASC NULLS LAST,
			CASE WHEN $4 = 'name' AND $5::boolean THEN system_name END DESC NULLS LAST,
			CASE WHEN $4 = 'system_name' AND NOT $5::boolean THEN system_name END ASC NULLS LAST,
			CASE WHEN $4 = 'system_name' AND $5::boolean THEN system_name END DESC NULLS LAST,
			CASE WHEN $4 = 'updated_at' AND NOT $5::boolean THEN updated_at END ASC,
			CASE WHEN $4 = 'updated_at' AND $5::boolean THEN updated_at END DESC,
			canonical_collection_order ASC, system_name ASC
		LIMIT $6 OFFSET $7
	`, projectID, version, cfg.Search, cfg.OrderBy, cfg.OrderDesc, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list archived weave collections: %w", err)
	}
	defer rows.Close()
	out := []*domain.Collection{}
	for rows.Next() {
		collection, err := scanArchivedCollection(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan archived weave collection: %w", err)
		}
		out = append(out, collection)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate archived weave collections: %w", err)
	}
	var total int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM weave_collections_archive
		WHERE project_id = $1
		  AND version_number = $2
		  AND ($3 = ''
		       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || $3 || '%')
		       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || $3 || '%')
		       OR system_name ILIKE '%' || $3 || '%')
	`, projectID, version, cfg.Search).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count archived weave collections: %w", err)
	}
	return out, total, nil
}

func (s *postgresStore) ListOptions(ctx context.Context, projectID string) ([]domain.EntityOption, error) {
	rows, err := s.queries.WeaveListCollectionOptions(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list collection options: %w", err)
	}
	out := make([]domain.EntityOption, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.EntityOption{
			ID: row.ID,
			// id IS the semantic ID for collections (migration 004) — no
			// separate column to read from.
			SemanticID: row.ID,
			SystemName: derefStr(row.SystemName),
			UIName:     unmarshalTranslations(row.UiName),
			Status:     row.Status,
		})
	}
	return out, nil
}

func (s *postgresStore) IsInUse(ctx context.Context, projectID, collectionID string) (bool, error) {
	in, err := s.queries.WeaveCollectionIsInUse(ctx, sqlcgen.WeaveCollectionIsInUseParams{
		TargetID:  collectionID,
		ProjectID: projectID,
	})
	if err != nil {
		return false, fmt.Errorf("collection is-in-use: %w", err)
	}
	return in, nil
}

func (s *postgresStore) Usage(ctx context.Context, projectID, collectionID string) (UsageReport, error) {
	count, err := s.queries.WeaveCollectionUsageCount(ctx, sqlcgen.WeaveCollectionUsageCountParams{
		TargetID:  collectionID,
		ProjectID: projectID,
	})
	if err != nil {
		return UsageReport{}, fmt.Errorf("collection usage count: %w", err)
	}
	report := UsageReport{FieldCount: count}
	if count > 0 {
		rows, err := s.queries.WeaveCollectionUsageSamples(ctx, sqlcgen.WeaveCollectionUsageSamplesParams{
			TargetID:  collectionID,
			ProjectID: projectID,
		})
		if err != nil {
			return UsageReport{}, fmt.Errorf("collection usage samples: %w", err)
		}
		samples := make([]FieldRef, 0, len(rows))
		for _, r := range rows {
			samples = append(samples, FieldRef{
				FieldID:         r.FieldID,
				FieldSemanticID: derefStr(r.FieldSemanticID),
				FieldName:       r.FieldName,
			})
		}
		report.FieldSamples = samples
	}
	return report, nil
}

func (s *postgresStore) UsageVersion(ctx context.Context, projectID, collectionID, version string) (UsageReport, error) {
	var count int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT o.field_id) AS field_count
		FROM weave_override_refs_archive r
		JOIN weave_field_overrides_archive o
		  ON o.id = r.override_id
		 AND o.version_number = r.version_number
		WHERE r.target_id = $1
		  AND r.ref_type IN ('resource_model', 'collection_model')
		  AND o.project_id = $2
		  AND o.version_number = $3
	`, collectionID, projectID, version).Scan(&count); err != nil {
		return UsageReport{}, fmt.Errorf("archived collection usage count: %w", err)
	}
	report := UsageReport{FieldCount: count}
	if count == 0 {
		return report, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT
			f.id AS field_id,
			f.semantic_id AS field_semantic_id,
			COALESCE(f.ui_name ->> 'en', f.system_name, '') AS field_name
		FROM weave_override_refs_archive r
		JOIN weave_field_overrides_archive o
		  ON o.id = r.override_id
		 AND o.version_number = r.version_number
		JOIN weave_fields_archive f
		  ON f.id = o.field_id
		 AND f.version_number = o.version_number
		WHERE r.target_id = $1
		  AND r.ref_type IN ('resource_model', 'collection_model')
		  AND o.project_id = $2
		  AND o.version_number = $3
		ORDER BY field_name
		LIMIT 10
	`, collectionID, projectID, version)
	if err != nil {
		return UsageReport{}, fmt.Errorf("archived collection usage samples: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ref FieldRef
		var semanticID *string
		if err := rows.Scan(&ref.FieldID, &semanticID, &ref.FieldName); err != nil {
			return UsageReport{}, fmt.Errorf("scan archived collection usage sample: %w", err)
		}
		ref.FieldSemanticID = derefStr(semanticID)
		report.FieldSamples = append(report.FieldSamples, ref)
	}
	if err := rows.Err(); err != nil {
		return UsageReport{}, fmt.Errorf("iterate archived collection usage samples: %w", err)
	}
	return report, nil
}

// BatchCompositionCounts computes per-collection field counts for a whole list
// page in one query. entity_id identifies the collection uniquely, so no
// project filter is needed.
func (s *postgresStore) BatchCompositionCounts(ctx context.Context, collectionIDs []string) (map[string]domain.CompositionCounts, error) {
	out := make(map[string]domain.CompositionCounts, len(collectionIDs))
	if len(collectionIDs) == 0 {
		return out, nil
	}
	const q = `
SELECT fo.entity_id, count(*) AS field_count
FROM weave_field_overrides fo
WHERE fo.entity_type = 'collection' AND fo.entity_id = ANY($1)
GROUP BY fo.entity_id`
	rows, err := s.pool.Query(ctx, q, collectionIDs)
	if err != nil {
		return nil, fmt.Errorf("batch collection composition counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id string
			fc int
		)
		if err := rows.Scan(&id, &fc); err != nil {
			return nil, fmt.Errorf("scan collection composition: %w", err)
		}
		out[id] = domain.CompositionCounts{FieldCount: fc}
	}
	return out, rows.Err()
}

func (s *postgresStore) Deprecate(ctx context.Context, collectionID string) error {
	if err := s.queries.WeaveDeprecateCollection(ctx, collectionID); err != nil {
		return fmt.Errorf("deprecate collection: %w", err)
	}
	return nil
}

// ListUsage returns the models that reuse fields from this collection
// (weave_field_overrides rows where entity_type='model' and
// part_of_collection_id = collectionID). Drives the Reuse tab on the
// collection detail view. Same-project only.
func (s *postgresStore) ListUsage(ctx context.Context, collectionID, projectID string) ([]domain.FieldUsageRef, error) {
	// No project filter on the collection's owner: include models in OTHER
	// projects that bundle this collection, so the Reuse tab can show
	// cross-project usage. The override that ties a model to the collection
	// lives in the model's own project (fo.project_id = m.project_id).
	const q = `
SELECT
    m.id                                     AS id,
    m.project_id                             AS project_id,
    COALESCE(m.system_name, '')              AS system_name,
    COALESCE(m.ui_name, '{}'::jsonb)         AS ui_name,
    COALESCE(m.description, '{}'::jsonb)     AS description
FROM weave_models m
WHERE EXISTS (
      SELECT 1 FROM weave_field_overrides fo
      WHERE fo.entity_type = 'model'
        AND fo.entity_id = m.id
        AND fo.part_of_collection_id = $1
        AND fo.project_id = m.project_id
  )
ORDER BY m.project_id ASC, COALESCE(m.ui_name ->> 'en', m.system_name, '') ASC, m.id ASC`

	rows, err := s.pool.Query(ctx, q, collectionID)
	if err != nil {
		return nil, fmt.Errorf("list collection usage: %w", err)
	}
	defer rows.Close()

	var out []domain.FieldUsageRef
	for rows.Next() {
		var (
			id, projectID2, sysName string
			uiNameJSON, descJSON    []byte
		)
		if err := rows.Scan(&id, &projectID2, &sysName, &uiNameJSON, &descJSON); err != nil {
			return nil, fmt.Errorf("scan collection usage row: %w", err)
		}
		out = append(out, domain.FieldUsageRef{
			ID:          id,
			SemanticID:  id,
			SystemName:  sysName,
			Name:        unmarshalTranslations(uiNameJSON),
			Description: unmarshalTranslations(descJSON),
			ProjectID:   projectID2,
			URL:         "/projects/" + projectID2 + "/models/" + id,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate collection usage rows: %w", err)
	}
	return out, nil
}

func (s *postgresStore) Activate(ctx context.Context, collectionID string) error {
	if err := s.queries.WeaveActivateCollection(ctx, collectionID); err != nil {
		return fmt.Errorf("activate collection: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row converters + helpers
// ---------------------------------------------------------------------------

func rowToCollection(row sqlcgen.WeaveCollection) *domain.Collection {
	c := &domain.Collection{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  row.ID,
			SystemName:  derefStr(row.SystemName),
			UIName:      unmarshalTranslations(row.UiName),
			Description: unmarshalTranslations(row.Description),
			Status:      domain.Status(row.Status),
			ProjectID:   row.ProjectID,
			Deprecated:  row.Deprecated,
		},
		CollectionNumber:         derefInt32(row.CollectionNumber),
		CanonicalCollectionOrder: derefInt32(row.CanonicalCollectionOrder),
		DefaultCategoryID:        row.DefaultCategoryID,
		StagingID:                row.StagingID,
	}
	if len(row.OntologyScope) > 0 {
		_ = json.Unmarshal(row.OntologyScope, &c.OntologyScope)
	}
	return c
}

func rowToCollectionList(row sqlcgen.WeaveListCollectionsRow) *domain.Collection {
	c := &domain.Collection{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  row.ID,
			SystemName:  derefStr(row.SystemName),
			UIName:      unmarshalTranslations(row.UiName),
			Description: unmarshalTranslations(row.Description),
			Status:      domain.Status(row.Status),
			ProjectID:   row.ProjectID,
			Deprecated:  row.Deprecated,
		},
		CollectionNumber:         derefInt32(row.CollectionNumber),
		CanonicalCollectionOrder: derefInt32(row.CanonicalCollectionOrder),
		DefaultCategoryID:        row.DefaultCategoryID,
		StagingID:                row.StagingID,
	}
	if len(row.OntologyScope) > 0 {
		_ = json.Unmarshal(row.OntologyScope, &c.OntologyScope)
	}
	return c
}

type archivedCollectionScanner interface {
	Scan(dest ...any) error
}

func scanArchivedCollection(row archivedCollectionScanner) (*domain.Collection, error) {
	var (
		id, projectID, status, version string
		createdAt, updatedAt           time.Time
		systemName                     *string
		uiName, description            []byte
		ontologyScope                  []byte
		collectionNumber               *int32
		canonicalCollectionOrder       *int32
		stagingID                      *int64
		deprecated                     bool
		defaultCategoryID              *string
	)
	if err := row.Scan(
		&id, &createdAt, &updatedAt, &systemName, &uiName, &description,
		&status, &projectID, &ontologyScope, &collectionNumber,
		&canonicalCollectionOrder, &stagingID, &deprecated, &defaultCategoryID, &version,
	); err != nil {
		return nil, err
	}
	c := &domain.Collection{
		Entity: domain.Entity{
			ID:            id,
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
			SemanticID:    id,
			SystemName:    derefStr(systemName),
			UIName:        unmarshalTranslations(uiName),
			Description:   unmarshalTranslations(description),
			Status:        domain.Status(status),
			VersionNumber: version,
			ProjectID:     projectID,
			Deprecated:    deprecated,
		},
		CollectionNumber:         derefInt32(collectionNumber),
		CanonicalCollectionOrder: derefInt32(canonicalCollectionOrder),
		DefaultCategoryID:        defaultCategoryID,
		StagingID:                stagingID,
	}
	if len(ontologyScope) > 0 {
		_ = json.Unmarshal(ontologyScope, &c.OntologyScope)
	}
	return c, nil
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func int32Ptr(i int) *int32 {
	v := int32(i)
	return &v
}

func derefInt32(p *int32) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

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

func marshalJSON(v any) []byte {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

// AnchorIndex returns every non-deprecated collection paired with its
// anchor class URI. The anchor is computed as the URI of the last
// class element in the longest path prefix common to every member
// field — same definition the snap builder uses via
// ComputeSharedPathPrefix.
//
// Implementation: a single SQL pulls each (collection_id,
// field_class_chain) pair across the whole DB (~15k rows for the
// OGEE+SI workspace); Go groups by collection and walks the chains
// in lock-step to find the prefix. Cost is one query plus an O(N)
// pass, no per-collection round-trips.
//
// Used by snap-level collection composition to graft cross-project
// collections like LA's Dimension under a model's Collection-stub
// fields whose path ends at the same class.
func (s *postgresStore) AnchorIndex(ctx context.Context) ([]domain.CollectionAnchor, error) {
	// Pull all (collection_id, field_id, class-uri-chain) triples.
	// Filter at SQL: only field-overrides that actually belong to a
	// collection, only non-deprecated collections, only fields with
	// non-empty path_elements.
	// DISTINCT to collapse multiple overrides of the same field in the
	// same collection (collection-level + project-level + model-level
	// overrides for the same field-collection pair each spawn a row;
	// path_elements is on the field itself, not the override).
	const q = `
SELECT DISTINCT
       fo.part_of_collection_id,
       c.project_id,
       f.id,
       COALESCE((
         SELECT jsonb_agg(pe_el->>'uri' ORDER BY (pe_el->>'position')::int)
         FROM jsonb_array_elements(f.path_elements) AS pe_el
         WHERE pe_el->>'type' = 'class'
       ), '[]'::jsonb)
FROM weave_field_overrides fo
JOIN weave_fields f       ON f.id = fo.field_id
JOIN weave_collections c  ON c.id = fo.part_of_collection_id
WHERE fo.part_of_collection_id <> ''
  AND NOT c.deprecated
  AND f.path_elements IS NOT NULL;`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("collection anchor index: %w", err)
	}
	defer rows.Close()

	type collFields struct {
		projectID string
		chains    [][]string
	}
	byColl := map[string]*collFields{}
	for rows.Next() {
		var (
			collID, projectID, fieldID string
			chainJSON                  []byte
		)
		if err := rows.Scan(&collID, &projectID, &fieldID, &chainJSON); err != nil {
			return nil, fmt.Errorf("scan anchor row: %w", err)
		}
		var chain []string
		if err := json.Unmarshal(chainJSON, &chain); err != nil {
			return nil, fmt.Errorf("unmarshal class chain: %w", err)
		}
		if len(chain) == 0 {
			continue
		}
		entry := byColl[collID]
		if entry == nil {
			entry = &collFields{projectID: projectID}
			byColl[collID] = entry
		}
		entry.chains = append(entry.chains, chain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate anchor index rows: %w", err)
	}

	out := make([]domain.CollectionAnchor, 0, len(byColl))
	for collID, entry := range byColl {
		anchor := longestCommonClassPrefixTail(entry.chains)
		if anchor == "" {
			continue
		}
		out = append(out, domain.CollectionAnchor{
			CollectionID: collID,
			ProjectID:    entry.projectID,
			AnchorURI:    anchor,
		})
	}
	return out, nil
}

// longestCommonClassPrefixTail returns the URI of the last element of
// the longest class-URI prefix shared across every chain. Empty when
// no chain is supplied or the shortest chain has no shared first
// element with the others. Mirrors the structural meaning of
// ComputeSharedPathPrefix from pkg/weave/resolve.go for class-only
// chains.
func longestCommonClassPrefixTail(chains [][]string) string {
	if len(chains) == 0 {
		return ""
	}
	prefixLen := len(chains[0])
	for _, c := range chains[1:] {
		if len(c) < prefixLen {
			prefixLen = len(c)
		}
		for i := 0; i < prefixLen; i++ {
			if c[i] != chains[0][i] {
				prefixLen = i
				break
			}
		}
		if prefixLen == 0 {
			return ""
		}
	}
	if prefixLen == 0 {
		return ""
	}
	return chains[0][prefixLen-1]
}
