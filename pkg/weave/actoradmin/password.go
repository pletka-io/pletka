package actoradmin

import (
	"context"
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// minPasswordLen mirrors the self-service change-password rule.
const minPasswordLen = 12

// passwordAlphabet excludes visually ambiguous characters (0/O/1/l/I) so a
// generated password is safe to read off a screen or a CSV.
const passwordAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"

// generatePassword returns a crypto-random password of n characters drawn
// from passwordAlphabet.
func generatePassword(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	for i := range b {
		b[i] = passwordAlphabet[int(b[i])%len(passwordAlphabet)]
	}
	return string(b), nil
}

// hashPassword bcrypts the plaintext at the same cost the login path uses.
func hashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// SetPassword sets an actor's password with admin authority — it does NOT
// require the current password (unlike ChangePassword). It upserts the
// weave_auth row: when the actor has no auth row yet (e.g. a user created
// through the admin form before auth-row creation existed), it inserts one;
// otherwise it updates the hash. Callers must gate this behind the
// super-admin route guard.
func (s *Service) SetPassword(ctx context.Context, actorID, newPassword string) error {
	if s.auth == nil {
		return fmt.Errorf("auth store not configured")
	}
	if len(newPassword) < minPasswordLen {
		return fmt.Errorf("new password must be at least %d characters", minPasswordLen)
	}
	hash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	rec, err := s.auth.GetByActorID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("load auth record: %w", err)
	}
	if rec == nil {
		if _, err := s.auth.Create(ctx, actorID, hash, nil); err != nil {
			return fmt.Errorf("create auth record: %w", err)
		}
		return nil
	}
	if err := s.auth.UpdatePassword(ctx, actorID, hash); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}
