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
//
// Reuse is name-keyed, not content-hashed: ensureTemplate only checks that a
// database of this exact name exists and contains sentinelTable (see
// templateHasSentinel) — it never hashes the fixture snapshots' content to
// detect drift. Across a `go test` run this is fine (the template is built
// fresh in a throwaway container), but container_integration.go attaches to a
// *reuse-by-name* Postgres container (reuseContainerName) that can survive
// between local runs and even between checkouts, so a stale template built
// from an older, incompatible fixture generation would otherwise be reused
// silently — it has the sentinel table, so ensureTemplate would trust it.
// The name is versioned for exactly this reason: bump the suffix whenever the
// fixture snapshots change shape enough that an old template must not be
// reused (e.g. the FXSINGLE/FXPARENT/FXCHILD synthetic generator -> real
// AME/LA/ING project fixtures switch). A version bump makes ensureTemplate
// look for a database that cannot exist yet in a container built under the
// old name, forcing a rebuild; the old, now-orphaned "fixture_template"
// database is harmless and can be dropped manually or left for the container
// to be recycled.
const templateDBName = "fixture_template_v3"

// templateBuildingDBName is the fixed temp name the template is built under
// before it is atomically promoted (renamed) to templateDBName. A crash before
// the rename leaves only this half-built name, which the next run drops and
// rebuilds — templateDBName is never observed in a half-built state. Kept in
// sync with templateDBName's version suffix so a stale building-db from an
// older generation cannot collide with (or be mistaken for) the current one.
const templateBuildingDBName = "fixture_template_v3_building"

// sentinelTable is the schema table whose presence proves a template is fully
// migrated (not an empty, poisoned shell). ensureTemplate reuses a template
// only if it contains this table; an existing template without it is dropped
// and rebuilt.
const sentinelTable = "weave_projects"

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

	// Republish the clone DSN through TEST_DATABASE_URL so DB helpers that
	// cannot import this package reach the same per-package clone. internal/testdb
	// imports pkg/service/gitmaterializer and pkg/weave/ontology (for fixture
	// hydration), so an *internal* test package for either of those (e.g.
	// gitmaterializer's restore_hydrator_test.go, which needs unexported
	// helpers) would form an import cycle if it imported testdb. Those helpers
	// read TEST_DATABASE_URL directly instead; overwriting it here (process-local
	// to this package's test binary) makes them clone-aware without the import.
	os.Setenv("TEST_DATABASE_URL", cloneDSNValue)

	return m.Run()
}

// ensureTemplate guarantees a valid, migrated template exists, serialized by a
// session advisory lock so parallel package binaries do not race the build.
//
// The build is atomic: it runs under a temp name and is promoted to
// templateDBName by a rename that is the sole commit point. A reused template
// is trusted only if it contains the sentinel table — an empty, poisoned shell
// (a build that created the database but died before migrating) is detected,
// dropped, and rebuilt rather than cloned into failing tests forever.
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
		valid, reason, err := templateIsCurrent(ctx, maintDSN)
		if err != nil {
			return err
		}
		if valid {
			return nil
		}
		// Poisoned shell, or a template built before a migration this binary
		// carries: drop and fall through to rebuild.
		slog.Warn("testdb: reusing container held an unusable template; dropping and rebuilding",
			"template", templateDBName, "reason", reason)
		if _, err := conn.ExecContext(ctx,
			"DROP DATABASE IF EXISTS "+quoteIdent(templateDBName)+" WITH (FORCE)"); err != nil {
			return fmt.Errorf("drop invalid template: %w", err)
		}
	}

	return buildTemplate(ctx, conn, maintDSN)
}

// templateIsCurrent reports whether the existing template is safe to clone,
// and when it is not, why — the reason is logged before the rebuild.
//
// Two conditions, and the second is the one that matters on a long-lived box.
// The sentinel table proves the template was migrated at all (an empty shell
// from a build that died before migrating is not). The goose version proves it
// was migrated by a binary carrying the same migrations as this one: the
// container is reused by name across runs and even across checkouts, so a
// template built yesterday can predate a migration added today. Keying on a
// table's presence cannot see that — weave_projects has existed since the
// baseline — and the stale schema is then cloned into every package, where a
// test either fails confusingly or passes while asserting nothing about the
// new schema.
//
// It opens a dedicated handle to the template DB (information_schema and
// goose_db_version are both per-database) and closes it before returning, so
// it leaves no session that would block a later rename.
func templateIsCurrent(ctx context.Context, maintDSN string) (bool, string, error) {
	tmplDSN, err := withDatabase(maintDSN, templateDBName)
	if err != nil {
		return false, "", fmt.Errorf("derive template dsn: %w", err)
	}
	return databaseIsCurrent(ctx, tmplDSN)
}

