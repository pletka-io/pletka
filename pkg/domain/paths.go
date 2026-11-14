package domain

import "fmt"

// PathSpec identifies an entity to compute a file path for.
type PathSpec struct {
	EntityType string // "project", "category", "field", "model", "collection",
	//                   "base_override", "model_override", "collection_override"
	EntityID  string // semantic ID for entities; OverrideToken for scoped overrides
	FieldID   string // semantic ID of the field the override belongs to
	OwnerType string // "model" or "collection" for scoped overrides
	OwnerID   string // semantic ID of the owning model/collection
}

// EntityTypeConceptListPath is the PathSpec.EntityType of a concept-list file.
const EntityTypeConceptListPath = "concept_list"

// EntityTypeVocabularyPath is the PathSpec.EntityType of a vocabulary file.
const EntityTypeVocabularyPath = "vocabulary"

// FilePath returns the path relative to the project working tree where
// an entity should be materialized.
func FilePath(spec PathSpec) string {
	switch spec.EntityType {
	case "project":
		return "project.yaml"
	case "category":
		return fmt.Sprintf("categories/%s.yaml", spec.EntityID)
	case EntityTypeConceptListPath:
		return fmt.Sprintf("concept-lists/%s.yaml", spec.EntityID)
	case EntityTypeVocabularyPath:
		return fmt.Sprintf("vocabularies/%s/vocabulary.yaml", spec.EntityID)
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

// OverrideToken is the part after "@" in a scoped override's file name: the
// placement's category ("_" for none) and, when set, its collection. The name
// follows the placement, not a database row id, so it survives load-git.
func OverrideToken(categoryID, collectionID string) string {
	t := categoryID
	if t == "" {
		t = "_"
	}
	if collectionID != "" {
		t += "@" + collectionID
	}
	return t
}

// VocabularyConceptsPath is the concepts file of a vocabulary.
func VocabularyConceptsPath(id string) string {
	return fmt.Sprintf("vocabularies/%s/concepts.yaml", id)
}

// ConceptListEntriesPath is the entries file of a concept list.
func ConceptListEntriesPath(id string) string {
	return fmt.Sprintf("concept-lists/%s/entries.yaml", id)
}
