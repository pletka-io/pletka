//go:build integration

package category_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/category"
)

// seedScopedCategories lays down a live category and a DIFFERENT archived one
// at 1.0.0, idempotently: the tests share one database and each wants the same
// rows, so re-seeding must be a no-op rather than a unique violation. The two
// rows carry different ids AND different names, so a read that answers with
// the live row under a release scope is visible as a wrong answer rather than
// a coincidence.
func seedScopedCategories(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	execCat(t, pool, `INSERT INTO weave_actors (id, display_name, slug)
	                  VALUES ('scoc','SCOC','scoc') ON CONFLICT DO NOTHING`)
	execCat(t, pool, `INSERT INTO weave_projects (id, owner_id)
	                  VALUES ('SCC','scoc') ON CONFLICT DO NOTHING`)
	execCat(t, pool, `INSERT INTO weave_categories (id, project_id, system_name, semantic_id, ui_name)
	                  VALUES ('CATLIVE','SCC','live_cat','SCC.C.1','{"en":"draft name"}')
	                  ON CONFLICT DO NOTHING`)
	execCat(t, pool, `INSERT INTO weave_categories_archive
	                    (id, project_id, system_name, semantic_id, ui_name, version_number)
	                  VALUES ('CATREL','SCC','rel_cat','SCC.C.2','{"en":"released name"}','1.0.0')
	                  ON CONFLICT DO NOTHING`)
}

func execCat(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("seed: %v\nSQL: %s", err, sql)
	}
}

// A read without a scope must fail loudly. Forgetting one is the defect the
// Store/Reader split exists to make unrepresentable, and the zero value is the
// last place it can still happen, so it errors at the first call rather than
// quietly reading the draft.
func TestCategoryStoreAtRejectsTheZeroScope(t *testing.T) {
	pool := testdb.Pool(t)
	if _, err := category.NewPostgresStore(pool).At(auth.ReadScope{}); err == nil {
		t.Fatal("At(zero scope) returned no error; a caller that forgets a scope must fail, not read draft")
	}
}

func TestCategoryReaderDraftReadsLive(t *testing.T) {
	pool := testdb.Pool(t)
	seedScopedCategories(t, pool)

	r, err := category.NewPostgresStore(pool).At(auth.Draft())
	if err != nil {
		t.Fatalf("At(draft): %v", err)
	}
	got, err := r.GetByID(context.Background(), "SCC", "CATLIVE")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("draft scope did not find the live category")
	}
	if got.UIName["en"] != "draft name" {
		t.Errorf("draft scope read %q, want the live row's %q", got.UIName["en"], "draft name")
	}
}

// The release scope must read the archive, and must NOT fall back to the live
// table when the archive has no such row. A release that archived nothing
// answers "not here"; falling back would present draft content under a
// release tag, which is the whole defect.
func TestCategoryReaderReleaseReadsArchiveAndDoesNotFallBack(t *testing.T) {
	pool := testdb.Pool(t)
	seedScopedCategories(t, pool)

	r, err := category.NewPostgresStore(pool).At(auth.Release("1.0.0"))
	if err != nil {
		t.Fatalf("At(release): %v", err)
	}
	ctx := context.Background()

	got, err := r.GetByID(ctx, "SCC", "CATREL")
	if err != nil {
		t.Fatalf("GetByID archived: %v", err)
	}
	if got == nil {
		t.Fatal("release scope did not find the archived category")
	}
	if got.UIName["en"] != "released name" {
		t.Errorf("release scope read %q, want the archived row's %q", got.UIName["en"], "released name")
	}

	// CATLIVE exists live but was never archived at 1.0.0.
	missing, err := r.GetByID(ctx, "SCC", "CATLIVE")
	if err != nil {
		t.Fatalf("GetByID un-archived: %v", err)
	}
	if missing != nil {
		t.Errorf("release scope fell back to the live row for a category the release never contained: %+v", missing)
	}
}

// List is the read the allowlist called out: "a pinned view lists categories
// renamed or added after the release."
func TestCategoryReaderListIsScoped(t *testing.T) {
	pool := testdb.Pool(t)
	seedScopedCategories(t, pool)
	ctx := context.Background()
	store := category.NewPostgresStore(pool)

	draft, err := store.At(auth.Draft())
	if err != nil {
		t.Fatalf("At(draft): %v", err)
	}
	live, err := draft.List(ctx, "SCC")
	if err != nil {
		t.Fatalf("List draft: %v", err)
	}
	if !containsCategoryNamed(live, "draft name") {
		t.Error("draft List did not return the live category")
	}
	if containsCategoryNamed(live, "released name") {
		t.Error("draft List returned an archived-only category")
	}

	rel, err := store.At(auth.Release("1.0.0"))
	if err != nil {
		t.Fatalf("At(release): %v", err)
	}
	archived, err := rel.List(ctx, "SCC")
	if err != nil {
		t.Fatalf("List release: %v", err)
	}
	if !containsCategoryNamed(archived, "released name") {
		t.Error("release List did not return the archived category")
	}
	if containsCategoryNamed(archived, "draft name") {
		t.Error("release List leaked a live category that the release never contained")
	}
}

func containsCategoryNamed(cats []*domain.Category, name string) bool {
	for _, c := range cats {
		if c != nil && c.UIName["en"] == name {
			return true
		}
	}
	return false
}
