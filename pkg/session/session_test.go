package session

import (
	"context"
	"testing"
	"time"
)

// newTestManagerContext returns a Manager backed by the default in-memory
// store and a context carrying a loaded (but not yet persisted) session, the
// minimum needed to exercise Manager methods that read/write session data.
func newTestManagerContext(t *testing.T) (*Manager, context.Context) {
	t.Helper()
	m := New(DefaultConfig())
	ctx, err := m.SessionManager.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	return m, ctx
}

func TestEstablishAuthenticatedSession(t *testing.T) {
	m, ctx := newTestManagerContext(t)
	if err := m.EstablishAuthenticatedSession(ctx, "actor-123", "a@b.se"); err != nil {
		t.Fatalf("EstablishAuthenticatedSession: %v", err)
	}
	if got := m.GetString(ctx, KeyUserID); got != "actor-123" {
		t.Errorf("KeyUserID = %q, want actor-123", got)
	}
	if got := m.GetString(ctx, KeyUserEmail); got != "a@b.se" {
		t.Errorf("KeyUserEmail = %q, want a@b.se", got)
	}
	if !m.GetBool(ctx, KeyIsAuthenticated) {
		t.Error("KeyIsAuthenticated = false, want true")
	}
}

func TestDefaultConfig_SlidingIdleTimeout(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.IdleTimeout != 8*time.Hour {
		t.Errorf("IdleTimeout = %v, want 8h", cfg.IdleTimeout)
	}
	if cfg.Lifetime != 30*24*time.Hour {
		t.Errorf("Lifetime = %v, want 720h (30d)", cfg.Lifetime)
	}
}

func TestNew_AppliesIdleTimeout(t *testing.T) {
	m := New(Config{IdleTimeout: 8 * time.Hour, Lifetime: 30 * 24 * time.Hour})
	// Manager embeds *scs.SessionManager; IdleTimeout is a public field on it.
	if m.IdleTimeout != 8*time.Hour {
		t.Errorf("scs IdleTimeout = %v, want 8h", m.IdleTimeout)
	}
	if m.Lifetime != 30*24*time.Hour {
		t.Errorf("scs Lifetime = %v, want 720h", m.Lifetime)
	}
}
