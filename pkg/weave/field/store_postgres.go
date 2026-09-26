package field

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// postgresStore is the pgx + sqlc implementation of Store, backed by
// the weave_fields table. Cross-references (GetModels / GetCollections)
// query weave_field_overrides — that's the canonical source post the
// model/collection-field migrations.
type postgresStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

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

func (s *postgresStore) Create(ctx context.Context, f *domain.Field) error {
	if f.Status == "" {
		f.Status = domain.StatusDraft
	}
	row, err := s.queries.WeaveCreateField(ctx, sqlcgen.WeaveCreateFieldParams{
		ID:                f.ID,
		SemanticID:        dbutil.EmptyToNil(f.SemanticID),
		SystemName:        dbutil.EmptyToNil(f.SystemName),
		UiName:            marshalTranslations(f.UIName),
		Description:       marshalTranslations(f.Description),
		Status:            string(f.Status),
		ProjectID:         f.ProjectID,
		OntologyScope:     marshalJSON(f.OntologyScope),
		OntologyPath:      dbutil.EmptyToNil(f.OntologyPath()),
		PathElements:      marshalJSON(f.PathElements),
		ExpectedValueType: dbutil.EmptyToNil(f.ExpectedValueType),
		Examples:          marshalJSON(f.Examples),
	})
	if err != nil {
		return fmt.Errorf("create weave field: %w", err)
	}
	f.CreatedAt = row.CreatedAt
	f.UpdatedAt = row.UpdatedAt
	return nil
}

func (s *postgresStore) Update(ctx context.Context, f *domain.Field) error {
	if f.Status == "" {
		f.Status = domain.StatusDraft
	}
	row, err := s.queries.WeaveUpdateField(ctx, sqlcgen.WeaveUpdateFieldParams{
		ID:                f.ID,
		UiName:            marshalTranslations(f.UIName),
		Description:       marshalTranslations(f.Description),
		SystemName:        dbutil.EmptyToNil(f.SystemName),
		Status:            string(f.Status),
		OntologyScope:     marshalJSON(f.OntologyScope),
		OntologyPath:      dbutil.EmptyToNil(f.OntologyPath()),
		PathElements:      marshalJSON(f.PathElements),
		ExpectedValueType: dbutil.EmptyToNil(f.ExpectedValueType),
		Examples:          marshalJSON(f.Examples),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("update weave field %s: not found", f.ID)
		}
		return fmt.Errorf("update weave field: %w", err)
	}
	f.UpdatedAt = row.UpdatedAt
	return nil
}

func (s *postgresStore) Delete(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete field tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.queries.WithTx(tx)
	// The field's own override rows are removed explicitly, in the same
	// transaction as the field row — weave_field_overrides carries no FK on
	// field_id, so nothing cascades on its own. Leaving the base row behind
	// orphans a category_id that the category in-use count keeps counting,
	// with no drill-down able to explain the number.
	if err := q.WeaveDeleteOverridesForField(ctx, id); err != nil {
		return fmt.Errorf("delete field overrides: %w", err)
	}
	if err := q.WeaveDeleteField(ctx, id); err != nil {
		return fmt.Errorf("delete weave field: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete field tx: %w", err)
	}
	return nil
}

func (s *postgresStore) GetByID(ctx context.Context, id string) (*domain.Field, error) {
	row, err := s.queries.WeaveGetFieldByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get weave field by id: %w", err)
	}
	return rowToField(row), nil
}

// ListReferenceAdopted returns every field from another project that
// the current project's overrides reference via field_id — task 3b
// reference-adopted slice.
func (s *postgresStore) ListReferenceAdopted(ctx context.Context, projectID string) ([]*domain.Field, error) {
	rows, err := s.queries.WeaveListReferenceAdoptedFields(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list reference-adopted fields: %w", err)
	}
	out := make([]*domain.Field, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToField(r))
	}
	return out, nil
}

// ListReceiptFieldClosure returns fields used by any override in the
// transitive closure rooted at a single seed entity. Used by the
// Adoptions tab's per-receipt bill-of-materials. Field-as-seed edge
// case: the walker can't recurse from a field, so the SQL returns
// empty — caller (handler) should short-circuit to [seedID] when
// seedKind is "field".
func (s *postgresStore) ListReceiptFieldClosure(ctx context.Context, seedID, seedKind string) ([]string, error) {
	if seedID == "" {
		return nil, nil
	}
	if seedKind == "field" {
		// Walker doesn't traverse from a field — return the seed itself
		// so the receipt's "fields" section has one row to show.
		return []string{seedID}, nil
	}
	ids, err := s.queries.WeaveListReceiptFieldClosure(ctx, sqlcgen.WeaveListReceiptFieldClosureParams{
		Column1: seedID,
		Column2: seedKind,
	})
	if err != nil {
		return nil, fmt.Errorf("list receipt field closure: %w", err)
	}
	return ids, nil
}

