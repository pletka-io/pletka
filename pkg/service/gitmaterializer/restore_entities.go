package gitmaterializer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"
)

type canonicalCategoryDoc struct {
	AdoptedFrom    string              `json:"_adopted_from,omitempty"`
	SemanticID     string              `json:"semantic_id"`
	SystemName     string              `json:"system_name,omitempty"`
	UIName         domain.Translations `json:"ui_name,omitempty"`
	Description    domain.Translations `json:"description,omitempty"`
	Status         string              `json:"status,omitempty"`
	Deprecated     bool                `json:"deprecated,omitempty"`
	CanonicalOrder int                 `json:"canonical_order,omitempty"`
}

type canonicalFieldDoc struct {
	AdoptedFrom       string                `json:"_adopted_from,omitempty"`
	SemanticID        string                `json:"semantic_id"`
	SystemName        string                `json:"system_name,omitempty"`
	UIName            domain.Translations   `json:"ui_name,omitempty"`
	Description       domain.Translations   `json:"description,omitempty"`
	OntologyScope     string                `json:"ontology_scope,omitempty"`
	PathElements      []string              `json:"path_elements,omitempty"`
	ExpectedValueType string                `json:"expected_value_type,omitempty"`
	Status            string                `json:"status,omitempty"`
	Deprecated        bool                  `json:"deprecated,omitempty"`
	Examples          []domain.FieldExample `json:"examples,omitempty"`
}

type canonicalModelDoc struct {
	AdoptedFrom   string              `json:"_adopted_from,omitempty"`
	SemanticID    string              `json:"semantic_id"`
	SystemName    string              `json:"system_name,omitempty"`
	UIName        domain.Translations `json:"ui_name,omitempty"`
	Description   domain.Translations `json:"description,omitempty"`
	OntologyScope string              `json:"ontology_scope,omitempty"`
	Status        string              `json:"status,omitempty"`
	Deprecated    bool                `json:"deprecated,omitempty"`
}

type canonicalCollectionDoc struct {
	AdoptedFrom              string              `json:"_adopted_from,omitempty"`
	SemanticID               string              `json:"semantic_id"`
	SystemName               string              `json:"system_name,omitempty"`
	UIName                   domain.Translations `json:"ui_name,omitempty"`
	Description              domain.Translations `json:"description,omitempty"`
	OntologyScope            string              `json:"ontology_scope,omitempty"`
	Status                   string              `json:"status,omitempty"`
	Deprecated               bool                `json:"deprecated,omitempty"`
	CollectionNumber         int                 `json:"collection_number,omitempty"`
	CanonicalCollectionOrder int                 `json:"canonical_collection_order,omitempty"`
}

