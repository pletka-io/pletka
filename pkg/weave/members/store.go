package members

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// MemberRow is one project membership joined with actor display info.
// IsOwner=true marks the actor whose ID matches weave_projects.owner_id
// — they're the project creator and can't be removed via this pane,
// regardless of whether the row is explicit (in weave_memberships) or
// synthesised from owner_id when no explicit row exists yet.
//
// ID is a required identity field for ListManager (svelte-dnd-action
// keys rows by item.id; the {id} URL substitution in row actions also
// reads it). We mirror ActorID into ID at the boundary so the JSON
// shape works with the generic list machinery without leaking the
// actor_id specific name.
type MemberRow struct {
	ID          string `json:"id"`
	ActorID     string `json:"actor_id"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
	Role        string `json:"role"`
	IsOwner     bool   `json:"is_owner,omitempty"`
}

// Store is the data-access contract for the members slice.
type Store interface {
	// ListByProject returns all explicit memberships for projectID
	// plus a synthesised owner row when the project's owner has no
	// explicit membership. IsOwner flagged accordingly.
	ListByProject(ctx context.Context, projectID string) ([]MemberRow, error)

	// FindActorByEmailOrSlug resolves an email or slug to an actor.
	// Returns (nil, nil) when no actor matches.
	FindActorByEmailOrSlug(ctx context.Context, identifier string) (*MemberRow, error)

	// GetActorByID resolves an actor ULID to its display row. Returns
	// (nil, nil) when no actor matches.
	GetActorByID(ctx context.Context, actorID string) (*MemberRow, error)

	// ListAvailableActors returns actors that are not yet members of
	// projectID and are not the project owner. Feeds the Add Member
	// form's autocomplete select. Capped at 500 rows.
	ListAvailableActors(ctx context.Context, projectID string) ([]MemberRow, error)

	// Upsert inserts or updates a membership row.
	Upsert(ctx context.Context, m domain.Membership) error

	// Delete removes a project membership for the given actor.
	Delete(ctx context.Context, projectID, actorID string) error

	// ProjectOwnerID returns the owner_id for a project, or "" when
	// the project has no explicit owner. Used by Service.Remove to
	// reject deletion of the owner's membership.
	ProjectOwnerID(ctx context.Context, projectID string) (string, error)
}
