// Package testdb provides a shared Postgres test-pool helper so DB-gated
// tests share one connection/skip policy instead of copy-pasting it.
package testdb

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// cloneDSN is stashed by Setup: when non-empty it points at the per-package
// clone database, so Pool serves the clone instead of the shared dev DB. Left
// empty in the unit lane (no Setup), where Pool falls back to the env/default
// DSN below.
var cloneDSN string

// RequireRealDataDB skips a real-data smoke test unless TEST_DATABASE_URL
// points at a seeded database. These tests assert on real customer ontology /
// path data that the synthetic fixtures deliberately do not reproduce, so their
// package must NOT run testdb.Setup (a fixture clone would make them fail). In
// the fixture lane (pure testcontainers, no TEST_DATABASE_URL in the outer env)
// they skip cleanly; against a seeded DB pointed at by TEST_DATABASE_URL they
// run. It must be called before any pool is acquired so the REQUIRE_DB
// unreachable-DB gate in Pool never triggers for these tests.
func RequireRealDataDB(t *testing.T) {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("real-data smoke: set TEST_DATABASE_URL to a seeded DB to run")
	}
}

// Pool returns a Postgres pool for DB-gated tests. When Setup has provisioned a
// per-package clone its DSN is used; otherwise the DSN comes from
// TEST_DATABASE_URL, defaulting to the local dev database. When the DB is
// unavailable the test is skipped — unless REQUIRE_DB is set, in which case
// it fails (so CI cannot silently skip DB tests). The pool is closed via
// t.Cleanup.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := cloneDSN
	if dsn == "" {
		dsn = os.Getenv("TEST_DATABASE_URL")
	}
	if dsn == "" {
		dsn = "postgres://postgres:pw123@localhost:5433/pletka_weave?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err == nil {
		err = pool.Ping(context.Background())
	}
	if err != nil {
		if pool != nil {
			pool.Close()
		}
		if os.Getenv("REQUIRE_DB") != "" {
			t.Fatalf("REQUIRE_DB set but test database unavailable: %v", err)
		}
		t.Skipf("test database unavailable: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
