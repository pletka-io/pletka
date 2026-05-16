package weave

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domainweave "github.com/pletka-io/pletka/domain/weave"
	"github.com/pletka-io/pletka/store/dbutil"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

type fieldOverrideStore struct {
	store *Store
}

var _ domainweave.FieldOverrideStore = (*fieldOverrideStore)(nil)

// FieldOverrides returns the store slice for model/collection field composition.
func (s *Store) FieldOverrides() domainweave.FieldOverrideStore {
	return &fieldOverrideStore{store: s}
}

func (s *fieldOverrideStore) ListForModel(ctx context.Context, projectID, modelID string) ([]*domainweave.ResolvedFieldOverride, error) {
	return s.listResolved(ctx, fieldOverrideResolvedSelect("weave_field_overrides", "weave_fields", "fo.set_value_entry_id")+`
		WHERE fo.entity_type = 'model'
		  AND fo.entity_id = $1
		  AND fo.project_id = $2
		ORDER BY fo.category_id, fo.part_of_collection_id, fo.position
	`, modelID, projectID)
}

func (s *fieldOverrideStore) ListForModelVersion(ctx context.Context, projectID, modelID, version string) ([]*domainweave.ResolvedFieldOverride, error) {
	return s.listResolved(ctx, fieldOverrideResolvedSelect("weave_field_overrides_archive", "weave_fields_archive", "NULL::text")+`
		WHERE fo.entity_type = 'model'
		  AND fo.entity_id = $1
		  AND fo.project_id = $2
		  AND fo.version_number = $3
		  AND f.version_number = fo.version_number
		ORDER BY fo.category_id, fo.part_of_collection_id, fo.position
	`, modelID, projectID, version)
}

func (s *fieldOverrideStore) ListForCollection(ctx context.Context, projectID, collectionID string) ([]*domainweave.ResolvedFieldOverride, error) {
	return s.listResolved(ctx, fieldOverrideResolvedSelect("weave_field_overrides", "weave_fields", "fo.set_value_entry_id")+`
		WHERE fo.entity_type = 'collection'
		  AND fo.entity_id = $1
		  AND fo.project_id = $2
		ORDER BY fo.category_id, fo.position
	`, collectionID, projectID)
}

func (s *fieldOverrideStore) ListForCollectionVersion(ctx context.Context, projectID, collectionID, version string) ([]*domainweave.ResolvedFieldOverride, error) {
	return s.listResolved(ctx, fieldOverrideResolvedSelect("weave_field_overrides_archive", "weave_fields_archive", "NULL::text")+`
		WHERE fo.entity_type = 'collection'
		  AND fo.entity_id = $1
		  AND fo.project_id = $2
		  AND fo.version_number = $3
		  AND f.version_number = fo.version_number
		ORDER BY fo.category_id, fo.position
	`, collectionID, projectID, version)
}

func (s *fieldOverrideStore) ListBaseForFields(ctx context.Context, projectID string, fieldIDs []string) ([]*domainweave.FieldOverride, error) {
	if len(fieldIDs) == 0 {
		return []*domainweave.FieldOverride{}, nil
	}
	return s.listOverrides(ctx, fieldOverrideSelect("weave_field_overrides", "set_value_entry_id")+`
		WHERE entity_type = ''
		  AND project_id = $1
		  AND field_id = ANY($2::text[])
		ORDER BY field_id, position
	`, projectID, fieldIDs)
}

func (s *fieldOverrideStore) ListBaseForFieldsVersion(ctx context.Context, projectID, version string, fieldIDs []string) ([]*domainweave.FieldOverride, error) {
	if len(fieldIDs) == 0 {
		return []*domainweave.FieldOverride{}, nil
	}
	return s.listOverrides(ctx, fieldOverrideSelect("weave_field_overrides_archive", "NULL::text")+`
		WHERE entity_type = ''
		  AND project_id = $1
		  AND version_number = $2
		  AND field_id = ANY($3::text[])
		ORDER BY field_id, position
	`, projectID, version, fieldIDs)
}

