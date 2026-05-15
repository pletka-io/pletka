package weave

import (
	"context"
	"errors"
	"fmt"

	domainweave "github.com/pletka-io/pletka/domain/weave"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

type membershipStore struct {
	queries *sqlcgen.Queries
}

var _ domainweave.MembershipStore = (*membershipStore)(nil)

// Memberships returns the store slice for actor memberships and authorization snapshots.
func (s *Store) Memberships() domainweave.MembershipStore {
	if s == nil {
		return &membershipStore{}
	}
	return &membershipStore{queries: s.queries}
}

func (s *membershipStore) Upsert(ctx context.Context, membership domainweave.Membership) error {
	queries, err := s.queryFacade()
	if err != nil {
		return err
	}
	err = queries.WeaveMembershipUpsert(ctx, sqlcgen.WeaveMembershipUpsertParams{
		ActorID:   membership.ActorID,
		ScopeType: membership.ScopeType,
		ScopeID:   membership.ScopeID,
		Role:      membership.Role,
	})
	if err != nil {
		return fmt.Errorf("upsert membership: %w", err)
	}
	return nil
}

func (s *membershipStore) Delete(ctx context.Context, actorID, scopeType, scopeID string) error {
	queries, err := s.queryFacade()
	if err != nil {
		return err
	}
	err = queries.WeaveMembershipDelete(ctx, sqlcgen.WeaveMembershipDeleteParams{
		ActorID:   actorID,
		ScopeType: scopeType,
		ScopeID:   scopeID,
	})
	if err != nil {
		return fmt.Errorf("delete membership: %w", err)
	}
	return nil
}

func (s *membershipStore) ListByActor(ctx context.Context, actorID string) ([]domainweave.Membership, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	rows, err := queries.WeaveMembershipListByActor(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("list memberships by actor: %w", err)
	}
	memberships := make([]domainweave.Membership, 0, len(rows))
	for _, row := range rows {
		memberships = append(memberships, membershipFromRow(row))
	}
	return memberships, nil
}

func (s *membershipStore) ListByScope(ctx context.Context, scopeType, scopeID string) ([]domainweave.Membership, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	rows, err := queries.WeaveMembershipListByScope(ctx, sqlcgen.WeaveMembershipListByScopeParams{
		ScopeType: scopeType,
		ScopeID:   scopeID,
	})
	if err != nil {
		return nil, fmt.Errorf("list memberships by scope: %w", err)
	}
	memberships := make([]domainweave.Membership, 0, len(rows))
	for _, row := range rows {
		memberships = append(memberships, membershipFromScopeRow(row))
	}
	return memberships, nil
}

func (s *membershipStore) SnapshotForActor(ctx context.Context, actorID string) ([]domainweave.SnapshotRow, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	rows, err := queries.WeaveMembershipSnapshotForActor(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("snapshot for actor: %w", err)
	}
	snapshot := make([]domainweave.SnapshotRow, 0, len(rows))
	for _, row := range rows {
		snapshot = append(snapshot, snapshotFromRow(row))
	}
	return snapshot, nil
}

func (s *membershipStore) queryFacade() (*sqlcgen.Queries, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("membership store is not initialized")
	}
	return s.queries, nil
}

func membershipFromRow(row sqlcgen.WeaveMembership) domainweave.Membership {
	return domainweave.Membership{
		ActorID:   row.ActorID,
		ScopeType: row.ScopeType,
		ScopeID:   row.ScopeID,
		Role:      row.Role,
		CreatedAt: row.CreatedAt,
	}
}

func membershipFromScopeRow(row sqlcgen.WeaveMembershipListByScopeRow) domainweave.Membership {
	return domainweave.Membership{
		ActorID:   row.ActorID,
		ScopeType: row.ScopeType,
		ScopeID:   row.ScopeID,
		Role:      row.Role,
		CreatedAt: row.CreatedAt,
	}
}

func snapshotFromRow(row sqlcgen.WeaveMembershipSnapshotForActorRow) domainweave.SnapshotRow {
	return domainweave.SnapshotRow{
		Kind:      row.Kind,
		ScopeType: row.ScopeType,
		ScopeID:   row.ScopeID,
		Role:      row.Role,
	}
}
