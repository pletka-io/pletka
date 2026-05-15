package weave

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domainweave "github.com/pletka-io/pletka/domain/weave"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

func TestAuthStoreNilGuard(t *testing.T) {
	var store *authStore
	ctx := context.Background()

	if _, err := store.RegisterPerson(ctx, domainweave.RegisterPersonParams{}); err == nil {
		t.Fatalf("nil RegisterPerson() error = nil; want error")
	}
	if _, err := store.Create(ctx, "actor", "hash", nil); err == nil {
		t.Fatalf("nil Create() error = nil; want error")
	}
	if _, err := store.GetByActorID(ctx, "actor"); err == nil {
		t.Fatalf("nil GetByActorID() error = nil; want error")
	}
	if _, err := store.GetByEmail(ctx, "actor@example.test"); err == nil {
		t.Fatalf("nil GetByEmail() error = nil; want error")
	}
	if _, err := store.GetByEmailOrSlug(ctx, "actor"); err == nil {
		t.Fatalf("nil GetByEmailOrSlug() error = nil; want error")
	}
	if _, err := store.GetProfileByActorID(ctx, "actor"); err == nil {
		t.Fatalf("nil GetProfileByActorID() error = nil; want error")
	}
	if err := store.UpdatePassword(ctx, "actor", "hash"); err == nil {
		t.Fatalf("nil UpdatePassword() error = nil; want error")
	}
	if err := store.MarkLogin(ctx, "actor"); err == nil {
		t.Fatalf("nil MarkLogin() error = nil; want error")
	}
	if err := store.SetResetToken(ctx, "actor", "token", time.Now()); err == nil {
		t.Fatalf("nil SetResetToken() error = nil; want error")
	}
	if _, err := store.GetByResetToken(ctx, "token"); err == nil {
		t.Fatalf("nil GetByResetToken() error = nil; want error")
	}
	if err := store.ClearResetToken(ctx, "actor"); err == nil {
		t.Fatalf("nil ClearResetToken() error = nil; want error")
	}
	if err := store.Delete(ctx, "actor"); err == nil {
		t.Fatalf("nil Delete() error = nil; want error")
	}
	if err := store.BumpPermsVersion(ctx, "actor"); err == nil {
		t.Fatalf("nil BumpPermsVersion() error = nil; want error")
	}
}

func TestAuthRecordMapping(t *testing.T) {
	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	token := "reset-token"

	record := authRecordFromRow(sqlcgen.WeaveAuth{
		ActorID:                "actor-1",
		PasswordHash:           "hash",
		EmailVerifiedAt:        pgtype.Timestamptz{Time: now, Valid: true},
		PasswordResetToken:     &token,
		PasswordResetExpiresAt: pgtype.Timestamptz{Time: now.Add(time.Hour), Valid: true},
		LastLoginAt:            pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
		PermsVersion:           3,
		CreatedAt:              now.Add(-time.Hour),
		UpdatedAt:              now,
	})

	if record.ActorID != "actor-1" || record.PasswordHash != "hash" || record.PermsVersion != 3 {
		t.Fatalf("authRecordFromRow() = %#v", record)
	}
	if record.EmailVerifiedAt == nil || !record.EmailVerifiedAt.Equal(now) {
		t.Fatalf("EmailVerifiedAt = %#v", record.EmailVerifiedAt)
	}
	if record.PasswordResetToken == nil || *record.PasswordResetToken != token {
		t.Fatalf("PasswordResetToken = %#v", record.PasswordResetToken)
	}
}

func TestAuthWithActorMappingIncludesRole(t *testing.T) {
	email := "actor@example.test"
	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)

	auth := authWithActorFromEmailRow(sqlcgen.WeaveAuthGetByEmailRow{
		ActorID:      "actor-1",
		PasswordHash: "hash",
		PermsVersion: 2,
		CreatedAt:    now,
		UpdatedAt:    now,
		Email:        &email,
		Slug:         "actor",
		DisplayName:  "Actor",
		ActorRole:    "admin",
	})

	if auth.Email != email || auth.Slug != "actor" || auth.DisplayName != "Actor" || auth.Role != "admin" {
		t.Fatalf("authWithActorFromEmailRow() = %#v", auth)
	}
}