func (s *fieldOverrideStore) ListRefsForOverrides(ctx context.Context, overrideIDs []int64) (map[int64][]domainweave.OverrideRef, error) {
	if len(overrideIDs) == 0 {
		return map[int64][]domainweave.OverrideRef{}, nil
	}
	return s.listRefs(ctx, `
		SELECT override_id, ref_type, target_id, semantic_id, position, ''::text AS project_id, ''::text AS version_number
		FROM weave_override_refs
		WHERE override_id = ANY($1::bigint[])
		ORDER BY override_id, ref_type, position
	`, overrideIDs)
}

func (s *fieldOverrideStore) ListRefsForOverridesVersion(ctx context.Context, projectID, version string, overrideIDs []int64) (map[int64][]domainweave.OverrideRef, error) {
	if len(overrideIDs) == 0 {
		return map[int64][]domainweave.OverrideRef{}, nil
	}
	return s.listRefs(ctx, `
		SELECT override_id, ref_type, target_id, semantic_id, position, project_id, version_number
		FROM weave_override_refs_archive
		WHERE project_id = $1
		  AND version_number = $2
		  AND override_id = ANY($3::bigint[])
		ORDER BY override_id, ref_type, position
	`, projectID, version, overrideIDs)
}

func (s *fieldOverrideStore) listResolved(ctx context.Context, query string, args ...any) ([]*domainweave.ResolvedFieldOverride, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list resolved field overrides: %w", err)
	}
	defer rows.Close()

	overrides := []*domainweave.ResolvedFieldOverride{}
	for rows.Next() {
		override, err := scanResolvedFieldOverride(rows)
		if err != nil {
			return nil, fmt.Errorf("scan resolved field override: %w", err)
		}
		overrides = append(overrides, override)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate resolved field overrides: %w", err)
	}
	return overrides, nil
}

func (s *fieldOverrideStore) listOverrides(ctx context.Context, query string, args ...any) ([]*domainweave.FieldOverride, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list field overrides: %w", err)
	}
	defer rows.Close()

	overrides := []*domainweave.FieldOverride{}
	for rows.Next() {
		override, err := scanFieldOverride(rows)
		if err != nil {
			return nil, fmt.Errorf("scan field override: %w", err)
		}
		overrides = append(overrides, override)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate field overrides: %w", err)
	}
	return overrides, nil
}

func (s *fieldOverrideStore) listRefs(ctx context.Context, query string, args ...any) (map[int64][]domainweave.OverrideRef, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list override refs: %w", err)
	}
	defer rows.Close()

	refs := []domainweave.OverrideRef{}
	for rows.Next() {
		ref, err := scanOverrideRef(rows)
		if err != nil {
			return nil, fmt.Errorf("scan override ref: %w", err)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate override refs: %w", err)
	}
	return groupOverrideRefsByOverrideID(refs), nil
}

func (s *fieldOverrideStore) connectionPool() (pgxPool, error) {
	if s == nil || s.store == nil || s.store.pool == nil {
		return nil, errors.New("field override store is not initialized")
	}
	return s.store.pool, nil
}

func fieldOverrideFromRow(row sqlcgen.WeaveFieldOverride) *domainweave.FieldOverride {
	return fieldOverrideFromValues(fieldOverrideValues{
		ID:                 row.ID,
		FieldID:            row.FieldID,
		ProjectID:          row.ProjectID,
		EntityType:         row.EntityType,
		EntityID:           row.EntityID,
		Position:           row.Position,
		CollectionOrder:    row.CollectionOrder,
		DisplayName:        row.DisplayName,
		Description:        row.Description,
		CollectionName:     row.CollectionName,
		CategoryID:         row.CategoryID,
		PartOfCollectionID: row.PartOfCollectionID,
		ExpectedValueType:  row.ExpectedValueType,
		SetValue:           row.SetValue,
		IsRequired:         row.IsRequired,
		MinOccurs:          row.MinOccurs,
		MaxOccurs:          row.MaxOccurs,
		IsHidden:           row.IsHidden,
		Visibility:         row.Visibility,
		StagingID:          row.StagingID,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
		ContentHash:        row.ContentHash,
		VersionNumber:      row.VersionNumber,
		SetValueEntryID:    row.SetValueEntryID,
	})
}

