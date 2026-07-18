package apikey

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

type postgresStore struct {
	queries *sqlcgen.Queries
}

var _ Store = (*postgresStore)(nil)

// NewPostgresStore returns the pgx-backed API key store.
func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{queries: sqlcgen.New(pool)}
}

func (s *postgresStore) Create(ctx context.Context, key *domain.APIKey) (*domain.APIKey, error) {
	row, err := s.queries.WeaveAPIKeyCreate(ctx, sqlcgen.WeaveAPIKeyCreateParams{
		ID:        key.ID,
		ActorID:   key.ActorID,
		Name:      key.Name,
		KeyHash:   key.KeyHash,
		KeyPrefix: key.KeyPrefix,
		ExpiresAt: dbutil.TimePtrToTimestamptz(key.ExpiresAt),
	})
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}
	return rowToAPIKey(row), nil
}

func (s *postgresStore) GetByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	row, err := s.queries.WeaveAPIKeyGetByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}
	return rowToAPIKey(row), nil
}

func (s *postgresStore) List(ctx context.Context, actorID string) ([]*domain.APIKey, error) {
	var rows []sqlcgen.WeaveApiKey
	var err error
	if actorID == "" {
		rows, err = s.queries.WeaveAPIKeyList(ctx)
	} else {
		rows, err = s.queries.WeaveAPIKeyListByActor(ctx, actorID)
	}
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	keys := make([]*domain.APIKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, rowToAPIKey(row))
	}
	return keys, nil
}

func (s *postgresStore) Touch(ctx context.Context, id string) error {
	if err := s.queries.WeaveAPIKeyTouch(ctx, id); err != nil {
		return fmt.Errorf("touch api key: %w", err)
	}
	return nil
}

func (s *postgresStore) Revoke(ctx context.Context, idOrPrefix string) (int64, error) {
	n, err := s.queries.WeaveAPIKeyRevoke(ctx, idOrPrefix)
	if err != nil {
		return 0, fmt.Errorf("revoke api key: %w", err)
	}
	return n, nil
}

func rowToAPIKey(row sqlcgen.WeaveApiKey) *domain.APIKey {
	return &domain.APIKey{
		ID:         row.ID,
		ActorID:    row.ActorID,
		Name:       row.Name,
		KeyHash:    row.KeyHash,
		KeyPrefix:  row.KeyPrefix,
		CreatedAt:  row.CreatedAt,
		LastUsedAt: dbutil.TimestamptzToTimePtr(row.LastUsedAt),
		ExpiresAt:  dbutil.TimestamptzToTimePtr(row.ExpiresAt),
		RevokedAt:  dbutil.TimestamptzToTimePtr(row.RevokedAt),
	}
}