// databaseIsCurrent is templateIsCurrent's check against an arbitrary
// database, split out so both verdicts can be exercised against a scratch
// database rather than only against whichever template the container happens
// to hold.
func databaseIsCurrent(ctx context.Context, dsn string) (bool, string, error) {
	want, err := database.LatestMigrationVersion()
	if err != nil {
		return false, "", fmt.Errorf("read expected migration version: %w", err)
	}

	tdb, err := sql.Open("pgx", dsn)
	if err != nil {
		return false, "", fmt.Errorf("open template db: %w", err)
	}
	defer func() { _ = tdb.Close() }()

	var found bool
	if err := tdb.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)", sentinelTable).Scan(&found); err != nil {
		return false, "", fmt.Errorf("check template sentinel: %w", err)
	}
	if !found {
		return false, "template has no schema (missing " + sentinelTable + ")", nil
	}

	// A template with the sentinel table but no goose_db_version is not a
	// shape this code produces; treat it as stale rather than guessing.
	var got sql.NullInt64
	if err := tdb.QueryRowContext(ctx,
		"SELECT max(version_id) FROM goose_db_version WHERE is_applied").Scan(&got); err != nil {
		return false, fmt.Sprintf("template has no readable goose_db_version (%v)", err), nil
	}
	if !got.Valid || got.Int64 != want {
		return false, fmt.Sprintf("template is at migration %d, this binary carries %d", got.Int64, want), nil
	}

	return true, "", nil
}

// buildTemplate (re)builds the template under a temp name and promotes it
// atomically. The rename is the commit point: templateDBName is either absent
// or fully built, never half-built. On any failure the temp name is dropped
// best-effort so a failed build never leaves a poisoned name behind. Must be
// called while holding the advisory lock.
func buildTemplate(ctx context.Context, conn *sql.Conn, maintDSN string) (err error) {
	// Clean any prior aborted build before creating a fresh temp database.
	if _, err := conn.ExecContext(ctx,
		"DROP DATABASE IF EXISTS "+quoteIdent(templateBuildingDBName)+" WITH (FORCE)"); err != nil {
		return fmt.Errorf("drop stale building db: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "CREATE DATABASE "+quoteIdent(templateBuildingDBName)); err != nil {
		return fmt.Errorf("create building db: %w", err)
	}

	// From here on, any failure (migrate, hydrate, rename) must not leave the
	// temp name behind for the next run to trip over.
	defer func() {
		if err == nil {
			return
		}
		if _, derr := conn.ExecContext(ctx,
			"DROP DATABASE IF EXISTS "+quoteIdent(templateBuildingDBName)+" WITH (FORCE)"); derr != nil {
			slog.Warn("testdb: drop building db after failed build", "err", derr)
		}
	}()

	if err = migrateBuildingTemplate(ctx, maintDSN); err != nil {
		return err
	}

	// Promote: the rename is atomic and is the sole commit point. It requires
	// no sessions on the building DB (migrateBuildingTemplate closed its handle)
	// and that templateDBName is absent (ensureTemplate dropped it if invalid,
	// and returned early if valid).
	if _, err = conn.ExecContext(ctx,
		"ALTER DATABASE "+quoteIdent(templateBuildingDBName)+" RENAME TO "+quoteIdent(templateDBName)); err != nil {
		return fmt.Errorf("promote building db: %w", err)
	}
	return nil
}

// migrateBuildingTemplate opens a dedicated handle to the building database,
// runs the goose schema migrations plus the fixture hydration hook, then closes
// the handle — the promotion rename refuses to proceed while any session is
// connected to the building DB, so this connection must not outlive the build.
func migrateBuildingTemplate(ctx context.Context, maintDSN string) error {
	buildDSN, err := withDatabase(maintDSN, templateBuildingDBName)
	if err != nil {
		return fmt.Errorf("derive building dsn: %w", err)
	}
	tdb, err := sql.Open("pgx", buildDSN)
	if err != nil {
		return fmt.Errorf("open building db: %w", err)
	}

	if err := database.Migrate(tdb); err != nil {
		tdb.Close()
		return fmt.Errorf("migrate template: %w", err)
	}
	if err := hydrateFixtures(ctx, buildDSN); err != nil {
		tdb.Close()
		return fmt.Errorf("hydrate fixtures: %w", err)
	}
	if err := tdb.Close(); err != nil {
		return fmt.Errorf("close building db: %w", err)
	}
	return nil
}

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
