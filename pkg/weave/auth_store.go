package weave

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type authStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

var _ domain.AuthStore = (*authStore)(nil)

// Create inserts a new auth row for the given actor.
func (s *authStore) Create(ctx context.Context, actorID, hash string, emailVerifiedAt *time.Time) (*domain.AuthRecord, error) {
	row, err := s.queries.WeaveAuthCreate(ctx, sqlcgen.WeaveAuthCreateParams{
		ActorID:         actorID,
		PasswordHash:    hash,
		EmailVerifiedAt: dbutil.TimePtrToTimestamptz(emailVerifiedAt),
	})
	if err != nil {
		return nil, fmt.Errorf("create auth: %w", err)
	}
	return authRowToRecord(row), nil
}

// GetByActorID returns the auth record for the given actor ID, or nil if not found.
func (s *authStore) GetByActorID(ctx context.Context, actorID string) (*domain.AuthRecord, error) {
	row, err := s.queries.WeaveAuthGetByActorID(ctx, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get auth by actor: %w", err)
	}
	return authRowToRecord(row), nil
}

// GetByEmail returns the auth record joined with actor fields for the given email,
// or nil if not found.
func (s *authStore) GetByEmail(ctx context.Context, email string) (*domain.AuthWithActor, error) {
	row, err := s.queries.WeaveAuthGetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get auth by email: %w", err)
	}
	return authWithActorFromEmailRow(row), nil
}

// GetByEmailOrSlug returns the auth record joined with actor fields matching
// either the email or slug, or nil if not found.
func (s *authStore) GetByEmailOrSlug(ctx context.Context, loginIdentifier string) (*domain.AuthWithActor, error) {
	row, err := s.queries.WeaveAuthGetByEmailOrSlug(ctx, dbutil.EmptyToNil(loginIdentifier))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get auth by email-or-slug: %w", err)
	}
	return &domain.AuthWithActor{
		AuthRecord: domain.AuthRecord{
			ActorID:                row.ActorID,
			PasswordHash:           row.PasswordHash,
			EmailVerifiedAt:        dbutil.TimestamptzToTimePtr(row.EmailVerifiedAt),
			PasswordResetToken:     row.PasswordResetToken,
			PasswordResetExpiresAt: dbutil.TimestamptzToTimePtr(row.PasswordResetExpiresAt),
			LastLoginAt:            dbutil.TimestamptzToTimePtr(row.LastLoginAt),
			PermsVersion:           row.PermsVersion,
			CreatedAt:              row.CreatedAt,
			UpdatedAt:              row.UpdatedAt,
		},
		Email:       dbutil.NilToEmpty(row.Email),
		Slug:        row.Slug,
		DisplayName: row.DisplayName,
		Role:        row.ActorRole,
	}, nil
}

// GetProfileByActorID joins auth with the actor row for the given actor ID,
// or nil if not found.
func (s *authStore) GetProfileByActorID(ctx context.Context, actorID string) (*domain.AuthWithActor, error) {
	row, err := s.queries.WeaveAuthGetByActorIDWithActor(ctx, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get profile by actor: %w", err)
	}
	return &domain.AuthWithActor{
		AuthRecord: domain.AuthRecord{
			ActorID:                row.ActorID,
			PasswordHash:           row.PasswordHash,
			EmailVerifiedAt:        dbutil.TimestamptzToTimePtr(row.EmailVerifiedAt),
			PasswordResetToken:     row.PasswordResetToken,
			PasswordResetExpiresAt: dbutil.TimestamptzToTimePtr(row.PasswordResetExpiresAt),
			LastLoginAt:            dbutil.TimestamptzToTimePtr(row.LastLoginAt),
			PermsVersion:           row.PermsVersion,
			CreatedAt:              row.CreatedAt,
			UpdatedAt:              row.UpdatedAt,
		},
		Email:       dbutil.NilToEmpty(row.Email),
		Slug:        row.Slug,
		DisplayName: row.DisplayName,
		Role:        row.ActorRole,
	}, nil
}

// RegisterPerson runs WeaveActorCreatePerson + WeaveAuthCreate in one pgx
// transaction so a partial registration cannot leak a half-created user.
func (s *authStore) RegisterPerson(ctx context.Context, p domain.RegisterPersonParams) (*domain.AuthWithActor, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin register tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	if _, err := qtx.WeaveActorCreatePerson(ctx, sqlcgen.WeaveActorCreatePersonParams{
		ID:          p.ActorID,
		DisplayName: p.DisplayName,
		SystemName:  dbutil.EmptyToNil(p.Slug),
		Slug:        p.Slug,
		Email:       dbutil.EmptyToNil(p.Email),
	}); err != nil {
		return nil, fmt.Errorf("insert person actor: %w", err)
	}

	if _, err := qtx.WeaveAuthCreate(ctx, sqlcgen.WeaveAuthCreateParams{
		ActorID:         p.ActorID,
		PasswordHash:    p.PasswordHash,
		EmailVerifiedAt: dbutil.TimePtrToTimestamptz(nil),
	}); err != nil {
		return nil, fmt.Errorf("insert auth: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit register tx: %w", err)
	}

	return s.GetProfileByActorID(ctx, p.ActorID)
}

