package domain

import "fmt"

// PathSpec identifies an entity to compute a file path for.
type PathSpec struct {
	EntityType string // "project", "category", "field", "model", "collection",
	//                   "base_override", "model_override", "collection_override"
	EntityID  string // semantic ID for entities; bigserial-as-string for overrides
	FieldID   string // semantic ID of the field the override belongs to
	OwnerType string // "model" or "collection" for scoped overrides
	OwnerID   string // semantic ID of the owning model/collection
}

// FilePath returns the path relative to the project working tree where
// an entity should be materialized.
func FilePath(spec PathSpec) string {
	switch spec.EntityType {
	case "project":
		return "project.yaml"
	case "category":
		return fmt.Sprintf("categories/%s.yaml", spec.EntityID)
	case "field":
		return fmt.Sprintf("fields/%s/field.yaml", spec.EntityID)
	case "model":
		return fmt.Sprintf("models/%s/model.yaml", spec.EntityID)
	case "collection":
		return fmt.Sprintf("collections/%s/collection.yaml", spec.EntityID)
	case "base_override":
		return fmt.Sprintf("fields/%s/base-override.yaml", spec.FieldID)
	case "model_override":
		return fmt.Sprintf("models/%s/overrides/%s@%s.yaml",
			spec.OwnerID, spec.FieldID, spec.EntityID)
	case "collection_override":
		return fmt.Sprintf("collections/%s/overrides/%s@%s.yaml",
			spec.OwnerID, spec.FieldID, spec.EntityID)
	default:
		return fmt.Sprintf("unknown/%s-%s.yaml", spec.EntityType, spec.EntityID)
	}
}
