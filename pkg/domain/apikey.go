package domain

import "time"

// APIKey is one row of weave_api_keys: a bearer credential that
// authenticates as its owning actor. Only the SHA-256 hash of the secret is
// stored; the plaintext is shown once at mint time.
type APIKey struct {
	ID         string     `json:"id"`
	ActorID    string     `json:"actor_id"`
	Name       string     `json:"name"`
	KeyHash    string     `json:"-"`
	KeyPrefix  string     `json:"key_prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// Active reports whether the key is neither revoked nor expired at t.
func (k *APIKey) Active(t time.Time) bool {
	if k.RevokedAt != nil {
		return false
	}
	if k.ExpiresAt != nil && k.ExpiresAt.Before(t) {
		return false
	}
	return true
}
