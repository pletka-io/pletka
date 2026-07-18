package apikey

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
)

type fakeStore struct {
	byHash  map[string]*domain.APIKey
	touched []string
}

func newFakeStore() *fakeStore { return &fakeStore{byHash: map[string]*domain.APIKey{}} }

func (f *fakeStore) Create(_ context.Context, k *domain.APIKey) (*domain.APIKey, error) {
	k.CreatedAt = time.Now()
	f.byHash[k.KeyHash] = k
	return k, nil
}
func (f *fakeStore) GetByHash(_ context.Context, hash string) (*domain.APIKey, error) {
	k, ok := f.byHash[hash]
	if !ok {
		return nil, nil
	}
	return k, nil
}
func (f *fakeStore) List(_ context.Context, _ string) ([]*domain.APIKey, error) { return nil, nil }
func (f *fakeStore) Touch(_ context.Context, id string) error {
	f.touched = append(f.touched, id)
	return nil
}
func (f *fakeStore) Revoke(_ context.Context, _ string) (int64, error) { return 0, nil }
func (f *fakeStore) RevokeOwned(_ context.Context, id, actorID string) (int64, error) {
	for _, k := range f.byHash {
		if k.ID == id && k.ActorID == actorID && k.RevokedAt == nil {
			now := time.Now()
			k.RevokedAt = &now
			return 1, nil
		}
	}
	return 0, nil
}

func newTestService(store Store) *Service { return NewService(store, slog.Default()) }

func TestMintFormat(t *testing.T) {
	svc := newTestService(newFakeStore())
	secret, key, err := svc.Mint(context.Background(), "actor1", "laptop", 0)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !strings.HasPrefix(secret, "pk_") {
		t.Fatalf("secret %q missing pk_ prefix", secret)
	}
	if len(secret) < 40 {
		t.Fatalf("secret too short: %d", len(secret))
	}
	sum := sha256.Sum256([]byte(secret))
	if key.KeyHash != hex.EncodeToString(sum[:]) {
		t.Fatal("stored hash is not sha256(secret)")
	}
	if key.KeyPrefix != secret[:8] {
		t.Fatalf("prefix %q != secret[:8] %q", key.KeyPrefix, secret[:8])
	}
	if key.ExpiresAt != nil {
		t.Fatal("ttlDays=0 must mean no expiry")
	}
}

func TestVerify(t *testing.T) {
	store := newFakeStore()
	svc := newTestService(store)
	secret, key, _ := svc.Mint(context.Background(), "actor1", "laptop", 0)

	got, err := svc.Verify(context.Background(), secret)
	if err != nil {
		t.Fatalf("verify valid: %v", err)
	}
	if got.ID != key.ID {
		t.Fatal("verify returned wrong key")
	}
	if len(store.touched) != 1 || store.touched[0] != key.ID {
		t.Fatal("verify must touch last_used_at")
	}

	if _, err := svc.Verify(context.Background(), "pk_nonsense"); err != ErrInvalidKey {
		t.Fatalf("unknown secret: want ErrInvalidKey, got %v", err)
	}

	now := time.Now()
	key.RevokedAt = &now
	if _, err := svc.Verify(context.Background(), secret); err != ErrInvalidKey {
		t.Fatalf("revoked: want ErrInvalidKey, got %v", err)
	}

	key.RevokedAt = nil
	past := now.Add(-time.Hour)
	key.ExpiresAt = &past
	if _, err := svc.Verify(context.Background(), secret); err != ErrInvalidKey {
		t.Fatalf("expired: want ErrInvalidKey, got %v", err)
	}
}

func TestRevokeOwned(t *testing.T) {
	store := newFakeStore()
	svc := newTestService(store)
	_, key, _ := svc.Mint(context.Background(), "actor1", "k", 0)

	n, err := svc.RevokeOwned(context.Background(), "other-actor", key.ID)
	if err != nil || n != 0 {
		t.Fatalf("foreign: n=%d err=%v", n, err)
	}
	n, err = svc.RevokeOwned(context.Background(), "actor1", key.ID)
	if err != nil || n != 1 {
		t.Fatalf("owned: n=%d err=%v", n, err)
	}
	n, err = svc.RevokeOwned(context.Background(), "actor1", key.ID)
	if err != nil || n != 0 {
		t.Fatalf("double revoke: n=%d err=%v, want 0,nil (idempotent)", n, err)
	}
}
