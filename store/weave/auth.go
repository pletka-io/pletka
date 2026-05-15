package weave

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domainweave "github.com/pletka-io/pletka/domain/weave"
	"github.com/pletka-io/pletka/store/dbutil"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

type authStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

var _ domainweave.AuthStore = (*authStore)(nil)

// Auth returns the store slice for password auth and actor profile login data.
func (s *Store) Auth() domainweave.AuthStore {
	if s == nil {
		return &authStore{}
	}
	return &authStore{queries: s.queries, pool: s.pool}
}

func (s *authStore) RegisterPerson(ctx context.Context, params domainweave.RegisterPersonParams) (*domainweave.AuthWithActor, error) {
	queries, pool, err := s.queryFacadeAndPool()
	if err != nil {
		return nil, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin register person transaction: %w", err)
	}
	defer rollback(ctx, tx)

	qtx := queries.WithTx(tx)
	if _, err := qtx.WeaveActorCreatePerson(ctx, sqlcgen.WeaveActorCreatePersonParams{
		ID:          params.ActorID,
		DisplayName: params.DisplayName,
		SystemName:  dbutil.EmptyToNil(params.Slug),
		Slug:        params.Slug,
		Email:       dbutil.EmptyToNil(params.Email),
	}); err != nil {
		return nil, fmt.Errorf("insert person actor: %w", err)
	}

	if _, err := qtx.WeaveAuthCreate(ctx, sqlcgen.WeaveAuthCreateParams{
		ActorID:         params.ActorID,
		PasswordHash:    params.PasswordHash,
		EmailVerifiedAt: dbutil.TimePtrToTimestamptz(nil),
	}); err != nil {
		return nil, fmt.Errorf("insert auth: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit register person transaction: %w", err)
	}
	return s.GetProfileByActorID(ctx, params.ActorID)
}

func (s *authStore) Create(ctx context.Context, actorID, passwordHash string, emailVerifiedAt *time.Time) (*domainweave.AuthRecord, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	row, err := queries.WeaveAuthCreate(ctx, sqlcgen.WeaveAuthCreateParams{
		ActorID:         actorID,
		PasswordHash:    passwordHash,
		EmailVerifiedAt: dbutil.TimePtrToTimestamptz(emailVerifiedAt),
	})
	if err != nil {
		return nil, fmt.Errorf("create auth: %w", err)
	}
	return authRecordFromRow(row), nil
}

func (s *authStore) GetByActorID(ctx context.Context, actorID string) (*domainweave.AuthRecord, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	row, err := queries.WeaveAuthGetByActorID(ctx, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get auth by actor: %w", err)
	}
	return authRecordFromRow(row), nil
}

func (s *authStore) GetByEmail(ctx context.Context, email string) (*domainweave.AuthWithActor, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	row, err := queries.WeaveAuthGetByEmail(ctx, dbutil.EmptyToNil(email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get auth by email: %w", err)
	}
	return authWithActorFromEmailRow(row), nil
}

func (s *authStore) GetByEmailOrSlug(ctx context.Context, loginIdentifier string) (*domainweave.AuthWithActor, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	row, err := queries.WeaveAuthGetByEmailOrSlug(ctx, dbutil.EmptyToNil(loginIdentifier))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get auth by email-or-slug: %w", err)
	}
	return authWithActorFromEmailOrSlugRow(row), nil
}

func (s *authStore) GetProfileByActorID(ctx context.Context, actorID string) (*domainweave.AuthWithActor, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	row, err := queries.WeaveAuthGetByActorIDWithActor(ctx, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get profile by actor: %w", err)
	}
	return authWithActorFromProfileRow(row), nil
}

func (s *authStore) UpdatePassword(ctx context.Context, actorID, hash string) error {
	queries, err := s.queryFacade()
	if err != nil {
		return err
	}
	if err := queries.WeaveAuthUpdatePassword(ctx, sqlcgen.WeaveAuthUpdatePasswordParams{
		ActorID:      actorID,
		PasswordHash: hash,
	}); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

func (s *authStore) MarkLogin(ctx context.Context, actorID string) error {
	queries, err := s.queryFacade()
	if err != nil {
		return err
	}
	if err := queries.WeaveAuthMarkLogin(ctx, actorID); err != nil {
		return fmt.Errorf("mark login: %w", err)
	}
	return nil
}

func (s *authStore) SetResetToken(ctx context.Context, actorID, token string, expires time.Time) error {
	queries, err := s.queryFacade()
	if err != nil {
		return err
	}
	if err := queries.WeaveAuthSetResetToken(ctx, sqlcgen.WeaveAuthSetResetTokenParams{
		ActorID:                actorID,
		PasswordResetToken:     &token,
		PasswordResetExpiresAt: dbutil.TimePtrToTimestamptz(&expires),
	}); err != nil {
		return fmt.Errorf("set reset token: %w", err)
	}
	return nil
}

func (s *authStore) GetByResetToken(ctx context.Context, token string) (*domainweave.AuthRecord, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	row, err := queries.WeaveAuthGetByResetToken(ctx, &token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get auth by reset token: %w", err)
	}
	return authRecordFromRow(row), nil
}

func (s *authStore) ClearResetToken(ctx context.Context, actorID string) error {
	queries, err := s.queryFacade()
	if err != nil {
		return err
	}
	if err := queries.WeaveAuthClearResetToken(ctx, actorID); err != nil {
		return fmt.Errorf("clear reset token: %w", err)
	}
	return nil
}

func (s *authStore) Delete(ctx context.Context, actorID string) error {
	queries, err := s.queryFacade()
	if err != nil {
		return err
	}
	if err := queries.WeaveAuthDelete(ctx, actorID); err != nil {
		return fmt.Errorf("delete auth: %w", err)
	}
	return nil
}

func (s *authStore) BumpPermsVersion(ctx context.Context, actorID string) error {
	queries, err := s.queryFacade()
	if err != nil {
		return err
	}
	if err := queries.WeaveAuthBumpPermsVersion(ctx, actorID); err != nil {
		return fmt.Errorf("bump perms version: %w", err)
	}
	return nil
}

func (s *authStore) queryFacade() (*sqlcgen.Queries, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("auth store is not initialized")
	}
	return s.queries, nil
}

func (s *authStore) queryFacadeAndPool() (*sqlcgen.Queries, *pgxpool.Pool, error) {
	if s == nil || s.queries == nil || s.pool == nil {
		return nil, nil, errors.New("auth store is not initialized")
	}
	return s.queries, s.pool, nil
}

func authRecordFromRow(row sqlcgen.WeaveAuth) *domainweave.AuthRecord {
	return &domainweave.AuthRecord{
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

func authWithActorFromEmailRow(row sqlcgen.WeaveAuthGetByEmailRow) *domainweave.AuthWithActor {
	return &domainweave.AuthWithActor{
		AuthRecord: domainweave.AuthRecord{
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
	}
}

func authWithActorFromEmailOrSlugRow(row sqlcgen.WeaveAuthGetByEmailOrSlugRow) *domainweave.AuthWithActor {
	return &domainweave.AuthWithActor{
		AuthRecord: domainweave.AuthRecord{
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
	}
}

func authWithActorFromProfileRow(row sqlcgen.WeaveAuthGetByActorIDWithActorRow) *domainweave.AuthWithActor {
	return &domainweave.AuthWithActor{
		AuthRecord: domainweave.AuthRecord{
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
	}
}
