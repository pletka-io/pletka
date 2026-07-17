package orgmembers

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

type MemberRow struct {
	ID          string `json:"id"`
	ActorID     string `json:"actor_id"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
	Role        string `json:"role"`
	IsOwner     bool   `json:"is_owner,omitempty"`
	OwnerLocked bool   `json:"owner_locked,omitempty"`
}

type Store interface {
	ListByOrg(ctx context.Context, orgID string) ([]MemberRow, error)
	FindActorByEmailOrSlug(ctx context.Context, identifier string) (*MemberRow, error)
	GetActorByID(ctx context.Context, actorID string) (*MemberRow, error)
	ListAvailableActors(ctx context.Context, orgID string) ([]MemberRow, error)
	Upsert(ctx context.Context, m domain.Membership) error
	Delete(ctx context.Context, orgID, actorID string) error
}