func (m *Materializer) HydrateProjectEntities(ctx context.Context, plan *RestorePlan) error {
	if plan == nil || plan.Snapshot == nil {
		return fmt.Errorf("hydrate project entities: missing snapshot")
	}
	projectID := strings.TrimSpace(plan.ProjectID)
	if projectID == "" {
		return fmt.Errorf("hydrate project entities: missing project id")
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("hydrate project entities: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := m.queries.WithTx(tx)
	for _, item := range plan.Snapshot.Entities.Categories {
		if err := hydrateCategoryFile(ctx, q, projectID, item); err != nil {
			return err
		}
	}
	for _, item := range plan.Snapshot.Entities.Fields {
		if err := hydrateFieldFile(ctx, q, projectID, item); err != nil {
			return err
		}
	}
	for _, item := range plan.Snapshot.Entities.Models {
		if err := hydrateModelFile(ctx, q, projectID, item); err != nil {
			return err
		}
	}
	for _, item := range plan.Snapshot.Entities.Collections {
		if err := hydrateCollectionFile(ctx, q, projectID, item); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("hydrate project entities: commit tx: %w", err)
	}
	return nil
}

func hydrateCategoryFile(ctx context.Context, q *sqlcgen.Queries, projectID string, item SnapshotEntityFile) error {
	doc, err := decodeCanonicalYAML[canonicalCategoryDoc](item.Payload)
	if err != nil {
		return fmt.Errorf("hydrate category %s: decode payload: %w", item.Path, err)
	}
	if strings.TrimSpace(doc.AdoptedFrom) != "" {
		return nil
	}
	semanticID := strings.TrimSpace(doc.SemanticID)
	if semanticID == "" {
		return fmt.Errorf("hydrate category %s: missing semantic_id", item.Path)
	}

	existing, err := q.WeaveGetCategoryByIdentifier(ctx, sqlcgen.WeaveGetCategoryByIdentifierParams{
		ProjectID:  projectID,
		SemanticID: nullableString(semanticID),
	})
	createParams := sqlcgen.WeaveCreateCategoryParams{
		ID:             ids.GenerateULID(),
		SemanticID:     nullableString(semanticID),
		SystemName:     nullableString(doc.SystemName),
		UiName:         marshalTranslationsForRestore(doc.UIName),
		Description:    marshalTranslationsForRestore(doc.Description),
		Status:         restoreStatus(doc.Status),
		ProjectID:      projectID,
		CanonicalOrder: int32(doc.CanonicalOrder),
	}
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("hydrate category %s: lookup existing: %w", item.Path, err)
		}
		created, err := q.WeaveCreateCategory(ctx, createParams)
		if err != nil {
			return fmt.Errorf("hydrate category %s: create: %w", item.Path, err)
		}
		if err := setDeprecatedStateCategory(ctx, q, created.ID, doc.Deprecated); err != nil {
			return fmt.Errorf("hydrate category %s: set deprecated: %w", item.Path, err)
		}
		return nil
	}

	if _, err := q.WeaveUpdateCategory(ctx, sqlcgen.WeaveUpdateCategoryParams{
		ID:             existing.ID,
		UiName:         marshalTranslationsForRestore(doc.UIName),
		Description:    marshalTranslationsForRestore(doc.Description),
		SystemName:     nullableString(doc.SystemName),
		Status:         restoreStatus(doc.Status),
		CanonicalOrder: int32(doc.CanonicalOrder),
	}); err != nil {
		return fmt.Errorf("hydrate category %s: update: %w", item.Path, err)
	}
	if err := setDeprecatedStateCategory(ctx, q, existing.ID, doc.Deprecated); err != nil {
		return fmt.Errorf("hydrate category %s: set deprecated: %w", item.Path, err)
	}
	return nil
}

