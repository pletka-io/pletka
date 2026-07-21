package router

import (
	"context"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/session"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"testing"
)

// TestPool returns a Postgres pool for router DB tests. It delegates to
// testdb.Pool, which serves the per-package clone provisioned by TestMain
// (or the env/default DSN in the unit lane) and applies the shared
// skip/REQUIRE_DB policy. The exported signature is preserved so
// cross-package importers keep working.
func TestPool(t *testing.T) *pgxpool.Pool {
	return testdb.Pool(t)
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
