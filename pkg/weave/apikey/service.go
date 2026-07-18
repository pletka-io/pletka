package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
)

// ErrInvalidKey is returned by Verify for any unknown, revoked, or expired
// secret. Callers must not distinguish the cases (no oracle).
var ErrInvalidKey = errors.New("invalid api key")

// Service mints and verifies API keys. No capability checks: v1 callers are
// the operator CLI (mint/list/revoke) and the auth middleware (verify).
type Service struct {
	store Store
	log   *slog.Logger
}

// NewService returns the API key service.
func NewService(store Store, log *slog.Logger) *Service {
	return &Service{store: store, log: log}
}

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// encodeBase62 encodes b as a big-endian base62 integer string.
func encodeBase62(b []byte) string {
	n := new(big.Int).SetBytes(b)
	base := big.NewInt(62)
	mod := new(big.Int)
	var out []byte
	for n.Sign() > 0 {
		n.DivMod(n, base, mod)
		out = append(out, base62Alphabet[mod.Int64()])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// Mint creates a key for actorID and returns the plaintext secret exactly
// once. ttlDays <= 0 means no expiry.
func (s *Service) Mint(ctx context.Context, actorID, name string, ttlDays int) (string, *domain.APIKey, error) {
	if actorID == "" {
		return "", nil, errors.New("mint api key: actor id required")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("mint api key: %w", err)
	}
	secret := "pk_" + encodeBase62(raw)

	var expiresAt *time.Time
	if ttlDays > 0 {
		t := time.Now().AddDate(0, 0, ttlDays)
		expiresAt = &t
	}
	key, err := s.store.Create(ctx, &domain.APIKey{
		ID:        ids.GenerateULID(),
		ActorID:   actorID,
		Name:      name,
		KeyHash:   hashSecret(secret),
		KeyPrefix: secret[:8],
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return "", nil, err
	}
	return secret, key, nil
}

// Verify resolves a presented secret to its active key and records the use.
func (s *Service) Verify(ctx context.Context, secret string) (*domain.APIKey, error) {
	key, err := s.store.GetByHash(ctx, hashSecret(secret))
	if err != nil || key == nil {
		return nil, ErrInvalidKey
	}
	if !key.Active(time.Now()) {
		return nil, ErrInvalidKey
	}
	if err := s.store.Touch(ctx, key.ID); err != nil {
		s.log.Warn("api key touch failed", "key_id", key.ID, "err", err)
	}
	return key, nil
}

// List returns all keys, or actorID's keys when actorID is non-empty.
func (s *Service) List(ctx context.Context, actorID string) ([]*domain.APIKey, error) {
	return s.store.List(ctx, actorID)
}

// Revoke revokes a key by ID or prefix; returns the number revoked.
func (s *Service) Revoke(ctx context.Context, idOrPrefix string) (int64, error) {
	return s.store.Revoke(ctx, idOrPrefix)
}
