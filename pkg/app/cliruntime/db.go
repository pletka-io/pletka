package cliruntime

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"

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

// lockConnFloor is the minimum pgx pool size this process opens with, when
// the connection string does not set its own pool size. It exists because
// pkg/weave/override.WithEntityLock pins one pooled connection for the
// entire duration of a save while its callback acquires further
// connections from the same pool for the actual write work; pgx's own
// default (max(4, NumCPU)) leaves no headroom; on a small instance a
// handful of concurrent saves can each hold a lock connection and then all
// block acquiring one for their own work, wedging unrelated reads too.
// pkg/service/gitmaterializer.Materializer.LockProjectForRestore now pins a
// connection the same way, for a whole restore's pipeline rather than one
// save — a longer hold, but restores are rare and effectively one-at-a-time
// (an operator-triggered job, not concurrent request traffic), so it adds
// at most one more long-lived pinned connection at a time, not the
// many-at-once pressure concurrent saves create; the floor set for saves
// already covers it without needing to be raised further. An explicit
// pool_max_conns in the connection string always wins, higher or lower
// than the floor: an operator who deliberately caps the pool below 16 —
// e.g. to fit several instances into one database's connection limit —
// must not have that choice silently overridden.
const lockConnFloor = 16

// explicitPoolSizePattern matches a pool_max_conns setting in either
// connection-string form pgxpool.ParseConfig accepts: a space-separated
// keyword/value pair (host=... pool_max_conns=8) or a URL query parameter
// (?pool_max_conns=8&...). Checked against the raw string before
// ParseConfig runs, because ParseConfig deletes the key from
// RuntimeParams once it consumes it — by the time it returns, cfg.MaxConns
// alone can't distinguish an explicit low setting from pgx's own untouched
// default.
var explicitPoolSizePattern = regexp.MustCompile(`(?:^|[\s?&])pool_max_conns=`)

// applyLockConnFloor raises cfg.MaxConns to lockConnFloor, unless
// connString itself already sets pool_max_conns — in which case that
// explicit setting wins regardless of which side of the floor it falls on.
func applyLockConnFloor(cfg *pgxpool.Config, connString string) {
	if cfg.MaxConns < lockConnFloor && !explicitPoolSizePattern.MatchString(connString) {
		cfg.MaxConns = lockConnFloor
	}
}

// OpenPGXPoolWithSettings creates a pgx pool from explicit settings.
func OpenPGXPoolWithSettings(ctx context.Context, settings database.Settings) (*pgxpool.Pool, error) {
	connString := settings.PgxConnString()
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}
	applyLockConnFloor(cfg, connString)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
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
