package canonical

import "github.com/pletka-io/pletka/pkg/domain"

// Category serializes a domain.Category to canonical YAML.
// Excluded from output: id, project_id, created_at, updated_at.
func Category(c *domain.Category) ([]byte, error) {
	data := map[string]any{
		"semantic_id":     c.SemanticID,
		"system_name":     c.SystemName,
		"ui_name":         translationsToMap(c.UIName),
		"description":     translationsToMap(c.Description),
		"status":          c.Status,
		"deprecated":      c.Deprecated,
		"canonical_order": c.CanonicalOrder,
	}

	return Encode(data)
}
