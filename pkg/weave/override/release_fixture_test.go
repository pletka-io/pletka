//go:build integration

package override_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// This metadata accompanies synthetic archive rows, never certifies real history.
func seedVerifiedRelease(t *testing.T, pool *pgxpool.Pool, projectID, version string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO weave_projects(id,owner_id) VALUES ($1,'unite') ON CONFLICT DO NOTHING`, projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO weave_releases(project_id,version,created_by_id,archive_ownership_version) VALUES ($1,$2,'unite',1) ON CONFLICT DO NOTHING`, projectID, version); err != nil {
		t.Fatal(err)
	}
}
