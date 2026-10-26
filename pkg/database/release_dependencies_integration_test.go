//go:build integration

package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestReleaseDependenciesMigrationDoesNotInventHistory(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	t.Cleanup(func() { _ = db.Close() })
	t.Cleanup(func() {
		if err := database.Migrate(db); err != nil {
			t.Error(err)
		}
	})
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version >= 22; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	var projectID, ownerID string
	if err := pool.QueryRow(ctx, `SELECT id, owner_id FROM weave_projects ORDER BY id LIMIT 1`).Scan(&projectID, &ownerID); err != nil {
		t.Fatal(err)
	}
	const version = "999.22.0"
	if _, err := pool.Exec(ctx, `INSERT INTO weave_releases (project_id, version, created_by_id, archive_ownership_version) VALUES ($1,$2,$3,1)`, projectID, version, ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Migrate(db); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM weave_releases WHERE project_id=$1 AND version=$2`, projectID, version); err != nil {
			t.Error(err)
		}
	})
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for current := latest; current > 22; current-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	var marker, pins int
	if err := pool.QueryRow(ctx, `SELECT project_dependencies_version FROM weave_releases WHERE project_id=$1 AND version=$2`, projectID, version).Scan(&marker); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM weave_release_project_dependencies WHERE project_id=$1 AND version_number=$2`, projectID, version).Scan(&pins); err != nil {
		t.Fatal(err)
	}
	if marker != 0 || pins != 0 {
		t.Fatalf("migration invented history: marker=%d pins=%d", marker, pins)
	}
	rows, err := release.AuditProjectDependencies(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.ProjectID == projectID && row.Version == version && row.ArchiveOwnershipVersion == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("audit missed owned archive with unverified dependencies")
	}
	// Synthetic marker only tests the downgrade guard, never historical certification.
	if _, err := pool.Exec(ctx, `UPDATE weave_releases SET project_dependencies_version=1 WHERE project_id=$1 AND version=$2`, projectID, version); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "cannot downgrade captured release project dependencies") {
		t.Fatalf("downgrade should refuse data loss: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT project_dependencies_version FROM weave_releases WHERE project_id=$1 AND version=$2`, projectID, version).Scan(&marker); err != nil || marker != 1 {
		t.Fatalf("failed downgrade changed marker: %d, %v", marker, err)
	}
}
