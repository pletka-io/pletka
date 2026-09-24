package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

type postgresStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

// entityTypeModel is the weave_field_overrides.entity_type value for
// placements owned by a model. Named to match the same constant in
// pkg/service/gitmaterializer and pkg/weave/project (unexported in each,
// there being no shared entity-type package to import it from).
const entityTypeModel = "model"

var _ Store = (*postgresStore)(nil)

// NewPostgresStore returns a Store backed by the supplied pgx pool.
func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{queries: sqlcgen.New(pool), pool: pool}
}

func (s *postgresStore) Create(ctx context.Context, m *domain.Model) error {
	if m.Status == "" {
		m.Status = domain.StatusDraft
	}
	row, err := s.queries.WeaveCreateModel(ctx, sqlcgen.WeaveCreateModelParams{
		ID:            m.ID,
		SystemName:    dbutil.EmptyToNil(m.SystemName),
		UiName:        marshalTranslations(m.UIName),
		Description:   marshalTranslations(m.Description),
		Status:        string(m.Status),
		ProjectID:     m.ProjectID,
		OntologyScope: marshalJSON(m.OntologyScope),
		StagingID:     m.StagingID,
		ModelType:     m.ModelType,
	})
	if err != nil {
		return fmt.Errorf("create weave model: %w", err)
	}
	m.CreatedAt = row.CreatedAt
	m.UpdatedAt = row.UpdatedAt
	return nil
}

func (s *postgresStore) GetByID(ctx context.Context, id string) (*domain.Model, error) {
	row, err := s.queries.WeaveGetModelByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get weave model: %w", err)
	}
	return rowToModel(row), nil
}

// ListReferenceAdopted returns every model from another project that
// the current project's overrides reference as a value target — task 3b.
func (s *postgresStore) ListReferenceAdopted(ctx context.Context, projectID string) ([]*domain.Model, error) {
	rows, err := s.queries.WeaveListReferenceAdoptedModels(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list reference-adopted models: %w", err)
	}
	out := make([]*domain.Model, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToModel(r))
	}
	return out, nil
}

// ListConnectedModelIDs runs the recursive walker — Q8 of the
// adopt/adapt rollout. Empty seed returns an empty slice so callers can
// short-circuit on "no local models to start from".
func (s *postgresStore) ListConnectedModelIDs(ctx context.Context, seedIDs []string) ([]string, error) {
	if len(seedIDs) == 0 {
		return nil, nil
	}
	ids, err := s.queries.WeaveListConnectedModelIDs(ctx, seedIDs)
	if err != nil {
		return nil, fmt.Errorf("list connected model ids: %w", err)
	}
	return ids, nil
}

// ListReceiptModelDirect returns models the seed entity references
// directly (depth-1). Used by the Adoptions tab's per-receipt
// bill-of-materials alongside the transitive closure.
func (s *postgresStore) ListReceiptModelDirect(ctx context.Context, seedID, seedKind string) ([]string, error) {
	if seedID == "" {
		return nil, nil
	}
	ids, err := s.queries.WeaveListReceiptModelDirect(ctx, sqlcgen.WeaveListReceiptModelDirectParams{
		Column1: seedID,
		Column2: seedKind,
	})
	if err != nil {
		return nil, fmt.Errorf("list receipt model direct: %w", err)
	}
	return ids, nil
}

// ListReceiptModelClosure returns models in the transitive closure
// rooted at a single seed entity. Used by the Adoptions tab's
// per-receipt bill-of-materials.
func (s *postgresStore) ListReceiptModelClosure(ctx context.Context, seedID, seedKind string) ([]string, error) {
	if seedID == "" {
		return nil, nil
	}
	ids, err := s.queries.WeaveListReceiptModelClosure(ctx, sqlcgen.WeaveListReceiptModelClosureParams{
		Column1: seedID,
		Column2: seedKind,
	})
	if err != nil {
		return nil, fmt.Errorf("list receipt model closure: %w", err)
	}
	return ids, nil
}