type fieldOverrideValues struct {
	ID                 int64
	FieldID            string
	ProjectID          string
	EntityType         string
	EntityID           string
	Position           int32
	CollectionOrder    int32
	DisplayName        []byte
	Description        []byte
	CollectionName     []byte
	CategoryID         *string
	PartOfCollectionID *string
	ExpectedValueType  *string
	SetValue           *string
	IsRequired         *bool
	MinOccurs          *int32
	MaxOccurs          *int32
	IsHidden           *bool
	Visibility         *string
	StagingID          *int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ContentHash        *string
	VersionNumber      string
	SetValueEntryID    *string
}

type resolvedFieldValues struct {
	Override               fieldOverrideValues
	FieldID                string
	FieldSemanticID        *string
	FieldSystemName        *string
	FieldOntologyScope     []byte
	FieldOntologyPath      *string
	FieldExpectedValueType *string
	FieldPathElements      []byte
}

func scanFieldOverride(row interface{ Scan(...any) error }) (*domainweave.FieldOverride, error) {
	var values fieldOverrideValues
	if err := row.Scan(
		&values.ID,
		&values.FieldID,
		&values.ProjectID,
		&values.EntityType,
		&values.EntityID,
		&values.Position,
		&values.CollectionOrder,
		&values.DisplayName,
		&values.Description,
		&values.CollectionName,
		&values.CategoryID,
		&values.PartOfCollectionID,
		&values.ExpectedValueType,
		&values.SetValue,
		&values.IsRequired,
		&values.MinOccurs,
		&values.MaxOccurs,
		&values.IsHidden,
		&values.Visibility,
		&values.StagingID,
		&values.CreatedAt,
		&values.UpdatedAt,
		&values.ContentHash,
		&values.VersionNumber,
		&values.SetValueEntryID,
	); err != nil {
		return nil, err
	}
	return fieldOverrideFromValues(values), nil
}

func scanResolvedFieldOverride(row interface{ Scan(...any) error }) (*domainweave.ResolvedFieldOverride, error) {
	var values resolvedFieldValues
	if err := row.Scan(
		&values.Override.ID,
		&values.Override.FieldID,
		&values.Override.ProjectID,
		&values.Override.EntityType,
		&values.Override.EntityID,
		&values.Override.Position,
		&values.Override.CollectionOrder,
		&values.Override.DisplayName,
		&values.Override.Description,
		&values.Override.CollectionName,
		&values.Override.CategoryID,
		&values.Override.PartOfCollectionID,
		&values.Override.ExpectedValueType,
		&values.Override.SetValue,
		&values.Override.IsRequired,
		&values.Override.MinOccurs,
		&values.Override.MaxOccurs,
		&values.Override.IsHidden,
		&values.Override.Visibility,
		&values.Override.StagingID,
		&values.Override.CreatedAt,
		&values.Override.UpdatedAt,
		&values.Override.ContentHash,
		&values.Override.VersionNumber,
		&values.Override.SetValueEntryID,
		&values.FieldSemanticID,
		&values.FieldSystemName,
		&values.FieldOntologyScope,
		&values.FieldOntologyPath,
		&values.FieldExpectedValueType,
		&values.FieldPathElements,
	); err != nil {
		return nil, err
	}
	values.FieldID = values.Override.FieldID
	return resolvedFieldOverrideFromValues(values), nil
}

func fieldOverrideFromValues(values fieldOverrideValues) *domainweave.FieldOverride {
	return &domainweave.FieldOverride{
		ID:                 values.ID,
		FieldID:            values.FieldID,
		ProjectID:          values.ProjectID,
		EntityType:         values.EntityType,
		EntityID:           values.EntityID,
		Position:           int(values.Position),
		CollectionOrder:    int(values.CollectionOrder),
		DisplayName:        translationsFromJSON(values.DisplayName),
		Description:        translationsFromJSON(values.Description),
		CollectionName:     translationsFromJSON(values.CollectionName),
		CategoryID:         values.CategoryID,
		PartOfCollectionID: values.PartOfCollectionID,
		ExpectedValueType:  values.ExpectedValueType,
		SetValue:           values.SetValue,
		IsRequired:         values.IsRequired,
		MinOccurs:          intPtrFromInt32Ptr(values.MinOccurs),
		MaxOccurs:          intPtrFromInt32Ptr(values.MaxOccurs),
		IsHidden:           values.IsHidden,
		Visibility:         values.Visibility,
		StagingID:          values.StagingID,
		CreatedAt:          values.CreatedAt,
		UpdatedAt:          values.UpdatedAt,
		ContentHash:        values.ContentHash,
		VersionNumber:      values.VersionNumber,
		SetValueEntryID:    values.SetValueEntryID,
	}
}