// ListReceiptFieldDirect — see Store doc. Field-as-seed short-circuits
// to [seedID] for the same reason as the closure variant.
func (s *postgresStore) ListReceiptFieldDirect(ctx context.Context, seedID, seedKind string) ([]string, error) {
	if seedID == "" {
		return nil, nil
	}
	if seedKind == "field" {
		return []string{seedID}, nil
	}
	ids, err := s.queries.WeaveListReceiptFieldDirect(ctx, sqlcgen.WeaveListReceiptFieldDirectParams{
		Column1: seedID,
		Column2: seedKind,
	})
	if err != nil {
		return nil, fmt.Errorf("list receipt field direct: %w", err)
	}
	return ids, nil
}

func (s *postgresStore) GetByIDVersion(ctx context.Context, projectID, id, version string) (*domain.Field, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT
			id, created_at, updated_at, semantic_id, system_name, ui_name, description,
			status, project_id, ontology_scope, ontology_path, path_elements,
			expected_value_type, examples, staging_id, deprecated, version_number
		FROM weave_fields_archive
		WHERE id = $1 AND project_id = $2 AND version_number = $3
	`, id, projectID, version)
	field, err := scanArchivedField(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archived weave field by id: %w", err)
	}
	return field, nil
}

func (s *postgresStore) GetByIdentifier(ctx context.Context, projectID, identifier string) (*domain.Field, error) {
	row, err := s.queries.WeaveGetFieldByIdentifier(ctx, sqlcgen.WeaveGetFieldByIdentifierParams{
		SemanticID: dbutil.EmptyToNil(identifier),
		ProjectID:  projectID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get weave field by identifier: %w", err)
	}
	return rowToField(row), nil
}

func (s *postgresStore) List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Field, int64, error) {
	cfg := domain.ApplyOptions(opts)

	limit := int32(cfg.Limit)
	if limit <= 0 {
		limit = 100
	}
	offset := int32(cfg.Offset)
	if offset < 0 {
		offset = 0
	}

	status, _ := cfg.Filters["status"].(string)
	owner, _ := cfg.Filters["owner_id"].(string)

	rows, err := s.queries.WeaveListFields(ctx, sqlcgen.WeaveListFieldsParams{
		ProjectID:    cfg.ProjectID,
		Search:       cfg.Search,
		Status:       status,
		OwnerID:      owner,
		SortBy:       cfg.OrderBy,
		SortDesc:     cfg.OrderDesc,
		ResultLimit:  limit,
		ResultOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list weave fields: %w", err)
	}

	total, err := s.queries.WeaveCountFields(ctx, sqlcgen.WeaveCountFieldsParams{
		ProjectID: cfg.ProjectID,
		Search:    cfg.Search,
		Status:    status,
		OwnerID:   owner,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count weave fields: %w", err)
	}

	out := make([]*domain.Field, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToFieldList(row))
	}
	return out, total, nil
}

func (s *postgresStore) ListVersion(ctx context.Context, projectID, version string, opts ...domain.QueryOption) ([]*domain.Field, int64, error) {
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
			id, created_at, updated_at, semantic_id, system_name, ui_name, description,
			status, project_id, ontology_scope, ontology_path, path_elements,
			expected_value_type, examples, staging_id, deprecated, version_number
		FROM weave_fields_archive
		WHERE project_id = $1
		  AND version_number = $2
		  AND ($3 = ''
		       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || $3 || '%')
		       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || $3 || '%')
		       OR system_name ILIKE '%' || $3 || '%'
		       OR COALESCE(semantic_id, '') ILIKE '%' || $3 || '%'
		       OR COALESCE(ontology_path, '') ILIKE '%' || $3 || '%')
		ORDER BY
			CASE WHEN $4 = 'ui_name' AND NOT $5::boolean THEN ui_name->>'en' END ASC NULLS LAST,
			CASE WHEN $4 = 'ui_name' AND $5::boolean THEN ui_name->>'en' END DESC NULLS LAST,
			CASE WHEN $4 = 'name' AND NOT $5::boolean THEN system_name END ASC NULLS LAST,
			CASE WHEN $4 = 'name' AND $5::boolean THEN system_name END DESC NULLS LAST,
			CASE WHEN $4 = 'system_name' AND NOT $5::boolean THEN system_name END ASC NULLS LAST,
			CASE WHEN $4 = 'system_name' AND $5::boolean THEN system_name END DESC NULLS LAST,
			CASE WHEN $4 = 'updated_at' AND NOT $5::boolean THEN updated_at END ASC,
			CASE WHEN $4 = 'updated_at' AND $5::boolean THEN updated_at END DESC,
			system_name ASC
		LIMIT $6 OFFSET $7
	`, projectID, version, cfg.Search, cfg.OrderBy, cfg.OrderDesc, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list archived weave fields: %w", err)
	}
	defer rows.Close()

	out := []*domain.Field{}
	for rows.Next() {
		field, err := scanArchivedField(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan archived weave field: %w", err)
		}
		out = append(out, field)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate archived weave fields: %w", err)
	}

	var total int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM weave_fields_archive
		WHERE project_id = $1
		  AND version_number = $2
		  AND ($3 = ''
		       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || $3 || '%')
		       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || $3 || '%')
		       OR system_name ILIKE '%' || $3 || '%'
		       OR COALESCE(semantic_id, '') ILIKE '%' || $3 || '%'
		       OR COALESCE(ontology_path, '') ILIKE '%' || $3 || '%')
	`, projectID, version, cfg.Search).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count archived weave fields: %w", err)
	}
	return out, total, nil
}

// ---------------------------------------------------------------------------
// Path queries
// ---------------------------------------------------------------------------

func (s *postgresStore) FindByPathSequence(ctx context.Context, query domain.PathQuery) ([]*domain.Field, error) {
	n := len(query.LocalNames)
	if n < 2 || n > 3 {
		return nil, fmt.Errorf("path sequence requires 2-3 elements, got %d", n)
	}

	if query.Anchor != "" {
		if n != 2 {
			return nil, fmt.Errorf("anchored path sequence requires exactly 2 elements, got %d", n)
		}
		rows, err := s.queries.FindFieldsByAnchoredSequence2(ctx, sqlcgen.FindFieldsByAnchoredSequence2Params{
			Anchor: query.Anchor,
			Name1:  query.LocalNames[0],
			Name2:  query.LocalNames[1],
		})
		if err != nil {
			return nil, fmt.Errorf("find fields by anchored sequence: %w", err)
		}
		return rowsToFields(rows), nil
	}

	if query.Contiguous {
		switch n {
		case 2:
			rows, err := s.queries.FindFieldsByContiguousSequence2(ctx, sqlcgen.FindFieldsByContiguousSequence2Params{
				Name1: query.LocalNames[0],
				Name2: query.LocalNames[1],
			})
			if err != nil {
				return nil, fmt.Errorf("find fields by contiguous sequence: %w", err)
			}
			return rowsToFields(rows), nil
		case 3:
			rows, err := s.queries.FindFieldsByContiguousSequence3(ctx, sqlcgen.FindFieldsByContiguousSequence3Params{
				Name1: query.LocalNames[0],
				Name2: query.LocalNames[1],
				Name3: query.LocalNames[2],
			})
			if err != nil {
				return nil, fmt.Errorf("find fields by contiguous sequence: %w", err)
			}
			return rowsToFields(rows), nil
		}
	}

	switch n {
	case 2:
		rows, err := s.queries.FindFieldsBySubsequence2(ctx, sqlcgen.FindFieldsBySubsequence2Params{
			Name1: query.LocalNames[0],
			Name2: query.LocalNames[1],
		})
		if err != nil {
			return nil, fmt.Errorf("find fields by subsequence: %w", err)
		}
		return rowsToFields(rows), nil
	case 3:
		rows, err := s.queries.FindFieldsBySubsequence3(ctx, sqlcgen.FindFieldsBySubsequence3Params{
			Name1: query.LocalNames[0],
			Name2: query.LocalNames[1],
			Name3: query.LocalNames[2],
		})
		if err != nil {
			return nil, fmt.Errorf("find fields by subsequence: %w", err)
		}
		return rowsToFields(rows), nil
	}

	return nil, fmt.Errorf("unreachable: n=%d", n)
}

// ---------------------------------------------------------------------------
// Cross-references (via weave_field_overrides)
// ---------------------------------------------------------------------------

func (s *postgresStore) GetModels(ctx context.Context, fieldID string) ([]domain.FieldModelRef, error) {
	rows, err := s.queries.WeaveGetFieldModels(ctx, fieldID)
	if err != nil {
		return nil, fmt.Errorf("get field models: %w", err)
	}
	out := make([]domain.FieldModelRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.FieldModelRef{
			ModelID:   r.ModelID,
			ModelName: r.ModelName,
		})
	}
	return out, nil
}

func (s *postgresStore) GetCollections(ctx context.Context, fieldID string) ([]domain.FieldCollectionRef, error) {
	rows, err := s.queries.WeaveGetFieldCollections(ctx, fieldID)
	if err != nil {
		return nil, fmt.Errorf("get field collections: %w", err)
	}
	out := make([]domain.FieldCollectionRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.FieldCollectionRef{
			CollectionID:   r.CollectionID,
			CollectionName: r.CollectionName,
		})
	}
	return out, nil
}

func (s *postgresStore) UsageVersion(ctx context.Context, projectID, fieldID, version string) (UsageReport, error) {
	var usage UsageReport
	if err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE entity_type = 'model') AS model_count,
			COUNT(*) FILTER (WHERE entity_type = 'collection') AS collection_count
		FROM weave_field_overrides_archive
		WHERE field_id = $1
		  AND project_id = $2
		  AND version_number = $3
		  AND entity_type IN ('model', 'collection')
	`, fieldID, projectID, version).Scan(&usage.ModelCount, &usage.CollectionCount); err != nil {
		return UsageReport{}, fmt.Errorf("archived field usage counts: %w", err)
	}

	modelRows, err := s.pool.Query(ctx, `
		SELECT
			m.id,
			COALESCE(m.ui_name ->> 'en', m.system_name, '') AS model_name
		FROM weave_field_overrides_archive fo
		JOIN weave_models_archive m
		  ON m.id = fo.entity_id
		 AND m.project_id = fo.project_id
		 AND m.version_number = fo.version_number
		WHERE fo.entity_type = 'model'
		  AND fo.field_id = $1
		  AND fo.project_id = $2
		  AND fo.version_number = $3
		ORDER BY model_name
		LIMIT 10
	`, fieldID, projectID, version)
	if err != nil {
		return UsageReport{}, fmt.Errorf("archived field model samples: %w", err)
	}
	defer modelRows.Close()
	for modelRows.Next() {
		var ref domain.FieldModelRef
		if err := modelRows.Scan(&ref.ModelID, &ref.ModelName); err != nil {
			return UsageReport{}, fmt.Errorf("scan archived field model sample: %w", err)
		}
		usage.ModelSamples = append(usage.ModelSamples, ref)
	}
	if err := modelRows.Err(); err != nil {
		return UsageReport{}, fmt.Errorf("iterate archived field model samples: %w", err)
	}

	collectionRows, err := s.pool.Query(ctx, `
		SELECT
			c.id,
			COALESCE(c.ui_name ->> 'en', c.system_name, '') AS collection_name
		FROM weave_field_overrides_archive fo
		JOIN weave_collections_archive c
		  ON c.id = fo.entity_id
		 AND c.project_id = fo.project_id
		 AND c.version_number = fo.version_number
		WHERE fo.entity_type = 'collection'
		  AND fo.field_id = $1
		  AND fo.project_id = $2
		  AND fo.version_number = $3
		ORDER BY collection_name
		LIMIT 10
	`, fieldID, projectID, version)
	if err != nil {
		return UsageReport{}, fmt.Errorf("archived field collection samples: %w", err)
	}
	defer collectionRows.Close()
	for collectionRows.Next() {
		var ref domain.FieldCollectionRef
		if err := collectionRows.Scan(&ref.CollectionID, &ref.CollectionName); err != nil {
			return UsageReport{}, fmt.Errorf("scan archived field collection sample: %w", err)
		}
		usage.CollectionSamples = append(usage.CollectionSamples, ref)
	}
	if err := collectionRows.Err(); err != nil {
		return UsageReport{}, fmt.Errorf("iterate archived field collection samples: %w", err)
	}

	return usage, nil
}

