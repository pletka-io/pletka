package canonical

import "github.com/pletka-io/pletka/pkg/domain"

// Collection serializes a domain.Collection to canonical YAML.
// Excluded from output: id, project_id, created_at, updated_at, staging_id.
func Collection(c *domain.Collection) ([]byte, error) {
	data := map[string]any{
		"semantic_id":                c.SemanticID,
		"system_name":                c.SystemName,
		"ui_name":                    translationsToMap(c.UIName),
		"description":                translationsToMap(c.Description),
		"ontology_scope":             c.OntologyScope.PrefixedName(),
		"status":                     c.Status,
		"deprecated":                 c.Deprecated,
		"collection_number":          c.CollectionNumber,
		"canonical_collection_order": c.CanonicalCollectionOrder,
	}

	return Encode(data)
}
