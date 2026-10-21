//go:build integration

package collection

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

func newServiceWithPool(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	return NewService(NewPostgresStore(pool), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil), pool
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
	seedProject(t, pool, projectID)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO weave_collections_archive (id, project_id, system_name, version_number) VALUES ($1, $2, $3, $4)`,
		id, projectID, name, version); err != nil {
		t.Fatalf("seed archived collection: %v", err)
	}
}

// TestListAtReleaseThatArchivedNothingReturnsEmpty pins Review Focus 1.
func TestListAtReleaseThatArchivedNothingReturnsEmpty(t *testing.T) {
	svc, pool := newServiceWithPool(t)
	seedLiveNamed(t, pool, "SRI1", "SRI1.c1", "coll-live")

	got, n, err := svc.List(context.Background(), auth.Release("9.9.9"), "SRI1")
	if err != nil {
		t.Fatalf("List at empty release: %v", err)
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
	if err != nil {
		t.Fatalf("Get at release: %v", err)
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