// ---------------------------------------------------------------------------
// Row converter + helpers (slice-private)
// ---------------------------------------------------------------------------

func rowToField(row sqlcgen.WeaveField) *domain.Field {
	f := &domain.Field{
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
		ExpectedValueType: derefStr(row.ExpectedValueType),
	}
	if len(row.OntologyScope) > 0 {
		_ = json.Unmarshal(row.OntologyScope, &f.OntologyScope)
	}
	if len(row.PathElements) > 0 {
		_ = json.Unmarshal(row.PathElements, &f.PathElements)
	}
	if len(row.SubfieldPaths) > 0 {
		_ = json.Unmarshal(row.SubfieldPaths, &f.SubfieldPaths)
	}
	if len(row.Examples) > 0 {
		_ = json.Unmarshal(row.Examples, &f.Examples)
	}
	return f
}

func rowToFieldList(row sqlcgen.WeaveListFieldsRow) *domain.Field {
	f := &domain.Field{
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
		ExpectedValueType: derefStr(row.ExpectedValueType),
	}
	if len(row.OntologyScope) > 0 {
		_ = json.Unmarshal(row.OntologyScope, &f.OntologyScope)
	}
	if len(row.PathElements) > 0 {
		_ = json.Unmarshal(row.PathElements, &f.PathElements)
	}
	if len(row.SubfieldPaths) > 0 {
		_ = json.Unmarshal(row.SubfieldPaths, &f.SubfieldPaths)
	}
	if len(row.Examples) > 0 {
		_ = json.Unmarshal(row.Examples, &f.Examples)
	}
	return f
}

