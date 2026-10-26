//go:build integration

package database_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }

func TestReleaseOwnershipMigrationPreservesLegacyAndGuardsDowngrade(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	t.Cleanup(func() { _ = db.Close() })
	t.Cleanup(func() {
		if err := database.Migrate(db); err != nil {
			t.Error(err)
		}
	})
	// Exercise migration 021 itself even after newer migrations are added.
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version > 21; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	var projectID, ownerID string
	if err := pool.QueryRow(ctx, `SELECT id, owner_id FROM weave_projects ORDER BY id LIMIT 1`).Scan(&projectID, &ownerID); err != nil {
		t.Fatal(err)
	}
	const version = "999.21.0"
	if _, err := pool.Exec(ctx, `INSERT INTO weave_releases (project_id, version, created_by_id) VALUES ($1, $2, $3)`, projectID, version, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_namespace_bindings_archive (id, created_at, updated_at, prefix, namespace, version_number)
		VALUES ('MIGRATION_OWNERSHIP_LEGACY', now(), now(), 'legacy', 'https://legacy.example/', $1)`, version); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Migrate(db); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM weave_releases WHERE project_id=$1 AND version=$2`, projectID, version); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM weave_namespace_bindings_archive WHERE id='MIGRATION_OWNERSHIP_LEGACY'`); err != nil {
			t.Error(err)
		}
	})
	if err := database.MigrateDown(db); err != nil {
		t.Fatal(err)
	}
	// Migrate restores later migrations too; step down to 021 before its
	// guarded downgrade assertion below.
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for version := latest; version > 21; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	var ownership, captured, legacy int
	if err := pool.QueryRow(ctx, `SELECT archive_ownership_version FROM weave_releases WHERE project_id=$1 AND version=$2`, projectID, version).Scan(&ownership); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM weave_release_namespace_bindings_archive WHERE release_project_id=$1 AND version_number=$2`, projectID, version).Scan(&captured); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM weave_namespace_bindings_archive WHERE id='MIGRATION_OWNERSHIP_LEGACY'`).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if ownership != 0 || captured != 0 || legacy != 1 {
		t.Fatalf("migration invented or lost history: ownership=%d captured=%d legacy=%d", ownership, captured, legacy)
	}
	if _, err := pool.Exec(ctx, `UPDATE weave_releases SET archive_ownership_version=1 WHERE project_id=$1 AND version=$2`, projectID, version); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "cannot downgrade verified release archives") {
		t.Fatalf("downgrade should refuse data loss: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT archive_ownership_version FROM weave_releases WHERE project_id=$1 AND version=$2`, projectID, version).Scan(&ownership); err != nil || ownership != 1 {
		t.Fatalf("failed downgrade changed ownership: %d, %v", ownership, err)
	}
}
