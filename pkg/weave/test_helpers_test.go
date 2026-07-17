package weave_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool returns a pgxpool connected to TEST_DATABASE_URL (default: pletka_weave
// on localhost:5433). Skips the test if the database isn't reachable. Pool is
// closed automatically at test end.
func testPool(t *testing.T) *pgxpool.Pool {
	return testdb.Pool(t)
}

// seedTestActor inserts a minimal weave_actors row for tests and returns
// its id. Slug, system_name, and display_name all equal `name`. Caller is
// responsible for cleanup.
func seedTestActor(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	id := ids.GenerateULID()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO weave_actors (id, type, display_name, system_name, slug, email)
		VALUES ($1, 'person', $2, $2, $2, $3)
	`, id, name, name+"@test.local")
	if err != nil {
		t.Fatalf("seed actor %s: %v", name, err)
	}
	return id
}