type archivedFieldScanner interface {
	Scan(dest ...any) error
}

func scanArchivedField(row archivedFieldScanner) (*domain.Field, error) {
	var (
		id, projectID, status, version string
		createdAt, updatedAt           time.Time
		semanticID, systemName         *string
		uiName, description            []byte
		ontologyScope, pathElements    []byte
		ontologyPath, expectedValue    *string
		examples                       []byte
		stagingID                      *int64
		deprecated                     bool
	)
	if err := row.Scan(
		&id, &createdAt, &updatedAt, &semanticID, &systemName, &uiName, &description,
		&status, &projectID, &ontologyScope, &ontologyPath, &pathElements,
		&expectedValue, &examples, &stagingID, &deprecated, &version,
	); err != nil {
		return nil, err
	}
	f := &domain.Field{
		Entity: domain.Entity{
			ID:            id,
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
			SemanticID:    derefStr(semanticID),
			SystemName:    derefStr(systemName),
			UIName:        unmarshalTranslations(uiName),
			Description:   unmarshalTranslations(description),
			Status:        domain.Status(status),
			VersionNumber: version,
			ProjectID:     projectID,
			Deprecated:    deprecated,
		},
		ExpectedValueType: derefStr(expectedValue),
	}
	if len(ontologyScope) > 0 {
		_ = json.Unmarshal(ontologyScope, &f.OntologyScope)
	}
	if len(pathElements) > 0 {
		_ = json.Unmarshal(pathElements, &f.PathElements)
	}
	if len(examples) > 0 {
		_ = json.Unmarshal(examples, &f.Examples)
	}
	return f, nil
}

