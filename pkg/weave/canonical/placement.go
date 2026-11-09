package canonical

import "github.com/pletka-io/pletka/pkg/domain"

// Placement serializes a collection placement to canonical YAML.
// Excluded from output: id, project_id, created_at, updated_at.
func Placement(p *domain.CollectionPlacement) ([]byte, error) {
	return Encode(map[string]any{
		"model_id":      p.ModelID,
		"category_id":   p.CategoryID,
		"collection_id": p.CollectionID,
		"is_required":   p.IsRequired,
		"min_occurs":    p.MinOccurs,
		"max_occurs":    nilableInt(p.MaxOccurs),
		"is_hidden":     p.IsHidden,
	})
}
