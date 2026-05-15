package weave

import (
	"context"
	"time"
)

// Membership binds an actor to an org or project scope with a role.
type Membership struct {
	ActorID   string    `json:"actor_id"`
	ScopeType string    `json:"scope_type"`
	ScopeID   string    `json:"scope_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// SnapshotRow is a flattened authorization row for actor, membership, and
// project-owner grants.
type SnapshotRow struct {
	Kind      string `json:"kind"`
	ScopeType string `json:"scope_type"`
	ScopeID   string `json:"scope_id"`
	Role      string `json:"role"`
}

// MembershipStore accesses actor memberships and derived authorization rows.
type MembershipStore interface {
	Upsert(ctx context.Context, membership Membership) error
	Delete(ctx context.Context, actorID, scopeType, scopeID string) error
	ListByActor(ctx context.Context, actorID string) ([]Membership, error)
	ListByScope(ctx context.Context, scopeType, scopeID string) ([]Membership, error)
	SnapshotForActor(ctx context.Context, actorID string) ([]SnapshotRow, error)
}