func rowsToFields(rows []sqlcgen.WeaveField) []*domain.Field {
	out := make([]*domain.Field, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToField(row))
	}
	return out
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
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

// ---------------------------------------------------------------------------
// Lifecycle + in-use gating
// ---------------------------------------------------------------------------

func (s *postgresStore) IsInUse(ctx context.Context, projectID, fieldID string) (bool, error) {
	in, err := s.queries.WeaveFieldIsInUse(ctx, sqlcgen.WeaveFieldIsInUseParams{
		FieldID:   fieldID,
		ProjectID: projectID,
	})
	if err != nil {
		return false, fmt.Errorf("field is-in-use: %w", err)
	}
	return in, nil
}

func (s *postgresStore) Usage(ctx context.Context, projectID, fieldID string) (UsageReport, error) {
	counts, err := s.queries.WeaveFieldUsageCounts(ctx, sqlcgen.WeaveFieldUsageCountsParams{
		FieldID:   fieldID,
		ProjectID: projectID,
	})
	if err != nil {
		return UsageReport{}, fmt.Errorf("field usage counts: %w", err)
	}
	report := UsageReport{
		ModelCount:      counts.ModelCount,
		CollectionCount: counts.CollectionCount,
	}
	if report.ModelCount > 0 {
		rows, err := s.queries.WeaveFieldUsageModels(ctx, sqlcgen.WeaveFieldUsageModelsParams{
			FieldID:   fieldID,
			ProjectID: projectID,
		})
		if err != nil {
			return UsageReport{}, fmt.Errorf("field usage models: %w", err)
		}
		samples := make([]domain.FieldModelRef, 0, len(rows))
		for _, r := range rows {
			samples = append(samples, domain.FieldModelRef{
				ModelID:   r.ModelID,
				ModelName: r.ModelName,
			})
		}
		report.ModelSamples = samples
	}
	if report.CollectionCount > 0 {
		rows, err := s.queries.WeaveFieldUsageCollections(ctx, sqlcgen.WeaveFieldUsageCollectionsParams{
			FieldID:   fieldID,
			ProjectID: projectID,
		})
		if err != nil {
			return UsageReport{}, fmt.Errorf("field usage collections: %w", err)
		}
		samples := make([]domain.FieldCollectionRef, 0, len(rows))
		for _, r := range rows {
			samples = append(samples, domain.FieldCollectionRef{
				CollectionID:   r.CollectionID,
				CollectionName: r.CollectionName,
			})
		}
		report.CollectionSamples = samples
	}
	return report, nil
}

