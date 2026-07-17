package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
	"github.com/jackc/pgx/v5"
)

func (m *Materializer) loadArchivedCategories(ctx context.Context, projectID, version string) ([]*domain.Category, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT id, created_at, updated_at, semantic_id, system_name, ui_name, description, status, project_id, canonical_order, deprecated
		FROM weave_categories_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY canonical_order ASC, id ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived categories: %w", err)
	}
	defer rows.Close()

	out := make([]*domain.Category, 0)
	for rows.Next() {
		var cat domain.Category
		var semanticID, systemName *string
		var uiName, description []byte
		if err := rows.Scan(
			&cat.ID, &cat.CreatedAt, &cat.UpdatedAt, &semanticID, &systemName, &uiName, &description,
			&cat.Status, &cat.ProjectID, &cat.CanonicalOrder, &cat.Deprecated,
		); err != nil {
			return nil, fmt.Errorf("scan archived category: %w", err)
		}
		cat.SemanticID = derefStr(semanticID)
		cat.SystemName = derefStr(systemName)
		cat.UIName = unmarshalTranslations(uiName)
		cat.Description = unmarshalTranslations(description)
		out = append(out, &cat)
	}
	return out, rows.Err()
}

func (m *Materializer) loadArchivedFields(ctx context.Context, projectID, version string) ([]*domain.Field, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT id, created_at, updated_at, semantic_id, system_name, ui_name, description, status, project_id,
		       ontology_scope, path_elements, expected_value_type, examples, deprecated
		FROM weave_fields_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY id ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived fields: %w", err)
	}
	defer rows.Close()

	out := make([]*domain.Field, 0)
	for rows.Next() {
		var field domain.Field
		var semanticID, systemName, expectedValueType *string
		var uiName, description, ontologyScope, pathElements, examples []byte
		if err := rows.Scan(
			&field.ID, &field.CreatedAt, &field.UpdatedAt, &semanticID, &systemName, &uiName, &description, &field.Status, &field.ProjectID,
			&ontologyScope, &pathElements, &expectedValueType, &examples, &field.Deprecated,
		); err != nil {
			return nil, fmt.Errorf("scan archived field: %w", err)
		}
		field.SemanticID = derefStr(semanticID)
		field.SystemName = derefStr(systemName)
		field.UIName = unmarshalTranslations(uiName)
		field.Description = unmarshalTranslations(description)
		field.OntologyScope = unmarshalPathElement(ontologyScope)
		field.PathElements = unmarshalPathElements(pathElements)
		field.ExpectedValueType = derefStr(expectedValueType)
		field.Examples = parseExamples(examples)
		out = append(out, &field)
	}
	return out, rows.Err()
}

func (m *Materializer) loadArchivedModels(ctx context.Context, projectID, version string) ([]*domain.Model, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT id, created_at, updated_at, system_name, ui_name, description, status, project_id, ontology_scope, staging_id, deprecated
		FROM weave_models_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY id ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived models: %w", err)
	}
	defer rows.Close()

	out := make([]*domain.Model, 0)
	for rows.Next() {
		var model domain.Model
		var systemName *string
		var uiName, description, ontologyScope []byte
		if err := rows.Scan(
			&model.ID, &model.CreatedAt, &model.UpdatedAt, &systemName, &uiName, &description, &model.Status, &model.ProjectID, &ontologyScope, &model.StagingID, &model.Deprecated,
		); err != nil {
			return nil, fmt.Errorf("scan archived model: %w", err)
		}
		model.SemanticID = model.ID
		model.SystemName = derefStr(systemName)
		model.UIName = unmarshalTranslations(uiName)
		model.Description = unmarshalTranslations(description)
		model.OntologyScope = unmarshalPathElement(ontologyScope)
		out = append(out, &model)
	}
	return out, rows.Err()
}

