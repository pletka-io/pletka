package weave

import (
	"context"
	"time"

	"github.com/pletka-io/pletka/domain"
)

const (
	FieldOverrideEntityBase       = ""
	FieldOverrideEntityModel      = "model"
	FieldOverrideEntityCollection = "collection"
)

// FieldOverride stores the per-field override metadata used to compose models
// and collections. Nil pointer fields mean "not set" and must remain distinct
// from explicit empty values for faithful release materialization.
type FieldOverride struct {
	ID                 int64               `json:"id"`
	FieldID            string              `json:"field_id"`
	ProjectID          string              `json:"project_id"`
	EntityType         string              `json:"entity_type"`
	EntityID           string              `json:"entity_id"`
	Position           int                 `json:"position"`
	CollectionOrder    int                 `json:"collection_order"`
	DisplayName        domain.Translations `json:"display_name,omitempty"`
	Description        domain.Translations `json:"description,omitempty"`
	CollectionName     domain.Translations `json:"collection_name,omitempty"`
	CategoryID         *string             `json:"category_id,omitempty"`
	PartOfCollectionID *string             `json:"part_of_collection_id,omitempty"`
	ExpectedValueType  *string             `json:"expected_value_type,omitempty"`
	SetValue           *string             `json:"set_value,omitempty"`
	IsRequired         *bool               `json:"is_required,omitempty"`
	MinOccurs          *int                `json:"min_occurs,omitempty"`
	MaxOccurs          *int                `json:"max_occurs,omitempty"`
	IsHidden           *bool               `json:"is_hidden,omitempty"`
	Visibility         *string             `json:"visibility,omitempty"`
	StagingID          *int64              `json:"staging_id,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
	ContentHash        *string             `json:"content_hash,omitempty"`
	VersionNumber      string              `json:"version_number,omitempty"`
	SetValueEntryID    *string             `json:"set_value_entry_id,omitempty"`
}

// OverrideRef links an override to another semantic object, for example a
// set-value entry or ontology target.
type OverrideRef struct {
	OverrideID    int64  `json:"override_id"`
	RefType       string `json:"ref_type"`
	TargetID      string `json:"target_id"`
	SemanticID    string `json:"semantic_id"`
	Position      int    `json:"position"`
	ProjectID     string `json:"project_id,omitempty"`
	VersionNumber string `json:"version_number,omitempty"`
}

// ResolvedFieldSnapshot is the field identity/path data joined onto an
// override for materializers. It intentionally is not a full Field entity.
type ResolvedFieldSnapshot struct {
	ID                string               `json:"id"`
	SemanticID        string               `json:"semantic_id,omitempty"`
	SystemName        string               `json:"system_name,omitempty"`
	OntologyScope     domain.PathElement   `json:"ontology_scope"`
	OntologyPath      string               `json:"ontology_path,omitempty"`
	ExpectedValueType string               `json:"expected_value_type,omitempty"`
	PathElements      []domain.PathElement `json:"path_elements,omitempty"`
}

// ResolvedFieldOverride is one model/collection field slot after joining the
// override row to the field identity/path snapshot.
type ResolvedFieldOverride struct {
	Override FieldOverride         `json:"override"`
	Field    ResolvedFieldSnapshot `json:"field"`
}

// FieldOverrideStore reads the override graph used to compose model and
// collection field layouts for materialization.
type FieldOverrideStore interface {
	ListForModel(ctx context.Context, projectID, modelID string) ([]*ResolvedFieldOverride, error)
	ListForModelVersion(ctx context.Context, projectID, modelID, version string) ([]*ResolvedFieldOverride, error)
	ListForCollection(ctx context.Context, projectID, collectionID string) ([]*ResolvedFieldOverride, error)
	ListForCollectionVersion(ctx context.Context, projectID, collectionID, version string) ([]*ResolvedFieldOverride, error)
	ListBaseForFields(ctx context.Context, projectID string, fieldIDs []string) ([]*FieldOverride, error)
	ListBaseForFieldsVersion(ctx context.Context, projectID, version string, fieldIDs []string) ([]*FieldOverride, error)
	ListRefsForOverrides(ctx context.Context, overrideIDs []int64) (map[int64][]OverrideRef, error)
	ListRefsForOverridesVersion(ctx context.Context, projectID, version string, overrideIDs []int64) (map[int64][]OverrideRef, error)
}
