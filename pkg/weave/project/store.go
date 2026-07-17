package project

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// AboutUpdate carries the four About fields editable from the project
// settings About section. The slice service validates these against
// the schema before calling Store.UpdateAbout.
type AboutUpdate struct {
	License string              `json:"license"`
	README  domain.Translations `json:"readme"`
	Topics  []string            `json:"topics"`
	BaseURL string              `json:"base_url"`
}

// Store is the data-access contract for projects. The interface is
// roughly congruent with domain.WeaveProjectStore but lives inside the
// slice so handlers/services depend only on this package.
//
// The full CRUD is exposed even though the read-only first-landing
// commit doesn't wire all of it through to HTTP — keeps the contract
// stable so write handlers can land in a follow-up without touching
// the interface.
type Store interface {
	// --- CRUD ---

	Create(ctx context.Context, p *domain.Project) error

	// GetByID returns (nil, nil) when no row matches. id is the project's
	// IDPrefix (weave_projects.id IS the prefix).
	GetByID(ctx context.Context, id string) (*domain.Project, error)

	Update(ctx context.Context, p *domain.Project) error

	// UpdateAbout writes only the four About columns (license, readme,
	// topics, base_url) plus updated_at. Does not touch identity or
	// access-control fields. Returns the updated project.
	UpdateAbout(ctx context.Context, projectID string, in AboutUpdate) (*domain.Project, error)

	Delete(ctx context.Context, id string) error

	// List returns projects + total count for pagination. opts honours
	// search, institution_id filter, sort, limit, offset.
	List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Project, int64, error)

	// ListChildren returns the projects whose parent_project_id == parentID.
	ListChildren(ctx context.Context, parentID string) ([]*domain.Project, error)

	// --- Read composition ---

	// StatsForProjects returns entity counts (fields, models, collections,
	// categories) per project ID. Empty input returns empty map.
	StatsForProjects(ctx context.Context, ids []string) (map[string]*domain.WeaveProjectStats, error)

	// OwnersForProjects returns the owning institution actor (if any)
	// per project ID.
	OwnersForProjects(ctx context.Context, ids []string) (map[string]*domain.ProjectActor, error)

	// ListOwnerInstitutions returns all institutions that own at least
	// one project, sorted by display name. Used to populate the
	// institution filter on the project list.
	ListOwnerInstitutions(ctx context.Context) ([]*domain.ProjectActor, error)

	// --- Hierarchy ---

	// GetParentID returns the parent project's ID, or (nil, nil) when
	// the project has no parent.
	GetParentID(ctx context.Context, projectID string) (*string, error)
}
