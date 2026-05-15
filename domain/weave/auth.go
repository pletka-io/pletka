package weave

import (
	"context"
	"time"
)

// AuthRecord represents one row in weave_auth.
type AuthRecord struct {
	ActorID                string     `json:"actor_id"`
	PasswordHash           string     `json:"-"`
	EmailVerifiedAt        *time.Time `json:"email_verified_at,omitempty"`
	PasswordResetToken     *string    `json:"-"`
	PasswordResetExpiresAt *time.Time `json:"password_reset_expires_at,omitempty"`
	LastLoginAt            *time.Time `json:"last_login_at,omitempty"`
	PermsVersion           int32      `json:"perms_version"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// AuthWithActor is an auth record joined with the actor's display fields.
type AuthWithActor struct {
	AuthRecord
	Email       string `json:"email"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// RegisterPersonParams carries the fields needed to create a person actor and
// auth row in one transaction.
type RegisterPersonParams struct {
	ActorID      string `json:"actor_id"`
	DisplayName  string `json:"display_name"`
	Slug         string `json:"slug"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
}

// AuthStore accesses weave_auth plus related actor profile queries.
type AuthStore interface {
	RegisterPerson(ctx context.Context, params RegisterPersonParams) (*AuthWithActor, error)
	Create(ctx context.Context, actorID, passwordHash string, emailVerifiedAt *time.Time) (*AuthRecord, error)
	GetByActorID(ctx context.Context, actorID string) (*AuthRecord, error)
	GetByEmail(ctx context.Context, email string) (*AuthWithActor, error)
	GetByEmailOrSlug(ctx context.Context, loginIdentifier string) (*AuthWithActor, error)
	GetProfileByActorID(ctx context.Context, actorID string) (*AuthWithActor, error)
	UpdatePassword(ctx context.Context, actorID, hash string) error
	MarkLogin(ctx context.Context, actorID string) error
	SetResetToken(ctx context.Context, actorID, token string, expires time.Time) error
	GetByResetToken(ctx context.Context, token string) (*AuthRecord, error)
	ClearResetToken(ctx context.Context, actorID string) error
	Delete(ctx context.Context, actorID string) error
	BumpPermsVersion(ctx context.Context, actorID string) error
}
