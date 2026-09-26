//go:build integration

package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/pletka-io/pletka/pkg/database"
)

// TestStaleTemplateIsDetected is the regression gate for a silent-pass trap.
//
// The template is cached in a reuse-by-name container that survives between
// runs and between checkouts, and the old freshness check only asked whether
// the sentinel table existed. weave_projects has existed since the baseline,
// so a template built before a newly added migration passed that check and was
// cloned into every package — tests then ran against the pre-migration schema
// and either failed confusingly or passed while asserting nothing about the
// new schema. Found the hard way on 2026-09-26, when a test asserting a new
// unique index saw pg_indexes count 0 against a template that predated
// migration 014.
//
// Both verdicts are pinned: a database at the wrong migration version must be
// rejected even with the sentinel table present, and one at the right version
// must be accepted — a check that rejected everything would rebuild the
// template on every run, which is slow rather than wrong and would not be
// noticed by a test that only covers the failure path.
func TestStaleTemplateIsDetected(t *testing.T) {
	ctx := context.Background()

	adminDSN, err := provideAdminDSN(ctx)
	if err != nil {
		t.Fatalf("admin dsn: %v", err)
	}

	current, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatalf("LatestMigrationVersion: %v", err)
	}

	tests := []struct {
		name    string
		version int64
		want    bool
	}{
		{"a template one migration behind is rejected", current - 1, false},
		{"a template at the current version is accepted", current, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dbName := fmt.Sprintf("freshness_probe_%d", tc.version)
			dsn := createProbeDatabase(ctx, t, adminDSN, dbName, tc.version)

			got, reason, err := databaseIsCurrent(ctx, dsn)
			if err != nil {
				t.Fatalf("databaseIsCurrent: %v", err)
			}
			if got != tc.want {
				t.Errorf("databaseIsCurrent = %v (reason %q), want %v", got, reason, tc.want)
			}
			if !tc.want && reason == "" {
				t.Error("a rejected template carried no reason — the rebuild would be unexplained in the log")
			}
		})
	}
}

// createProbeDatabase makes a database shaped like a template that HAS been
// migrated — sentinel table present, goose_db_version populated — but stamped
// at the given version. That is precisely the shape the old sentinel-only
// check could not tell apart from a current one.
func createProbeDatabase(ctx context.Context, t *testing.T, adminDSN, name string, version int64) string {
	t.Helper()

	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}
	defer admin.Close()

	if _, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quoteIdent(name)+" WITH (FORCE)"); err != nil {
		t.Fatalf("drop probe db: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+quoteIdent(name)); err != nil {
		t.Fatalf("create probe db: %v", err)
	}
	// Cleanup opens its own handle: this function's `admin` is closed by its
	// own defer when it returns, long before cleanup runs. Leaving probe
	// databases behind would litter the reuse-by-name container that the check
	// under test exists to keep trustworthy.
	t.Cleanup(func() {
		cleanupCtx := context.WithoutCancel(ctx)
		conn, err := sql.Open("pgx", adminDSN)
		if err != nil {
			t.Logf("open admin db to drop probe %s: %v", name, err)
			return
		}
		defer conn.Close()
		if _, err := conn.ExecContext(cleanupCtx,
			"DROP DATABASE IF EXISTS "+quoteIdent(name)+" WITH (FORCE)"); err != nil {
			t.Logf("drop probe db %s: %v", name, err)
		}
	})

	dsn, err := withDatabase(adminDSN, name)
	if err != nil {
		t.Fatalf("derive probe dsn: %v", err)
	}
	probe, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open probe db: %v", err)
	}
	defer probe.Close()

	for _, stmt := range []string{
		"CREATE TABLE " + quoteIdent(sentinelTable) + " (id text primary key)",
		`CREATE TABLE goose_db_version (
			id serial primary key,
			version_id bigint not null,
			is_applied boolean not null,
			tstamp timestamp default now()
		)`,
	} {
		if _, err := probe.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed probe schema: %v", err)
		}
	}
	if _, err := probe.ExecContext(ctx,
		"INSERT INTO goose_db_version (version_id, is_applied) VALUES ($1, true)", version); err != nil {
		t.Fatalf("stamp probe version: %v", err)
	}

	return dsn
}
