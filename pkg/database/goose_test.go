package database

import (
	"io/fs"
	"strconv"
	"strings"
	"testing"
)

// TestLatestMigrationVersionMatchesTheEmbeddedFiles pins the value the fixture
// template's freshness check compares against. If this drifts — an empty set,
// a miscollected directory, a max taken from the wrong end — internal/testdb
// silently reuses a template built before the newest migration, which is the
// failure this function exists to prevent.
func TestLatestMigrationVersionMatchesTheEmbeddedFiles(t *testing.T) {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}

	// Derive the expectation independently of goose: the highest numeric
	// prefix among the embedded .sql files.
	var want int64
	var counted int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			t.Fatalf("migration %q has no version prefix", name)
		}
		v, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			t.Fatalf("migration %q has an unparseable version prefix: %v", name, err)
		}
		counted++
		if v > want {
			want = v
		}
	}
	if counted == 0 {
		t.Fatal("no .sql migrations are embedded — the embed pattern is broken")
	}

	got, err := LatestMigrationVersion()
	if err != nil {
		t.Fatalf("LatestMigrationVersion: %v", err)
	}
	if got != want {
		t.Errorf("LatestMigrationVersion = %d, want %d (highest of %d embedded files)", got, want, counted)
	}
}