func hydrateFieldFile(ctx context.Context, q *sqlcgen.Queries, projectID string, item SnapshotEntityFile) error {
	doc, err := decodeCanonicalYAML[canonicalFieldDoc](item.Payload)
	if err != nil {
		return fmt.Errorf("hydrate field %s: decode payload: %w", item.Path, err)
	}
	if strings.TrimSpace(doc.AdoptedFrom) != "" {
		return nil
	}
	semanticID := strings.TrimSpace(doc.SemanticID)
	if semanticID == "" {
		return fmt.Errorf("hydrate field %s: missing semantic_id", item.Path)
	}
	scope, err := parsePathElement(doc.OntologyScope, "class", 0)
	if err != nil {
		return fmt.Errorf("hydrate field %s: parse ontology_scope: %w", item.Path, err)
	}
	pathElements, err := parsePathElements(doc.PathElements)
	if err != nil {
		return fmt.Errorf("hydrate field %s: parse path_elements: %w", item.Path, err)
	}

	existing, err := q.WeaveGetFieldByIdentifier(ctx, sqlcgen.WeaveGetFieldByIdentifierParams{
		ProjectID:  projectID,
		SemanticID: nullableString(semanticID),
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("hydrate field %s: lookup existing: %w", item.Path, err)
	}

	params := sqlcgen.WeaveCreateFieldParams{
		ID:                ids.GenerateULID(),
		SemanticID:        nullableString(semanticID),
		SystemName:        nullableString(doc.SystemName),
		UiName:            marshalTranslationsForRestore(doc.UIName),
		Description:       marshalTranslationsForRestore(doc.Description),
		Status:            restoreStatus(doc.Status),
		ProjectID:         projectID,
		OntologyScope:     marshalJSONForRestore(scope),
		OntologyPath:      nullableString(pathString(pathElements)),
		PathElements:      marshalJSONForRestore(pathElements),
		ExpectedValueType: nullableString(doc.ExpectedValueType),
		Examples:          marshalJSONForRestore(doc.Examples),
	}

	if errors.Is(err, pgx.ErrNoRows) {
		created, err := q.WeaveCreateField(ctx, params)
		if err != nil {
			return fmt.Errorf("hydrate field %s: create: %w", item.Path, err)
		}
		if err := setDeprecatedStateField(ctx, q, created.ID, doc.Deprecated); err != nil {
			return fmt.Errorf("hydrate field %s: set deprecated: %w", item.Path, err)
		}
		return nil
	}

	if _, err := q.WeaveUpdateField(ctx, sqlcgen.WeaveUpdateFieldParams{
		ID:                existing.ID,
		UiName:            marshalTranslationsForRestore(doc.UIName),
		Description:       marshalTranslationsForRestore(doc.Description),
		SystemName:        nullableString(doc.SystemName),
		Status:            restoreStatus(doc.Status),
		OntologyScope:     marshalJSONForRestore(scope),
		OntologyPath:      nullableString(pathString(pathElements)),
		PathElements:      marshalJSONForRestore(pathElements),
		ExpectedValueType: nullableString(doc.ExpectedValueType),
		Examples:          marshalJSONForRestore(doc.Examples),
	}); err != nil {
		return fmt.Errorf("hydrate field %s: update: %w", item.Path, err)
	}
	if err := setDeprecatedStateField(ctx, q, existing.ID, doc.Deprecated); err != nil {
		return fmt.Errorf("hydrate field %s: set deprecated: %w", item.Path, err)
	}
	return nil
}

func hydrateModelFile(ctx context.Context, q *sqlcgen.Queries, projectID string, item SnapshotEntityFile) error {
	doc, err := decodeCanonicalYAML[canonicalModelDoc](item.Payload)
	if err != nil {
		return fmt.Errorf("hydrate model %s: decode payload: %w", item.Path, err)
	}
	if strings.TrimSpace(doc.AdoptedFrom) != "" {
		return nil
	}
	semanticID := strings.TrimSpace(doc.SemanticID)
	if semanticID == "" {
		return fmt.Errorf("hydrate model %s: missing semantic_id", item.Path)
	}
	scope, err := parsePathElement(doc.OntologyScope, "class", 0)
	if err != nil {
		return fmt.Errorf("hydrate model %s: parse ontology_scope: %w", item.Path, err)
	}

	existing, err := q.WeaveGetModelByIdentifier(ctx, sqlcgen.WeaveGetModelByIdentifierParams{
		ProjectID:  projectID,
		Identifier: semanticID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("hydrate model %s: lookup existing: %w", item.Path, err)
	}

	params := sqlcgen.WeaveCreateModelParams{
		ID:            semanticID,
		SystemName:    nullableString(doc.SystemName),
		UiName:        marshalTranslationsForRestore(doc.UIName),
		Description:   marshalTranslationsForRestore(doc.Description),
		Status:        restoreStatus(doc.Status),
		ProjectID:     projectID,
		OntologyScope: marshalJSONForRestore(scope),
	}
	if errors.Is(err, pgx.ErrNoRows) {
		created, err := q.WeaveCreateModel(ctx, params)
		if err != nil {
			return fmt.Errorf("hydrate model %s: create: %w", item.Path, err)
		}
		if err := setDeprecatedStateModel(ctx, q, created.ID, doc.Deprecated); err != nil {
			return fmt.Errorf("hydrate model %s: set deprecated: %w", item.Path, err)
		}
		return nil
	}

	if _, err := q.WeaveUpdateModel(ctx, sqlcgen.WeaveUpdateModelParams{
		ID:            existing.ID,
		UiName:        marshalTranslationsForRestore(doc.UIName),
		Description:   marshalTranslationsForRestore(doc.Description),
		SystemName:    nullableString(doc.SystemName),
		Status:        restoreStatus(doc.Status),
		OntologyScope: marshalJSONForRestore(scope),
	}); err != nil {
		return fmt.Errorf("hydrate model %s: update: %w", item.Path, err)
	}
	if err := setDeprecatedStateModel(ctx, q, existing.ID, doc.Deprecated); err != nil {
		return fmt.Errorf("hydrate model %s: set deprecated: %w", item.Path, err)
	}
	return nil
}

func hydrateCollectionFile(ctx context.Context, q *sqlcgen.Queries, projectID string, item SnapshotEntityFile) error {
	doc, err := decodeCanonicalYAML[canonicalCollectionDoc](item.Payload)
	if err != nil {
		return fmt.Errorf("hydrate collection %s: decode payload: %w", item.Path, err)
	}
	if strings.TrimSpace(doc.AdoptedFrom) != "" {
		return nil
	}
	semanticID := strings.TrimSpace(doc.SemanticID)
	if semanticID == "" {
		return fmt.Errorf("hydrate collection %s: missing semantic_id", item.Path)
	}
	scope, err := parsePathElement(doc.OntologyScope, "class", 0)
	if err != nil {
		return fmt.Errorf("hydrate collection %s: parse ontology_scope: %w", item.Path, err)
	}

	existing, err := q.WeaveGetCollectionByIdentifier(ctx, sqlcgen.WeaveGetCollectionByIdentifierParams{
		ProjectID:  projectID,
		Identifier: semanticID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("hydrate collection %s: lookup existing: %w", item.Path, err)
	}

	params := sqlcgen.WeaveCreateCollectionParams{
		ID:                       semanticID,
		SystemName:               nullableString(doc.SystemName),
		UiName:                   marshalTranslationsForRestore(doc.UIName),
		Description:              marshalTranslationsForRestore(doc.Description),
		Status:                   restoreStatus(doc.Status),
		ProjectID:                projectID,
		OntologyScope:            marshalJSONForRestore(scope),
		CollectionNumber:         int32Ptr(doc.CollectionNumber),
		CanonicalCollectionOrder: int32Ptr(doc.CanonicalCollectionOrder),
	}
	if errors.Is(err, pgx.ErrNoRows) {
		created, err := q.WeaveCreateCollection(ctx, params)
		if err != nil {
			return fmt.Errorf("hydrate collection %s: create: %w", item.Path, err)
		}
		if err := setDeprecatedStateCollection(ctx, q, created.ID, doc.Deprecated); err != nil {
			return fmt.Errorf("hydrate collection %s: set deprecated: %w", item.Path, err)
		}
		return nil
	}

	if _, err := q.WeaveUpdateCollection(ctx, sqlcgen.WeaveUpdateCollectionParams{
		ID:                       existing.ID,
		UiName:                   marshalTranslationsForRestore(doc.UIName),
		Description:              marshalTranslationsForRestore(doc.Description),
		SystemName:               nullableString(doc.SystemName),
		Status:                   restoreStatus(doc.Status),
		OntologyScope:            marshalJSONForRestore(scope),
		CollectionNumber:         int32Ptr(doc.CollectionNumber),
		CanonicalCollectionOrder: int32Ptr(doc.CanonicalCollectionOrder),
	}); err != nil {
		return fmt.Errorf("hydrate collection %s: update: %w", item.Path, err)
	}
	if err := setDeprecatedStateCollection(ctx, q, existing.ID, doc.Deprecated); err != nil {
		return fmt.Errorf("hydrate collection %s: set deprecated: %w", item.Path, err)
	}
	return nil
}

func marshalTranslationsForRestore(t domain.Translations) []byte {
	if t == nil {
		return nil
	}
	raw, _ := json.Marshal(t)
	return raw
}

func marshalJSONForRestore(v any) []byte {
	if v == nil {
		return nil
	}
	raw, _ := json.Marshal(v)
	return raw
}

func restoreStatus(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "draft"
	}
	return v
}

func parsePathElements(items []string) ([]domain.PathElement, error) {
	out := make([]domain.PathElement, 0, len(items))
	for i, item := range items {
		pe, err := parsePathElement(item, "", i)
		if err != nil {
			return nil, err
		}
		out = append(out, pe)
	}
	return out, nil
}

func parsePathElement(input, defaultType string, position int) (domain.PathElement, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return domain.PathElement{}, nil
	}
	parts := strings.Split(input, "/")
	primary, err := parseSingleTypeRef(parts[0])
	if err != nil {
		return domain.PathElement{}, err
	}
	pe := domain.PathElement{
		Type:      defaultType,
		URI:       primary.PrefixedName(),
		Prefix:    primary.Prefix,
		LocalName: primary.LocalName,
		ClassCode: primary.ClassCode,
		Position:  position,
	}
	for _, raw := range parts[1:] {
		ref, err := parseSingleTypeRef(raw)
		if err != nil {
			return domain.PathElement{}, err
		}
		pe.AdditionalTypes = append(pe.AdditionalTypes, ref)
	}
	if pe.Type == "" {
		switch pe.Prefix {
		case "xsd", "rdf":
			pe.Type = "literal"
		default:
			pe.Type = "property"
		}
	}
	return pe, nil
}

func parseSingleTypeRef(input string) (domain.TypeRef, error) {
	input = strings.TrimSpace(input)
	parts := strings.SplitN(input, ":", 2)
	if len(parts) != 2 {
		return domain.TypeRef{}, fmt.Errorf("invalid prefixed name %q", input)
	}
	return domain.TypeRef{
		URI:       input,
		Prefix:    strings.TrimSpace(parts[0]),
		LocalName: strings.TrimSpace(parts[1]),
		ClassCode: strings.TrimSpace(parts[1]),
	}, nil
}

func pathString(elements []domain.PathElement) string {
	if len(elements) == 0 {
		return ""
	}
	var b strings.Builder
	for _, pe := range elements {
		b.WriteString("->")
		b.WriteString(pe.PrefixedName())
	}
	return b.String()
}

func setDeprecatedStateCategory(ctx context.Context, q *sqlcgen.Queries, id string, deprecated bool) error {
	if deprecated {
		return q.WeaveDeprecateCategory(ctx, id)
	}
	return q.WeaveActivateCategory(ctx, id)
}

func setDeprecatedStateField(ctx context.Context, q *sqlcgen.Queries, id string, deprecated bool) error {
	if deprecated {
		return q.WeaveDeprecateField(ctx, id)
	}
	return q.WeaveActivateField(ctx, id)
}

func setDeprecatedStateModel(ctx context.Context, q *sqlcgen.Queries, id string, deprecated bool) error {
	if deprecated {
		return q.WeaveDeprecateModel(ctx, id)
	}
	return q.WeaveActivateModel(ctx, id)
}

func setDeprecatedStateCollection(ctx context.Context, q *sqlcgen.Queries, id string, deprecated bool) error {
	if deprecated {
		return q.WeaveDeprecateCollection(ctx, id)
	}
	return q.WeaveActivateCollection(ctx, id)
}

func int32Ptr(v int) *int32 {
	val := int32(v)
	return &val
}

func hasAdoptionMarker(payload []byte) bool {
	var raw map[string]any
	if err := yaml.Unmarshal(payload, &raw); err != nil {
		return false
	}
	_, ok := raw["_adopted_from"]
	return ok
}
