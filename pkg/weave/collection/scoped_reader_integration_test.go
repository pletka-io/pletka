//go:build integration

package collection

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// newServiceWithPool builds the service over a real pool. The adoption store
// is always wired, with the receipts the caller names: a nil AdoptionStore
// makes ListAdoptedExplicit return early, so leaving it nil means no test can
// reach the adopted-collection resolution at all.
func newServiceWithPool(t *testing.T, receipts ...domain.Adoption) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	adoptions := &stubAdoptions{rows: receipts}
	return NewService(NewPostgresStore(pool), NewReadSessions(readsession.NewFactory(pool)), nil, adoptions, nil, nil, nil, nil, nil, nil, nil, nil), pool
}

// stubAdoptions stands in for the real receipt store, which lives in pkg/weave
// — a package that imports this one, so it cannot be used from here. It
// filters the way the real store does: a receipt is the ADOPTING project's own
// row, so a version filter selects on the receipt's own version_number.
type stubAdoptions struct{ rows []domain.Adoption }

func (s *stubAdoptions) List(_ context.Context, opts ...domain.QueryOption) ([]domain.Adoption, error) {
	cfg := domain.ApplyOptions(opts)
	out := []domain.Adoption{}
	for _, a := range s.rows {
		if cfg.ProjectID != "" && a.ProjectID != cfg.ProjectID {
			continue
		}
		if v, _ := cfg.Filters["entity_type"].(string); v != "" && a.EntityType != v {
			continue
		}
		if v, _ := cfg.Filters["context_entity_type"].(string); v != "" && a.ContextEntityType != v {
			continue
		}
		if v, _ := cfg.Filters["context_entity_id"].(string); v != "" && a.ContextEntityID != v {
			continue
		}
		if cfg.Version != "" && a.VersionNumber != cfg.Version {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

func (s *stubAdoptions) ReplaceForContext(_ context.Context, _, _, _ string, _ []domain.Adoption) error {
	return nil
}

func seedProject(t *testing.T, pool *pgxpool.Pool, projectID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO weave_projects (id, owner_id) VALUES ($1, 'unite') ON CONFLICT (id) DO NOTHING`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
}

func seedLiveNamed(t *testing.T, pool *pgxpool.Pool, projectID, id, name string) {
	t.Helper()
	seedProject(t, pool, projectID)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO weave_collections (id, project_id, system_name) VALUES ($1, $2, $3)`, id, projectID, name); err != nil {
		t.Fatalf("seed live collection: %v", err)
	}
}

func seedArchivedNamed(t *testing.T, pool *pgxpool.Pool, projectID, id, name, version string) {
	t.Helper()
	seedVerifiedRelease(t, pool, projectID, version)
	seedProject(t, pool, projectID)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO weave_collections_archive (id, project_id, system_name, version_number) VALUES ($1, $2, $3, $4)`,
		id, projectID, name, version); err != nil {
		t.Fatalf("seed archived collection: %v", err)
	}
}

// Missing release metadata is unavailable, not an empty successfully read release.
func TestListAtUnknownReleaseFailsClosed(t *testing.T) {
	svc, pool := newServiceWithPool(t)
	seedLiveNamed(t, pool, "SRI1", "SRI1.c1", "coll-live")

	got, n, err := svc.List(context.Background(), auth.Release("9.9.9"), "SRI1")
	if apierror.FromError(err).Code != apierror.CodeNotInRelease {
		t.Fatalf("List at unknown release: %v", err)
	}
	if len(got) != 0 || n != 0 {
		t.Fatalf("got %d rows (count %d); a release that archived nothing must not fall back to live rows", len(got), n)
	}
}

// TestGetAtReleaseMissesACollectionCreatedAfterIt pins Review Focus 2.
func TestGetAtReleaseMissesACollectionCreatedAfterIt(t *testing.T) {
	svc, pool := newServiceWithPool(t)
	seedArchivedNamed(t, pool, "SRI2", "SRI2.old", "coll-old", "1.0.0")
	seedLiveNamed(t, pool, "SRI2", "SRI2.new", "coll-new")

	got, err := svc.Get(context.Background(), auth.Release("1.0.0"), "SRI2", "SRI2.new")
	if apierror.FromError(err).Code != apierror.CodeNotInRelease {
		t.Fatalf("missing content at release: %v", err)
	}
	if got != nil {
		t.Fatal("a collection created after the release resolved under it")
	}
}

// TestListAppliesQueryOptionsOnBothPaths pins Review Focus 3.
func TestListAppliesQueryOptionsOnBothPaths(t *testing.T) {
	svc, pool := newServiceWithPool(t)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		seedArchivedNamed(t, pool, "SRI3", "SRI3.a."+name, name, "1.0.0")
		seedLiveNamed(t, pool, "SRI3", "SRI3.l."+name, name)
	}

	for _, scope := range []auth.ReadScope{auth.Draft(), auth.Release("1.0.0")} {
		got, _, err := svc.List(context.Background(), scope, "SRI3", domain.WithSearch("bet"))
		if err != nil {
			t.Fatalf("List(%v): %v", scope, err)
		}
		if len(got) != 1 || got[0].SystemName != "beta" {
			t.Fatalf("scope %v: search returned %d rows, want 1 (beta) — the archive path must honour the same options", scope, len(got))
		}
	}
}

// collectionReceipt is an explicit adoption receipt recorded by adopting at
// adoptedAt, naming the source entity at the source project's own release.
func collectionReceipt(adopting, adoptedAt, sourceProject, sourceEntity, sourceVersion string) domain.Adoption {
	return domain.Adoption{
		ProjectID:         adopting,
		ContextEntityType: "project",
		ContextEntityID:   adopting,
		EntityType:        "collection",
		SourceProjectID:   sourceProject,
		SourceEntityID:    sourceEntity,
		SourceVersion:     sourceVersion,
		VersionNumber:     adoptedAt,
	}
}

func seedArchivedOverrideRef(t *testing.T, pool *pgxpool.Pool, projectID, partOfCollectionID, version string) {
	t.Helper()
	seedProject(t, pool, projectID)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO weave_field_overrides_archive (field_id, project_id, part_of_collection_id, version_number)
		 VALUES ($1, $2, $3, $4)`, projectID+".f1", projectID, partOfCollectionID, version); err != nil {
		t.Fatalf("seed archived override: %v", err)
	}
}

// TestListAdoptedExplicitResolvesAtTheSourceVersion pins the rule that the
// archive PK is (id, version_number) global across projects while version
// strings are per-project: a foreign collection can only be resolved at the
// version its OWN project recorded.
func TestListAdoptedExplicitResolvesAtTheSourceVersion(t *testing.T) {
	svc, pool := newServiceWithPool(t, collectionReceipt("SRI4", "1.0.0", "SRI5", "SRI5.x", "7.0.0"))
	seedArchivedNamed(t, pool, "SRI4", "SRI4.own", "a-own", "1.0.0")
	// Project B archived its collection at ITS OWN version only; it has no
	// release called 1.0.0 at all.
	seedArchivedNamed(t, pool, "SRI5", "SRI5.x", "b-coll-at-7", "7.0.0")

	got, origins, err := svc.ListAdoptedExplicit(context.Background(), auth.Release("1.0.0"), "SRI4")
	if err != nil {
		t.Fatalf("ListAdoptedExplicit: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d adopted collections, want 1 — a foreign collection must be resolved at the SOURCE project's version (7.0.0), not the adopting project's (1.0.0)", len(got))
	}
	if got[0].SystemName != "b-coll-at-7" {
		t.Fatalf("got %q, want %q as the source's own release recorded it", got[0].SystemName, "b-coll-at-7")
	}
	if _, ok := origins["SRI5.x"]; !ok {
		t.Fatalf("origins %v carries no entry for the adopted id", origins)
	}
}

// TestListAdoptedExplicitIgnoresAVersionStringCollision proves the resolution
// follows the receipt's SourceVersion rather than a version string that
// happens to exist in both projects with different content.
func TestListAdoptedExplicitIgnoresAVersionStringCollision(t *testing.T) {
	svc, pool := newServiceWithPool(t, collectionReceipt("SRI6", "1.0.0", "SRI7", "SRI7.x", "7.0.0"))
	seedArchivedNamed(t, pool, "SRI7", "SRI7.x", "right-at-7", "7.0.0")
	// The same id in the same source project, in an unrelated release that
	// shares the adopting project's version string.
	seedArchivedNamed(t, pool, "SRI7", "SRI7.x", "wrong-at-1", "1.0.0")

	got, _, err := svc.ListAdoptedExplicit(context.Background(), auth.Release("1.0.0"), "SRI6")
	if err != nil {
		t.Fatalf("ListAdoptedExplicit: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d adopted collections, want 1", len(got))
	}
	if got[0].SystemName != "right-at-7" {
		t.Fatalf("got %q, want %q — a same-named release of the source project is not the adopted snapshot", got[0].SystemName, "right-at-7")
	}
}

// TestListAdoptedByReferenceAtReleaseSeesTheForeignCollection pins that a
// reference adoption stays visible in a release view. The overrides are read
// at the release; the foreign collection they point at is not archived under
// the adopting project's version and never will be.
func TestListAdoptedByReferenceAtReleaseSeesTheForeignCollection(t *testing.T) {
	svc, pool := newServiceWithPool(t)
	seedLiveNamed(t, pool, "SRI9", "SRI9.x", "foreign-live")
	seedArchivedOverrideRef(t, pool, "SRI8", "SRI9.x", "1.0.0")

	got, err := svc.ListAdoptedByReference(context.Background(), auth.Release("1.0.0"), "SRI8")
	if err != nil {
		t.Fatalf("ListAdoptedByReference: %v", err)
	}
	if len(got) != 1 || got[0].ID != "SRI9.x" {
		t.Fatalf("got %d reference-adopted collections %v, want exactly [SRI9.x] — a reference adoption must not vanish from a release view", len(got), got)
	}
}

// seedFilterable seeds the same collection shape on both paths: live, and
// archived at version. scopeClass drives the scope_class filter and categoryID
// the category_id filter.
func seedFilterable(t *testing.T, pool *pgxpool.Pool, projectID, id, name, scopeClass, categoryID, version string) {
	t.Helper()
	seedVerifiedRelease(t, pool, projectID, version)
	ctx := context.Background()
	seedProject(t, pool, projectID)
	prefix, local, _ := strings.Cut(scopeClass, ":")
	scope := fmt.Sprintf(`{"prefix":%q,"local_name":%q}`, prefix, local)
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_categories (id, project_id) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
		categoryID, projectID); err != nil {
		t.Fatalf("seed category: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_collections (id, project_id, system_name, ontology_scope, default_category_id)
		 VALUES ($1, $2, $3, $4::jsonb, $5)`,
		id+".live", projectID, name, scope, categoryID); err != nil {
		t.Fatalf("seed live filterable collection: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_collections_archive (id, project_id, system_name, ontology_scope, default_category_id, version_number)
		 VALUES ($1, $2, $3, $4::jsonb, $5, $6)`,
		id+".arch", projectID, name, scope, categoryID, version); err != nil {
		t.Fatalf("seed archived filterable collection: %v", err)
	}
}

// TestListFiltersApplyOnBothPaths pins the Reader's own contract: the same
// QueryOption handling on both paths. A filter honored on the draft and
// dropped on the archive makes the two disagree for one query — the release
// view would return every archived collection, with a matching total, while
// the dropdown that produced the filter was itself scoped to the archive.
func TestListFiltersApplyOnBothPaths(t *testing.T) {
	svc, pool := newServiceWithPool(t)
	seedFilterable(t, pool, "SRIF", "SRIF.place", "place", "crm:E53_Place", "SRIF.cat-a", "1.0.0")
	seedFilterable(t, pool, "SRIF", "SRIF.object", "object", "crm:E22_Man-Made_Object", "SRIF.cat-b", "1.0.0")

	cases := []struct {
		name string
		opt  domain.QueryOption
		want string
	}{
		{"scope_class", domain.WithFilter("scope_class", "crm:E53_Place"), "place"},
		{"category_id", domain.WithFilter("category_id", "SRIF.cat-b"), "object"},
	}
	for _, tc := range cases {
		for _, scope := range []auth.ReadScope{auth.Draft(), auth.Release("1.0.0")} {
			got, n, err := svc.List(context.Background(), scope, "SRIF", tc.opt)
			if err != nil {
				t.Fatalf("%s at %v: %v", tc.name, scope, err)
			}
			if len(got) != 1 || got[0].SystemName != tc.want {
				names := make([]string, 0, len(got))
				for _, c := range got {
					names = append(names, c.SystemName)
				}
				t.Errorf("%s filter at %v returned %v, want exactly [%s] — the archive path must honour the same filters as the draft", tc.name, scope, names, tc.want)
			}
			if n != 1 {
				t.Errorf("%s filter at %v reported total %d, want 1 — the COUNT query must carry the same filters as the row query", tc.name, scope, n)
			}
		}
	}
}
