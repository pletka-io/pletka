package weave

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// resolver encapsulates the query dependencies for resolve operations.
type resolver struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

// categoryInfo is a lightweight struct for grouping fields by category.
type categoryInfo struct {
	ID       string
	Name     domain.Translations
	Position int
}

// ---------------------------------------------------------------------------
// Row converters
// ---------------------------------------------------------------------------

// resolveModelRowToField converts a WeaveResolveForModelRow to a domain.ResolvedField.
func resolveModelRowToField(row sqlcgen.WeaveResolveForModelRow) domain.ResolvedField {
	rf := domain.ResolvedField{
		ID:                 row.FieldID,
		SemanticID:         dbutil.NilToEmpty(row.FieldSemanticID),
		SystemName:         dbutil.NilToEmpty(row.FieldSystemName),
		ProjectID:          domain.ParseSemanticID(row.FieldID).ProjectID,
		OntologyPath:       dbutil.NilToEmpty(row.FieldOntologyPath),
		DisplayName:        unmarshalDomainTranslations(row.DisplayName),
		Description:        unmarshalDomainTranslations(row.Description),
		Position:           int(row.Position),
		ExpectedValueType:  dbutil.NilToEmpty(row.FieldExpectedValueType),
		SetValue:           dbutil.NilToEmpty(row.SetValue),
		IsRequired:         dbutil.Deref(row.IsRequired),
		MinOccurs:          dbutil.DerefInt32(row.MinOccurs),
		MaxOccurs:          dbutil.DerefInt32ToIntPtr(row.MaxOccurs),
		IsHidden:           dbutil.Deref(row.IsHidden),
		Visibility:         dbutil.NilToEmpty(row.Visibility),
		OverrideID:         row.ID,
		CategoryID:         dbutil.NilToEmpty(row.CategoryID),
		PartOfCollectionID: dbutil.NilToEmpty(row.PartOfCollectionID),
		CollectionOrder:    int(row.CollectionOrder),
		CollectionName:     unmarshalDomainTranslations(row.CollectionName),
	}

	// Determine override source from entity type.
	switch row.EntityType {
	case "model":
		rf.OverrideSource = "model"
	case "collection":
		rf.OverrideSource = "collection"
	default:
		rf.OverrideSource = "base"
	}

	// Parse path elements from JSONB.
	if len(row.FieldPathElements) > 0 {
		_ = json.Unmarshal(row.FieldPathElements, &rf.PathElements)
	}
	if len(row.FieldSubfieldPaths) > 0 {
		_ = json.Unmarshal(row.FieldSubfieldPaths, &rf.SubfieldPaths)
	}

	return rf
}

// resolveCollectionRowToField converts a WeaveResolveForCollectionRow to a domain.ResolvedField.
func resolveCollectionRowToField(row sqlcgen.WeaveResolveForCollectionRow) domain.ResolvedField {
	rf := domain.ResolvedField{
		ID:                 row.FieldID,
		SemanticID:         dbutil.NilToEmpty(row.FieldSemanticID),
		SystemName:         dbutil.NilToEmpty(row.FieldSystemName),
		ProjectID:          domain.ParseSemanticID(row.FieldID).ProjectID,
		OntologyPath:       dbutil.NilToEmpty(row.FieldOntologyPath),
		DisplayName:        unmarshalDomainTranslations(row.DisplayName),
		Description:        unmarshalDomainTranslations(row.Description),
		Position:           int(row.Position),
		ExpectedValueType:  dbutil.NilToEmpty(row.FieldExpectedValueType),
		SetValue:           dbutil.NilToEmpty(row.SetValue),
		IsRequired:         dbutil.Deref(row.IsRequired),
		MinOccurs:          dbutil.DerefInt32(row.MinOccurs),
		MaxOccurs:          dbutil.DerefInt32ToIntPtr(row.MaxOccurs),
		IsHidden:           dbutil.Deref(row.IsHidden),
		Visibility:         dbutil.NilToEmpty(row.Visibility),
		OverrideSource:     "collection",
		OverrideID:         row.ID,
		CategoryID:         dbutil.NilToEmpty(row.CategoryID),
		PartOfCollectionID: dbutil.NilToEmpty(row.PartOfCollectionID),
		CollectionOrder:    int(row.CollectionOrder),
		CollectionName:     unmarshalDomainTranslations(row.CollectionName),
	}

	// Parse path elements from JSONB.
	if len(row.FieldPathElements) > 0 {
		_ = json.Unmarshal(row.FieldPathElements, &rf.PathElements)
	}
	if len(row.FieldSubfieldPaths) > 0 {
		_ = json.Unmarshal(row.FieldSubfieldPaths, &rf.SubfieldPaths)
	}

	return rf
}

// ---------------------------------------------------------------------------
// Ref batch loading
// ---------------------------------------------------------------------------

// groupRefsByOverride groups refs by override_id for batch attachment.
func groupRefsByOverride(refs []sqlcgen.WeaveOverrideRef) map[int64][]sqlcgen.WeaveOverrideRef {
	grouped := make(map[int64][]sqlcgen.WeaveOverrideRef, len(refs))
	for _, ref := range refs {
		grouped[ref.OverrideID] = append(grouped[ref.OverrideID], ref)
	}
	return grouped
}

