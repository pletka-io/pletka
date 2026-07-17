package cliruntime

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/pkg/database"
)

// OpenSQLDBWithSettings creates a database/sql connection from explicit settings.
func OpenSQLDBWithSettings(settings database.Settings) (*sql.DB, error) {
	db, err := sql.Open("pgx", settings.PgxConnString())
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return db, nil
}

// OpenSQLDB creates a database/sql connection from the current process config.
func OpenSQLDB() (*sql.DB, error) {
	return OpenSQLDBWithSettings(DatabaseSettingsFromViper().WithEnvOverrides())
}

// OpenPingedSQLDBWithSettings creates and verifies a database/sql connection
// from explicit settings.
func OpenPingedSQLDBWithSettings(ctx context.Context, settings database.Settings) (*sql.DB, error) {
	db, err := OpenSQLDBWithSettings(settings)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
}

// OpenPingedSQLDB creates and verifies a database/sql connection.
func OpenPingedSQLDB(ctx context.Context) (*sql.DB, error) {
	return OpenPingedSQLDBWithSettings(ctx, DatabaseSettingsFromViper().WithEnvOverrides())
}

// OpenPGXPoolWithSettings creates a pgx pool from explicit settings.
func OpenPGXPoolWithSettings(ctx context.Context, settings database.Settings) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, settings.PgxConnString())
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	return pool, nil
}

// OpenPGXPool creates a pgx pool from the current process config.
func OpenPGXPool(ctx context.Context) (*pgxpool.Pool, error) {
	return OpenPGXPoolWithSettings(ctx, DatabaseSettingsFromViper().WithEnvOverrides())
}

// OpenPingedPGXPoolWithSettings creates and verifies a pgx pool from explicit
// settings.
func OpenPingedPGXPoolWithSettings(ctx context.Context, settings database.Settings) (*pgxpool.Pool, error) {
	pool, err := OpenPGXPoolWithSettings(ctx, settings)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// OpenPingedPGXPool creates and verifies a pgx pool.
func OpenPingedPGXPool(ctx context.Context) (*pgxpool.Pool, error) {
	return OpenPingedPGXPoolWithSettings(ctx, DatabaseSettingsFromViper().WithEnvOverrides())
}
