//go:build integration

package publication

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// seedLatestReleaseFixture builds a project TEST_LATEST with two releases,
// 0.10.0 and 0.9.0, where 0.10.0 has an EARLIER created_at than 0.9.0 — this
// proves LatestReleaseVersion orders by semver, not created_at. It also
// builds an unreleased project TEST_LATEST_NOREL.
func seedLatestReleaseFixture(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	cleanup := func() {
		for _, tbl := range []string{"weave_releases", "weave_projects"} {
			_, _ = pool.Exec(ctx, "DELETE FROM "+tbl+" WHERE project_id LIKE 'TEST_LATEST%'")
		}
		_, _ = pool.Exec(ctx, "DELETE FROM weave_actors WHERE id = 'TEST_LATEST_OWNER'")
	}
	cleanup()
	t.Cleanup(cleanup)

	must := func(sql string, args ...any) {
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed exec failed: %v\nSQL: %s", err, sql)
		}
	}

	must(`INSERT INTO weave_actors (id, type, display_name, slug, visibility) VALUES ('TEST_LATEST_OWNER','organization','Latest Org','latest-org','private') ON CONFLICT DO NOTHING`)
	must(`INSERT INTO weave_projects (id, ui_name, status, owner_id, visibility) VALUES ('TEST_LATEST','{"en":"Latest"}','draft','TEST_LATEST_OWNER','private') ON CONFLICT DO NOTHING`)
	must(`INSERT INTO weave_projects (id, ui_name, status, owner_id, visibility) VALUES ('TEST_LATEST_NOREL','{"en":"LatestNoRel"}','draft','TEST_LATEST_OWNER','private') ON CONFLICT DO NOTHING`)

	// 0.10.0 is inserted with an earlier created_at than 0.9.0 — semver
	// ordering must still pick 0.10.0 as the latest release.
	must(`INSERT INTO weave_releases (project_id, version, title, created_by_id, created_at) VALUES
		('TEST_LATEST','0.10.0','ten','TEST_LATEST_OWNER','2026-01-01T00:00:00Z'),
		('TEST_LATEST','0.9.0','nine','TEST_LATEST_OWNER','2026-02-01T00:00:00Z')`)
}

func TestLatestReleaseVersion(t *testing.T) {
	pool := testPool(t)
	seedLatestReleaseFixture(t, pool)
	r := NewReader(pool)
	ctx := context.Background()

	got, err := r.LatestReleaseVersion(ctx, "TEST_LATEST")
	if err != nil {
		t.Fatalf("LatestReleaseVersion: %v", err)
	}
	if got != "0.10.0" {
		t.Errorf("got %q want %q (semver order, not created_at order)", got, "0.10.0")
	}

	got, err = r.LatestReleaseVersion(ctx, "TEST_LATEST_NOREL")
	if err != nil {
		t.Fatalf("LatestReleaseVersion norel: %v", err)
	}
	if got != "" {
		t.Errorf("got %q want empty for a project with no release", got)
	}
}

// TestLatestReleaseVersion_SkipsArchived pins that archiving a release
// actually retires it. Before this, archiving changed nothing observable: the
// archived release stayed "latest", so it kept deciding what a public
// non-editor was served and kept supplying the created_at that every entity's
// state is derived against. A release cut by mistake made the whole draft
// render as "published" and archiving it did not move the baseline back.
func TestLatestReleaseVersion_SkipsArchived(t *testing.T) {
	pool := testPool(t)
	seedLatestReleaseFixture(t, pool)
	r := NewReader(pool)
	ctx := context.Background()

	// Sanity: 0.10.0 is latest while nothing is archived. Without this the
	// assertion below could pass because the fixture never had 0.10.0.
	if got, err := r.LatestReleaseVersion(ctx, "TEST_LATEST"); err != nil || got != "0.10.0" {
		t.Fatalf("precondition: got %q err %v, want 0.10.0", got, err)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE weave_releases SET archived_at = NOW(), archived_message = 'cut by mistake'
		 WHERE project_id = 'TEST_LATEST' AND version = '0.10.0'`); err != nil {
		t.Fatalf("archive 0.10.0: %v", err)
	}

	got, err := r.LatestReleaseVersion(ctx, "TEST_LATEST")
	if err != nil {
		t.Fatalf("LatestReleaseVersion after archiving: %v", err)
	}
	if got != "0.9.0" {
		t.Errorf("got %q, want 0.9.0 — an archived release must not remain latest", got)
	}

	// Every release archived reads the same as no release at all: callers
	// fall back to the working state rather than to a retired snapshot.
	if _, err := pool.Exec(ctx,
		`UPDATE weave_releases SET archived_at = NOW(), archived_message = 'cut by mistake'
		 WHERE project_id = 'TEST_LATEST'`); err != nil {
		t.Fatalf("archive all: %v", err)
	}
	got, err = r.LatestReleaseVersion(ctx, "TEST_LATEST")
	if err != nil {
		t.Fatalf("LatestReleaseVersion all archived: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty when every release is archived", got)
	}
}
