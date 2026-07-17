package actoradmin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// postgresStore is the pgx + sqlc implementation of Store, backed by the
// weave_actors, weave_project_actors, and weave_auth tables. Constructed via
// NewPostgresStore.
type postgresStore struct {
	queries *sqlcgen.Queries
}

// Compile-time interface check.
var _ Store = (*postgresStore)(nil)

// NewPostgresStore returns a Store backed by the supplied pgx pool.
func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{queries: sqlcgen.New(pool)}
}

func (s *postgresStore) ListActorsByType(ctx context.Context, actorType string) ([]sqlcgen.WeaveActor, error) {
	rows, err := s.queries.WeaveListActorsByType(ctx, actorType)
	if err != nil {
		return nil, fmt.Errorf("list actors by type: %w", err)
	}
	return rows, nil
}

func (s *postgresStore) GetActorByID(ctx context.Context, id string) (*sqlcgen.WeaveActor, error) {
	row, err := s.queries.WeaveGetActorByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get actor by id: %w", err)
	}
	return &row, nil
}

func (s *postgresStore) CreateActor(ctx context.Context, params sqlcgen.WeaveCreateActorParams) (*sqlcgen.WeaveActor, error) {
	row, err := s.queries.WeaveCreateActor(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create actor: %w", err)
	}
	return &row, nil
}

func (s *postgresStore) ProjectCounts(ctx context.Context, ids []string) (map[string]int, error) {
	if len(ids) == 0 {
		return map[string]int{}, nil
	}
	rows, err := s.queries.WeaveCountProjectsByActor(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("project counts: %w", err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.ActorID] = int(row.Count)
	}
	return out, nil
}

func (s *postgresStore) OwnedProjectCounts(ctx context.Context, ids []string) (map[string]int, error) {
	if len(ids) == 0 {
		return map[string]int{}, nil
	}
	rows, err := s.queries.WeaveCountOwnedProjectsByActor(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("owned project counts: %w", err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.ActorID] = int(row.Count)
	}
	return out, nil
}

func (s *postgresStore) MemberCounts(ctx context.Context, institutionIDs []string) (map[string]int, error) {
	if len(institutionIDs) == 0 {
		return map[string]int{}, nil
	}
	rows, err := s.queries.WeaveCountMembersByInstitution(ctx, institutionIDs)
	if err != nil {
		return nil, fmt.Errorf("member counts: %w", err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		if row.ParentID == nil {
			continue
		}
		out[*row.ParentID] = int(row.Count)
	}
	return out, nil
}

func (s *postgresStore) LastLogins(ctx context.Context, ids []string) (map[string]*time.Time, error) {
	if len(ids) == 0 {
		return map[string]*time.Time{}, nil
	}
	rows, err := s.queries.WeaveGetLastLoginsByActor(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("last logins: %w", err)
	}
	out := make(map[string]*time.Time, len(rows))
	for _, row := range rows {
		if row.LastLoginAt.Valid {
			t := row.LastLoginAt.Time
			out[row.ActorID] = &t
		} else {
			out[row.ActorID] = nil
		}
	}
	return out, nil
}