func (s *postgresStore) GetByIDVersion(ctx context.Context, projectID, id, version string) (*domain.Model, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT
			id, created_at, updated_at, system_name, ui_name, description,
			status, project_id, ontology_scope, staging_id, deprecated, version_number, model_type
		FROM weave_models_archive
		WHERE id = $1 AND project_id = $2 AND version_number = $3
	`, id, projectID, version)
	model, err := scanArchivedModel(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archived weave model: %w", err)
	}
	return model, nil
}

func (s *postgresStore) GetByIdentifier(ctx context.Context, projectID, identifier string) (*domain.Model, error) {
	row, err := s.queries.WeaveGetModelByIdentifier(ctx, sqlcgen.WeaveGetModelByIdentifierParams{
		ProjectID:  projectID,
		Identifier: identifier,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get weave model by identifier: %w", err)
	}
	return rowToModel(row), nil
}

func (s *postgresStore) Update(ctx context.Context, m *domain.Model) error {
	if m.Status == "" {
		m.Status = domain.StatusDraft
	}
	row, err := s.queries.WeaveUpdateModel(ctx, sqlcgen.WeaveUpdateModelParams{
		ID:            m.ID,
		UiName:        marshalTranslations(m.UIName),
		Description:   marshalTranslations(m.Description),
		SystemName:    dbutil.EmptyToNil(m.SystemName),
		Status:        string(m.Status),
		OntologyScope: marshalJSON(m.OntologyScope),
		ModelType:     m.ModelType,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("update weave model %s: not found", m.ID)
		}
		return fmt.Errorf("update weave model: %w", err)
	}
	m.UpdatedAt = row.UpdatedAt
	return nil
}

// Delete removes the model and, in the same transaction, the placement
// rows the model owns (entity_type='model', entity_id=id) in
// weave_field_overrides. That table carries no FK on entity_id, so
// nothing cascades on a bare model delete — leaving those rows behind
// permanently inflates category usage counts. weave_override_refs cascade
// from the deleted override rows via their own FK.
func (s *postgresStore) Delete(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete model tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.queries.WithTx(tx)
	if err := q.WeaveDeleteOverridesForEntity(ctx, sqlcgen.WeaveDeleteOverridesForEntityParams{
		EntityType: entityTypeModel,
		EntityID:   id,
	}); err != nil {
		return fmt.Errorf("delete model placements: %w", err)
	}
	if err := q.WeaveDeleteModel(ctx, id); err != nil {
		return fmt.Errorf("delete weave model: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete model tx: %w", err)
	}
	return nil
}

func (s *postgresStore) List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Model, int64, error) {
	cfg := domain.ApplyOptions(opts)
	limit := int32(cfg.Limit)
	if limit <= 0 {
		limit = 100
	}
	offset := int32(cfg.Offset)
	if offset < 0 {
		offset = 0
	}
	modelType, _ := cfg.Filters["model_type"].(string)
	scopeClass, _ := cfg.Filters["scope_class"].(string)
	status, _ := cfg.Filters["status"].(string)
	rows, err := s.queries.WeaveListModels(ctx, sqlcgen.WeaveListModelsParams{
		ProjectID:    cfg.ProjectID,
		Search:       cfg.Search,
		ModelType:    modelType,
		ScopeClass:   scopeClass,
		Status:       status,
		SortBy:       cfg.OrderBy,
		SortDesc:     cfg.OrderDesc,
		ResultLimit:  limit,
		ResultOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list weave models: %w", err)
	}
	total, err := s.queries.WeaveCountModels(ctx, sqlcgen.WeaveCountModelsParams{
		ProjectID:  cfg.ProjectID,
		Search:     cfg.Search,
		ModelType:  modelType,
		ScopeClass: scopeClass,
		Status:     status,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count weave models: %w", err)
	}
	out := make([]*domain.Model, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToModelList(row))
	}
	return out, total, nil
}

// ScopeClasses returns distinct ontology scope classes
// (prefix:local_name) used by models in this project. Drives the
// scope_class filter dropdown.
func (s *postgresStore) ScopeClasses(ctx context.Context, projectID string) ([]string, error) {
	rows, err := s.queries.WeaveListModelScopeClasses(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list model scope classes: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r != "" {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *postgresStore) ListVersion(ctx context.Context, projectID, version string, opts ...domain.QueryOption) ([]*domain.Model, int64, error) {
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
			status, project_id, ontology_scope, staging_id, deprecated, version_number, model_type
		FROM weave_models_archive
		WHERE project_id = $1
		  AND version_number = $2
		  AND ($3 = ''
		       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || $3 || '%')
		       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || $3 || '%')
		       OR system_name ILIKE '%' || $3 || '%')
		ORDER BY
			CASE WHEN model_type = 'core' THEN 0 ELSE 1 END,
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
		return nil, 0, fmt.Errorf("list archived weave models: %w", err)
	}
	defer rows.Close()
	out := []*domain.Model{}
	for rows.Next() {
		model, err := scanArchivedModel(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan archived weave model: %w", err)
		}
		out = append(out, model)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate archived weave models: %w", err)
	}
	var total int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM weave_models_archive
		WHERE project_id = $1
		  AND version_number = $2
		  AND ($3 = ''
		       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || $3 || '%')
		       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || $3 || '%')
		       OR system_name ILIKE '%' || $3 || '%')
	`, projectID, version, cfg.Search).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count archived weave models: %w", err)
	}
	return out, total, nil
}

