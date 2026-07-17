// Package testdb provides a shared Postgres test-pool helper so DB-gated
// tests share one connection/skip policy instead of copy-pasting it.
package testdb

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool returns a Postgres pool for DB-gated tests. DSN comes from
// TEST_DATABASE_URL, defaulting to the local dev database. When the DB is
// unavailable the test is skipped — unless REQUIRE_DB is set, in which case
// it fails (so CI cannot silently skip DB tests). The pool is closed via
// t.Cleanup.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
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
