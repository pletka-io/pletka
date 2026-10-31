//go:build integration

package collection_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Synthetic query fixtures model a verified cut; actual Service.Create is
// exercised by database/readsession's deletion and isolation regressions.
func seedVerifiedRelease(t *testing.T, pool *pgxpool.Pool, projectID, version string) {
	t.Helper()
	seedProject(t, pool, projectID)
	if _, err := pool.Exec(context.Background(), `INSERT INTO weave_releases(project_id,version,created_by_id,archive_ownership_version) VALUES ($1,$2,'unite',1) ON CONFLICT DO NOTHING`, projectID, version); err != nil {
		t.Fatal(err)
	}
}