func (m *Materializer) loadArchivedCollections(ctx context.Context, projectID, version string) ([]*domain.Collection, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT id, created_at, updated_at, system_name, ui_name, description, status, project_id, ontology_scope,
		       collection_number, canonical_collection_order, staging_id, deprecated, default_category_id
		FROM weave_collections_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY canonical_collection_order ASC, id ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived collections: %w", err)
	}
	defer rows.Close()

	out := make([]*domain.Collection, 0)
	for rows.Next() {
		var col domain.Collection
		var systemName *string
		var uiName, description, ontologyScope []byte
		var collectionNumber, canonicalOrder *int32
		if err := rows.Scan(
			&col.ID, &col.CreatedAt, &col.UpdatedAt, &systemName, &uiName, &description, &col.Status, &col.ProjectID, &ontologyScope,
			&collectionNumber, &canonicalOrder, &col.StagingID, &col.Deprecated, &col.DefaultCategoryID,
		); err != nil {
			return nil, fmt.Errorf("scan archived collection: %w", err)
		}
		col.SemanticID = col.ID
		col.SystemName = derefStr(systemName)
		col.UIName = unmarshalTranslations(uiName)
		col.Description = unmarshalTranslations(description)
		col.OntologyScope = unmarshalPathElement(ontologyScope)
		col.CollectionNumber = derefInt32(collectionNumber)
		col.CanonicalCollectionOrder = derefInt32(canonicalOrder)
		out = append(out, &col)
	}
	return out, rows.Err()
}

