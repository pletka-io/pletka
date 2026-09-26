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

// LatestMigrationVersion returns the highest version in the embedded migration
// set — the version a fully migrated database's goose_db_version should reach.
//
// It exists so a caller holding a database built earlier can ask whether that
// schema still matches this binary's migrations, instead of inferring it from
// the presence of some table. internal/testdb uses it to decide whether a
// cached fixture template predates a newly added migration.
func LatestMigrationVersion() (int64, error) {
	goose.SetBaseFS(migrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return 0, fmt.Errorf("goose set dialect: %w", err)
	}

	collected, err := goose.CollectMigrations("migrations", 0, goose.MaxVersion)
	if err != nil {
		return 0, fmt.Errorf("collect migrations: %w", err)
	}
	if len(collected) == 0 {
		return 0, fmt.Errorf("collect migrations: embedded migration set is empty")
	}

	// CollectMigrations sorts ascending, but take the max explicitly rather
	// than trusting the ordering of a set this check is the gate for.
	var latest int64
	for _, m := range collected {
		if m.Version > latest {
			latest = m.Version
		}
	}
	return latest, nil
}

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
