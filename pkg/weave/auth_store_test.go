package weave_test

import (
	"context"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
)

func TestAuthStore_CreateGetByEmail(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	as := store.Auth()
	ctx := context.Background()

	actorID := seedTestActor(t, pool, "authtest_1")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id = $1`, actorID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE id = $1`, actorID)
	})

	rec, err := as.Create(ctx, actorID, "hashed-pw-abc", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.ActorID != actorID {
		t.Errorf("Create returned actor_id %q want %q", rec.ActorID, actorID)
	}

	byEmail, err := as.GetByEmail(ctx, "authtest_1@test.local")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if byEmail == nil {
		t.Fatal("GetByEmail: nil")
	}
	if byEmail.PasswordHash != "hashed-pw-abc" {
		t.Errorf("GetByEmail password_hash %q want %q", byEmail.PasswordHash, "hashed-pw-abc")
	}
	if byEmail.Slug != "authtest_1" {
		t.Errorf("GetByEmail slug %q want authtest_1", byEmail.Slug)
	}
}

func TestAuthStore_MarkLogin(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	as := store.Auth()
	ctx := context.Background()

	actorID := seedTestActor(t, pool, "authtest_2")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id = $1`, actorID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE id = $1`, actorID)
	})
	if _, err := as.Create(ctx, actorID, "hash", nil); err != nil {
		t.Fatalf("Create: %v", err)
	}

	before := time.Now().Add(-time.Second)
	if err := as.MarkLogin(ctx, actorID); err != nil {
		t.Fatalf("MarkLogin: %v", err)
	}
	rec, _ := as.GetByActorID(ctx, actorID)
	if rec.LastLoginAt == nil || rec.LastLoginAt.Before(before) {
		t.Errorf("LastLoginAt not updated: %v", rec.LastLoginAt)
	}
}

func TestAuthStore_GetByEmail_NotFound(t *testing.T) {
	pool := testPool(t)
	as := weave.NewPostgresStore(pool).Auth()
	ctx := context.Background()

	rec, err := as.GetByEmail(ctx, "nobody@test.local")
	if err != nil {
		t.Fatalf("GetByEmail not-found returned error: %v", err)
	}
	if rec != nil {
		t.Errorf("expected nil for not-found, got %+v", rec)
	}
}

func TestAuthStore_RegisterPerson(t *testing.T) {
	pool := testPool(t)
	as := weave.NewPostgresStore(pool).Auth()
	ctx := context.Background()

	actorID := "ulid_register_" + time.Now().Format("150405")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id = $1`, actorID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE id = $1`, actorID)
	})

	profile, err := as.RegisterPerson(ctx, domain.RegisterPersonParams{
		ActorID:      actorID,
		DisplayName:  "Reg Test",
		Slug:         "regtest_" + time.Now().Format("150405"),
		Email:        "regtest_" + time.Now().Format("150405") + "@test.local",
		PasswordHash: "hashed",
	})
	if err != nil {
		t.Fatalf("RegisterPerson: %v", err)
	}
	if profile == nil || profile.ActorID != actorID {
		t.Errorf("RegisterPerson profile: got %+v", profile)
	}
}

func TestAuthStore_GetByEmailOrSlug(t *testing.T) {
	pool := testPool(t)
	as := weave.NewPostgresStore(pool).Auth()
	ctx := context.Background()

	actorID := seedTestActor(t, pool, "slugtest_1")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id = $1`, actorID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE id = $1`, actorID)
	})
	if _, err := as.Create(ctx, actorID, "hash", nil); err != nil {
		t.Fatalf("Create: %v", err)
	}

	byEmail, err := as.GetByEmailOrSlug(ctx, "slugtest_1@test.local")
	if err != nil || byEmail == nil {
		t.Fatalf("by email: %v, %+v", err, byEmail)
	}
	bySlug, err := as.GetByEmailOrSlug(ctx, "slugtest_1")
	if err != nil || bySlug == nil {
		t.Fatalf("by slug: %v, %+v", err, bySlug)
	}
	if byEmail.ActorID != bySlug.ActorID {
		t.Errorf("mismatch: %q vs %q", byEmail.ActorID, bySlug.ActorID)
	}
}
