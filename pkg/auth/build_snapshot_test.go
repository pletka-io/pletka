//go:build integration

package auth_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	return testdb.Pool(t)
}

// seedAuthTestActor duplicates pkg/weave_test.seedTestActor because Go
// external test packages can't share unexported helpers across packages.
func seedAuthTestActor(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	id := "auth_test_" + name
	_, err := pool.Exec(context.Background(), `
		INSERT INTO weave_actors (id, type, display_name, system_name, slug, email)
		VALUES ($1, 'person', $2, $2, $2, $3)
	`, id, name, name+"@test.local")
	if err != nil {
		t.Fatalf("seed actor %s: %v", name, err)
	}
	return id
}

func TestBuildSnapshot_Anonymous(t *testing.T) {
	pool := testPool(t)
	ws := weave.NewPostgresStore(pool)

	s, err := auth.BuildSnapshot(context.Background(), ws, "")
	if err != nil {
		t.Fatalf("BuildSnapshot empty id: %v", err)
	}
	if !s.IsAnonymous || s.ActorID != "" {
		t.Errorf("empty user id should produce anonymous snapshot, got %+v", s)
	}
}

func TestBuildSnapshot_WithMemberships(t *testing.T) {
	pool := testPool(t)
	ws := weave.NewPostgresStore(pool)
	ctx := context.Background()

	actorID := seedAuthTestActor(t, pool, "snap_test_1")
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_memberships WHERE actor_id = $1`, actorID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, actorID)
	})

	if err := ws.Memberships().Upsert(ctx, domain.Membership{
		ActorID: actorID, ScopeType: "project", ScopeID: "LA", Role: "maintainer",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	s, err := auth.BuildSnapshot(ctx, ws, actorID)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if s.IsAnonymous {
		t.Error("not anonymous")
	}
	if s.Roles["project:LA"] != "maintainer" {
		t.Errorf("Roles[project:LA] = %q want maintainer", s.Roles["project:LA"])
	}
}
