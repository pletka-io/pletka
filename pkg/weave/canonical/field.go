package canonical

import "github.com/pletka-io/pletka/pkg/domain"

// Field serializes a domain.Field to canonical YAML.
// Excluded from output: id (= semantic_id for weave fields), project_id,
// created_at, updated_at, staging_id.
//
// SetValue and CategoryID moved to weave_field_overrides (base override
// row) in migration 027. Canonicalisation of those values lives on the
// override serializer, not on the field.
//
// path_elements, ontology_scope, and subfield_paths are serialized as full
// element maps (the JSONB contract), not compact qname strings — restore
// must be lossless: every element attribute is there for a reason.
// ontology_path stays as the derived human-readable string.
func Field(f *domain.Field) ([]byte, error) {
	pathElements, err := pathElementValues(f.PathElements)
	if err != nil {
		return nil, err
	}
	scope, err := pathElementValue(f.OntologyScope)
	if err != nil {
		return nil, err
	}
	subfields, err := subfieldPathValues(f.SubfieldPaths)
	if err != nil {
		return nil, err
	}

	m := map[string]any{
		"semantic_id":         f.SemanticID,
		"system_name":         f.SystemName,
		"ui_name":             translationsToMap(f.UIName),
		"description":         translationsToMap(f.Description),
		"ontology_scope":      scope,
		"ontology_path":       f.OntologyPath(),
		"path_elements":       pathElements,
		"expected_value_type": f.ExpectedValueType,
		"status":              f.Status,
		"deprecated":          f.Deprecated,
	}
	if subfields != nil {
		m["subfield_paths"] = subfields
	}

	return Encode(m)
}
