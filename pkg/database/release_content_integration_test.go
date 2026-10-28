//go:build integration

package database_test

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
)

func TestContentMigrationDoesNotInventHistoricalValues(t *testing.T) {
	pool := testdb.EmptyPool(t)
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version >= 32; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,display_name,slug) VALUES ('content-owner','Owner','content-owner')`)
	exec(`INSERT INTO weave_projects(id,owner_id,enforce_concept_lists) VALUES ('content-history','content-owner',true)`)
	exec(`INSERT INTO weave_projects_archive(id,owner_id,version_number) VALUES ('content-history','content-owner','1.0.0')`)
	exec(`INSERT INTO weave_field_overrides_archive(id,field_id,entity_type,entity_id,project_id,version_number) VALUES (900,'field','model','model','content-history','1.0.0')`)
	exec(`INSERT INTO weave_concept_broader_archive(id,concept_id,broader_id,scheme_id,version_number) VALUES ('edge','a','b','list','1.0.0')`)
	verifyUnknown := func() {
		t.Helper()
		var unknown bool
		if err := pool.QueryRow(t.Context(), `SELECT p.enforce_concept_lists IS NULL AND o.set_value_entry_id IS NULL AND b.created_at IS NULL
			FROM weave_projects_archive p CROSS JOIN weave_field_overrides_archive o CROSS JOIN weave_concept_broader_archive b`).Scan(&unknown); err != nil {
			t.Fatal(err)
		}
		if !unknown {
			t.Fatal("migration fabricated historical content")
		}
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	verifyUnknown()
	if err := database.MigrateDown(db); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	verifyUnknown()
	// Exercise migration 032 itself; no later receipt has been captured.
	for version := latest; version > 32; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		`UPDATE weave_projects_archive SET enforce_concept_lists=false`,
		`UPDATE weave_field_overrides_archive SET set_value_entry_id='captured-entry'`,
		`UPDATE weave_concept_broader_archive SET created_at='2025-01-01T00:00:00Z'`,
	} {
		exec(statement)
		if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "cannot downgrade captured release content fields") {
			t.Fatalf("captured field lost on downgrade: %v", err)
		}
		// These are explicitly unverified test archives, not certified history.
		exec(`UPDATE weave_projects_archive SET enforce_concept_lists=NULL`)
		exec(`UPDATE weave_field_overrides_archive SET set_value_entry_id=NULL`)
		exec(`UPDATE weave_concept_broader_archive SET created_at=NULL`)
	}
}
