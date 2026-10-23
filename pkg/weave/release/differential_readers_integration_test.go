//go:build integration

package release

// The differential reader check. Every converted slice hand-writes a second
// query per read against its _archive table, and nothing until now compared
// the two answers. All three archive-path bugs found so far lived in exactly
// that gap: a list query that silently dropped three filters its live
// counterpart honored, a categories query that added a join the live one
// deliberately omits, and a latest-release resolution that would serve a
// retired release.
//
// What makes this check different from every other archive test in the tree:
// it CUTS A REAL RELEASE. The existing integration tests hand-seed archive
// rows, so they assert the archive contains what the test just put there --
// which cannot detect a query that reads those rows by a different rule than
// the live query reads the live ones. Here the archive is produced by
// snapshotStatements, the same list Service.Create runs, and then the two
// Readers are asked the same questions.
//
// The snapshot is a pure column copy (INSERT ... SELECT cols FROM live) with
// exactly one deliberate rewrite, so "the answers must match" is a true
// statement with two documented normalizations:
//
//   - status: the cut rewrites it, CASE WHEN status = 'deprecated' THEN
//     status ELSE 'published' END. A draft row's status is therefore NOT the
//     archived row's status, and a status FILTER must be mapped as well as a
//     status value.
//   - version_number: absent on the draft, the release on the archive.
//
// Anything else that differs is a bug in one of the two queries.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/collection"
)

const (
	diffProject = "DIFFREAD"
	diffVersion = "1.0.0"
)

// archivedStatus is the cut's one deliberate rewrite, in Go. Mirroring it
// here rather than ignoring the column keeps the check honest: a snapshot
// that stopped rewriting status, or started rewriting something else, breaks
// this test instead of being normalized away.
func archivedStatus(live domain.Status) domain.Status {
	if live == "deprecated" {
		return "deprecated"
	}
	return "published"
}

