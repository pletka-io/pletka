package search

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func resolveTargetPathFieldIDs(ctx context.Context, pool *pgxpool.Pool, target selectorTarget, params domain.SearchParams) ([]string, error) {
	if strings.TrimSpace(target.Version) == "" {
		return resolvePathFieldIDs(ctx, pool, []string{target.ProjectID}, params)
	}
	return resolveArchivedPathFieldIDs(ctx, pool, target, params)
}

func resolveArchivedPathFieldIDs(ctx context.Context, pool *pgxpool.Pool, target selectorTarget, params domain.SearchParams) ([]string, error) {
	hasStart := params.PathStartsWith != ""
	hasEnd := params.PathEndsWith != ""
	hasDepth := params.PathDepth > 0
	hasContains := params.PathLocalName != ""

	if !hasStart && !hasEnd && !hasDepth && !hasContains {
		return nil, nil
	}

	rows, err := pool.Query(ctx, `
		SELECT id, path_elements
		FROM weave_fields_archive
		WHERE project_id = $1 AND version_number = $2 AND status IN ('draft', 'published')
	`, target.ProjectID, target.Version)
	if err != nil {
		return nil, fmt.Errorf("query archived field paths: %w", err)
	}
	defer rows.Close()

	startLocal, startPrefixes := parsePath(params.PathStartsWith)
	endLocal, endPrefixes := parsePath(params.PathEndsWith)
	result := make([]string, 0)
	for rows.Next() {
		var fieldID string
		var pathData []byte
		if err := rows.Scan(&fieldID, &pathData); err != nil {
			return nil, fmt.Errorf("scan archived field path: %w", err)
		}
		elements := parsePathElements(pathData)
		if hasContains && !pathContainsElement(elements, params.PathLocalName, params.PathPrefix) {
			continue
		}
		if hasStart && !pathStartsWithElements(elements, startLocal, startPrefixes) {
			continue
		}
		if hasEnd && !pathEndsWithElement(elements, endLocal, endPrefixes) {
			continue
		}
		if hasDepth && len(elements) != params.PathDepth {
			continue
		}
		result = append(result, fieldID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived field paths: %w", err)
	}

	return result, nil
}

func pathContainsElement(elements []domain.PathElement, localName, prefix string) bool {
	for _, element := range elements {
		if strings.HasPrefix(element.LocalName, localName) && (prefix == "" || element.Prefix == prefix) {
			return true
		}
	}
	return false
}

func pathStartsWithElements(elements []domain.PathElement, localNames, prefixes []string) bool {
	if len(localNames) == 0 {
		return true
	}
	if len(elements) < len(localNames) {
		return false
	}
	for i, local := range localNames {
		if !strings.HasPrefix(elements[i].LocalName, local) {
			return false
		}
		if prefixes[i] != "" && elements[i].Prefix != prefixes[i] {
			return false
		}
	}
	return true
}

func pathEndsWithElement(elements []domain.PathElement, localNames, prefixes []string) bool {
	if len(localNames) == 0 || len(elements) == 0 {
		return true
	}
	lastLocal := localNames[len(localNames)-1]
	lastPrefix := prefixes[len(prefixes)-1]
	last := elements[len(elements)-1]
	if !strings.HasPrefix(last.LocalName, lastLocal) {
		return false
	}
	return lastPrefix == "" || last.Prefix == lastPrefix
}

func matchesSearchText(params domain.SearchParams, semanticID, systemName, ontologyPath string, uiName, description domain.Translations) bool {
	query := strings.TrimSpace(strings.ToLower(params.Query))
	if query == "" {
		return true
	}
	if strings.Contains(strings.ToLower(systemName), query) ||
		strings.Contains(strings.ToLower(semanticID), query) ||
		strings.Contains(strings.ToLower(ontologyPath), query) {
		return true
	}
	for _, value := range uiName {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	for _, value := range description {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func matchesOntologyScope(scope *domain.PathElement, classFilter, prefixFilter string) bool {
	if classFilter == "" && prefixFilter == "" {
		return true
	}
	if scope == nil {
		return false
	}
	matches := func(prefix, local string) bool {
		if classFilter != "" && !strings.HasPrefix(local, classFilter) {
			return false
		}
		if prefixFilter != "" && prefix != prefixFilter {
			return false
		}
		return true
	}
	if matches(scope.Prefix, scope.LocalName) {
		return true
	}
	for _, additional := range scope.AdditionalTypes {
		if matches(additional.Prefix, additional.LocalName) {
			return true
		}
	}
	return false
}

func (h *Handler) searchArchivedFields(ctx context.Context, pool *pgxpool.Pool, target selectorTarget, params domain.SearchParams, rootProjectID string) ([]fieldSearchCandidate, error) {
	adoptionCounts := make(map[string]int)
	rows, err := pool.Query(ctx, `
		SELECT field_id, COUNT(DISTINCT entity_id)
		FROM weave_field_overrides_archive
		WHERE project_id = $1 AND version_number = $2 AND entity_type IN ('model', 'collection')
		GROUP BY field_id
	`, target.ProjectID, target.Version)
	if err != nil {
		return nil, fmt.Errorf("query archived field adoption counts: %w", err)
	}
	for rows.Next() {
		var fieldID string
		var count int64
		if err := rows.Scan(&fieldID, &count); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan archived field adoption count: %w", err)
		}
		adoptionCounts[fieldID] = int(count)
	}
	rows.Close()

	categoryByField := make(map[string]string)
	categoryRows, err := pool.Query(ctx, `
		SELECT field_id, COALESCE(category_id, '')
		FROM weave_field_overrides_archive
		WHERE project_id = $1 AND version_number = $2 AND entity_type = ''
	`, target.ProjectID, target.Version)
	if err != nil {
		return nil, fmt.Errorf("query archived field categories: %w", err)
	}
	for categoryRows.Next() {
		var fieldID, categoryID string
		if err := categoryRows.Scan(&fieldID, &categoryID); err != nil {
			categoryRows.Close()
			return nil, fmt.Errorf("scan archived field category: %w", err)
		}
		categoryByField[fieldID] = categoryID
	}
	categoryRows.Close()

	fieldRows, err := pool.Query(ctx, `
		SELECT id, semantic_id, system_name, ui_name, description, ontology_scope, ontology_path, path_elements, expected_value_type, project_id
		FROM weave_fields_archive
		WHERE project_id = $1 AND version_number = $2 AND status IN ('draft', 'published')
	`, target.ProjectID, target.Version)
	if err != nil {
		return nil, fmt.Errorf("query archived fields: %w", err)
	}
	defer fieldRows.Close()

	results := make([]fieldSearchCandidate, 0)
	for fieldRows.Next() {
		var row fieldSearchCandidate
		if err := fieldRows.Scan(&row.ID, &row.SemanticID, &row.SystemName, &row.UIName, &row.Description, &row.OntologyScope, &row.OntologyPath, &row.PathElements, &row.ExpectedValueType, &row.ProjectID); err != nil {
			return nil, fmt.Errorf("scan archived field: %w", err)
		}
		row.IsLocal = row.ProjectID == rootProjectID
		row.Version = target.Version
		row.AdoptionCount = adoptionCounts[row.ID]

		if params.Category != "" && categoryByField[row.ID] != params.Category {
			continue
		}
		if params.ExpectedValue != "" && derefStr(row.ExpectedValueType) != params.ExpectedValue {
			continue
		}
		if len(params.PathFieldIDs) > 0 && !slices.Contains(params.PathFieldIDs, row.ID) {
			continue
		}

		scope := parsePathElement(row.OntologyScope)
		if !matchesOntologyScope(scope, params.OntologyClass, params.OntologyPrefix) {
			continue
		}

		uiName := parseTranslations(row.UIName)
		description := parseTranslations(row.Description)
		if !matchesSearchText(params, derefStr(row.SemanticID), derefStr(row.SystemName), derefStr(row.OntologyPath), uiName, description) {
			continue
		}

		results = append(results, row)
	}
	if err := fieldRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived fields: %w", err)
	}

	return results, nil
}

func (h *Handler) searchArchivedCollections(ctx context.Context, pool *pgxpool.Pool, target selectorTarget, params domain.SearchParams, rootProjectID string) ([]collectionSearchCandidate, error) {
	adoptionCounts := make(map[string]int)
	rows, err := pool.Query(ctx, `
		SELECT part_of_collection_id, COUNT(DISTINCT entity_id)
		FROM weave_field_overrides_archive
		WHERE project_id = $1 AND version_number = $2 AND entity_type = 'model' AND COALESCE(part_of_collection_id, '') != ''
		GROUP BY part_of_collection_id
	`, target.ProjectID, target.Version)
	if err != nil {
		return nil, fmt.Errorf("query archived collection adoption counts: %w", err)
	}
	for rows.Next() {
		var collectionID string
		var count int64
		if err := rows.Scan(&collectionID, &count); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan archived collection adoption count: %w", err)
		}
		adoptionCounts[collectionID] = int(count)
	}
	rows.Close()

	fieldIDsByCollection := make(map[string][]string)
	overrideRows, err := pool.Query(ctx, `
		SELECT entity_id, field_id
		FROM weave_field_overrides_archive
		WHERE project_id = $1 AND version_number = $2 AND entity_type = 'collection'
	`, target.ProjectID, target.Version)
	if err != nil {
		return nil, fmt.Errorf("query archived collection field refs: %w", err)
	}
	for overrideRows.Next() {
		var collectionID, fieldID string
		if err := overrideRows.Scan(&collectionID, &fieldID); err != nil {
			overrideRows.Close()
			return nil, fmt.Errorf("scan archived collection field ref: %w", err)
		}
		fieldIDsByCollection[collectionID] = append(fieldIDsByCollection[collectionID], fieldID)
	}
	overrideRows.Close()

	collectionRows, err := pool.Query(ctx, `
		SELECT id, system_name, ui_name, description, ontology_scope, project_id
		FROM weave_collections_archive
		WHERE project_id = $1 AND version_number = $2 AND status IN ('draft', 'published')
	`, target.ProjectID, target.Version)
	if err != nil {
		return nil, fmt.Errorf("query archived collections: %w", err)
	}
	defer collectionRows.Close()

	results := make([]collectionSearchCandidate, 0)
	for collectionRows.Next() {
		var row collectionSearchCandidate
		if err := collectionRows.Scan(&row.ID, &row.SystemName, &row.UIName, &row.Description, &row.OntologyScope, &row.ProjectID); err != nil {
			return nil, fmt.Errorf("scan archived collection: %w", err)
		}
		row.IsLocal = row.ProjectID == rootProjectID
		row.Version = target.Version
		row.AdoptionCount = adoptionCounts[row.ID]

		if len(params.PathFieldIDs) > 0 {
			matches := false
			for _, fieldID := range fieldIDsByCollection[row.ID] {
				if slices.Contains(params.PathFieldIDs, fieldID) {
					matches = true
					break
				}
			}
			if !matches {
				continue
			}
		}

		scope := parsePathElement(row.OntologyScope)
		if !matchesOntologyScope(scope, params.OntologyClass, params.OntologyPrefix) {
			continue
		}

		uiName := parseTranslations(row.UIName)
		description := parseTranslations(row.Description)
		if !matchesSearchText(params, row.ID, derefStr(row.SystemName), "", uiName, description) {
			continue
		}

		results = append(results, row)
	}
	if err := collectionRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived collections: %w", err)
	}
	return results, nil
}