// attachRefs attaches expected entity refs to resolved fields.
// It batch-loads entity names to avoid N+1 queries.
func attachRefs(
	ctx context.Context,
	queries *sqlcgen.Queries,
	fields []domain.ResolvedField,
	refsByOverride map[int64][]sqlcgen.WeaveOverrideRef,
) {
	// Collect unique target IDs by type for batch name lookup.
	modelIDs := make(map[string]struct{})
	collectionIDs := make(map[string]struct{})
	conceptListIDs := make(map[string]struct{})

	for i := range fields {
		for _, ref := range refsByOverride[fields[i].OverrideID] {
			switch ref.RefType {
			case "resource_model":
				modelIDs[ref.TargetID] = struct{}{}
			case "collection_model":
				collectionIDs[ref.TargetID] = struct{}{}
			case "concept_list":
				conceptListIDs[ref.TargetID] = struct{}{}
			}
		}
	}

	lookup := batchLoadNames(ctx, queries, modelIDs, collectionIDs, conceptListIDs)

	// Attach refs with resolved names.
	for i := range fields {
		refs, ok := refsByOverride[fields[i].OverrideID]
		if !ok {
			continue
		}

		for _, ref := range refs {
			projectID := fields[i].ProjectID
			if ref.RefType == "concept_list" && lookup.projectIDs[ref.TargetID] != "" {
				projectID = lookup.projectIDs[ref.TargetID]
			}
			url := overrideRefURL(projectID, ref.RefType, ref.TargetID, ref.SemanticID)
			eRef := domain.EntityRef{
				ID:         ref.TargetID,
				SemanticID: ref.SemanticID,
				URL:        url,
				Name:       lookup.names[ref.TargetID],
			}

			switch ref.RefType {
			case "resource_model":
				fields[i].ResourceModels = append(fields[i].ResourceModels, eRef)
			case "collection_model":
				fields[i].CollectionModels = append(fields[i].CollectionModels, eRef)
			case "concept_list":
				fields[i].ConceptLists = append(fields[i].ConceptLists, eRef)
			}
		}
	}
}

// batchLoadNames loads ui_name for expected ref targets.
type refLookup struct {
	names      map[string]domain.Translations
	projectIDs map[string]string
}

func batchLoadNames(
	ctx context.Context,
	queries *sqlcgen.Queries,
	modelIDs, collectionIDs, conceptListIDs map[string]struct{},
) refLookup {
	lookup := refLookup{
		names:      make(map[string]domain.Translations),
		projectIDs: make(map[string]string),
	}

	if len(modelIDs) > 0 {
		ids := mapKeys(modelIDs)
		rows, err := queries.WeaveGetModelNamesByIDs(ctx, ids)
		if err == nil {
			for _, row := range rows {
				lookup.names[row.ID] = unmarshalDomainTranslations(row.UiName)
			}
		}
	}

	if len(collectionIDs) > 0 {
		ids := mapKeys(collectionIDs)
		rows, err := queries.WeaveGetCollectionNamesByIDs(ctx, ids)
		if err == nil {
			for _, row := range rows {
				lookup.names[row.ID] = unmarshalDomainTranslations(row.UiName)
			}
		}
	}
	if len(conceptListIDs) > 0 {
		ids := mapKeys(conceptListIDs)
		rows, err := queries.WeaveGetConceptListNamesByIDs(ctx, ids)
		if err == nil {
			for _, row := range rows {
				name := unmarshalDomainTranslations(row.UiName)
				lookup.names[row.ID] = name
				lookup.projectIDs[row.ID] = row.ProjectID
				if row.SemanticID != nil {
					lookup.names[*row.SemanticID] = name
					lookup.projectIDs[*row.SemanticID] = row.ProjectID
				}
			}
		}
	}

	return lookup
}

func overrideRefURL(projectID, refType, targetID, semanticID string) string {
	if refType == "concept_list" {
		return domain.EntityURLForRefType(projectID, refType, targetID)
	}
	sid := domain.ParseSemanticID(semanticID)
	url := sid.URL()
	if url == "" {
		// SemanticID parse miss (legacy / malformed row) — fall back to
		// ref-type + projectID + targetID so the wire shape always carries
		// a usable URL and the frontend never has to compose its own.
		url = domain.EntityURLForRefType(projectID, refType, targetID)
	}
	return url
}

// mapKeys extracts keys from a string set.
func mapKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ---------------------------------------------------------------------------
// Resolve operations
// ---------------------------------------------------------------------------

