//go:build integration

package example_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave/example"
)

// seedScoped lays down a live example and a DIFFERENT archived one at 1.0.0,
// idempotently: the tests share one database and all three want the same two
// rows, so re-seeding must be a no-op rather than a unique violation.
// so a read that returns the live row under a release scope is visible as a
// wrong answer rather than a coincidence.
func seedScoped(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	exec(t, pool, `INSERT INTO weave_actors (id, display_name, slug) VALUES ('sco','SCO','sco') ON CONFLICT DO NOTHING`)
	exec(t, pool, `INSERT INTO weave_projects (id, owner_id) VALUES ('SCP','sco') ON CONFLICT DO NOTHING`)
	// entity_id carries no foreign key, so no model row is needed: these
	// tests are about which table a scope reads, not referential integrity.
	// live
	exec(t, pool, `INSERT INTO weave_examples (id, project_id, entity_type, entity_id, title, status)
	               VALUES ('EXLIVE','SCP','model','SCM','{"en":"draft title"}','draft') ON CONFLICT DO NOTHING`)
	// archived at 1.0.0, deliberately a different id AND title
	exec(t, pool, `INSERT INTO weave_examples_archive (id, project_id, entity_type, entity_id, title, status, version_number)
	               VALUES ('EXREL','SCP','model','SCM','{"en":"released title"}','published','1.0.0') ON CONFLICT DO NOTHING`)
}

// A read without a scope must fail loudly. Forgetting one is the defect the
// split exists to make unrepresentable; the zero value is the last place it
// can still happen, so it errors at the first call.
func TestExampleStoreAtRejectsTheZeroScope(t *testing.T) {
	pool := testdb.Pool(t)
	if _, err := example.NewPostgresStore(pool).At(auth.ReadScope{}); err == nil {
		t.Fatal("At(zero scope) returned no error; a caller that forgets a scope must fail, not read draft")
	}
}

func TestExampleReaderDraftReadsLive(t *testing.T) {
	pool := testdb.Pool(t)
	seedScoped(t, pool)

	r, err := example.NewPostgresStore(pool).At(auth.Draft())
	if err != nil {
		t.Fatalf("At(Draft): %v", err)
	}
	got, err := r.GetByID(context.Background(), "EXLIVE")
	if err != nil || got == nil {
		t.Fatalf("draft GetByID(EXLIVE): got=%v err=%v", got, err)
	}
	if got.Title["en"] != "draft title" {
		t.Errorf("draft read returned title %q, want %q", got.Title["en"], "draft title")
	}
}

// The assertion that matters: a release scope reads the archive, and the live
// row is NOT what comes back.
func TestExampleReaderReleaseReadsTheArchiveNotLive(t *testing.T) {
	pool := testdb.Pool(t)
	seedScoped(t, pool)
	ctx := context.Background()

	r, err := example.NewPostgresStore(pool).At(auth.Release("1.0.0"))
	if err != nil {
		t.Fatalf("At(Release): %v", err)
	}

	got, err := r.GetByID(ctx, "EXREL")
	if err != nil || got == nil {
		t.Fatalf("release GetByID(EXREL): got=%v err=%v", got, err)
	}
	if got.Title["en"] != "released title" {
		t.Errorf("release read returned title %q, want %q", got.Title["en"], "released title")
	}

	// The live row is not in this release, so it must not resolve here.
	if live, err := r.GetByID(ctx, "EXLIVE"); err == nil && live != nil {
		t.Errorf("release scope resolved EXLIVE, a row that exists only in the draft: %+v", live)
	}
}

// A release that archived nothing returns an EMPTY list, never a fallback to
// draft. "This release carries no examples" is a legitimate answer and the
// view layer decides how to render it.
func TestExampleReaderUnknownReleaseIsEmptyNotDraft(t *testing.T) {
	pool := testdb.Pool(t)
	seedScoped(t, pool)

	r, err := example.NewPostgresStore(pool).At(auth.Release("9.9.9"))
	if err != nil {
		t.Fatalf("At(Release): %v", err)
	}
	rows, total, err := r.List(context.Background())
	if err != nil {
		t.Fatalf("List at an unreleased version: %v", err)
	}
	if len(rows) != 0 || total != 0 {
		t.Errorf("a version that archived nothing returned %d rows (total %d); it must be empty, not a draft fallback", len(rows), total)
	}
}