func (m *Materializer) loadArchivedBaseOverride(ctx context.Context, fieldID, projectID, version string) (*domain.FieldOverride, bool, error) {
	row := m.pool.QueryRow(ctx, `
		SELECT id, field_id, project_id, entity_type, entity_id, position, collection_order, display_name, description,
		       collection_name, category_id, part_of_collection_id, set_value, is_required, min_occurs, max_occurs,
		       is_hidden, visibility, staging_id, created_at, updated_at
		FROM weave_field_overrides_archive
		WHERE field_id = $1 AND project_id = $2 AND entity_type = '' AND version_number = $3
	`, fieldID, projectID, version)
	override, err := scanArchivedOverride(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return override, true, nil
}

func (m *Materializer) loadArchivedOverridesByProjectAndType(ctx context.Context, projectID, version, entityType string) ([]*domain.FieldOverride, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT id, field_id, project_id, entity_type, entity_id, position, collection_order, display_name, description,
		       collection_name, category_id, part_of_collection_id, set_value, is_required, min_occurs, max_occurs,
		       is_hidden, visibility, staging_id, created_at, updated_at
		FROM weave_field_overrides_archive
		WHERE project_id = $1 AND entity_type = $2 AND version_number = $3
		ORDER BY entity_id ASC, position ASC, id ASC
	`, projectID, entityType, version)
	if err != nil {
		return nil, fmt.Errorf("list archived overrides: %w", err)
	}
	defer rows.Close()

	out := make([]*domain.FieldOverride, 0)
	for rows.Next() {
		o, err := scanArchivedOverride(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (m *Materializer) loadArchivedOverrideRefs(ctx context.Context, overrideID int64, version string) ([]domain.OverrideRef, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT override_id, ref_type, target_id, semantic_id, position
		FROM weave_override_refs_archive
		WHERE override_id = $1 AND version_number = $2
		ORDER BY ref_type ASC, position ASC
	`, overrideID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived override refs for %d: %w", overrideID, err)
	}
	defer rows.Close()

	out := make([]domain.OverrideRef, 0)
	for rows.Next() {
		var ref domain.OverrideRef
		if err := rows.Scan(&ref.OverrideID, &ref.RefType, &ref.TargetID, &ref.SemanticID, &ref.Position); err != nil {
			return nil, fmt.Errorf("scan archived override ref: %w", err)
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

func scanArchivedOverride(scanner interface{ Scan(...any) error }) (*domain.FieldOverride, error) {
	var o domain.FieldOverride
	var displayName, description, collectionName []byte
	var categoryID, partOfCollectionID, setValue, visibility *string
	var isRequired, isHidden *bool
	var minOccurs, maxOccurs *int32
	if err := scanner.Scan(
		&o.ID, &o.FieldID, &o.ProjectID, &o.EntityType, &o.EntityID, &o.Position, &o.CollectionOrder, &displayName, &description,
		&collectionName, &categoryID, &partOfCollectionID, &setValue, &isRequired, &minOccurs, &maxOccurs,
		&isHidden, &visibility, &o.StagingID, &o.CreatedAt, &o.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan archived override: %w", err)
	}
	o.DisplayName = unmarshalTranslations(displayName)
	o.Description = unmarshalTranslations(description)
	o.CollectionName = unmarshalTranslations(collectionName)
	o.CategoryID = derefStr(categoryID)
	o.PartOfCollectionID = derefStr(partOfCollectionID)
	o.SetValue = derefStr(setValue)
	o.IsRequired = derefBool(isRequired)
	o.MinOccurs = derefInt32(minOccurs)
	if maxOccurs != nil {
		v := int(*maxOccurs)
		o.MaxOccurs = &v
	}
	o.IsHidden = derefBool(isHidden)
	o.Visibility = derefStr(visibility)
	return &o, nil
}

func (m *Materializer) loadProjectAdoptionsAtVersion(ctx context.Context, projectID, version string) ([]domain.Adoption, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT project_id, context_entity_type, context_entity_id, entity_type, source_project_id, source_entity_id,
		       source_version, adopted_at, created_by_id
		FROM weave_adoptions_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY entity_type ASC, source_entity_id ASC, context_entity_type ASC, context_entity_id ASC, adopted_at ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived adoptions for manifest: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Adoption, 0)
	for rows.Next() {
		var row domain.Adoption
		if err := rows.Scan(&row.ProjectID, &row.ContextEntityType, &row.ContextEntityID, &row.EntityType, &row.SourceProjectID, &row.SourceEntityID, &row.SourceVersion, &row.AdoptedAt, &row.CreatedByID); err != nil {
			return nil, fmt.Errorf("scan archived adoption: %w", err)
		}
		row.SourceVersion = strings.TrimSpace(row.SourceVersion)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (m *Materializer) loadProjectForksAtVersion(ctx context.Context, projectID, version string) ([]domain.EntityFork, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT project_id, entity_type, fork_entity_id, source_project_id, source_entity_id, source_version, forked_at, created_by_id
		FROM weave_entity_forks_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY entity_type ASC, fork_entity_id ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived forks for manifest: %w", err)
	}
	defer rows.Close()

	out := make([]domain.EntityFork, 0)
	for rows.Next() {
		var row domain.EntityFork
		if err := rows.Scan(&row.ProjectID, &row.EntityType, &row.ForkEntityID, &row.SourceProjectID, &row.SourceEntityID, &row.SourceVersion, &row.ForkedAt, &row.CreatedByID); err != nil {
			return nil, fmt.Errorf("scan archived fork: %w", err)
		}
		row.SourceVersion = strings.TrimSpace(row.SourceVersion)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (m *Materializer) loadAdoptedFieldAtSource(ctx context.Context, sourceProjectID, fieldID, sourceVersion string) (*domain.Field, error) {
	if strings.TrimSpace(sourceVersion) == "" {
		row, err := m.queries.WeaveGetFieldByID(ctx, fieldID)
		if err != nil {
			return nil, fmt.Errorf("get adopted field %s: %w", fieldID, err)
		}
		field := rowToField(row)
		if field.ProjectID == "" {
			field.ProjectID = sourceProjectID
		}
		return field, nil
	}
	row := m.pool.QueryRow(ctx, `
		SELECT id, created_at, updated_at, semantic_id, system_name, ui_name, description, status, project_id,
		       ontology_scope, path_elements, expected_value_type, examples, deprecated
		FROM weave_fields_archive
		WHERE id = $1 AND project_id = $2 AND version_number = $3
	`, fieldID, sourceProjectID, sourceVersion)
	var field domain.Field
	var semanticID, systemName, expectedValueType *string
	var uiName, description, ontologyScope, pathElements, examples []byte
	if err := row.Scan(&field.ID, &field.CreatedAt, &field.UpdatedAt, &semanticID, &systemName, &uiName, &description, &field.Status, &field.ProjectID, &ontologyScope, &pathElements, &expectedValueType, &examples, &field.Deprecated); err != nil {
		return nil, fmt.Errorf("get adopted archived field %s@%s: %w", fieldID, sourceVersion, err)
	}
	field.SemanticID = derefStr(semanticID)
	field.SystemName = derefStr(systemName)
	field.UIName = unmarshalTranslations(uiName)
	field.Description = unmarshalTranslations(description)
	field.OntologyScope = unmarshalPathElement(ontologyScope)
	field.PathElements = unmarshalPathElements(pathElements)
	field.ExpectedValueType = derefStr(expectedValueType)
	field.Examples = parseExamples(examples)
	return &field, nil
}

func (m *Materializer) loadAdoptedModelAtSource(ctx context.Context, sourceProjectID, modelID, sourceVersion string) (*domain.Model, error) {
	if strings.TrimSpace(sourceVersion) == "" {
		row, err := m.queries.WeaveGetModelByID(ctx, modelID)
		if err != nil {
			return nil, fmt.Errorf("get adopted model %s: %w", modelID, err)
		}
		return rowToModel(row), nil
	}
	row := m.pool.QueryRow(ctx, `
		SELECT id, created_at, updated_at, system_name, ui_name, description, status, project_id, ontology_scope, staging_id, deprecated
		FROM weave_models_archive
		WHERE id = $1 AND project_id = $2 AND version_number = $3
	`, modelID, sourceProjectID, sourceVersion)
	var model domain.Model
	var systemName *string
	var uiName, description, ontologyScope []byte
	if err := row.Scan(&model.ID, &model.CreatedAt, &model.UpdatedAt, &systemName, &uiName, &description, &model.Status, &model.ProjectID, &ontologyScope, &model.StagingID, &model.Deprecated); err != nil {
		return nil, fmt.Errorf("get adopted archived model %s@%s: %w", modelID, sourceVersion, err)
	}
	model.SemanticID = model.ID
	model.SystemName = derefStr(systemName)
	model.UIName = unmarshalTranslations(uiName)
	model.Description = unmarshalTranslations(description)
	model.OntologyScope = unmarshalPathElement(ontologyScope)
	return &model, nil
}

func (m *Materializer) loadAdoptedCollectionAtSource(ctx context.Context, sourceProjectID, collectionID, sourceVersion string) (*domain.Collection, error) {
	if strings.TrimSpace(sourceVersion) == "" {
		row, err := m.queries.WeaveGetCollectionByID(ctx, collectionID)
		if err != nil {
			return nil, fmt.Errorf("get adopted collection %s: %w", collectionID, err)
		}
		return rowToCollection(row), nil
	}
	row := m.pool.QueryRow(ctx, `
		SELECT id, created_at, updated_at, system_name, ui_name, description, status, project_id, ontology_scope,
		       collection_number, canonical_collection_order, staging_id, deprecated, default_category_id
		FROM weave_collections_archive
		WHERE id = $1 AND project_id = $2 AND version_number = $3
	`, collectionID, sourceProjectID, sourceVersion)
	var col domain.Collection
	var systemName *string
	var uiName, description, ontologyScope []byte
	var collectionNumber, canonicalOrder *int32
	if err := row.Scan(&col.ID, &col.CreatedAt, &col.UpdatedAt, &systemName, &uiName, &description, &col.Status, &col.ProjectID, &ontologyScope, &collectionNumber, &canonicalOrder, &col.StagingID, &col.Deprecated, &col.DefaultCategoryID); err != nil {
		return nil, fmt.Errorf("get adopted archived collection %s@%s: %w", collectionID, sourceVersion, err)
	}
	col.SemanticID = col.ID
	col.SystemName = derefStr(systemName)
	col.UIName = unmarshalTranslations(uiName)
	col.Description = unmarshalTranslations(description)
	col.OntologyScope = unmarshalPathElement(ontologyScope)
	col.CollectionNumber = derefInt32(collectionNumber)
	col.CanonicalCollectionOrder = derefInt32(canonicalOrder)
	return &col, nil
}

func (m *Materializer) writeAdoptedBaseOverrideAtSource(ctx context.Context, workDir, fieldID, ownerProjectID, sourceVersion string) error {
	var override *domain.FieldOverride
	var ok bool
	var err error
	if strings.TrimSpace(sourceVersion) == "" {
		row, err := m.queries.WeaveGetBaseOverride(ctx, sqlcgen.WeaveGetBaseOverrideParams{
			FieldID:   fieldID,
			ProjectID: ownerProjectID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("get adopted base override for %s: %w", fieldID, err)
		}
		override = rowToOverride(row)
		ok = true
	} else {
		override, ok, err = m.loadArchivedBaseOverride(ctx, fieldID, ownerProjectID, sourceVersion)
		if err != nil {
			return err
		}
	}
	if !ok || override == nil {
		return nil
	}

	var refs []domain.OverrideRef
	if strings.TrimSpace(sourceVersion) == "" {
		refRows, err := m.queries.WeaveListOverrideRefs(ctx, override.ID)
		if err != nil {
			return fmt.Errorf("list adopted base override refs for %d: %w", override.ID, err)
		}
		refs = make([]domain.OverrideRef, 0, len(refRows))
		for _, rr := range refRows {
			refs = append(refs, rowToOverrideRef(rr))
		}
	} else {
		refs, err = m.loadArchivedOverrideRefs(ctx, override.ID, sourceVersion)
		if err != nil {
			return err
		}
	}

	payload, err := canonical.Override(override, refs)
	if err != nil {
		return fmt.Errorf("encode adopted base override %d: %w", override.ID, err)
	}
	annotated, err := addAdoptionMarker(payload, ownerProjectID)
	if err != nil {
		return err
	}
	path := domain.FilePath(domain.PathSpec{EntityType: "base_override", EntityID: fmt.Sprintf("%d", override.ID), FieldID: fieldID})
	return writeEntityFile(workDir, path, annotated)
}