// resolveForModel fetches all overrides for a model and attaches refs.
// No DISTINCT ON — every override is returned (copy-on-adopt).
func (r *resolver) resolveForModel(ctx context.Context, modelID, projectID string) ([]domain.ResolvedField, error) {
	rows, err := r.queries.WeaveResolveForModel(ctx, sqlcgen.WeaveResolveForModelParams{
		ModelID:   modelID,
		ProjectID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve for model: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	// Convert rows to resolved fields and collect override IDs.
	fields := make([]domain.ResolvedField, 0, len(rows))
	overrideIDs := make([]int64, 0, len(rows))

	for _, row := range rows {
		rf := resolveModelRowToField(row)
		fields = append(fields, rf)
		overrideIDs = append(overrideIDs, rf.OverrideID)
	}

	// Batch-load refs.
	refs, err := r.queries.WeaveListRefsForOverrides(ctx, overrideIDs)
	if err != nil {
		return nil, fmt.Errorf("list refs for model overrides: %w", err)
	}

	// Attach refs to fields.
	if len(refs) > 0 {
		attachRefs(ctx, r.queries, fields, groupRefsByOverride(refs))
	}

	if err := r.applyBaseSetValueLock(ctx, fields, projectID); err != nil {
		return nil, fmt.Errorf("apply base set_value lock: %w", err)
	}

	return fields, nil
}

// resolveForCollection fetches all overrides for a collection and attaches refs.
func (r *resolver) resolveForCollection(ctx context.Context, collectionID, projectID string) ([]domain.ResolvedField, error) {
	rows, err := r.queries.WeaveResolveForCollection(ctx, sqlcgen.WeaveResolveForCollectionParams{
		CollectionID: collectionID,
		ProjectID:    projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve for collection: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	// Convert rows to resolved fields and collect override IDs.
	fields := make([]domain.ResolvedField, 0, len(rows))
	overrideIDs := make([]int64, 0, len(rows))

	for _, row := range rows {
		rf := resolveCollectionRowToField(row)
		fields = append(fields, rf)
		overrideIDs = append(overrideIDs, rf.OverrideID)
	}

	// Batch-load refs.
	refs, err := r.queries.WeaveListRefsForOverrides(ctx, overrideIDs)
	if err != nil {
		return nil, fmt.Errorf("list refs for collection overrides: %w", err)
	}

	// Attach refs to fields.
	if len(refs) > 0 {
		attachRefs(ctx, r.queries, fields, groupRefsByOverride(refs))
	}

	if err := r.applyBaseSetValueLock(ctx, fields, projectID); err != nil {
		return nil, fmt.Errorf("apply base set_value lock: %w", err)
	}

	return fields, nil
}

type archivedResolvedRow struct {
	OverrideID             int64
	FieldID                string
	ProjectID              string
	EntityType             string
	EntityID               string
	Position               int32
	CollectionOrder        int32
	DisplayName            []byte
	Description            []byte
	CollectionName         []byte
	CategoryID             *string
	PartOfCollectionID     *string
	SetValue               *string
	IsRequired             *bool
	MinOccurs              *int32
	MaxOccurs              *int32
	IsHidden               *bool
	Visibility             *string
	StagingID              *int64
	FieldSemanticID        *string
	FieldSystemName        *string
	FieldOntologyPath      *string
	FieldExpectedValueType *string
	FieldPathElements      []byte
}

func (r *resolver) resolveForModelVersion(ctx context.Context, modelID, projectID, version string) ([]domain.ResolvedField, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			fo.id, fo.field_id, fo.project_id, fo.entity_type, fo.entity_id, fo.position, fo.collection_order,
			fo.display_name, fo.description, fo.collection_name, fo.category_id, fo.part_of_collection_id,
			fo.set_value, fo.is_required, fo.min_occurs, fo.max_occurs, fo.is_hidden, fo.visibility,
			fo.staging_id,
			f.semantic_id, f.system_name, f.ontology_path, f.expected_value_type, f.path_elements
		FROM weave_field_overrides_archive fo
		JOIN weave_fields_archive f
		  ON f.id = fo.field_id
		 AND f.version_number = fo.version_number
		WHERE fo.entity_type = 'model'
		  AND fo.entity_id = $1
		  AND fo.project_id = $2
		  AND fo.version_number = $3
		ORDER BY fo.category_id, fo.part_of_collection_id, fo.position
	`, modelID, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("resolve archived model: %w", err)
	}
	defer rows.Close()
	fields := []domain.ResolvedField{}
	overrideIDs := []int64{}
	for rows.Next() {
		ar, err := scanArchivedResolvedRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan archived model resolved row: %w", err)
		}
		rf := archivedResolvedRowToField(ar, "model")
		fields = append(fields, rf)
		overrideIDs = append(overrideIDs, rf.OverrideID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived model resolved rows: %w", err)
	}
	if len(fields) == 0 {
		return nil, nil
	}
	if err := r.attachRefsVersion(ctx, fields, overrideIDs, version); err != nil {
		return nil, err
	}
	if err := r.applyBaseSetValueLockVersion(ctx, fields, projectID, version); err != nil {
		return nil, fmt.Errorf("apply archived base set_value lock: %w", err)
	}
	return fields, nil
}

func (r *resolver) resolveForCollectionVersion(ctx context.Context, collectionID, projectID, version string) ([]domain.ResolvedField, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			fo.id, fo.field_id, fo.project_id, fo.entity_type, fo.entity_id, fo.position, fo.collection_order,
			fo.display_name, fo.description, fo.collection_name, fo.category_id, fo.part_of_collection_id,
			fo.set_value, fo.is_required, fo.min_occurs, fo.max_occurs, fo.is_hidden, fo.visibility,
			fo.staging_id,
			f.semantic_id, f.system_name, f.ontology_path, f.expected_value_type, f.path_elements
		FROM weave_field_overrides_archive fo
		JOIN weave_fields_archive f
		  ON f.id = fo.field_id
		 AND f.version_number = fo.version_number
		WHERE fo.entity_type = 'collection'
		  AND fo.entity_id = $1
		  AND fo.project_id = $2
		  AND fo.version_number = $3
		ORDER BY fo.category_id, fo.position
	`, collectionID, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("resolve archived collection: %w", err)
	}
	defer rows.Close()
	fields := []domain.ResolvedField{}
	overrideIDs := []int64{}
	for rows.Next() {
		ar, err := scanArchivedResolvedRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan archived collection resolved row: %w", err)
		}
		rf := archivedResolvedRowToField(ar, "collection")
		fields = append(fields, rf)
		overrideIDs = append(overrideIDs, rf.OverrideID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived collection resolved rows: %w", err)
	}
	if len(fields) == 0 {
		return nil, nil
	}
	if err := r.attachRefsVersion(ctx, fields, overrideIDs, version); err != nil {
		return nil, err
	}
	if err := r.applyBaseSetValueLockVersion(ctx, fields, projectID, version); err != nil {
		return nil, fmt.Errorf("apply archived base set_value lock: %w", err)
	}
	return fields, nil
}

func scanArchivedResolvedRow(scanner interface{ Scan(...any) error }) (archivedResolvedRow, error) {
	var row archivedResolvedRow
	err := scanner.Scan(
		&row.OverrideID, &row.FieldID, &row.ProjectID, &row.EntityType, &row.EntityID, &row.Position, &row.CollectionOrder,
		&row.DisplayName, &row.Description, &row.CollectionName, &row.CategoryID, &row.PartOfCollectionID,
		&row.SetValue, &row.IsRequired, &row.MinOccurs, &row.MaxOccurs, &row.IsHidden, &row.Visibility,
		&row.StagingID,
		&row.FieldSemanticID, &row.FieldSystemName, &row.FieldOntologyPath, &row.FieldExpectedValueType, &row.FieldPathElements,
	)
	return row, err
}

func archivedResolvedRowToField(row archivedResolvedRow, source string) domain.ResolvedField {
	rf := domain.ResolvedField{
		ID:                 row.FieldID,
		SemanticID:         dbutil.NilToEmpty(row.FieldSemanticID),
		SystemName:         dbutil.NilToEmpty(row.FieldSystemName),
		ProjectID:          domain.ParseSemanticID(row.FieldID).ProjectID,
		OntologyPath:       dbutil.NilToEmpty(row.FieldOntologyPath),
		DisplayName:        unmarshalDomainTranslations(row.DisplayName),
		Description:        unmarshalDomainTranslations(row.Description),
		Position:           int(row.Position),
		ExpectedValueType:  dbutil.NilToEmpty(row.FieldExpectedValueType),
		SetValue:           dbutil.NilToEmpty(row.SetValue),
		IsRequired:         dbutil.Deref(row.IsRequired),
		MinOccurs:          dbutil.DerefInt32(row.MinOccurs),
		MaxOccurs:          dbutil.DerefInt32ToIntPtr(row.MaxOccurs),
		IsHidden:           dbutil.Deref(row.IsHidden),
		Visibility:         dbutil.NilToEmpty(row.Visibility),
		OverrideSource:     source,
		OverrideID:         row.OverrideID,
		CategoryID:         dbutil.NilToEmpty(row.CategoryID),
		PartOfCollectionID: dbutil.NilToEmpty(row.PartOfCollectionID),
		CollectionOrder:    int(row.CollectionOrder),
		CollectionName:     unmarshalDomainTranslations(row.CollectionName),
	}
	if len(row.FieldPathElements) > 0 {
		_ = json.Unmarshal(row.FieldPathElements, &rf.PathElements)
	}
	return rf
}

func (r *resolver) attachRefsVersion(ctx context.Context, fields []domain.ResolvedField, overrideIDs []int64, version string) error {
	refs, err := r.pool.Query(ctx, `
		SELECT override_id, ref_type, target_id, semantic_id, position
		FROM weave_override_refs_archive
		WHERE version_number = $2
		  AND override_id = ANY($1)
		ORDER BY override_id, ref_type, position
	`, overrideIDs, version)
	if err != nil {
		return fmt.Errorf("list archived refs for overrides: %w", err)
	}
	defer refs.Close()

	type archiveRef struct {
		OverrideID int64
		RefType    string
		TargetID   string
		SemanticID string
		Position   int32
	}
	grouped := map[int64][]archiveRef{}
	modelIDs := map[string]struct{}{}
	collectionIDs := map[string]struct{}{}
	conceptListIDs := map[string]struct{}{}
	for refs.Next() {
		var ref archiveRef
		if err := refs.Scan(&ref.OverrideID, &ref.RefType, &ref.TargetID, &ref.SemanticID, &ref.Position); err != nil {
			return fmt.Errorf("scan archived override ref: %w", err)
		}
		grouped[ref.OverrideID] = append(grouped[ref.OverrideID], ref)
		switch ref.RefType {
		case "resource_model":
			modelIDs[ref.TargetID] = struct{}{}
		case "collection_model":
			collectionIDs[ref.TargetID] = struct{}{}
		case "concept_list":
			conceptListIDs[ref.TargetID] = struct{}{}
		}
	}
	if err := refs.Err(); err != nil {
		return fmt.Errorf("iterate archived override refs: %w", err)
	}

	lookup := r.batchLoadNamesVersion(ctx, modelIDs, collectionIDs, conceptListIDs, version)
	for i := range fields {
		for _, ref := range grouped[fields[i].OverrideID] {
			projectID := fields[i].ProjectID
			if ref.RefType == "concept_list" && lookup.projectIDs[ref.TargetID] != "" {
				projectID = lookup.projectIDs[ref.TargetID]
			}
			url := overrideRefURL(projectID, ref.RefType, ref.TargetID, ref.SemanticID)
			eRef := domain.EntityRef{
				ID:         ref.TargetID,
				SemanticID: ref.SemanticID,
				URL:        url,
				Name:       lookup.names[ref.TargetID],
			}
			switch ref.RefType {
			case "resource_model":
				fields[i].ResourceModels = append(fields[i].ResourceModels, eRef)
			case "collection_model":
				fields[i].CollectionModels = append(fields[i].CollectionModels, eRef)
			case "concept_list":
				fields[i].ConceptLists = append(fields[i].ConceptLists, eRef)
			}
		}
	}
	return nil
}

func (r *resolver) batchLoadNamesVersion(ctx context.Context, modelIDs, collectionIDs, conceptListIDs map[string]struct{}, version string) refLookup {
	lookup := refLookup{
		names:      make(map[string]domain.Translations),
		projectIDs: make(map[string]string),
	}
	if len(modelIDs) > 0 {
		rows, err := r.pool.Query(ctx, `
			SELECT id, ui_name FROM weave_models_archive
			WHERE version_number = $2 AND id = ANY($1)
		`, mapKeys(modelIDs), version)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id string
				var uiName []byte
				if err := rows.Scan(&id, &uiName); err == nil {
					lookup.names[id] = unmarshalDomainTranslations(uiName)
				}
			}
		}
	}
	if len(collectionIDs) > 0 {
		rows, err := r.pool.Query(ctx, `
			SELECT id, ui_name FROM weave_collections_archive
			WHERE version_number = $2 AND id = ANY($1)
		`, mapKeys(collectionIDs), version)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id string
				var uiName []byte
				if err := rows.Scan(&id, &uiName); err == nil {
					lookup.names[id] = unmarshalDomainTranslations(uiName)
				}
			}
		}
	}
	if len(conceptListIDs) > 0 {
		rows, err := r.pool.Query(ctx, `
			SELECT id, semantic_id, ui_name, project_id FROM weave_concept_lists_archive
			WHERE version_number = $2 AND (id = ANY($1) OR semantic_id = ANY($1))
		`, mapKeys(conceptListIDs), version)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id string
				var semanticID *string
				var uiName []byte
				var projectID string
				if err := rows.Scan(&id, &semanticID, &uiName, &projectID); err == nil {
					name := unmarshalDomainTranslations(uiName)
					lookup.names[id] = name
					lookup.projectIDs[id] = projectID
					if semanticID != nil {
						lookup.names[*semanticID] = name
						lookup.projectIDs[*semanticID] = projectID
					}
				}
			}
		}
	}
	return lookup
}

