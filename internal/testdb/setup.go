package testdb

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pletka-io/pletka/pkg/database"

	// Registers the "pgx" database/sql driver used for the template build and
	// server-level CREATE/DROP DATABASE statements. pgx is already a core
	// dependency, so this stays Docker-free.
	_ "github.com/jackc/pgx/v5/stdlib"
)

// templateDBName is the migrated database that every per-package clone is
// cut from. It is built once per server and reused across runs.
const templateDBName = "fixture_template"

// maintenanceDB is the always-present database used for server-level
// operations (CREATE/DROP DATABASE) — it must never be the template or a
// clone, so it holds no connections to them.
const maintenanceDB = "postgres"

// templateAdvisoryLock serializes the template build across parallel package
// test binaries. The constant ("plkt") is arbitrary but fixed so every binary
// contends on the same lock.
const templateAdvisoryLock int64 = 0x706c6b74

// provideAdminDSN is the build-agnostic seam to the container provider. The
// integration-tagged container_integration.go populates it via init(); in the
// untagged unit build it stays nil, so Setup reports the provider absent and
// skips (or fails under REQUIRE_DB) without importing any Docker dependency.
var provideAdminDSN func(ctx context.Context) (string, error)

// cloneCounter disambiguates multiple clones derived within one test binary.
var cloneCounter atomic.Int64

// Setup ensures schema + fixture template on the target server, clones a
// per-package database from the template, points Pool at the clone, runs the
// package's tests, then drops the clone. Call from TestMain.
func Setup(m *testing.M) int {
	ctx := context.Background()
	require := os.Getenv("REQUIRE_DB") != ""

	if provideAdminDSN == nil {
		return skipOrFail(require, "container provider absent: build without -tags=integration")
	}

	adminDSN, err := provideAdminDSN(ctx)
	if err != nil {
		return skipOrFail(require, fmt.Sprintf("provision test database: %v", err))
	}
	if adminDSN == "" {
		return skipOrFail(require, "no test database available")
	}

	maintDSN, err := withDatabase(adminDSN, maintenanceDB)
	if err != nil {
		return skipOrFail(require, fmt.Sprintf("derive maintenance dsn: %v", err))
	}

	if err := ensureTemplate(ctx, maintDSN); err != nil {
		return skipOrFail(require, fmt.Sprintf("build fixture template: %v", err))
	}

	cloneName := deriveCloneName()
	if err := createClone(ctx, maintDSN, cloneName); err != nil {
		return skipOrFail(require, fmt.Sprintf("clone template: %v", err))
	}
	defer dropClone(ctx, maintDSN, cloneName)

	cloneDSNValue, err := withDatabase(adminDSN, cloneName)
	if err != nil {
		return skipOrFail(require, fmt.Sprintf("derive clone dsn: %v", err))
	}
	cloneDSN = cloneDSNValue

	return m.Run()
}

// ensureTemplate builds the migrated template database once, serialized by a
// session advisory lock so parallel package binaries do not race the
// CREATE DATABASE + migrate. If the template already exists it is reused as-is.
func ensureTemplate(ctx context.Context, maintDSN string) error {
	db, err := sql.Open("pgx", maintDSN)
	if err != nil {
		return fmt.Errorf("open maintenance db: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping maintenance db: %w", err)
	}

	// The advisory lock is session-scoped, so lock and unlock must run on the
	// same connection — pin one for the duration of the build.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("pin maintenance conn: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", templateAdvisoryLock); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	defer func() {
		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", templateAdvisoryLock); err != nil {
			slog.Warn("testdb: release advisory lock", "err", err)
		}
	}()

	var exists bool
	if err := conn.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", templateDBName).Scan(&exists); err != nil {
		return fmt.Errorf("check template existence: %w", err)
	}
	if exists {
		return nil
	}

	if _, err := conn.ExecContext(ctx, "CREATE DATABASE "+quoteIdent(templateDBName)); err != nil {
		return fmt.Errorf("create template db: %w", err)
	}

	if err := migrateTemplate(ctx, maintDSN); err != nil {
		return err
	}
	return nil
}

// migrateTemplate opens a dedicated handle to the freshly created template,
// runs the goose schema migrations plus the fixture hydration hook, then closes
// the handle — Postgres refuses CREATE DATABASE ... TEMPLATE while any session
// is connected to the template, so this connection must not outlive the build.
func migrateTemplate(ctx context.Context, maintDSN string) error {
	tmplDSN, err := withDatabase(maintDSN, templateDBName)
	if err != nil {
		return fmt.Errorf("derive template dsn: %w", err)
	}
	tdb, err := sql.Open("pgx", tmplDSN)
	if err != nil {
		return fmt.Errorf("open template db: %w", err)
	}

	if err := database.Migrate(tdb); err != nil {
		tdb.Close()
		return fmt.Errorf("migrate template: %w", err)
	}
	if err := hydrateFixtures(ctx, tdb); err != nil {
		tdb.Close()
		return fmt.Errorf("hydrate fixtures: %w", err)
	}
	if err := tdb.Close(); err != nil {
		return fmt.Errorf("close template db: %w", err)
	}
	return nil
}

// hydrateFixtures is the Task-3 seam. Task 2 ships a schema-only template, so
// this is intentionally a no-op; Task 3 loads fixture data into the template
// here, before any clone is cut, so every clone inherits the same seed data.
func hydrateFixtures(_ context.Context, _ *sql.DB) error { return nil }

// createClone cuts a fresh per-package database from the migrated template.
func createClone(ctx context.Context, maintDSN, name string) error {
	db, err := sql.Open("pgx", maintDSN)
	if err != nil {
		return fmt.Errorf("open maintenance db: %w", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx,
		"CREATE DATABASE "+quoteIdent(name)+" TEMPLATE "+quoteIdent(templateDBName)); err != nil {
		return fmt.Errorf("create clone db: %w", err)
	}
	return nil
}

// dropClone tears down a clone after the package's tests finish. WITH (FORCE)
// (PG13+) terminates any leftover connections so the drop cannot hang.
func dropClone(ctx context.Context, maintDSN, name string) {
	db, err := sql.Open("pgx", maintDSN)
	if err != nil {
		slog.Warn("testdb: open maintenance db for clone drop", "err", err)
		return
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx,
		"DROP DATABASE IF EXISTS "+quoteIdent(name)+" WITH (FORCE)"); err != nil {
		slog.Warn("testdb: drop clone db", "clone", name, "err", err)
	}
}

// deriveCloneName builds a valid, lowercase, length-safe Postgres identifier
// from a hash of the test binary path plus a per-process counter, so each
// package binary (and each Setup call within it) gets a distinct clone.
func deriveCloneName() string {
	h := sha1.Sum([]byte(os.Args[0]))
	return fmt.Sprintf("clone_%s_%d", hex.EncodeToString(h[:6]), cloneCounter.Add(1))
}

// withDatabase returns dsn with its database path swapped for db.
func withDatabase(dsn, db string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse dsn: %w", err)
	}
	u.Path = "/" + db
	return u.String(), nil
}

// quoteIdent double-quotes a Postgres identifier so it is safe in DDL.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// skipOrFail logs why the database is unavailable and returns a process exit
// code: 1 (failure) when REQUIRE_DB is set so CI cannot silently skip, else 0
// (skip the whole package).
func skipOrFail(require bool, reason string) int {
	if require {
		slog.Error("testdb: REQUIRE_DB set but database unavailable", "reason", reason)
		return 1
	}
	slog.Info("testdb: skipping DB tests", "reason", reason)
	return 0
}