func (s *postgresStore) Deprecate(ctx context.Context, fieldID string) error {
	if err := s.queries.WeaveDeprecateField(ctx, fieldID); err != nil {
		return fmt.Errorf("deprecate field: %w", err)
	}
	return nil
}

func (s *postgresStore) Activate(ctx context.Context, fieldID string) error {
	if err := s.queries.WeaveActivateField(ctx, fieldID); err != nil {
		return fmt.Errorf("activate field: %w", err)
	}
	return nil
}

// CountUsage returns model/collection adoption counts for a field in
// the named project. Version-aware via auth context: when a release
// version is in scope the query reads the archive table instead.
// Returns domain.FieldUsageCounts (numbers only, no samples).
func (s *postgresStore) CountUsage(ctx context.Context, fieldID, projectID string) (domain.FieldUsageCounts, error) {
	if version := weaveauth.ProjectVersionFromContext(ctx); version != "" {
		return s.countUsageVersion(ctx, fieldID, projectID, version)
	}
	const q = `
SELECT
    count(DISTINCT entity_id) FILTER (WHERE entity_type = 'model')::bigint        AS models_using,
    count(DISTINCT entity_id) FILTER (WHERE entity_type = 'collection')::bigint   AS collections_using,
    count(*)                  FILTER (WHERE entity_type IN ('model','collection'))::bigint AS override_count
FROM weave_field_overrides
WHERE field_id = $1 AND project_id = $2`
	var counts domain.FieldUsageCounts
	var modelsUsing, collectionsUsing, overrideCount int64
	if err := s.pool.QueryRow(ctx, q, fieldID, projectID).Scan(
		&modelsUsing, &collectionsUsing, &overrideCount,
	); err != nil {
		return domain.FieldUsageCounts{}, fmt.Errorf("count field usage: %w", err)
	}
	counts.ModelsUsing = int(modelsUsing)
	counts.CollectionsUsing = int(collectionsUsing)
	counts.OverrideRowCount = int(overrideCount)
	return counts, nil
}

func (s *postgresStore) countUsageVersion(ctx context.Context, fieldID, projectID, version string) (domain.FieldUsageCounts, error) {
	const q = `
SELECT
    count(DISTINCT entity_id) FILTER (WHERE entity_type = 'model')::bigint        AS models_using,
    count(DISTINCT entity_id) FILTER (WHERE entity_type = 'collection')::bigint   AS collections_using,
    count(*)                  FILTER (WHERE entity_type IN ('model','collection'))::bigint AS override_count
FROM weave_field_overrides_archive
WHERE field_id = $1 AND project_id = $2 AND version_number = $3`
	var counts domain.FieldUsageCounts
	var modelsUsing, collectionsUsing, overrideCount int64
	if err := s.pool.QueryRow(ctx, q, fieldID, projectID, version).Scan(
		&modelsUsing, &collectionsUsing, &overrideCount,
	); err != nil {
		return domain.FieldUsageCounts{}, fmt.Errorf("count archived field usage: %w", err)
	}
	counts.ModelsUsing = int(modelsUsing)
	counts.CollectionsUsing = int(collectionsUsing)
	counts.OverrideRowCount = int(overrideCount)
	return counts, nil
}

// ListUsage returns the models and collections in projectID that
// reference fieldID via weave_field_overrides. Drives the Reuse tab on
// the field detail view. Version-aware: when a release
// version is in scope the archive table is read.
func (s *postgresStore) ListUsage(ctx context.Context, fieldID, projectID string) (domain.FieldUsageList, error) {
	if version := weaveauth.ProjectVersionFromContext(ctx); version != "" {
		return s.listUsageVersion(ctx, fieldID, projectID, version)
	}
	// No project filter: cross-project usages are included so the Reuse tab can
	// show what references this field in OTHER projects (customer cleanup). Each
	// row carries its consuming project_id; the handler partitions this-project
	// vs other-projects. The JOIN LATERAL is global, so it resolves entities in
	// any project.
	const q = `
SELECT
    fo.entity_type                                           AS kind,
    fo.project_id                                            AS project_id,
    e.id                                                     AS id,
    COALESCE(e.system_name, '')                              AS system_name,
    COALESCE(e.ui_name, '{}'::jsonb)                         AS ui_name,
    COALESCE(e.description, '{}'::jsonb)                     AS description
FROM weave_field_overrides fo
JOIN LATERAL (
    SELECT id, system_name, ui_name, description
    FROM weave_models
    WHERE fo.entity_type = 'model' AND id = fo.entity_id
    UNION ALL
    SELECT id, system_name, ui_name, description
    FROM weave_collections
    WHERE fo.entity_type = 'collection' AND id = fo.entity_id
) e ON true
WHERE fo.field_id = $1
  AND fo.entity_type IN ('model', 'collection')
ORDER BY fo.project_id ASC, fo.entity_type ASC, COALESCE(e.ui_name ->> 'en', e.system_name, '') ASC`
	rows, err := s.pool.Query(ctx, q, fieldID)
	if err != nil {
		return domain.FieldUsageList{}, fmt.Errorf("list field usage: %w", err)
	}
	defer rows.Close()
	return scanFieldUsageRows(rows)
}