func (r *resolver) applyBaseSetValueLockVersion(ctx context.Context, fields []domain.ResolvedField, projectID, version string) error {
	if len(fields) == 0 {
		return nil
	}
	fieldIDs := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for i := range fields {
		id := fields[i].ID
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		fieldIDs = append(fieldIDs, id)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT field_id, set_value
		FROM weave_field_overrides_archive
		WHERE entity_type = ''
		  AND project_id = $1
		  AND version_number = $2
		  AND field_id = ANY($3)
	`, projectID, version, fieldIDs)
	if err != nil {
		return fmt.Errorf("list archived base overrides: %w", err)
	}
	defer rows.Close()
	baseSetValue := map[string]string{}
	for rows.Next() {
		var fieldID string
		var setValue *string
		if err := rows.Scan(&fieldID, &setValue); err != nil {
			return fmt.Errorf("scan archived base override: %w", err)
		}
		if v := dbutil.NilToEmpty(setValue); v != "" {
			baseSetValue[fieldID] = v
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate archived base overrides: %w", err)
	}
	for i := range fields {
		if v, ok := baseSetValue[fields[i].ID]; ok {
			fields[i].SetValue = v
			fields[i].OverrideSource = "base"
		}
	}
	return nil
}

// applyBaseSetValueLock enforces the rule "base SetValue is non-overridable".
//
// When the base override (entity_type=”) for a field has a non-empty
// SetValue, that value wins regardless of any model/collection override.
// The reason: a SetValue on the base is the project's commitment that
// the field's value is fixed at the field level — downstream contexts
// (a specific model or collection embedding) must not be able to
// silently change it.
//
// Mutates the slice in place. Empty base SetValue ("") is a no-op for
// that field — model/collection SetValue overrides win as usual.
func (r *resolver) applyBaseSetValueLock(ctx context.Context, fields []domain.ResolvedField, projectID string) error {
	if len(fields) == 0 {
		return nil
	}
	fieldIDs := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for i := range fields {
		id := fields[i].ID
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		fieldIDs = append(fieldIDs, id)
	}
	if len(fieldIDs) == 0 {
		return nil
	}

	rows, err := r.queries.WeaveListBaseOverridesForFields(ctx, sqlcgen.WeaveListBaseOverridesForFieldsParams{
		ProjectID: projectID,
		Column2:   fieldIDs,
	})
	if err != nil {
		return fmt.Errorf("list base overrides: %w", err)
	}

	baseSetValue := make(map[string]string, len(rows))
	for _, row := range rows {
		v := dbutil.NilToEmpty(row.SetValue)
		if v != "" {
			baseSetValue[row.FieldID] = v
		}
	}
	if len(baseSetValue) == 0 {
		return nil
	}
	for i := range fields {
		if v, ok := baseSetValue[fields[i].ID]; ok {
			fields[i].SetValue = v
			fields[i].OverrideSource = "base"
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Grouping
// ---------------------------------------------------------------------------

const (
	uncategorizedKey = "__uncategorized__"
	directKey        = "__direct__"
)

// groupByCategory groups flat resolved fields into CategoryGroup -> CollectionGroup -> Field hierarchy.
func groupByCategory(
	fields []domain.ResolvedField,
	categories map[string]*categoryInfo,
	collectionNames map[string]domain.Translations,
) []domain.CategoryGroup {
	// catCollFields: categoryID -> collectionID -> []ResolvedField
	type collEntry struct {
		id     string
		fields []domain.ResolvedField
	}

	catColls := make(map[string]map[string]*collEntry)

	for _, f := range fields {
		catKey := f.CategoryID
		if catKey == "" {
			catKey = uncategorizedKey
		}

		collKey := f.PartOfCollectionID
		if collKey == "" {
			collKey = directKey
		}

		if catColls[catKey] == nil {
			catColls[catKey] = make(map[string]*collEntry)
		}

		entry := catColls[catKey][collKey]
		if entry == nil {
			entry = &collEntry{id: collKey}
			catColls[catKey][collKey] = entry
		}

		entry.fields = append(entry.fields, f)
	}

	// Build CategoryGroup slices.
	groups := make([]domain.CategoryGroup, 0, len(catColls))

	for catID, colls := range catColls {
		cg := domain.CategoryGroup{
			ID: catID,
		}

		if info, ok := categories[catID]; ok {
			cg.Name = info.Name
			cg.Position = info.Position
		} else if catID == uncategorizedKey {
			cg.Name = domain.Translations{"en": "Uncategorized"}
			cg.Position = 9999 // sort last
		}

		// Build CollectionGroup slices.
		collGroups := make([]domain.CollectionGroup, 0, len(colls))

		for _, entry := range colls {
			collGroup := domain.CollectionGroup{
				ID: entry.id,
			}

			// Determine collection name: prefer override CollectionName from
			// first field, fall back to collectionNames map.
			if entry.id != directKey {
				if len(entry.fields) > 0 && len(entry.fields[0].CollectionName) > 0 {
					collGroup.Name = entry.fields[0].CollectionName
				} else if name, ok := collectionNames[entry.id]; ok {
					collGroup.Name = name
				}
			} else {
				collGroup.Name = domain.Translations{"en": "Direct Fields"}
			}

			// Position from first field's CollectionOrder.
			if len(entry.fields) > 0 {
				collGroup.Position = entry.fields[0].CollectionOrder
			}

			// Sort fields by Position within collection.
			sort.Slice(entry.fields, func(i, j int) bool {
				return entry.fields[i].Position < entry.fields[j].Position
			})

			collGroup.Fields = entry.fields
			collGroups = append(collGroups, collGroup)
		}

		// Sort collections by position.
		sort.Slice(collGroups, func(i, j int) bool {
			return collGroups[i].Position < collGroups[j].Position
		})

		cg.Collections = collGroups
		groups = append(groups, cg)
	}

	// Sort categories by position.
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Position < groups[j].Position
	})

	return groups
}

// ComputeSharedPathPrefix finds the longest common prefix of PathElements
// across all fields in a collection. Returns nil if fields have no common prefix.
func ComputeSharedPathPrefix(fields []domain.ResolvedField) []domain.PathElement {
	if len(fields) == 0 {
		return nil
	}

	// Start with the first field's path elements as candidate prefix.
	first := fields[0].PathElements
	if len(first) == 0 {
		return nil
	}

	prefixLen := len(first)

	for _, f := range fields[1:] {
		if len(f.PathElements) < prefixLen {
			prefixLen = len(f.PathElements)
		}

		for i := 0; i < prefixLen; i++ {
			if f.PathElements[i].URI != first[i].URI {
				prefixLen = i
				break
			}
		}

		if prefixLen == 0 {
			return nil
		}
	}

	if prefixLen == 0 {
		return nil
	}

	// Copy the shared prefix elements.
	prefix := make([]domain.PathElement, prefixLen)
	copy(prefix, first[:prefixLen])

	return prefix
}

// filterVisibleResolvedFields returns the fields with IsHidden == false,
// preserving order. Used wherever a derived computation (stats, shared path
// prefix) must not be skewed by a field a curator has hidden — callers
// that need the full set (the override editor, generators, unhide flow)
// must not use this filtered copy.
func filterVisibleResolvedFields(fields []domain.ResolvedField) []domain.ResolvedField {
	visible := make([]domain.ResolvedField, 0, len(fields))
	for _, f := range fields {
		if !f.IsHidden {
			visible = append(visible, f)
		}
	}
	return visible
}

// ---------------------------------------------------------------------------
// ModelView + CollectionView (called from PostgresStore)
// ---------------------------------------------------------------------------

// buildModelView assembles a complete ModelView for the given model.
func (r *resolver) buildModelView(ctx context.Context, modelID, projectID string) (*domain.ModelView, error) {
	// 1. Resolve fields with winning overrides + refs.
	fields, err := r.resolveForModel(ctx, modelID, projectID)
	if err != nil {
		return nil, err
	}

	// 2. Load categories for grouping.
	catRows, err := r.queries.WeaveListCategories(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list categories for model view: %w", err)
	}

	catMap := make(map[string]*categoryInfo, len(catRows))
	for _, row := range catRows {
		catMap[row.ID] = &categoryInfo{
			ID:       row.ID,
			Name:     unmarshalDomainTranslations(row.UiName),
			Position: int(row.CanonicalOrder),
		}
	}

	// 3. Collect unique collection IDs and load their names.
	collIDs := make(map[string]struct{})
	for _, f := range fields {
		if f.PartOfCollectionID != "" {
			collIDs[f.PartOfCollectionID] = struct{}{}
		}
	}

	collNames := make(map[string]domain.Translations, len(collIDs))

	for collID := range collIDs {
		coll, err := r.queries.WeaveGetCollectionByID(ctx, collID)
		if err != nil {
			if err == pgx.ErrNoRows {
				continue
			}

			return nil, fmt.Errorf("get collection %s for model view: %w", collID, err)
		}

		collNames[coll.ID] = unmarshalDomainTranslations(coll.UiName)
	}

	// 4. Set IsExternal on each field (field prefix doesn't match project).
	for i := range fields {
		fields[i].IsExternal = !strings.HasPrefix(fields[i].ID, projectID)
	}

	// 5. Group into category -> collection -> field hierarchy.
	categories := groupByCategory(fields, catMap, collNames)

	// 6. Compute shared path prefix for each collection group. Direct
	// fields (the "__direct__" bucket) share only a display category, not
	// an ontology root — hoisting an incidental shared prefix there hides
	// each field's own path on its row instead, so that
	// bucket never gets one. Real collections keep hoisting, computed over
	// visible fields only so a hidden field's divergent path can't shorten
	// or blank out the prefix.
	for i := range categories {
		for j := range categories[i].Collections {
			coll := &categories[i].Collections[j]
			if coll.ID == directKey {
				coll.SharedPathPrefix = nil
				continue
			}
			coll.SharedPathPrefix = ComputeSharedPathPrefix(filterVisibleResolvedFields(coll.Fields))
		}
	}

	// 6b. Attach collection placements: model-level
	// constraints per collection group. Keyed by the same category id the
	// override rows carry ('' for the uncategorized bucket) + collection
	// id. Absence of a row leaves Placement nil (defaults). The override
	// editor round-trips these values; the read-only detail view filters
	// hidden groups.
	placementRows, err := r.queries.WeaveListCollectionPlacementsByModel(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("list collection placements for model view %s: %w", modelID, err)
	}
	if len(placementRows) > 0 {
		byKey := make(map[string]sqlcgen.WeaveCollectionPlacement, len(placementRows))
		for _, p := range placementRows {
			byKey[p.CategoryID+"|"+p.CollectionID] = p
		}
		for i := range categories {
			catKey := categories[i].ID
			if catKey == uncategorizedKey {
				catKey = ""
			}
			for j := range categories[i].Collections {
				coll := &categories[i].Collections[j]
				if coll.ID == directKey {
					continue
				}
				row, ok := byKey[catKey+"|"+coll.ID]
				if !ok {
					continue
				}
				pl := domain.CollectionPlacement{
					ID:           row.ID,
					ProjectID:    row.ProjectID,
					ModelID:      row.ModelID,
					CategoryID:   row.CategoryID,
					CollectionID: row.CollectionID,
					IsRequired:   row.IsRequired,
					MinOccurs:    int(row.MinOccurs),
					IsHidden:     row.IsHidden,
				}
				if row.MaxOccurs != nil {
					n := int(*row.MaxOccurs)
					pl.MaxOccurs = &n
				}
				coll.Placement = &pl
			}
		}
	}

	// 7. Compute stats. Hidden fields are excluded from every stat —
	// computed from a filtered copy so this never
	// touches `fields`/`categories` above, which the override editor
	// (project/override_read.go), generators, and the example service
	// still need with hidden fields intact (unhide flow, snapshot
	// building, validation against override IDs).
	visibleFields := filterVisibleResolvedFields(fields)
	// Fields inside a hidden collection group (placement.is_hidden) are
	// excluded from stats too (same policy as hidden fields). Editor
	// payloads keep them.
	if len(placementRows) > 0 {
		hiddenGroups := make(map[string]struct{})
		for _, p := range placementRows {
			if p.IsHidden {
				hiddenGroups[p.CategoryID+"|"+p.CollectionID] = struct{}{}
			}
		}
		if len(hiddenGroups) > 0 {
			kept := visibleFields[:0]
			for _, f := range visibleFields {
				if _, hidden := hiddenGroups[f.CategoryID+"|"+f.PartOfCollectionID]; !hidden {
					kept = append(kept, f)
				}
			}
			visibleFields = kept
		}
	}
	statsCategories := groupByCategory(visibleFields, catMap, collNames)

	overridden := 0
	required := 0
	uniqueCollections := make(map[string]struct{})
	valueTypeCounts := make(map[string]int)

	for _, f := range visibleFields {
		if f.OverrideSource != "" {
			overridden++
		}
		if f.IsRequired {
			required++
		}
		if f.ExpectedValueType != "" {
			valueTypeCounts[f.ExpectedValueType]++
		}
		if f.PartOfCollectionID != "" {
			uniqueCollections[f.PartOfCollectionID] = struct{}{}
		}
	}

	catBreakdown, fieldBreakdown, fieldScopeBreakdown, collScopeBreakdown, classBreakdown, propBreakdown, scopesCount := computeBreakdowns(statsCategories, visibleFields)

	stats := domain.ModelViewStats{
		TotalFields:      len(visibleFields),
		TotalCategories:  len(statsCategories),
		TotalCollections: len(uniqueCollections),
		OverriddenFields: overridden,
		RequiredFields:   required,
		OptionalFields:   len(visibleFields) - required,
		ValueTypeCounts:  valueTypeCounts,
		ScopesCount:      scopesCount,

		CategoriesBreakdown:  catBreakdown,
		FieldsBreakdown:      fieldBreakdown,
		FieldScopesBreakdown: fieldScopeBreakdown,
		CollScopesBreakdown:  collScopeBreakdown,
		ClassesBreakdown:     classBreakdown,
		PropertiesBreakdown:  propBreakdown,
	}

	return &domain.ModelView{
		ModelID:    modelID,
		ProjectID:  projectID,
		Categories: categories,
		Stats:      stats,
	}, nil
}

// buildModelViewVersion assembles a complete ModelView from archived release rows.
func (r *resolver) buildModelViewVersion(ctx context.Context, modelID, projectID, version string) (*domain.ModelView, error) {
	// 1. Resolve archived fields with winning overrides + refs.
	fields, err := r.resolveForModelVersion(ctx, modelID, projectID, version)
	if err != nil {
		return nil, err
	}

	// 2. Load archived categories for grouping.
	catRows, err := r.pool.Query(ctx, `
		SELECT id, ui_name, canonical_order
		FROM weave_categories_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY canonical_order, id
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived categories for model view: %w", err)
	}
	defer catRows.Close()

	catMap := make(map[string]*categoryInfo)
	for catRows.Next() {
		var (
			id             string
			uiName         []byte
			canonicalOrder int32
		)
		if err := catRows.Scan(&id, &uiName, &canonicalOrder); err != nil {
			return nil, fmt.Errorf("scan archived category for model view: %w", err)
		}
		catMap[id] = &categoryInfo{
			ID:       id,
			Name:     unmarshalDomainTranslations(uiName),
			Position: int(canonicalOrder),
		}
	}
	if err := catRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived categories for model view: %w", err)
	}

	// 3. Collect unique collection IDs and load archived names where present.
	collIDs := make(map[string]struct{})
	for _, f := range fields {
		if f.PartOfCollectionID != "" {
			collIDs[f.PartOfCollectionID] = struct{}{}
		}
	}

	collNames := make(map[string]domain.Translations, len(collIDs))
	if len(collIDs) > 0 {
		rows, err := r.pool.Query(ctx, `
			SELECT id, ui_name
			FROM weave_collections_archive
			WHERE version_number = $2 AND id = ANY($1)
		`, mapKeys(collIDs), version)
		if err != nil {
			return nil, fmt.Errorf("list archived collections for model view: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var uiName []byte
			if err := rows.Scan(&id, &uiName); err != nil {
				return nil, fmt.Errorf("scan archived collection for model view: %w", err)
			}
			collNames[id] = unmarshalDomainTranslations(uiName)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate archived collections for model view: %w", err)
		}
	}

	// 4. Set IsExternal on each field.
	for i := range fields {
		fields[i].IsExternal = !strings.HasPrefix(fields[i].ID, projectID)
	}

	// 5. Group into category -> collection -> field hierarchy.
	categories := groupByCategory(fields, catMap, collNames)

	// 6. Compute shared path prefix for each collection group. The direct
	// bucket never hoists one — same rationale as buildModelView:
	// direct fields share a category, not an ontology root.
	for i := range categories {
		for j := range categories[i].Collections {
			coll := &categories[i].Collections[j]
			if coll.ID == directKey {
				coll.SharedPathPrefix = nil
				continue
			}
			coll.SharedPathPrefix = ComputeSharedPathPrefix(coll.Fields)
		}
	}

	// 7. Compute stats.
	overridden := 0
	required := 0
	uniqueCollections := make(map[string]struct{})
	valueTypeCounts := make(map[string]int)

	for _, f := range fields {
		if f.OverrideSource != "" {
			overridden++
		}
		if f.IsRequired {
			required++
		}
		if f.ExpectedValueType != "" {
			valueTypeCounts[f.ExpectedValueType]++
		}
		if f.PartOfCollectionID != "" {
			uniqueCollections[f.PartOfCollectionID] = struct{}{}
		}
	}

	catBreakdown, fieldBreakdown, fieldScopeBreakdown, collScopeBreakdown, classBreakdown, propBreakdown, scopesCount := computeBreakdowns(categories, fields)

	stats := domain.ModelViewStats{
		TotalFields:          len(fields),
		TotalCategories:      len(categories),
		TotalCollections:     len(uniqueCollections),
		OverriddenFields:     overridden,
		RequiredFields:       required,
		OptionalFields:       len(fields) - required,
		ValueTypeCounts:      valueTypeCounts,
		ScopesCount:          scopesCount,
		CategoriesBreakdown:  catBreakdown,
		FieldsBreakdown:      fieldBreakdown,
		FieldScopesBreakdown: fieldScopeBreakdown,
		CollScopesBreakdown:  collScopeBreakdown,
		ClassesBreakdown:     classBreakdown,
		PropertiesBreakdown:  propBreakdown,
	}

	return &domain.ModelView{
		ModelID:    modelID,
		ProjectID:  projectID,
		Categories: categories,
		Stats:      stats,
	}, nil
}

