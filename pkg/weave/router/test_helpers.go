package router

import (
	"context"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/session"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"os"
	"testing"
)

// TestPool returns a pgx pool pointed at TEST_DATABASE_URL. It skips the test
// when the database is unreachable.
func TestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:pw123@localhost:5433/pletka_weave?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err == nil {
		// pgxpool.New is lazy — it never dials. Ping so an unreachable DB
		// skips the test here instead of hard-failing on the first query
		// (which is what breaks CI, where no Postgres is available).
		err = pool.Ping(context.Background())
	}
	if err != nil {
		if pool != nil {
			pool.Close()
		}
		t.Skip("database not available:", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func ensureSessionsTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS sessions (
			token  TEXT PRIMARY KEY,
			data   BYTEA NOT NULL,
			expiry TIMESTAMPTZ NOT NULL
		);
		CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions (expiry);
	`)
	if err != nil {
		t.Fatalf("ensureSessionsTable: %v", err)
	}
}

// NewForTests builds a minimal chi router with auth routes wired to the
// weave-backed auth handler.
func NewForTests(t *testing.T) (http.Handler, *pgxpool.Pool) {
	t.Helper()
	pool := TestPool(t)
	ensureSessionsTable(t, pool)
	ws := weave.NewPostgresStore(pool)
	sm := session.NewManager(nil)
	if err := sm.SetupPgxStore(pool); err != nil {
		t.Fatalf("SetupPgxStore: %v", err)
	}

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	r.Use(weaveauth.NewMiddleware(sm, ws))

	ah := weaveauth.NewAuthHandlerForTests(ws, sm)
	r.Post("/api/v1/auth/register", ah.Register)
	r.Post("/api/v1/auth/login", ah.Login)
	r.Post("/api/v1/auth/logout", ah.Logout)
	r.Get("/api/v1/auth/me", ah.Me)
	return r, pool
}
