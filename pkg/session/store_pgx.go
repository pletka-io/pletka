package session

import (
	"github.com/alexedwards/scs/pgxstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SetupPgxStore configures the session manager to use a pgx-backed store.
// The underlying `sessions` table is created on first use via the
// pgxstore package; see its docs for schema details.
func (m *Manager) SetupPgxStore(pool *pgxpool.Pool) error {
	m.Store = pgxstore.New(pool)
	return nil
}