func resolvedFieldOverrideFromValues(values resolvedFieldValues) *domainweave.ResolvedFieldOverride {
	resolved := &domainweave.ResolvedFieldOverride{
		Override: *fieldOverrideFromValues(values.Override),
		Field: domainweave.ResolvedFieldSnapshot{
			ID:                values.FieldID,
			SemanticID:        dbutil.NilToEmpty(values.FieldSemanticID),
			SystemName:        dbutil.NilToEmpty(values.FieldSystemName),
			OntologyPath:      dbutil.NilToEmpty(values.FieldOntologyPath),
			ExpectedValueType: dbutil.NilToEmpty(values.FieldExpectedValueType),
		},
	}
	if len(values.FieldOntologyScope) > 0 {
		_ = json.Unmarshal(values.FieldOntologyScope, &resolved.Field.OntologyScope)
	}
	if len(values.FieldPathElements) > 0 {
		_ = json.Unmarshal(values.FieldPathElements, &resolved.Field.PathElements)
	}
	return resolved
}

func scanOverrideRef(row interface{ Scan(...any) error }) (domainweave.OverrideRef, error) {
	var ref domainweave.OverrideRef
	var position int32
	err := row.Scan(
		&ref.OverrideID,
		&ref.RefType,
		&ref.TargetID,
		&ref.SemanticID,
		&position,
		&ref.ProjectID,
		&ref.VersionNumber,
	)
	ref.Position = int(position)
	return ref, err
}

func groupOverrideRefsByOverrideID(refs []domainweave.OverrideRef) map[int64][]domainweave.OverrideRef {
	grouped := make(map[int64][]domainweave.OverrideRef, len(refs))
	for _, ref := range refs {
		grouped[ref.OverrideID] = append(grouped[ref.OverrideID], ref)
	}
	return grouped
}

func intPtrFromInt32Ptr(value *int32) *int {
	if value == nil {
		return nil
	}
	converted := int(*value)
	return &converted
}

func fieldOverrideSelect(table, setValueEntryIDExpr string) string {
	return fmt.Sprintf(`
		SELECT
			id, field_id, project_id, entity_type, entity_id, position, collection_order,
			display_name, description, collection_name, category_id, part_of_collection_id,
			expected_value_type, set_value, is_required, min_occurs, max_occurs, is_hidden,
			visibility, staging_id, created_at, updated_at, content_hash, version_number,
			%s AS set_value_entry_id
		FROM %s
	`, setValueEntryIDExpr, table)
}

func fieldOverrideResolvedSelect(overrideTable, fieldTable, setValueEntryIDExpr string) string {
	return fmt.Sprintf(`
		SELECT
			fo.id, fo.field_id, fo.project_id, fo.entity_type, fo.entity_id, fo.position, fo.collection_order,
			fo.display_name, fo.description, fo.collection_name, fo.category_id, fo.part_of_collection_id,
			fo.expected_value_type, fo.set_value, fo.is_required, fo.min_occurs, fo.max_occurs, fo.is_hidden,
			fo.visibility, fo.staging_id, fo.created_at, fo.updated_at, fo.content_hash, fo.version_number,
			%s AS set_value_entry_id,
			f.semantic_id AS field_semantic_id,
			f.system_name AS field_system_name,
			f.ontology_scope AS field_ontology_scope,
			f.ontology_path AS field_ontology_path,
			f.expected_value_type AS field_expected_value_type,
			f.path_elements AS field_path_elements
		FROM %s fo
		JOIN %s f ON f.id = fo.field_id AND f.project_id = fo.project_id
	`, setValueEntryIDExpr, overrideTable, fieldTable)
}
