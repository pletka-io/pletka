//go:build integration

package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
)

// Migration 045 replaces a view and a trigger function in place. Its down
// path must restore the per-pair forms from 030 so an older binary keeps the
// same validation, and its up path must be reapplicable afterwards.
func TestSemanticReferenceGuardMigrationRoundTrips(t *testing.T) {
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
	for version := latest; version > 45; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	definitions := func() (view, guard string) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT pg_get_viewdef('weave_project_semantic_candidates'::regclass, true)`).Scan(&view); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT pg_get_functiondef('weave_guard_project_semantic_references'::regproc)`).Scan(&guard); err != nil {
			t.Fatal(err)
		}
		return view, guard
	}
	view, guard := definitions()
	if !strings.Contains(view, "required") || !strings.Contains(guard, "MATERIALIZED") {
		t.Fatal("migration 045 is not applied at version 45")
	}
	if err := database.MigrateDown(db); err != nil {
		t.Fatal(err)
	}
	view, guard = definitions()
	if !strings.Contains(view, "weave_semantic_term_matches") || strings.Contains(guard, "MATERIALIZED") {
		t.Fatal("down did not restore the migration 030 view and guard")
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	view, guard = definitions()
	if !strings.Contains(view, "required") || !strings.Contains(guard, "MATERIALIZED") {
		t.Fatal("migration 045 did not reapply")
	}
}