func fieldRefKey(version, fieldID string) string {
	return strings.TrimSpace(version) + "|" + fieldID
}

func loadFieldExpectedRefsForCandidates(ctx context.Context, q *sqlcgen.Queries, pool *pgxpool.Pool, rootProjectID string, candidates []fieldSearchCandidate) (map[string]fieldExpectedRefs, error) {
	liveRows := make([]sqlcgen.WeaveSearchFieldsRow, 0)
	archivedByVersion := make(map[string][]fieldSearchCandidate)
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Version) == "" {
			liveRows = append(liveRows, sqlcgen.WeaveSearchFieldsRow{ID: candidate.ID})
			continue
		}
		archivedByVersion[candidate.Version] = append(archivedByVersion[candidate.Version], candidate)
	}

	out := make(map[string]fieldExpectedRefs)
	if len(liveRows) > 0 {
		refs, err := loadFieldExpectedRefs(ctx, q, rootProjectID, liveRows)
		if err != nil {
			return nil, err
		}
		for fieldID, ref := range refs {
			out[fieldRefKey("", fieldID)] = ref
		}
	}
	for version, rows := range archivedByVersion {
		refs, err := loadArchivedFieldExpectedRefs(ctx, pool, version, rows)
		if err != nil {
			return nil, err
		}
		for fieldID, ref := range refs {
			out[fieldRefKey(version, fieldID)] = ref
		}
	}
	return out, nil
}

