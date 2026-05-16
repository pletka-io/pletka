package weave

import (
	"context"

	"github.com/pletka-io/pletka/domain"
)

// Collection represents a group of fields sharing a root ontology context.
type Collection struct {
	domain.Entity

	OntologyScope            domain.PathElement `json:"ontology_scope"`
	CollectionNumber         int                `json:"collection_number,omitempty"`
	CanonicalCollectionOrder int                `json:"canonical_collection_order,omitempty"`
	DefaultCategoryID        *string            `json:"default_category_id,omitempty"`
	StagingID                *int64             `json:"staging_id,omitempty"`
}

// CollectionStore reads project-scoped collections for materialization.
type CollectionStore interface {
	GetByID(ctx context.Context, projectID, id string) (*Collection, error)
	GetByIDVersion(ctx context.Context, projectID, id, version string) (*Collection, error)
	List(ctx context.Context, projectID string) ([]*Collection, error)
	ListVersion(ctx context.Context, projectID, version string) ([]*Collection, error)
}
