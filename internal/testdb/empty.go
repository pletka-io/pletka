package testdb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/pkg/database"
)

// EmptyPool creates a schema-only database on the isolated test server, for
// restore tests that must not inherit the fixture template. Only its own
// randomly named database is dropped at cleanup.
func EmptyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	config := Pool(t).Config()
	config.ConnConfig.Database = "postgres"
	admin, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("ontology_restore_%d", time.Now().UnixNano())
	if _, err := admin.Exec(t.Context(), "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE template0"); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer admin.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	config = config.Copy()
	config.ConnConfig.Database = name
	db := stdlib.OpenDB(*config.ConnConfig)
	if err := database.Migrate(db); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	_ = db.Close()
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
