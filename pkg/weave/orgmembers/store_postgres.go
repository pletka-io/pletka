package orgmembers

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

type postgresStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

var _ Store = (*postgresStore)(nil)

func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{queries: sqlcgen.New(pool), pool: pool}
}

func (s *postgresStore) ListByOrg(ctx context.Context, orgID string) ([]MemberRow, error) {
	rows, err := s.queries.WeaveMembershipListByScope(ctx, sqlcgen.WeaveMembershipListByScopeParams{
		ScopeType: "org",
		ScopeID:   orgID,
	})
	if err != nil {
		return nil, fmt.Errorf("list org members: %w", err)
	}
	ownerCount := 0
	for _, r := range rows {
		if r.Role == "owner" {
			ownerCount++
		}
	}
	out := make([]MemberRow, 0, len(rows))
	for _, r := range rows {
		email := ""
		actor, err := s.queries.WeaveGetActorByID(ctx, r.ActorID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get actor %s: %w", r.ActorID, err)
		}
		if err == nil && actor.Email != nil {
			email = *actor.Email
		}
		out = append(out, MemberRow{
			ID:          r.ActorID,
			ActorID:     r.ActorID,
			Slug:        r.Slug,
			DisplayName: r.DisplayName,
			Email:       email,
			Role:        r.Role,
			IsOwner:     r.Role == "owner",
			OwnerLocked: r.Role == "owner" && ownerCount <= 1,
		})
	}
	return out, nil
}

func (s *postgresStore) FindActorByEmailOrSlug(ctx context.Context, identifier string) (*MemberRow, error) {
	row, err := s.queries.WeaveMembershipFindActorByEmailOrSlug(ctx, identifier)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find actor: %w", err)
	}
	actor, err := s.queries.WeaveGetActorByID(ctx, row.ActorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get actor: %w", err)
	}
	if actor.Type != "person" {
		return nil, nil
	}
	return &MemberRow{
		ID:          row.ActorID,
		ActorID:     row.ActorID,
		Slug:        row.Slug,
		DisplayName: row.DisplayName,
		Email:       row.Email,
	}, nil
}

func (s *postgresStore) GetActorByID(ctx context.Context, actorID string) (*MemberRow, error) {
	row, err := s.queries.WeaveGetActorByID(ctx, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get actor: %w", err)
	}
	if row.Type != "person" {
		return nil, nil
	}
	email := ""
	if row.Email != nil {
		email = *row.Email
	}
	return &MemberRow{
		ID:          row.ID,
		ActorID:     row.ID,
		Slug:        row.Slug,
		DisplayName: row.DisplayName,
		Email:       email,
	}, nil
}

func (s *postgresStore) ListAvailableActors(ctx context.Context, orgID string) ([]MemberRow, error) {
	actors, err := s.queries.WeaveListActorsByType(ctx, "person")
	if err != nil {
		return nil, fmt.Errorf("list actors by type: %w", err)
	}
	members, err := s.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	memberIDs := make(map[string]struct{}, len(members))
	for _, member := range members {
		memberIDs[member.ActorID] = struct{}{}
	}
	out := make([]MemberRow, 0, len(actors))
	for _, actor := range actors {
		if _, exists := memberIDs[actor.ID]; exists {
			continue
		}
		email := ""
		if actor.Email != nil {
			email = *actor.Email
		}
		out = append(out, MemberRow{
			ID:          actor.ID,
			ActorID:     actor.ID,
			Slug:        actor.Slug,
			DisplayName: actor.DisplayName,
			Email:       email,
		})
	}
	return out, nil
}

func (s *postgresStore) Upsert(ctx context.Context, m domain.Membership) error {
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

func (s *postgresStore) Delete(ctx context.Context, orgID, actorID string) error {
	err := s.queries.WeaveMembershipDelete(ctx, sqlcgen.WeaveMembershipDeleteParams{
		ActorID:   actorID,
		ScopeType: "org",
		ScopeID:   orgID,
	})
	if err != nil {
		return fmt.Errorf("delete membership: %w", err)
	}
	return nil
}
