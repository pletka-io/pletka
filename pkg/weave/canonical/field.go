package canonical

import "github.com/pletka-io/pletka/pkg/domain"

// Field serializes a domain.Field to canonical YAML.
// Excluded from output: id (= semantic_id for weave fields), project_id,
// created_at, updated_at, staging_id.
//
// SetValue and CategoryID moved to weave_field_overrides (base override
// row) in migration 027. Canonicalisation of those values lives on the
// override serializer, not on the field.
func Field(f *domain.Field) ([]byte, error) {
	pathElements := make([]any, len(f.PathElements))
	for i, pe := range f.PathElements {
		pathElements[i] = pe.PrefixedName()
	}

	m := map[string]any{
		"semantic_id":         f.SemanticID,
		"system_name":         f.SystemName,
		"ui_name":             translationsToMap(f.UIName),
		"description":         translationsToMap(f.Description),
		"ontology_scope":      f.OntologyScope.PrefixedName(),
		"ontology_path":       f.OntologyPath(),
		"path_elements":       pathElements,
		"expected_value_type": f.ExpectedValueType,
		"status":              f.Status,
		"deprecated":          f.Deprecated,
	}

	return Encode(m)
}
