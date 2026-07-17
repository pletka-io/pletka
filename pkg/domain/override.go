package domain

import (
	"fmt"
	"time"
)

// FieldOverride holds display properties for a field in a specific context.
// EntityType="" is the base definition. "model"/"collection" rows are overrides.
type FieldOverride struct {
	ID        int64  `json:"id"`
	FieldID   string `json:"field_id"`
	ProjectID string `json:"project_id"`

	// EntityType differentiates base (""), model, and collection overrides.
	EntityType string `json:"entity_type"`
	// EntityID is the weave_models.id or weave_collections.id this override belongs to.
	EntityID string `json:"entity_id"`

	Position        int `json:"position"`
	CollectionOrder int `json:"collection_order"`

	DisplayName    Translations `json:"display_name,omitempty"`
	Description    Translations `json:"description,omitempty"`
	CollectionName Translations `json:"collection_name,omitempty"`

	CategoryID         string `json:"category_id,omitempty"`
	PartOfCollectionID string `json:"part_of_collection_id,omitempty"`

	SetValue string `json:"set_value,omitempty"`

	IsRequired bool   `json:"is_required"`
	MinOccurs  int    `json:"min_occurs"`
	MaxOccurs  *int   `json:"max_occurs,omitempty"`
	IsHidden   bool   `json:"is_hidden"`
	Visibility string `json:"visibility,omitempty"`

	// SemanticID is a domain concept (e.g., the field's semantic ID), not an import artifact.
	SemanticID string `json:"semantic_id,omitempty"`

	// StagingID links to the import staging record. Nil for manual edits.
	StagingID *int64 `json:"staging_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AuditType returns the entity type for audit/change tracking purposes.
// Named AuditType (not EntityType) to avoid collision with the EntityType field.
func (o *FieldOverride) AuditType() string { return "override" }

// AuditID returns the entity ID for audit/change tracking purposes.
func (o *FieldOverride) AuditID() string { return fmt.Sprintf("%d", o.ID) }

// OverrideRef links an override to a target entity (model, collection, or concept list)
// that the field expects as its value. Replaces comma-separated semantic ID strings.
type OverrideRef struct {
	OverrideID int64  `json:"override_id"`
	RefType    string `json:"ref_type"`
	TargetID   string `json:"target_id"`
	SemanticID string `json:"semantic_id"`
	Position   int    `json:"position"`
}

// EntityRef is a resolved reference to an entity, ready for API responses.
type EntityRef struct {
	ID         string       `json:"id"`
	SemanticID string       `json:"semantic_id"`
	Name       Translations `json:"name,omitempty"`
	URL        string       `json:"url,omitempty"`
	Origin     Origin       `json:"origin,omitempty"`
}

// ResolvedField is a field with its winning override applied and refs resolved.
type ResolvedField struct {
	ID            string         `json:"id"`
	SemanticID    string         `json:"semantic_id"`
	SystemName    string         `json:"system_name"`
	ProjectID     string         `json:"project_id,omitempty"`
	OntologyPath  string         `json:"ontology_path,omitempty"`
	PathElements  []PathElement  `json:"path_elements,omitempty"`
	SubfieldPaths []SubfieldPath `json:"subfield_paths,omitempty"` // legacy <br><br> additional paths

	DisplayName Translations `json:"display_name"`
	Description Translations `json:"description,omitempty"`
	Position    int          `json:"position"`

	ExpectedValueType string      `json:"expected_value_type,omitempty"`
	SetValue          string      `json:"set_value,omitempty"`
	ResourceModels    []EntityRef `json:"resource_models,omitempty"`
	CollectionModels  []EntityRef `json:"collection_models,omitempty"`
	ConceptLists      []EntityRef `json:"concept_lists,omitempty"`

	IsRequired bool   `json:"is_required"`
	MinOccurs  int    `json:"min_occurs"`
	MaxOccurs  *int   `json:"max_occurs,omitempty"`
	IsHidden   bool   `json:"is_hidden"`
	Visibility string `json:"visibility,omitempty"`
	IsExternal bool   `json:"is_external"` // true when field is from a different project

	// OverrideSource indicates which level provided the winning override.
	OverrideSource string `json:"override_source"`
	// OverrideID is the ID of the winning override row.
	OverrideID int64 `json:"override_id"`

	CategoryID         string       `json:"category_id,omitempty"`
	PartOfCollectionID string       `json:"part_of_collection_id,omitempty"`
	CollectionOrder    int          `json:"collection_order"`
	CollectionName     Translations `json:"collection_name,omitempty"`
}

// AllPaths returns the resolved field's primary path followed by any legacy
// subfield paths, in source order. Generators iterate this to emit one graph
// pattern per path under the same field binding.
func (rf *ResolvedField) AllPaths() [][]PathElement {
	paths := make([][]PathElement, 0, 1+len(rf.SubfieldPaths))
	if len(rf.PathElements) > 0 {
		paths = append(paths, rf.PathElements)
	}
	for _, sf := range rf.SubfieldPaths {
		if len(sf.PathElements) > 0 {
			paths = append(paths, sf.PathElements)
		}
	}
	return paths
}

// ModelView is the complete API response for a model — ready to serialize.
type ModelView struct {
	ModelID    string          `json:"model_id"`
	ProjectID  string          `json:"project_id"`
	Categories []CategoryGroup `json:"categories"`
	Stats      ModelViewStats  `json:"stats"`
}

// CategoryGroup is a display category containing collections of fields.
type CategoryGroup struct {
	ID          string            `json:"id"`
	Name        Translations      `json:"name"`
	Position    int               `json:"position"`
	Collections []CollectionGroup `json:"collections"`
}

// CollectionGroup is a collection within a category.
type CollectionGroup struct {
	ID               string          `json:"id"`
	Name             Translations    `json:"name"`
	Position         int             `json:"position"`
	Fields           []ResolvedField `json:"fields"`
	SharedPathPrefix []PathElement   `json:"shared_path_prefix,omitempty"`

	// Placement carries the group's model-level constraints. nil = no
	// placement row = optional, 0..unbounded, visible.
	Placement *CollectionPlacement `json:"placement,omitempty"`
}

// StatItem represents a single item in a stats breakdown.
type StatItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Count      int    `json:"count"`
	Percentage int    `json:"percentage"`
}

// ModelViewStats provides summary counts for a model view, or a
// field/collection view when the entity type warrants it. The
// model/collection counts (TotalFields, TotalCategories, etc.) are
// the canonical use; field-specific signals
// (ModelsUsing/CollectionsUsing/OverrideCount) are populated only
// when the entity is a field. Frontend StatsTab branches on entity
// type to render the right subset.
type ModelViewStats struct {
	TotalFields      int            `json:"total_fields"`
	TotalCategories  int            `json:"total_categories"`
	TotalCollections int            `json:"total_collections"`
	OverriddenFields int            `json:"overridden_fields"`
	RequiredFields   int            `json:"required_fields"`
	OptionalFields   int            `json:"optional_fields"`
	ValueTypeCounts  map[string]int `json:"value_type_counts"`
	ScopesCount      int            `json:"scopes_count"`

	// Detailed breakdowns
	CategoriesBreakdown  []StatItem `json:"categories_breakdown"`
	FieldsBreakdown      []StatItem `json:"fields_breakdown"`
	FieldScopesBreakdown []StatItem `json:"field_scopes_breakdown"`
	CollScopesBreakdown  []StatItem `json:"coll_scopes_breakdown"`
	ClassesBreakdown     []StatItem `json:"classes_breakdown"`
	PropertiesBreakdown  []StatItem `json:"properties_breakdown"`

	// Field-only signals. Populated by detailview.StatsAPI
	// when entityType="field". Zero on model/collection views.
	ModelsUsing        int `json:"models_using,omitempty"`
	CollectionsUsing   int `json:"collections_using,omitempty"`
	FieldOverrideCount int `json:"field_override_count,omitempty"`
}

// CollectionPlacement carries the constraints of one collection group placed
// in a model's category. Absence of a placement row means
// the zero-value semantics: optional, 0..unbounded, visible. Field-level
// min/max inside the collection are relative to each collection instance;
// these placement-level values are relative to the model.
type CollectionPlacement struct {
	ID           int64  `json:"id"`
	ProjectID    string `json:"project_id"`
	ModelID      string `json:"model_id"`
	CategoryID   string `json:"category_id"`
	CollectionID string `json:"collection_id"`
	IsRequired   bool   `json:"is_required"`
	MinOccurs    int    `json:"min_occurs"`
	MaxOccurs    *int   `json:"max_occurs,omitempty"` // nil = unbounded
	IsHidden     bool   `json:"is_hidden"`
}
