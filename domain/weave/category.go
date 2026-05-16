package weave

import (
	"context"

	"github.com/pletka-io/pletka/domain"
)

// Category is a display grouping for fields within a project.
type Category struct {
	domain.Entity
	Origin         domain.Origin `json:"origin,omitempty"`
	CanonicalOrder int           `json:"canonical_order"`
}

// CategoryStore reads project-scoped categories for materialization.
type CategoryStore interface {
	GetByID(ctx context.Context, projectID, id string) (*Category, error)
	GetByIDVersion(ctx context.Context, projectID, id, version string) (*Category, error)
	List(ctx context.Context, projectID string) ([]*Category, error)
	ListVersion(ctx context.Context, projectID, version string) ([]*Category, error)
}
