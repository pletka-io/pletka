package apikey

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Store is the persistence surface for API keys.
type Store interface {
	Create(ctx context.Context, key *domain.APIKey) (*domain.APIKey, error)
	// GetByHash returns the API key matching the hash, or (nil, nil) if no key matches.
	GetByHash(ctx context.Context, hash string) (*domain.APIKey, error)
	// List returns all keys, or only actorID's keys when actorID is non-empty.
	List(ctx context.Context, actorID string) ([]*domain.APIKey, error)
	// Touch sets last_used_at to now.
	Touch(ctx context.Context, id string) error
	// Revoke revokes by ID or key prefix; returns the number of rows revoked.
	Revoke(ctx context.Context, idOrPrefix string) (int64, error)
}
