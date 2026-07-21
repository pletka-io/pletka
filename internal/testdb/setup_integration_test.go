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