// breakdownEntry tracks a name and count for breakdown computation.
type breakdownEntry struct {
	name  string
	count int
}

// computeBreakdowns extracts detailed breakdown statistics from categories and fields.
func computeBreakdowns(categories []domain.CategoryGroup, fields []domain.ResolvedField) (
	catBreakdown, fieldBreakdown, fieldScopeBreakdown, collScopeBreakdown, classBreakdown, propBreakdown []domain.StatItem,
	scopesCount int,
) {
	// Categories breakdown: count fields per category.
	catCounts := make(map[string]breakdownEntry)
	for _, cat := range categories {
		fieldCount := 0
		for _, coll := range cat.Collections {
			fieldCount += len(coll.Fields)
		}
		catCounts[cat.ID] = breakdownEntry{
			name:  cat.Name.Get("en", cat.ID),
			count: fieldCount,
		}
	}

	// Field usage: count occurrences per unique field ID.
	fieldCounts := make(map[string]breakdownEntry)

	// Scope tracking.
	fieldScopes := make(map[string]int)
	collScopes := make(map[string]int)

	// Class/property tracking from PathElements.
	classUsage := make(map[string]breakdownEntry)
	propUsage := make(map[string]breakdownEntry)

	for _, f := range fields {
		// Field usage count.
		fc := fieldCounts[f.ID]
		fc.count++
		if fc.name == "" {
			fc.name = f.DisplayName.Get("en", f.SemanticID)
		}
		fieldCounts[f.ID] = fc

		// Path elements: classes and properties.
		firstClass := true
		seenClasses := make(map[string]bool)
		seenProps := make(map[string]bool)

		for _, elem := range f.PathElements {
			key := elem.LocalName
			if elem.Prefix != "" {
				key = elem.Prefix + ":" + elem.LocalName
			}

			switch elem.Type {
			case "class":
				if !seenClasses[key] {
					seenClasses[key] = true
					e := classUsage[key]
					e.count++
					if e.name == "" {
						e.name = elem.LocalName
					}
					classUsage[key] = e
				}
				// First class in path is the field scope.
				if firstClass {
					firstClass = false
					fieldScopes[key]++
				}
			case "property":
				if !seenProps[key] {
					seenProps[key] = true
					e := propUsage[key]
					e.count++
					if e.name == "" {
						e.name = elem.LocalName
					}
					propUsage[key] = e
				}
			}
		}
	}

	// Collection scopes from category groups.
	for _, cat := range categories {
		for _, coll := range cat.Collections {
			if coll.ID == "" || coll.ID == "__direct__" || len(coll.SharedPathPrefix) == 0 {
				continue
			}
			for _, elem := range coll.SharedPathPrefix {
				if elem.Type == "class" {
					key := elem.LocalName
					if elem.Prefix != "" {
						key = elem.Prefix + ":" + elem.LocalName
					}
					collScopes[key] += len(coll.Fields)
					break
				}
			}
		}
	}

	scopesCount = len(fieldScopes) + len(collScopes)

	// Build sorted StatItem slices.
	catBreakdown = buildBreakdownFromEntries(catCounts)
	fieldBreakdown = buildBreakdownFromEntries(fieldCounts)
	fieldScopeBreakdown = buildBreakdownFromCounts(fieldScopes)
	collScopeBreakdown = buildBreakdownFromCounts(collScopes)
	classBreakdown = buildBreakdownFromEntries(classUsage)
	propBreakdown = buildBreakdownFromEntries(propUsage)

	return
}

