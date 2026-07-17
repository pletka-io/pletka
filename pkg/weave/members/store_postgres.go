package members

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// postgresStore is the pgx + sqlc implementation of Store.
type postgresStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

var _ Store = (*postgresStore)(nil)

// NewPostgresStore returns a Store backed by the supplied pgx pool.
func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{queries: sqlcgen.New(pool), pool: pool}
}

func (s *postgresStore) ListByProject(ctx context.Context, projectID string) ([]MemberRow, error) {
	rows, err := s.queries.WeaveMembershipListProjectMembers(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project members: %w", err)
	}
	out := make([]MemberRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, MemberRow{
			ID:          r.ActorID,
			ActorID:     r.ActorID,
			Slug:        r.Slug,
			DisplayName: r.DisplayName,
			Email:       r.Email,
			Role:        r.Role,
			IsOwner:     r.IsOwner,
		})
	}
	return out, nil
}

func (s *postgresStore) ProjectOwnerID(ctx context.Context, projectID string) (string, error) {
	owner, err := s.queries.WeaveProjectOwnerID(ctx, projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("get project owner: %w", err)
	}
	return owner, nil
}

func (s *postgresStore) FindActorByEmailOrSlug(ctx context.Context, identifier string) (*MemberRow, error) {
	row, err := s.queries.WeaveMembershipFindActorByEmailOrSlug(ctx, identifier)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find actor: %w", err)
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

func (s *postgresStore) ListAvailableActors(ctx context.Context, projectID string) ([]MemberRow, error) {
	rows, err := s.queries.WeaveMembershipListAvailableActors(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list available actors: %w", err)
	}
	out := make([]MemberRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, MemberRow{
			ID:          r.ActorID,
			ActorID:     r.ActorID,
			Slug:        r.Slug,
			DisplayName: r.DisplayName,
			Email:       r.Email,
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

func (s *postgresStore) Delete(ctx context.Context, projectID, actorID string) error {
	err := s.queries.WeaveMembershipDelete(ctx, sqlcgen.WeaveMembershipDeleteParams{
		ActorID:   actorID,
		ScopeType: "project",
		ScopeID:   projectID,
	})
	if err != nil {
		return fmt.Errorf("delete membership: %w", err)
	}
	return nil
}
