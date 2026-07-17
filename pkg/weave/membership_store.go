package weave

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type membershipStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

var _ domain.MembershipStore = (*membershipStore)(nil)

// Upsert inserts or updates the membership record for the given actor/scope pair.
func (s *membershipStore) Upsert(ctx context.Context, m domain.Membership) error {
	err := s.queries.WeaveMembershipUpsert(ctx, sqlcgen.WeaveMembershipUpsertParams{
		ActorID:   m.ActorID,
		ScopeType: m.ScopeType,
		ScopeID:   m.ScopeID,
		Role:      m.Role,
	})
	if err != nil {
		return fmt.Errorf("upsert membership: %w", err)
	}
	return nil
}

// Delete removes the membership record for the given actor/scope combination.
func (s *membershipStore) Delete(ctx context.Context, actorID, scopeType, scopeID string) error {
	err := s.queries.WeaveMembershipDelete(ctx, sqlcgen.WeaveMembershipDeleteParams{
		ActorID:   actorID,
		ScopeType: scopeType,
		ScopeID:   scopeID,
	})
	if err != nil {
		return fmt.Errorf("delete membership: %w", err)
	}
	return nil
}

// ListByActor returns all memberships for a given actor.
func (s *membershipStore) ListByActor(ctx context.Context, actorID string) ([]domain.Membership, error) {
	rows, err := s.queries.WeaveMembershipListByActor(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("list memberships by actor: %w", err)
	}
	out := make([]domain.Membership, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Membership{
			ActorID:   r.ActorID,
			ScopeType: r.ScopeType,
			ScopeID:   r.ScopeID,
			Role:      r.Role,
			CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// ListByScope returns all memberships for a given scope (with actor slug and display name).
func (s *membershipStore) ListByScope(ctx context.Context, scopeType, scopeID string) ([]domain.Membership, error) {
	rows, err := s.queries.WeaveMembershipListByScope(ctx, sqlcgen.WeaveMembershipListByScopeParams{
		ScopeType: scopeType,
		ScopeID:   scopeID,
	})
	if err != nil {
		return nil, fmt.Errorf("list memberships by scope: %w", err)
	}
	out := make([]domain.Membership, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Membership{
			ActorID:   r.ActorID,
			ScopeType: r.ScopeType,
			ScopeID:   r.ScopeID,
			Role:      r.Role,
			CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// SnapshotForActor returns the flattened membership + ownership rows for BuildSnapshot.
func (s *membershipStore) SnapshotForActor(ctx context.Context, actorID string) ([]domain.SnapshotRow, error) {
	rows, err := s.queries.WeaveMembershipSnapshotForActor(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("snapshot for actor: %w", err)
	}
	out := make([]domain.SnapshotRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.SnapshotRow{
			Kind:      r.Kind,
			ScopeType: r.ScopeType,
			ScopeID:   r.ScopeID,
			Role:      r.Role,
		})
	}
	return out, nil
}