// buildBreakdownFromEntries builds a sorted StatItem slice from a map of ID -> breakdownEntry.
func buildBreakdownFromEntries(m map[string]breakdownEntry) []domain.StatItem {
	if len(m) == 0 {
		return nil
	}

	maxCount := 0
	for _, v := range m {
		if v.count > maxCount {
			maxCount = v.count
		}
	}

	items := make([]domain.StatItem, 0, len(m))
	for id, v := range m {
		pct := 0
		if maxCount > 0 {
			pct = (v.count * 100) / maxCount
		}
		items = append(items, domain.StatItem{
			ID:         id,
			Name:       v.name,
			Count:      v.count,
			Percentage: pct,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Count > items[j].Count
	})

	return items
}

// buildBreakdownFromCounts builds a sorted StatItem slice from a map of name -> count.
func buildBreakdownFromCounts(m map[string]int) []domain.StatItem {
	if len(m) == 0 {
		return nil
	}

	maxCount := 0
	for _, c := range m {
		if c > maxCount {
			maxCount = c
		}
	}

	items := make([]domain.StatItem, 0, len(m))
	for name, count := range m {
		pct := 0
		if maxCount > 0 {
			pct = (count * 100) / maxCount
		}
		items = append(items, domain.StatItem{
			ID:         name,
			Name:       name,
			Count:      count,
			Percentage: pct,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Count > items[j].Count
	})

	return items
}
