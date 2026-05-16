package weave

import (
	"context"

	"github.com/pletka-io/pletka/domain"
)

const (
	ModelTypeCore      = "core"
	ModelTypeAuxiliary = "auxiliary"
	ModelTypeExample   = "example"
)

// Model represents an entity type definition.
type Model struct {
	domain.Entity

	OntologyScope domain.PathElement `json:"ontology_scope"`
	ModelType     string             `json:"model_type"`
	StagingID     *int64             `json:"staging_id,omitempty"`
}

// ModelTypes returns the canonical list of allowed model type values.
func ModelTypes() []string {
	return []string{ModelTypeCore, ModelTypeAuxiliary, ModelTypeExample}
}

// IsValidModelType reports whether modelType is recognised.
func IsValidModelType(modelType string) bool {
	switch modelType {
	case "", ModelTypeCore, ModelTypeAuxiliary, ModelTypeExample:
		return true
	default:
		return false
	}
}

// ModelStore reads project-scoped models for materialization.
type ModelStore interface {
	GetByID(ctx context.Context, projectID, id string) (*Model, error)
	GetByIDVersion(ctx context.Context, projectID, id, version string) (*Model, error)
	List(ctx context.Context, projectID string) ([]*Model, error)
	ListVersion(ctx context.Context, projectID, version string) ([]*Model, error)
}