// UpdatePassword sets a new password hash for the given actor.
func (s *authStore) UpdatePassword(ctx context.Context, actorID, hash string) error {
	if err := s.queries.WeaveAuthUpdatePassword(ctx, sqlcgen.WeaveAuthUpdatePasswordParams{
		ActorID:      actorID,
		PasswordHash: hash,
	}); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// MarkLogin records the current time as the last login time for the actor.
func (s *authStore) MarkLogin(ctx context.Context, actorID string) error {
	if err := s.queries.WeaveAuthMarkLogin(ctx, actorID); err != nil {
		return fmt.Errorf("mark login: %w", err)
	}
	return nil
}

// SetResetToken stores a password-reset token and expiry for the actor.
func (s *authStore) SetResetToken(ctx context.Context, actorID, token string, expires time.Time) error {
	if err := s.queries.WeaveAuthSetResetToken(ctx, sqlcgen.WeaveAuthSetResetTokenParams{
		ActorID:                actorID,
		PasswordResetToken:     &token,
		PasswordResetExpiresAt: dbutil.TimePtrToTimestamptz(&expires),
	}); err != nil {
		return fmt.Errorf("set reset token: %w", err)
	}
	return nil
}

// GetByResetToken looks up an auth row by reset token (must not be expired),
// or returns nil if not found or expired.
func (s *authStore) GetByResetToken(ctx context.Context, token string) (*domain.AuthRecord, error) {
	row, err := s.queries.WeaveAuthGetByResetToken(ctx, &token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get by reset token: %w", err)
	}
	return authRowToRecord(row), nil
}

// ClearResetToken removes the password-reset token and expiry from the auth row.
func (s *authStore) ClearResetToken(ctx context.Context, actorID string) error {
	if err := s.queries.WeaveAuthClearResetToken(ctx, actorID); err != nil {
		return fmt.Errorf("clear reset token: %w", err)
	}
	return nil
}

// Delete removes the auth row for the given actor.
func (s *authStore) Delete(ctx context.Context, actorID string) error {
	if err := s.queries.WeaveAuthDelete(ctx, actorID); err != nil {
		return fmt.Errorf("delete auth: %w", err)
	}
	return nil
}

// BumpPermsVersion increments the perms_version counter for the actor, which
// invalidates any cached permission snapshots.
func (s *authStore) BumpPermsVersion(ctx context.Context, actorID string) error {
	if err := s.queries.WeaveAuthBumpPermsVersion(ctx, actorID); err != nil {
		return fmt.Errorf("bump perms version: %w", err)
	}
	return nil
}

// authRowToRecord converts a sqlcgen.WeaveAuth row to a domain.AuthRecord.
func authRowToRecord(row sqlcgen.WeaveAuth) *domain.AuthRecord {
	return &domain.AuthRecord{
		ActorID:                row.ActorID,
		PasswordHash:           row.PasswordHash,
		EmailVerifiedAt:        dbutil.TimestamptzToTimePtr(row.EmailVerifiedAt),
		PasswordResetToken:     row.PasswordResetToken,
		PasswordResetExpiresAt: dbutil.TimestamptzToTimePtr(row.PasswordResetExpiresAt),
		LastLoginAt:            dbutil.TimestamptzToTimePtr(row.LastLoginAt),
		PermsVersion:           row.PermsVersion,
		CreatedAt:              row.CreatedAt,
		UpdatedAt:              row.UpdatedAt,
	}
}

// authWithActorFromEmailRow converts a WeaveAuthGetByEmailRow to domain.AuthWithActor.
func authWithActorFromEmailRow(row sqlcgen.WeaveAuthGetByEmailRow) *domain.AuthWithActor {
	return &domain.AuthWithActor{
		AuthRecord: domain.AuthRecord{
			ActorID:                row.ActorID,
			PasswordHash:           row.PasswordHash,
			EmailVerifiedAt:        dbutil.TimestamptzToTimePtr(row.EmailVerifiedAt),
			PasswordResetToken:     row.PasswordResetToken,
			PasswordResetExpiresAt: dbutil.TimestamptzToTimePtr(row.PasswordResetExpiresAt),
			LastLoginAt:            dbutil.TimestamptzToTimePtr(row.LastLoginAt),
			PermsVersion:           row.PermsVersion,
			CreatedAt:              row.CreatedAt,
			UpdatedAt:              row.UpdatedAt,
		},
		Email:       dbutil.NilToEmpty(row.Email),
		Slug:        row.Slug,
		DisplayName: row.DisplayName,
	}
}