func loadArchivedFieldExpectedRefs(ctx context.Context, pool *pgxpool.Pool, version string, rows []fieldSearchCandidate) (map[string]fieldExpectedRefs, error) {
	if len(rows) == 0 {
		return map[string]fieldExpectedRefs{}, nil
	}
	projectID := rows[0].ProjectID
	fieldIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		fieldIDs = append(fieldIDs, row.ID)
	}

	baseRows, err := pool.Query(ctx, `
		SELECT id, field_id
		FROM weave_field_overrides_archive
		WHERE project_id = $1 AND version_number = $2 AND entity_type = '' AND field_id = ANY($3)
	`, projectID, version, fieldIDs)
	if err != nil {
		return nil, fmt.Errorf("query archived base overrides: %w", err)
	}
	overrideToField := make(map[int64]string)
	overrideIDs := make([]int64, 0)
	for baseRows.Next() {
		var overrideID int64
		var fieldID string
		if err := baseRows.Scan(&overrideID, &fieldID); err != nil {
			baseRows.Close()
			return nil, fmt.Errorf("scan archived base override: %w", err)
		}
		overrideToField[overrideID] = fieldID
		overrideIDs = append(overrideIDs, overrideID)
	}
	baseRows.Close()
	if len(overrideIDs) == 0 {
		return map[string]fieldExpectedRefs{}, nil
	}

	refRows, err := pool.Query(ctx, `
		SELECT override_id, ref_type, target_id, semantic_id
		FROM weave_override_refs_archive
		WHERE project_id = $1 AND version_number = $2 AND override_id = ANY($3)
		ORDER BY override_id, position
	`, projectID, version, overrideIDs)
	if err != nil {
		return nil, fmt.Errorf("query archived override refs: %w", err)
	}
	type archivedRef struct {
		overrideID int64
		refType    string
		targetID   string
		semanticID string
	}
	refsRaw := make([]archivedRef, 0)
	modelIDs := make(map[string]struct{})
	collectionIDs := make(map[string]struct{})
	for refRows.Next() {
		var ref archivedRef
		if err := refRows.Scan(&ref.overrideID, &ref.refType, &ref.targetID, &ref.semanticID); err != nil {
			refRows.Close()
			return nil, fmt.Errorf("scan archived override ref: %w", err)
		}
		refsRaw = append(refsRaw, ref)
		switch ref.refType {
		case "resource_model":
			modelIDs[ref.targetID] = struct{}{}
		case "collection_model":
			collectionIDs[ref.targetID] = struct{}{}
		}
	}
	refRows.Close()

	names := make(map[string]domain.Translations)
	if len(modelIDs) > 0 {
		rows, err := pool.Query(ctx, `
			SELECT id, ui_name
			FROM weave_models_archive
			WHERE project_id = $1 AND version_number = $2 AND id = ANY($3)
		`, projectID, version, mapKeys(modelIDs))
		if err != nil {
			return nil, fmt.Errorf("query archived model names: %w", err)
		}
		for rows.Next() {
			var id string
			var uiName []byte
			if err := rows.Scan(&id, &uiName); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan archived model name: %w", err)
			}
			names[id] = parseTranslations(uiName)
		}
		rows.Close()
	}
	if len(collectionIDs) > 0 {
		rows, err := pool.Query(ctx, `
			SELECT id, ui_name
			FROM weave_collections_archive
			WHERE project_id = $1 AND version_number = $2 AND id = ANY($3)
		`, projectID, version, mapKeys(collectionIDs))
		if err != nil {
			return nil, fmt.Errorf("query archived collection names: %w", err)
		}
		for rows.Next() {
			var id string
			var uiName []byte
			if err := rows.Scan(&id, &uiName); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan archived collection name: %w", err)
			}
			names[id] = parseTranslations(uiName)
		}
		rows.Close()
	}

	out := make(map[string]fieldExpectedRefs, len(fieldIDs))
	for _, row := range refsRaw {
		fieldID := overrideToField[row.overrideID]
		current := out[fieldID]
		sid := domain.ParseSemanticID(row.semanticID)
		url := sid.URL()
		if url == "" {
			url = domain.EntityURLForRefType(projectID, row.refType, row.targetID)
		}
		entityRef := domain.EntityRef{
			ID:         row.targetID,
			SemanticID: row.semanticID,
			Name:       names[row.targetID],
			URL:        url,
		}
		switch row.refType {
		case "resource_model":
			current.resourceModels = append(current.resourceModels, row.targetID)
			current.resourceModelRefs = append(current.resourceModelRefs, entityRef)
		case "collection_model":
			current.collectionModels = append(current.collectionModels, row.targetID)
			current.collectionModelRefs = append(current.collectionModelRefs, entityRef)
		}
		out[fieldID] = current
	}
	return out, nil
}
