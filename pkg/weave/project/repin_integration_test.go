//go:build integration

package project_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/project"
)

// TestRepinChildren covers the cases that decide whether an operator can
// trust this command, which are mostly the ones where a child does NOT move:
// a draft follower kept live by default, a child already at the target, a
// child outside --from, and a mistyped argument that must fail rather than
// report an empty run.
//
// Scoped to a synthetic parent so the real fixture projects' inheritance is
// untouched; the package's other tests assert on those.
func TestRepinChildren(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	const parent = "ZRP"
	const (
		childPinnedOld = "ZRPOLD"   // release 0.1.0 -> moves
		childDraft     = "ZRPDRAFT" // draft        -> left alone by default
		childAtTarget  = "ZRPAT"    // release 0.2.0 -> already there
		childOther     = "ZRPOTHER" // release 0.1.5 -> outside --from 0.1.0
	)

	// weave_projects.owner_id is NOT NULL and FK-enforced, so the probe
	// projects need an owner of their own rather than borrowing a fixture
	// actor this test would then have to be careful not to disturb.
	mustRepinExec(t, pool, `INSERT INTO weave_actors (id, type, display_name, slug, visibility, created_at, updated_at)
		VALUES ('ZRP_OWNER','organization','Repin Probe Org','zrp-owner','private',NOW(),NOW())
		ON CONFLICT (id) DO NOTHING`)

	seedRepinProject(t, pool, parent)
	for _, id := range []string{childPinnedOld, childDraft, childAtTarget, childOther} {
		seedRepinProject(t, pool, id)
	}

	mustRepinExec(t, pool, `INSERT INTO weave_releases (project_id, version, title, created_by_id, created_at)
		VALUES ($1, '0.1.0', 'old', 'ZRP_OWNER', now() - interval '2 days'),
		       ($1, '0.2.0', 'new', 'ZRP_OWNER', now() - interval '1 day')`, parent)

	link := func(child, mode, version string) {
		t.Helper()
		var v any
		if version != "" {
			v = version
		}
		mustRepinExec(t, pool, `INSERT INTO weave_project_inheritance
			(project_id, parent_project_id, is_primary, canonical_order, source_mode, source_version)
			VALUES ($1, $2, true, 0, $3, $4)`, child, parent, mode, v)
	}
	link(childPinnedOld, "release", "0.1.0")
	link(childDraft, "draft", "")
	link(childAtTarget, "release", "0.2.0")
	link(childOther, "release", "0.1.5")

	t.Cleanup(func() {
		bg := context.Background()
		for _, stmt := range []string{
			`DELETE FROM weave_project_inheritance WHERE parent_project_id=$1`,
			`DELETE FROM weave_releases WHERE project_id=$1`,
		} {
			if _, err := pool.Exec(bg, stmt, parent); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
		for _, id := range []string{parent, childPinnedOld, childDraft, childAtTarget, childOther} {
			if _, err := pool.Exec(bg, `DELETE FROM weave_projects WHERE id=$1`, id); err != nil {
				t.Errorf("cleanup project %s: %v", id, err)
			}
		}
		if _, err := pool.Exec(bg, `DELETE FROM weave_actors WHERE id='ZRP_OWNER'`); err != nil {
			t.Errorf("cleanup owner actor: %v", err)
		}
	})

	t.Run("a mistyped parent is an error, not an empty run", func(t *testing.T) {
		if _, err := project.RepinChildren(ctx, pool, "ZRPNOPE", "0.2.0", project.RepinOptions{}); !errors.Is(err, project.ErrNoSuchParent) {
			t.Fatalf("err = %v, want ErrNoSuchParent — a typo must not read as nothing to do", err)
		}
	})

	t.Run("a version the parent never released is an error", func(t *testing.T) {
		if _, err := project.RepinChildren(ctx, pool, parent, "9.9.9", project.RepinOptions{}); !errors.Is(err, project.ErrNoSuchRelease) {
			t.Fatalf("err = %v, want ErrNoSuchRelease — pinning to a version that does not exist would leave every child resolving nothing", err)
		}
	})

	t.Run("a dry run writes nothing", func(t *testing.T) {
		report, err := project.RepinChildren(ctx, pool, parent, "0.2.0", project.RepinOptions{})
		if err != nil {
			t.Fatalf("dry run: %v", err)
		}
		if report.Applied {
			t.Errorf("dry run reported Applied")
		}
		// Without --from, every already-pinned child moves whatever version
		// it sits on: the 0.1.0 pin and the 0.1.5 one. The draft follower and
		// the child already at 0.2.0 do not.
		if got := report.Moved(); got != 2 {
			t.Errorf("dry run would move %d children, want 2 (the 0.1.0 and 0.1.5 pins)", got)
		}
		if v := currentPin(t, pool, childPinnedOld, parent); v != "0.1.0" {
			t.Errorf("%s moved to %q during a DRY RUN", childPinnedOld, v)
		}
	})

	t.Run("apply moves the pinned child and leaves the rest", func(t *testing.T) {
		report, err := project.RepinChildren(ctx, pool, parent, "0.2.0", project.RepinOptions{Apply: true})
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if !report.Applied {
			t.Fatalf("apply reported Applied=false")
		}
		if v := currentPin(t, pool, childPinnedOld, parent); v != "0.2.0" {
			t.Errorf("%s = %q, want 0.2.0", childPinnedOld, v)
		}
		// The draft follower is the case worth guarding: pinning it would
		// silently take away the live view it deliberately asked for.
		if mode := currentMode(t, pool, childDraft, parent); mode != "draft" {
			t.Errorf("%s mode = %q, want draft — a draft follower must not be pinned without --include-draft", childDraft, mode)
		}
		if v := currentPin(t, pool, childAtTarget, parent); v != "0.2.0" {
			t.Errorf("%s = %q, want 0.2.0 (unchanged)", childAtTarget, v)
		}
	})

	t.Run("--from limits the move", func(t *testing.T) {
		// The subtest above moved both pinned children, so put them back:
		// this case needs one child at 0.1.0 and one at 0.1.5 to show that
		// --from selects between them.
		mustRepinExec(t, pool, `UPDATE weave_project_inheritance SET source_version='0.1.0'
			WHERE project_id=$1 AND parent_project_id=$2`, childPinnedOld, parent)
		mustRepinExec(t, pool, `UPDATE weave_project_inheritance SET source_version='0.1.5'
			WHERE project_id=$1 AND parent_project_id=$2`, childOther, parent)

		report, err := project.RepinChildren(ctx, pool, parent, "0.2.0", project.RepinOptions{From: "0.1.0", Apply: true})
		if err != nil {
			t.Fatalf("apply with --from: %v", err)
		}
		if got := report.Moved(); got != 1 {
			t.Errorf("moved %d children, want 1 — only the child pinned at 0.1.0", got)
		}
		if v := currentPin(t, pool, childPinnedOld, parent); v != "0.2.0" {
			t.Errorf("%s = %q, want 0.2.0 — --from 0.1.0 should have moved it", childPinnedOld, v)
		}
		if v := currentPin(t, pool, childOther, parent); v != "0.1.5" {
			t.Errorf("%s = %q, want 0.1.5 — --from 0.1.0 must not move a child pinned at 0.1.5", childOther, v)
		}
	})

	t.Run("--include-draft moves the draft follower", func(t *testing.T) {
		if _, err := project.RepinChildren(ctx, pool, parent, "0.2.0", project.RepinOptions{IncludeDraft: true, Apply: true}); err != nil {
			t.Fatalf("apply with --include-draft: %v", err)
		}
		if v := currentPin(t, pool, childDraft, parent); v != "0.2.0" {
			t.Errorf("%s = %q, want 0.2.0", childDraft, v)
		}
	})

	t.Run("the report names every child and why it was skipped", func(t *testing.T) {
		report, err := project.RepinChildren(ctx, pool, parent, "0.2.0", project.RepinOptions{})
		if err != nil {
			t.Fatalf("report run: %v", err)
		}
		var sb strings.Builder
		if err := report.Render(&sb); err != nil {
			t.Fatalf("render: %v", err)
		}
		out := sb.String()
		for _, id := range []string{childPinnedOld, childDraft, childAtTarget, childOther} {
			if !strings.Contains(out, id) {
				t.Errorf("report does not name %s:\n%s", id, out)
			}
		}
		if !strings.Contains(out, "already pinned here") {
			t.Errorf("report does not say why a child was skipped:\n%s", out)
		}
	})
}

func seedRepinProject(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	uiName, _ := json.Marshal(map[string]string{"en": id})
	mustRepinExec(t, pool, `INSERT INTO weave_projects (id, ui_name, description, status, owner_id, visibility, created_at, updated_at)
		VALUES ($1, $2::jsonb, $2::jsonb, 'draft', 'ZRP_OWNER', 'private', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING`, id, string(uiName))
}

func mustRepinExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func currentPin(t *testing.T, pool *pgxpool.Pool, child, parent string) string {
	t.Helper()
	var v string
	if err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(source_version, '') FROM weave_project_inheritance
		 WHERE project_id=$1 AND parent_project_id=$2`, child, parent).Scan(&v); err != nil {
		t.Fatalf("read pin for %s: %v", child, err)
	}
	return v
}

func currentMode(t *testing.T, pool *pgxpool.Pool, child, parent string) string {
	t.Helper()
	var m string
	if err := pool.QueryRow(context.Background(),
		`SELECT source_mode FROM weave_project_inheritance
		 WHERE project_id=$1 AND parent_project_id=$2`, child, parent).Scan(&m); err != nil {
		t.Fatalf("read mode for %s: %v", child, err)
	}
	return m
}