func (s *postgresStore) ListOptions(ctx context.Context, projectID string) ([]domain.EntityOption, error) {
	rows, err := s.queries.WeaveListModelOptions(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list model options: %w", err)
	}
	out := make([]domain.EntityOption, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.EntityOption{
			ID: row.ID,
			// id IS the semantic ID for models (migration 004) — no
			// separate column to read from.
			SemanticID: row.ID,
			SystemName: derefStr(row.SystemName),
			UIName:     unmarshalTranslations(row.UiName),
			Status:     row.Status,
		})
	}
	return out, nil
}

func (s *postgresStore) IsInUse(ctx context.Context, projectID, modelID string) (bool, error) {
	in, err := s.queries.WeaveModelIsInUse(ctx, sqlcgen.WeaveModelIsInUseParams{
		TargetID:  modelID,
		ProjectID: projectID,
	})
	if err != nil {
		return false, fmt.Errorf("model is-in-use: %w", err)
	}
	return in, nil
}

func (s *postgresStore) Usage(ctx context.Context, projectID, modelID string) (UsageReport, error) {
	count, err := s.queries.WeaveModelUsageCount(ctx, sqlcgen.WeaveModelUsageCountParams{
		TargetID:  modelID,
		ProjectID: projectID,
	})
	if err != nil {
		return UsageReport{}, fmt.Errorf("model usage count: %w", err)
	}
	report := UsageReport{FieldCount: count}
	if count > 0 {
		rows, err := s.queries.WeaveModelUsageSamples(ctx, sqlcgen.WeaveModelUsageSamplesParams{
			TargetID:  modelID,
			ProjectID: projectID,
		})
		if err != nil {
			return UsageReport{}, fmt.Errorf("model usage samples: %w", err)
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

func (s *postgresStore) UsageGlobal(ctx context.Context, modelID string) (UsageReport, error) {
	count, err := s.queries.WeaveModelUsageCountAllProjects(ctx, modelID)
	if err != nil {
		return UsageReport{}, fmt.Errorf("model usage count (all projects): %w", err)
	}
	report := UsageReport{FieldCount: count}
	if count == 0 {
		return report, nil
	}
	projects, err := s.queries.WeaveModelUsageProjectsAllProjects(ctx, modelID)
	if err != nil {
		return UsageReport{}, fmt.Errorf("model usage projects (all projects): %w", err)
	}
	report.ProjectIDs = projects
	rows, err := s.queries.WeaveModelUsageSamplesAllProjects(ctx, modelID)
	if err != nil {
		return UsageReport{}, fmt.Errorf("model usage samples (all projects): %w", err)
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
	return report, nil
}

// BatchCompositionCounts computes per-model composition aggregates for a whole
// list page in one query. entity_id identifies the model uniquely, so no
// project filter is needed. Models with no fields are absent (treated zero).
func (s *postgresStore) BatchCompositionCounts(ctx context.Context, modelIDs []string) (map[string]domain.CompositionCounts, error) {
	out := make(map[string]domain.CompositionCounts, len(modelIDs))
	if len(modelIDs) == 0 {
		return out, nil
	}
	const q = `
SELECT
    fo.entity_id,
    count(*)                                                                              AS field_count,
    count(DISTINCT fo.category_id) FILTER (WHERE fo.category_id <> '')                      AS category_count,
    count(DISTINCT fo.part_of_collection_id) FILTER (WHERE fo.part_of_collection_id <> '')  AS collection_count
FROM weave_field_overrides fo
WHERE fo.entity_type = 'model' AND fo.entity_id = ANY($1)
GROUP BY fo.entity_id`
	rows, err := s.pool.Query(ctx, q, modelIDs)
	if err != nil {
		return nil, fmt.Errorf("batch model composition counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id           string
			fc, cc, colc int
		)
		if err := rows.Scan(&id, &fc, &cc, &colc); err != nil {
			return nil, fmt.Errorf("scan model composition: %w", err)
		}
		out[id] = domain.CompositionCounts{FieldCount: fc, CategoryCount: cc, CollectionCount: colc}
	}
	return out, rows.Err()
}

// BatchInUse returns the subset of modelIDs that are referenced as a value
// target by a field in any project (weave_override_refs), keyed true.
func (s *postgresStore) BatchInUse(ctx context.Context, modelIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(modelIDs))
	if len(modelIDs) == 0 {
		return out, nil
	}
	const q = `SELECT DISTINCT target_id FROM weave_override_refs WHERE target_id = ANY($1)`
	rows, err := s.pool.Query(ctx, q, modelIDs)
	if err != nil {
		return nil, fmt.Errorf("batch model in-use: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan model in-use: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

// ListUsage returns the models/collections that contain a field targeting this
// model as a value type, across all projects. A field references a model via
// weave_override_refs (ref_type resource_model/collection_model); its container
// is the override's entity. Deduped by (kind, project, container) so the Reuse
// tab shows each container once. Version-aware: when a release version is in
// scope (auth.ProjectVersionFromContext) the archive tables are read instead
// of the live tables, mirroring field.ListUsage.
func (s *postgresStore) ListUsage(ctx context.Context, modelID string) (domain.FieldUsageList, error) {
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		return s.listUsageVersion(ctx, modelID, version)
	}
	const q = `
SELECT
    o.entity_type                        AS kind,
    o.project_id                         AS project_id,
    e.id                                 AS id,
    COALESCE(e.system_name, '')          AS system_name,
    COALESCE(e.ui_name, '{}'::jsonb)     AS ui_name,
    COALESCE(e.description, '{}'::jsonb) AS description
FROM weave_override_refs r
JOIN weave_field_overrides o ON o.id = r.override_id
JOIN LATERAL (
    SELECT id, system_name, ui_name, description
    FROM weave_models WHERE o.entity_type = 'model' AND id = o.entity_id
    UNION ALL
    SELECT id, system_name, ui_name, description
    FROM weave_collections WHERE o.entity_type = 'collection' AND id = o.entity_id
) e ON true
WHERE r.target_id = $1
  AND r.ref_type IN ('resource_model', 'collection_model')
  AND o.entity_type IN ('model', 'collection')
ORDER BY o.project_id ASC, o.entity_type ASC, COALESCE(e.ui_name ->> 'en', e.system_name, '') ASC`
	rows, err := s.pool.Query(ctx, q, modelID)
	if err != nil {
		return domain.FieldUsageList{}, fmt.Errorf("list model usage: %w", err)
	}
	defer rows.Close()
	return scanModelUsageRows(rows)
}

// listUsageVersion is the archive-table counterpart of ListUsage, read when a
// release version is pinned in the request context.
func (s *postgresStore) listUsageVersion(ctx context.Context, modelID, version string) (domain.FieldUsageList, error) {
	const q = `
SELECT
    o.entity_type                        AS kind,
    o.project_id                         AS project_id,
    e.id                                 AS id,
    COALESCE(e.system_name, '')          AS system_name,
    COALESCE(e.ui_name, '{}'::jsonb)     AS ui_name,
    COALESCE(e.description, '{}'::jsonb) AS description
FROM weave_override_refs_archive r
JOIN weave_field_overrides_archive o
  ON o.id = r.override_id AND o.version_number = r.version_number
JOIN LATERAL (
    SELECT id, system_name, ui_name, description
    FROM weave_models_archive
    WHERE o.entity_type = 'model' AND id = o.entity_id AND version_number = r.version_number
    UNION ALL
    SELECT id, system_name, ui_name, description
    FROM weave_collections_archive
    WHERE o.entity_type = 'collection' AND id = o.entity_id AND version_number = r.version_number
) e ON true
WHERE r.target_id = $1
  AND r.ref_type IN ('resource_model', 'collection_model')
  AND o.entity_type IN ('model', 'collection')
  AND r.version_number = $2
ORDER BY o.project_id ASC, o.entity_type ASC, COALESCE(e.ui_name ->> 'en', e.system_name, '') ASC`
	rows, err := s.pool.Query(ctx, q, modelID, version)
	if err != nil {
		return domain.FieldUsageList{}, fmt.Errorf("list archived model usage: %w", err)
	}
	defer rows.Close()
	return scanModelUsageRows(rows)
}

// scanModelUsageRows scans the shared (kind, project_id, id, system_name,
// ui_name, description) row shape produced by ListUsage/listUsageVersion,
// deduping by (kind, project, id) so the Reuse tab shows each container once.
func scanModelUsageRows(rows pgx.Rows) (domain.FieldUsageList, error) {
	var out domain.FieldUsageList
	seen := map[string]bool{}
	for rows.Next() {
		var (
			kind, projectID, id, sysName string
			uiNameJSON, descJSON         []byte
		)
		if err := rows.Scan(&kind, &projectID, &id, &sysName, &uiNameJSON, &descJSON); err != nil {
			return domain.FieldUsageList{}, fmt.Errorf("scan model usage row: %w", err)
		}
		key := kind + "|" + projectID + "|" + id
		if seen[key] {
			continue
		}
		seen[key] = true
		ref := domain.FieldUsageRef{
			ID:          id,
			SemanticID:  id,
			SystemName:  sysName,
			Name:        unmarshalTranslations(uiNameJSON),
			Description: unmarshalTranslations(descJSON),
			ProjectID:   projectID,
			URL:         "/projects/" + projectID + "/" + kind + "s/" + id,
		}
		switch kind {
		case "model":
			out.Models = append(out.Models, ref)
		case "collection":
			out.Collections = append(out.Collections, ref)
		}
	}
	return out, rows.Err()
}

func (s *postgresStore) UsageVersion(ctx context.Context, projectID, modelID, version string) (UsageReport, error) {
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
	`, modelID, projectID, version).Scan(&count); err != nil {
		return UsageReport{}, fmt.Errorf("archived model usage count: %w", err)
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
	`, modelID, projectID, version)
	if err != nil {
		return UsageReport{}, fmt.Errorf("archived model usage samples: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ref FieldRef
		var semanticID *string
		if err := rows.Scan(&ref.FieldID, &semanticID, &ref.FieldName); err != nil {
			return UsageReport{}, fmt.Errorf("scan archived model usage sample: %w", err)
		}
		ref.FieldSemanticID = derefStr(semanticID)
		report.FieldSamples = append(report.FieldSamples, ref)
	}
	if err := rows.Err(); err != nil {
		return UsageReport{}, fmt.Errorf("iterate archived model usage samples: %w", err)
	}
	return report, nil
}

func (s *postgresStore) Deprecate(ctx context.Context, modelID string) error {
	if err := s.queries.WeaveDeprecateModel(ctx, modelID); err != nil {
		return fmt.Errorf("deprecate model: %w", err)
	}
	return nil
}

func (s *postgresStore) Activate(ctx context.Context, modelID string) error {
	if err := s.queries.WeaveActivateModel(ctx, modelID); err != nil {
		return fmt.Errorf("activate model: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row converter + helpers
// ---------------------------------------------------------------------------

func rowToModel(row sqlcgen.WeaveModel) *domain.Model {
	m := &domain.Model{
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
		ModelType: row.ModelType,
		StagingID: row.StagingID,
	}
	if len(row.OntologyScope) > 0 {
		_ = json.Unmarshal(row.OntologyScope, &m.OntologyScope)
	}
	return m
}

func rowToModelList(row sqlcgen.WeaveListModelsRow) *domain.Model {
	m := &domain.Model{
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
		ModelType: row.ModelType,
		StagingID: row.StagingID,
	}
	if len(row.OntologyScope) > 0 {
		_ = json.Unmarshal(row.OntologyScope, &m.OntologyScope)
	}
	return m
}

type archivedModelScanner interface {
	Scan(dest ...any) error
}

func scanArchivedModel(row archivedModelScanner) (*domain.Model, error) {
	var (
		id, projectID, status, version string
		createdAt, updatedAt           time.Time
		systemName                     *string
		uiName, description            []byte
		ontologyScope                  []byte
		stagingID                      *int64
		deprecated                     bool
		modelType                      string
	)
	if err := row.Scan(
		&id, &createdAt, &updatedAt, &systemName, &uiName, &description,
		&status, &projectID, &ontologyScope, &stagingID, &deprecated, &version, &modelType,
	); err != nil {
		return nil, err
	}
	m := &domain.Model{
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
		ModelType: modelType,
		StagingID: stagingID,
	}
	if len(ontologyScope) > 0 {
		_ = json.Unmarshal(ontologyScope, &m.OntologyScope)
	}
	return m, nil
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
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
