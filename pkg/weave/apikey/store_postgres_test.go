package apikey

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestAPIKeyRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := NewPostgresStore(pool)

	// weave_api_keys.actor_id references weave_actors: insert a throwaway actor.
	actorID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_actors (id, display_name, slug, email) VALUES ($1, 'apikey test', $2, $3)`,
		actorID, "apikey-test-"+actorID, actorID+"@test.invalid"); err != nil {
		t.Fatalf("insert actor: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, actorID) })

	created, err := store.Create(ctx, &domain.APIKey{
		ID: ids.GenerateULID(), ActorID: actorID, Name: "test",
		KeyHash: "hash-" + actorID, KeyPrefix: "pk_test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.GetByHash(ctx, created.KeyHash)
	if err != nil {
		t.Fatalf("get by hash: %v", err)
	}
	if got.ID != created.ID || got.ActorID != actorID {
		t.Fatalf("round-trip mismatch: got %+v", got)
	}

	if err := store.Touch(ctx, created.ID); err != nil {
		t.Fatalf("touch: %v", err)
	}
	n, err := store.Revoke(ctx, created.KeyPrefix)
	if err != nil || n != 1 {
		t.Fatalf("revoke: n=%d err=%v", n, err)
	}
	got, err = store.GetByHash(ctx, created.KeyHash)
	if err != nil {
		t.Fatalf("get after revoke: %v", err)
	}
	if got.RevokedAt == nil || got.LastUsedAt == nil {
		t.Fatalf("expected revoked_at and last_used_at set, got %+v", got)
	}

	// Verify GetByHash returns (nil, nil) for non-existent hash.
	got, err = store.GetByHash(ctx, "no-such-hash")
	if err != nil || got != nil {
		t.Fatalf("expected (nil, nil) for no-such-hash, got (%+v, %v)", got, err)
	}
}
