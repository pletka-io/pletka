package canonical

import (
	"sort"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Override serializes a FieldOverride and its refs to canonical YAML.
// Excluded from output: id, project_id, created_at, updated_at, staging_id,
// content_hash, semantic_id.
func Override(o *domain.FieldOverride, refs []domain.OverrideRef) ([]byte, error) {
	m := map[string]any{
		"field_id":              o.FieldID,
		"entity_type":           o.EntityType,
		"entity_id":             o.EntityID,
		"position":              o.Position,
		"collection_order":      o.CollectionOrder,
		"display_name":          translationsToMap(o.DisplayName),
		"description":           translationsToMap(o.Description),
		"collection_name":       translationsToMap(o.CollectionName),
		"category_id":           o.CategoryID,
		"part_of_collection_id": o.PartOfCollectionID,
		"set_value":             o.SetValue,
		"is_required":           o.IsRequired,
		"min_occurs":            o.MinOccurs,
		"max_occurs":            nilableInt(o.MaxOccurs),
		"is_hidden":             o.IsHidden,
		"visibility":            o.Visibility,
		"refs":                  refsToList(refs),
	}

	return Encode(m)
}

// translationsToMap converts a Translations to a plain map. Nil becomes empty map.
func translationsToMap(t domain.Translations) map[string]string {
	if t == nil {
		return map[string]string{}
	}
	return map[string]string(t)
}

// nilableInt returns the int value or nil for YAML null rendering.
func nilableInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// refsToList converts OverrideRefs to a sorted list of maps.
// Sorted by (ref_type, position).
func refsToList(refs []domain.OverrideRef) []any {
	if len(refs) == 0 {
		return []any{}
	}

	sorted := make([]domain.OverrideRef, len(refs))
	copy(sorted, refs)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].RefType != sorted[j].RefType {
			return sorted[i].RefType < sorted[j].RefType
		}
		return sorted[i].Position < sorted[j].Position
	})

	result := make([]any, len(sorted))
	for i, r := range sorted {
		result[i] = map[string]any{
			"ref_type":    r.RefType,
			"semantic_id": r.SemanticID,
			"position":    r.Position,
		}
	}

	return result
}
