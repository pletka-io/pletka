//go:build integration

package testdb_test

import (
	"context"
	"os"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }

func TestClonedTemplateHasSchema(t *testing.T) {
	pool := testdb.Pool(t)
	var n int
	// weave_projects exists because the clone was made from a migrated template.
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables WHERE table_name='weave_projects'`).Scan(&n)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Fatalf("weave_projects table missing in clone: got %d", n)
	}
}

func TestClonedTemplateHasFixtures(t *testing.T) {
	pool := testdb.Pool(t)
	var n int
	// The three curated fixtures are hydrated into the template, so every
	// clone inherits them.
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM weave_projects WHERE id IN ($1, $2, $3)`,
		testdb.FixtureSingle, testdb.FixtureParent, testdb.FixtureChild).Scan(&n)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 fixture projects in clone, got %d", n)
	}
}
