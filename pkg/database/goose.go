package database

import (
	"database/sql"
	"embed"
	"fmt"
	"io"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate runs all pending migrations.
func Migrate(db *sql.DB) error {
	goose.SetBaseFS(migrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}

	// AllowMissing lets a lower-numbered migration that landed AFTER
	// higher-numbered ones (e.g. via a parallel branch) still apply.
	// Goose otherwise refuses to advance when "missing migrations
	// below current version" are detected.
	return goose.Up(db, "migrations", goose.WithAllowMissing())
}

// MigrateStatus prints the current migration status.
func MigrateStatus(db *sql.DB, w io.Writer) error {
	goose.SetBaseFS(migrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}

	return goose.Status(db, "migrations")
}

// MigrateDown rolls back the last migration.
func MigrateDown(db *sql.DB) error {
	goose.SetBaseFS(migrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}

	return goose.Down(db, "migrations")
}
