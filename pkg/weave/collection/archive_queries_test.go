//go:build integration

package collection

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// TestScopeClassesAtVersionReadsTheArchive proves the generated query exists
// and answers from weave_collections_archive rather than the live table.
func TestScopeClassesAtVersionReadsTheArchive(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := sqlcgen.New(pool)

	seedArchivedCollectionWithFieldPath(t, pool, "proj-1", "coll-1", "1.0.0", "crm:E22_Man-Made_Object")
	// A live-only path that the release never carried.
	seedLiveCollectionWithFieldPath(t, pool, "proj-1", "coll-2", "crm:E53_Place")

	got, err := q.WeaveScopeClassesAtVersion(ctx, sqlcgen.WeaveScopeClassesAtVersionParams{
		ProjectID:     "proj-1",
		VersionNumber: "1.0.0",
	})
	if err != nil {
		t.Fatalf("WeaveScopeClassesAtVersion: %v", err)
	}
	if len(got) != 1 || got[0] != "crm:E22_Man-Made_Object" {
		t.Fatalf("got %v, want exactly [crm:E22_Man-Made_Object] — the live-only path must not appear", got)
	}
}

// TestCategoriesUsedAtVersionReturnsEveryProjectCategory pins the semantics of
// the live query this mirrors: ALL of the project's categories at the version,
// not only the ones some placement referenced. The live query's own comment
// records why — a placements join returned 0 rows on projects whose collections
// were not yet tagged with default_category_id, and the filter dropdown looked
// broken. A release view must not bring that back behind a version.
func TestCategoriesUsedAtVersionReturnsEveryProjectCategory(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := sqlcgen.New(pool)

	// Two categories at 1.0.0. Only one is referenced by a placement; both must
	// come back. A third belongs to another version and must not.
	seedArchivedCategory(t, pool, "proj-1", "cat-placed", "1.0.0", 1)
	seedArchivedCategory(t, pool, "proj-1", "cat-unplaced", "1.0.0", 2)
	seedArchivedCategory(t, pool, "proj-1", "cat-other-version", "2.0.0", 3)
	seedArchivedPlacement(t, pool, "proj-1", "model-1", "cat-placed", "coll-1", "1.0.0")

	got, err := q.WeaveCategoriesUsedAtVersion(ctx, sqlcgen.WeaveCategoriesUsedAtVersionParams{
		ProjectID:     "proj-1",
		VersionNumber: "1.0.0",
	})
	if err != nil {
		t.Fatalf("WeaveCategoriesUsedAtVersion: %v", err)
	}
	ids := make([]string, 0, len(got))
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	want := []string{"cat-placed", "cat-unplaced"}
	if len(ids) != len(want) {
		t.Fatalf("got %v, want %v — the unplaced category must survive", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("got %v, want %v in canonical_order", ids, want)
		}
	}
}

func seedArchivedCategory(t *testing.T, pool *pgxpool.Pool, projectID, categoryID, version string, order int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO weave_categories_archive (id, project_id, canonical_order, version_number)
		 VALUES ($1, $2, $3, $4)`, categoryID, projectID, order, version); err != nil {
		t.Fatalf("seed archived category: %v", err)
	}
}

func seedArchivedCollectionWithFieldPath(t *testing.T, pool *pgxpool.Pool, projectID, collectionID, version, scopeClass string) {
	t.Helper()
	prefix, local, _ := strings.Cut(scopeClass, ":")
	scope := fmt.Sprintf(`{"prefix":%q,"local_name":%q}`, prefix, local)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO weave_collections_archive (id, project_id, ontology_scope, version_number)
		 VALUES ($1, $2, $3::jsonb, $4)`, collectionID, projectID, scope, version); err != nil {
		t.Fatalf("seed archived collection: %v", err)
	}
}

func seedLiveCollectionWithFieldPath(t *testing.T, pool *pgxpool.Pool, projectID, collectionID, scopeClass string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1, 'unite')
		ON CONFLICT (id) DO NOTHING`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	prefix, local, _ := strings.Cut(scopeClass, ":")
	scope := fmt.Sprintf(`{"prefix":%q,"local_name":%q}`, prefix, local)
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_collections (id, project_id, ontology_scope) VALUES ($1, $2, $3::jsonb)`,
		collectionID, projectID, scope); err != nil {
		t.Fatalf("seed live collection: %v", err)
	}
}

func seedArchivedPlacement(t *testing.T, pool *pgxpool.Pool, projectID, modelID, categoryID, collectionID, version string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO weave_collection_placements_archive (id, project_id, model_id, category_id, collection_id, version_number)
		 VALUES (1, $1, $2, $3, $4, $5)`, projectID, modelID, categoryID, collectionID, version); err != nil {
		t.Fatalf("seed archived placement: %v", err)
	}
}
