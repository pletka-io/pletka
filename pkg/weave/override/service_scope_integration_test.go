//go:build integration

package override

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
)

// serviceTestContext returns a context that satisfies Service's
// requireProjectRead/requireProjectWrite gates for projectID. It mirrors
// writeCtx (save_changelog_integration_test.go): a super-admin snapshot
// bypasses the role lookup entirely (AuthSnapshot.Can's first branch), so it
// grants ProjectRead/ProjectEdit on any project without needing a real
// membership row — projectID is accepted for readability at call sites, not
// because the snapshot is scoped to it.
func serviceTestContext(t *testing.T, projectID string) context.Context {
	t.Helper()
	return auth.WithSnapshot(context.Background(), &auth.AuthSnapshot{IsSuperAdmin: true})
}

// TestService_ReadsHonourTheScope proves the scope survives the service
// layer: the same call with two scopes returns two different answers.
func TestService_ReadsHonourTheScope(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := serviceTestContext(t, "ZSVC") // a context with ProjectRead on ZSVC
	svc := NewService(NewPostgresStore(pool), nil, nil)

	const projectID, entityID, version = "ZSVC", "ZSVCM.1", "1.0.0"
	seedScopeFixture(t, pool, projectID, entityID, version)

	live, err := svc.ListForEntity(ctx, auth.Draft(), projectID, "model", entityID)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	archived, err := svc.ListForEntity(ctx, auth.Release(version), projectID, "model", entityID)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if len(live) == len(archived) {
		t.Fatalf("both scopes returned %d overrides — the scope is not reaching the store", len(live))
	}
	if len(live) != 2 || len(archived) != 1 {
		t.Errorf("draft=%d release=%d, want 2 and 1", len(live), len(archived))
	}
}

func TestService_InvalidScopeIsRefused(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := serviceTestContext(t, "ZSVC")
	svc := NewService(NewPostgresStore(pool), nil, nil)

	var zero auth.ReadScope
	if _, err := svc.ListForEntity(ctx, zero, "ZSVC", "model", "ZSVCM.1"); err == nil {
		t.Fatal("a zero scope was accepted by the service")
	}
}
