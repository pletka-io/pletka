package attribution

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Input carries the mutable fields of an attribution row when
// curators edit through the settings panel.
type Input struct {
	ActorID string
	Kind    string
	Note    string
}

// Store is the slice-private data-access surface for project
// attributions. Implementations group rows by kind ordered by
// position so callers can render an "Authors / Funders / Adopters"
// block without further sorting.
type Store interface {
	ListForProject(ctx context.Context, projectID string) ([]domain.Attribution, error)
	ListForProjects(ctx context.Context, projectIDs []string) (map[string][]domain.Attribution, error)

	// ListAllActors returns every actor in the system as
	// ActorOption rows for the create form's actor picker. Drives
	// /projects/{id}/settings/attributions/options/actors.
	ListAllActors(ctx context.Context) ([]ActorOption, error)

	// Create appends an attribution to the next position within
	// (project, kind). Returns the created row including its position.
	Create(ctx context.Context, projectID string, in Input) (domain.Attribution, error)

	// CreateAt inserts at an explicit position. Used by the loader
	// wave to preserve source-list ordering verbatim. Upserts on the
	// composite key so re-runs are idempotent.
	CreateAt(ctx context.Context, projectID, actorID, kind string, position int, note string) (domain.Attribution, error)

	// UpdateNote edits only the freeform note. Actor + kind + position
	// are immutable through this API; curators delete + re-add to
	// move an actor across kinds.
	UpdateNote(ctx context.Context, projectID, actorID, kind string, position int, note string) error

	// Delete removes a single row identified by the composite key.
	Delete(ctx context.Context, projectID, actorID, kind string, position int) error

	// DeleteByKind wipes a (project, kind) bucket. The loader uses
	// this before re-inserting; the settings UI does not.
	DeleteByKind(ctx context.Context, projectID, kind string) error

	// SetPosition reassigns a single row's position within its
	// (project, kind) bucket. The settings reorder handler calls
	// this in sequence per row.
	SetPosition(ctx context.Context, projectID, actorID, kind string, oldPosition, newPosition int) error
}

// Reader is the narrow read-only surface other slices consume to
// surface attributions without taking a dependency on the full Store.
// projectpage uses this to fold credits into the overview schema.
type Reader interface {
	ListForProject(ctx context.Context, projectID string) ([]domain.Attribution, error)
}

// ActorOption is one entry in the actor-picker dropdown on the
// create form.
type ActorOption struct {
	ID          string
	DisplayName string
	Type        string
	Slug        string
}
