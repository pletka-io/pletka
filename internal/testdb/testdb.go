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
