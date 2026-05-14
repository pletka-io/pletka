package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/pressly/goose/v3"

	"github.com/pletka-io/pletka/store/migrations"
)

const migrationsDir = "."

// Up applies all pending embedded migrations.
func Up(ctx context.Context, cfg Config, out io.Writer) error {
	return withMigrationDB(ctx, cfg, out, func(db migrationDB) error {
		return goose.UpContext(ctx, db.SQL, migrationsDir)
	})
}

// Status prints the embedded migration status.
func Status(ctx context.Context, cfg Config, out io.Writer) error {
	return withMigrationDB(ctx, cfg, out, func(db migrationDB) error {
		return goose.StatusContext(ctx, db.SQL, migrationsDir)
	})
}

type migrationDB struct {
	SQL *sql.DB
}

func withMigrationDB(ctx context.Context, cfg Config, out io.Writer, fn func(migrationDB) error) error {
	sqlDB, err := OpenSQL(ctx, cfg)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(writerLogger{out: out})

	return fn(migrationDB{SQL: sqlDB})
}

type writerLogger struct {
	out io.Writer
}

func (l writerLogger) Printf(format string, args ...any) {
	if l.out == nil {
		return
	}
	fmt.Fprintf(l.out, format+"\n", args...)
}

func (l writerLogger) Fatalf(format string, args ...any) {
	l.Printf(format, args...)
}