func (s *postgresStore) listUsageVersion(ctx context.Context, fieldID, projectID, version string) (domain.FieldUsageList, error) {
	const q = `
SELECT
    fo.entity_type                                           AS kind,
    fo.project_id                                            AS project_id,
    e.id                                                     AS id,
    COALESCE(e.system_name, '')                              AS system_name,
    COALESCE(e.ui_name, '{}'::jsonb)                         AS ui_name,
    COALESCE(e.description, '{}'::jsonb)                     AS description
FROM weave_field_overrides_archive fo
JOIN LATERAL (
    SELECT id, system_name, ui_name, description
    FROM weave_models_archive
    WHERE fo.entity_type = 'model' AND id = fo.entity_id AND version_number = fo.version_number
    UNION ALL
    SELECT id, system_name, ui_name, description
    FROM weave_collections_archive
    WHERE fo.entity_type = 'collection' AND id = fo.entity_id AND version_number = fo.version_number
) e ON true
WHERE fo.field_id = $1
  AND fo.version_number = $2
  AND fo.entity_type IN ('model', 'collection')
ORDER BY fo.project_id ASC, fo.entity_type ASC, COALESCE(e.ui_name ->> 'en', e.system_name, '') ASC`
	rows, err := s.pool.Query(ctx, q, fieldID, version)
	if err != nil {
		return domain.FieldUsageList{}, fmt.Errorf("list archived field usage: %w", err)
	}
	defer rows.Close()
	return scanFieldUsageRows(rows)
}

func scanFieldUsageRows(rows pgx.Rows) (domain.FieldUsageList, error) {
	var out domain.FieldUsageList
	// A field can be referenced by the same model/collection via several
	// override rows (one per category/collection placement). The Reuse tab
	// wants each consuming entity once — dedupe by (kind, project, id) so the
	// frontend {#each ...(ref.id)} keys stay unique.
	seen := map[string]bool{}
	for rows.Next() {
		var (
			kind, projectID, id, sysName string
			uiNameJSON, descJSON         []byte
		)
		if err := rows.Scan(&kind, &projectID, &id, &sysName, &uiNameJSON, &descJSON); err != nil {
			return domain.FieldUsageList{}, fmt.Errorf("scan field usage row: %w", err)
		}
		dedupeKey := kind + "|" + projectID + "|" + id
		if seen[dedupeKey] {
			continue
		}
		seen[dedupeKey] = true
		ref := domain.FieldUsageRef{
			ID:          id,
			SemanticID:  id, // id is the semantic identifier in this schema (e.g. "SRDF.248")
			SystemName:  sysName,
			Name:        unmarshalTranslations(uiNameJSON),
			Description: unmarshalTranslations(descJSON),
			ProjectID:   projectID,
			// Link into the CONSUMING project (cross-project rows link elsewhere).
			URL: "/projects/" + projectID + "/" + kind + "s/" + id,
		}
		switch kind {
		case "model":
			out.Models = append(out.Models, ref)
		case "collection":
			out.Collections = append(out.Collections, ref)
		}
	}
	if err := rows.Err(); err != nil {
		return domain.FieldUsageList{}, fmt.Errorf("iterate field usage rows: %w", err)
	}
	return out, nil
}

// BatchUsageCounts computes per-field list aggregates for a whole list page in
// one query. projectID is the owning project, used to split same-project from
// other-project usage. Fields with no usage are simply absent from the map
// (the caller treats a miss as all-zero / not-in-use).
func (s *postgresStore) BatchUsageCounts(ctx context.Context, projectID string, fieldIDs []string) (map[string]domain.FieldListCounts, error) {
	out := make(map[string]domain.FieldListCounts, len(fieldIDs))
	if len(fieldIDs) == 0 {
		return out, nil
	}
	const q = `
SELECT
    fo.field_id,
    count(*) FILTER (WHERE fo.project_id = $1 AND fo.entity_type = 'model')       AS model_count,
    count(*) FILTER (WHERE fo.project_id = $1 AND fo.entity_type = 'collection')   AS collection_count,
    count(DISTINCT fo.project_id) FILTER (WHERE fo.project_id <> $1)               AS other_project_count,
    count(*)                                                                       AS total
FROM weave_field_overrides fo
WHERE fo.field_id = ANY($2) AND fo.entity_type IN ('model', 'collection')
GROUP BY fo.field_id`
	rows, err := s.pool.Query(ctx, q, projectID, fieldIDs)
	if err != nil {
		return nil, fmt.Errorf("batch field usage counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			fieldID                         string
			modelC, collC, otherProj, total int
		)
		if err := rows.Scan(&fieldID, &modelC, &collC, &otherProj, &total); err != nil {
			return nil, fmt.Errorf("scan field usage counts: %w", err)
		}
		out[fieldID] = domain.FieldListCounts{
			ModelCount:        modelC,
			CollectionCount:   collC,
			OtherProjectCount: otherProj,
			InUse:             total > 0,
		}
	}
	return out, rows.Err()
}

