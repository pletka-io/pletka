package canonical

import "github.com/pletka-io/pletka/pkg/domain"

// Model serializes a domain.Model to canonical YAML.
// Excluded from output: id, project_id, created_at, updated_at, staging_id.
func Model(m *domain.Model) ([]byte, error) {
	data := map[string]any{
		"semantic_id":    m.SemanticID,
		"system_name":    m.SystemName,
		"ui_name":        translationsToMap(m.UIName),
		"description":    translationsToMap(m.Description),
		"ontology_scope": m.OntologyScope.PrefixedName(),
		"status":         m.Status,
		"deprecated":     m.Deprecated,
	}

	return Encode(data)
}