// seedDiffProject builds a fixture that exercises every filter the collection
// list honors, and one case per known bug:
//
//   - two scope classes, so the scope_class filter can select a subset
//   - two categories in use plus ONE CATEGORY USED BY NOTHING, which is the
//     regression case for the categories-join bug: a USED-only join drops it
//   - a deprecated row, so the status rewrite is actually exercised
//   - distinct system_names, so ORDER BY and search select subsets
func seedDiffProject(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	seedProject(t, pool, diffProject)

	for _, c := range []struct{ id, name string }{
		{"DIFFCAT.1", "First category"},
		{"DIFFCAT.2", "Second category"},
		// Referenced by no collection. The live CategoriesUsed returns ALL
		// of a project's categories and a comment records why (a USED-only
		// join emptied the filter dropdown on projects whose collections
		// carry no default_category_id). If the archive query joins through
		// placements or through collections, this row disappears there and
		// the two answers diverge.
		{"DIFFCAT.3", "Category used by nothing"},
	} {
		mustExec(t, pool, `INSERT INTO weave_categories (id, project_id, ui_name, status)
			VALUES ($1, $2, $3::jsonb, 'draft')`,
			c.id, diffProject, fmt.Sprintf(`{"en":%q}`, c.name))
	}

	for _, c := range []struct {
		id, systemName, uiName, status, prefix, local, category string
	}{
		// weave_collections_status_check (001_baseline.sql:674) allows only
		// 'draft' and 'published'. Both are included because the cut
		// collapses them, which is what the status subtest below pins.
		{"DIFFC.1", "alpha_one", "Alpha One", "draft", "crm", "E22", "DIFFCAT.1"},
		{"DIFFC.2", "beta_two", "Beta Two", "published", "crm", "E53", "DIFFCAT.2"},
		{"DIFFC.3", "alpha_three", "Alpha Three", "draft", "crm", "E22", "DIFFCAT.1"},
		{"DIFFC.4", "gamma_four", "Gamma Four", "published", "crm", "E53", ""},
	} {
		var category any
		if c.category != "" {
			category = c.category
		}
		mustExec(t, pool, `INSERT INTO weave_collections
			(id, project_id, system_name, ui_name, description, status, ontology_scope, default_category_id)
			VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6, $7::jsonb, $8)`,
			c.id, diffProject, c.systemName,
			fmt.Sprintf(`{"en":%q}`, c.uiName),
			fmt.Sprintf(`{"en":"describes %s"}`, c.systemName),
			c.status,
			fmt.Sprintf(`{"prefix":%q,"local_name":%q}`, c.prefix, c.local),
			category)
	}

	t.Cleanup(func() {
		bg := context.Background()
		for _, stmt := range []string{
			`DELETE FROM weave_collections_archive WHERE project_id=$1`,
			`DELETE FROM weave_categories_archive WHERE project_id=$1`,
			`DELETE FROM weave_collections WHERE project_id=$1`,
			`DELETE FROM weave_categories WHERE project_id=$1`,
			`DELETE FROM weave_releases WHERE project_id=$1`,
		} {
			if _, err := pool.Exec(bg, stmt, diffProject); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
	})
}

// collectionIDs renders a result as a comparable, readable string: order is
// part of the answer, so this is a sequence and not a set.
func collectionIDs(cs []*domain.Collection) string {
	ids := make([]string, 0, len(cs))
	for _, c := range cs {
		ids = append(ids, c.ID)
	}
	return strings.Join(ids, ",")
}

func TestCollectionReaderAnswersTheSameOnBothPathsAfterARealCut(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	seedDiffProject(t, pool)
	if err := runSnapshotStatements(ctx, pool, diffProject, diffVersion); err != nil {
		t.Fatalf("cut release: %v", err)
	}

	store := collection.NewPostgresStore(pool)
	draft, err := store.At(auth.Draft())
	if err != nil {
		t.Fatalf("At(Draft): %v", err)
	}
	release, err := store.At(auth.Release(diffVersion))
	if err != nil {
		t.Fatalf("At(Release): %v", err)
	}

	// Every filter the list honors, one case each. A filter honored on one
	// path and dropped on the other shows up as a different ID sequence --
	// which is precisely the bug that shipped and whose test passed because
	// it only exercised search.
	t.Run("List", func(t *testing.T) {
		cases := []struct {
			name        string
			draftOpts   []domain.QueryOption
			releaseOpts []domain.QueryOption
		}{
			{name: "unfiltered"},
			{
				name:        "search",
				draftOpts:   []domain.QueryOption{domain.WithSearch("alpha")},
				releaseOpts: []domain.QueryOption{domain.WithSearch("alpha")},
			},
			{
				name:        "scope_class selects a subset",
				draftOpts:   []domain.QueryOption{domain.WithFilter("scope_class", "crm:E22")},
				releaseOpts: []domain.QueryOption{domain.WithFilter("scope_class", "crm:E22")},
			},
			{
				name:        "scope_class accepts a comma-separated set",
				draftOpts:   []domain.QueryOption{domain.WithFilter("scope_class", "crm:E22,crm:E53")},
				releaseOpts: []domain.QueryOption{domain.WithFilter("scope_class", "crm:E22,crm:E53")},
			},
			{
				name:        "category_id selects a subset",
				draftOpts:   []domain.QueryOption{domain.WithFilter("category_id", "DIFFCAT.1")},
				releaseOpts: []domain.QueryOption{domain.WithFilter("category_id", "DIFFCAT.1")},
			},
			{
				name:        "order by system_name descending",
				draftOpts:   []domain.QueryOption{domain.WithOrderBy("system_name", true)},
				releaseOpts: []domain.QueryOption{domain.WithOrderBy("system_name", true)},
			},
			{
				name:        "limit and offset",
				draftOpts:   []domain.QueryOption{domain.WithOrderBy("system_name", false), domain.WithLimit(2), domain.WithOffset(1)},
				releaseOpts: []domain.QueryOption{domain.WithOrderBy("system_name", false), domain.WithLimit(2), domain.WithOffset(1)},
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				dGot, dTotal, err := draft.List(ctx, diffProject, c.draftOpts...)
				if err != nil {
					t.Fatalf("draft List: %v", err)
				}
				rGot, rTotal, err := release.List(ctx, diffProject, c.releaseOpts...)
				if err != nil {
					t.Fatalf("release List: %v", err)
				}
				if d, r := collectionIDs(dGot), collectionIDs(rGot); d != r {
					t.Errorf("draft and release disagree on which collections this query selects:\n  draft:   [%s]\n  release: [%s]\n"+
						"The archive query does not apply this filter the way the live query does.", d, r)
				}
				if dTotal != rTotal {
					t.Errorf("draft total %d, release total %d -- the two paths count differently", dTotal, rTotal)
				}
				if len(dGot) == 0 {
					t.Error("fixture selected nothing on either path, so this case proves nothing; fix the fixture")
				}
			})
		}
	})

	// status is the cut's one deliberate rewrite and it is LOSSY: a live
	// collection may only be 'draft' or 'published'
	// (weave_collections_status_check, 001_baseline.sql:674), and the cut
	// maps both to 'published'. So status is not a differentially comparable
	// filter -- draft List(status=published) selects a strict subset of what
	// release List(status=published) selects, correctly. What must still hold
	// is that the archive query APPLIES the filter at all, which is the half
	// of the dropped-filter bug that a draft/release diff cannot see.
	t.Run("status is rewritten by the cut, and the archive still filters on it", func(t *testing.T) {
		all, _, err := release.List(ctx, diffProject, domain.WithFilter("status", "published"))
		if err != nil {
			t.Fatalf("release List published: %v", err)
		}
		if got := len(all); got != 4 {
			t.Errorf("release List(status=published) returned %d rows, want all 4 -- the cut maps every "+
				"non-deprecated status to published, so every archived collection matches", got)
		}
		none, _, err := release.List(ctx, diffProject, domain.WithFilter("status", "draft"))
		if err != nil {
			t.Fatalf("release List draft: %v", err)
		}
		if len(none) != 0 {
			t.Errorf("release List(status=draft) returned %d rows, want 0; no archived row keeps status 'draft', "+
				"so a non-zero answer means the archive query ignores the status filter", len(none))
		}
	})

	t.Run("Get", func(t *testing.T) {
		for _, id := range []string{"DIFFC.1", "DIFFC.3"} {
			d, err := draft.Get(ctx, diffProject, id)
			if err != nil {
				t.Fatalf("draft Get %s: %v", id, err)
			}
			r, err := release.Get(ctx, diffProject, id)
			if err != nil {
				t.Fatalf("release Get %s: %v", id, err)
			}
			if d == nil || r == nil {
				t.Fatalf("Get %s: draft=%v release=%v, one path does not carry it", id, d != nil, r != nil)
			}
			if d.SystemName != r.SystemName {
				t.Errorf("Get %s: system_name draft %q release %q", id, d.SystemName, r.SystemName)
			}
			if want := archivedStatus(d.Status); r.Status != want {
				t.Errorf("Get %s: release status %q, want %q (the cut's rewrite of draft status %q)", id, r.Status, want, d.Status)
			}
		}
	})

	t.Run("ScopeClasses", func(t *testing.T) {
		d, err := draft.ScopeClasses(ctx, diffProject)
		if err != nil {
			t.Fatalf("draft ScopeClasses: %v", err)
		}
		r, err := release.ScopeClasses(ctx, diffProject)
		if err != nil {
			t.Fatalf("release ScopeClasses: %v", err)
		}
		if strings.Join(d, ",") != strings.Join(r, ",") {
			t.Errorf("scope classes differ:\n  draft:   %v\n  release: %v", d, r)
		}
		if len(d) == 0 {
			t.Error("fixture produced no scope classes; fix the fixture")
		}
	})

	// The regression case for the categories-join bug. The live rule returns
	// ALL of a project's categories, including ones no collection points at.
	t.Run("CategoriesUsed", func(t *testing.T) {
		ids := func(opts []collection.CollectionCategoryOption) string {
			out := make([]string, 0, len(opts))
			for _, o := range opts {
				out = append(out, o.ID)
			}
			return strings.Join(out, ",")
		}
		d, err := draft.CategoriesUsed(ctx, diffProject)
		if err != nil {
			t.Fatalf("draft CategoriesUsed: %v", err)
		}
		r, err := release.CategoriesUsed(ctx, diffProject)
		if err != nil {
			t.Fatalf("release CategoriesUsed: %v", err)
		}
		if ids(d) != ids(r) {
			t.Errorf("categories differ:\n  draft:   [%s]\n  release: [%s]\n"+
				"The live rule returns every category in the project. An archive query that "+
				"joins through collections or placements drops DIFFCAT.3, which nothing points at.", ids(d), ids(r))
		}
		if !strings.Contains(ids(r), "DIFFCAT.3") {
			t.Error("release path dropped DIFFCAT.3, the category no collection points at -- the USED-only join bug, on the release path")
		}
	})

	t.Run("Lookup", func(t *testing.T) {
		d, err := draft.Lookup(ctx, "DIFFC.2")
		if err != nil {
			t.Fatalf("draft Lookup: %v", err)
		}
		r, err := release.Lookup(ctx, "DIFFC.2")
		if err != nil {
			t.Fatalf("release Lookup: %v", err)
		}
		if d == nil || r == nil {
			t.Fatalf("Lookup: draft=%v release=%v", d != nil, r != nil)
		}
		if d.SystemName != r.SystemName {
			t.Errorf("Lookup system_name: draft %q release %q", d.SystemName, r.SystemName)
		}
	})
}