// BatchUsageRefs returns, for each field ID, the models and collections that
// place it (weave_field_overrides entity_type 'model'/'collection'; base
// rows entity_type=” are not placements). Fields with no placements are
// absent from the map. Unlike ListUsage/CountUsage there is no
// version-pinned (archive-table) variant here — this is a live-rows-only
// read, deliberately, since the current project-scale consumers — including
// hosting-repo modules via the services-out seam (ADR-0008) — only ever
// want current state.
func (s *postgresStore) BatchUsageRefs(ctx context.Context, fieldIDs []string) (map[string]domain.FieldUsageList, error) {
	out := map[string]domain.FieldUsageList{}
	if len(fieldIDs) == 0 {
		return out, nil
	}
	const q = `
SELECT
    fo.field_id,
    fo.entity_type,
    e.id,
    COALESCE(e.system_name, '')
FROM weave_field_overrides fo
JOIN LATERAL (
    SELECT id, system_name FROM weave_models
    WHERE fo.entity_type = 'model' AND id = fo.entity_id
    UNION ALL
    SELECT id, system_name FROM weave_collections
    WHERE fo.entity_type = 'collection' AND id = fo.entity_id
) e ON true
WHERE fo.field_id = ANY($1) AND fo.entity_type IN ('model', 'collection')
ORDER BY fo.field_id, fo.entity_type, e.id`
	rows, err := s.pool.Query(ctx, q, fieldIDs)
	if err != nil {
		return nil, fmt.Errorf("batch field usage refs: %w", err)
	}
	defer rows.Close()

	// A field can be referenced by the same model/collection via several
	// override rows (one per category/collection placement) — dedupe by
	// (field, entity_type, id) same as scanFieldUsageRows does per-field.
	seen := map[string]bool{}
	for rows.Next() {
		var fieldID, entityType, id, sysName string
		if err := rows.Scan(&fieldID, &entityType, &id, &sysName); err != nil {
			return nil, fmt.Errorf("scan batch field usage ref row: %w", err)
		}
		dedupeKey := fieldID + "|" + entityType + "|" + id
		if seen[dedupeKey] {
			continue
		}
		seen[dedupeKey] = true
		ref := domain.FieldUsageRef{
			ID:         id,
			SemanticID: id, // id is the semantic identifier in this schema (no separate column)
			SystemName: sysName,
		}
		list := out[fieldID]
		switch entityType {
		case "model":
			list.Models = append(list.Models, ref)
		case "collection":
			list.Collections = append(list.Collections, ref)
		}
		out[fieldID] = list
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate batch field usage ref rows: %w", err)
	}
	return out, nil
}

// ListBaseFieldCategories returns the distinct categories assigned to
// base field overrides in this project. Drives the category filter
// dropdown on the field list.
func (s *postgresStore) ListBaseFieldCategories(ctx context.Context, projectID string) ([]FieldCategoryOption, error) {
	rows, err := s.queries.WeaveListBaseFieldCategoriesForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list base field categories: %w", err)
	}
	out := make([]FieldCategoryOption, 0, len(rows))
	for _, r := range rows {
		out = append(out, FieldCategoryOption{
			ID:             r.ID,
			UIName:         unmarshalTranslations(r.UiName),
			CanonicalOrder: int(r.CanonicalOrder),
		})
	}
	return out, nil
}

// ListBaseFieldCategoryAssignments returns field_id → category_id for
// every base override that has a category set.
func (s *postgresStore) ListBaseFieldCategoryAssignments(ctx context.Context, projectID string) (map[string]string, error) {
	rows, err := s.queries.WeaveListAllBaseFieldCategoryAssignments(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list base field category assignments: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		if r.CategoryID == nil {
			continue
		}
		out[r.FieldID] = *r.CategoryID
	}
	return out, nil
}
