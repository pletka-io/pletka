//go:build integration

package override

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
)

// TestScopedReader_ReadsTheRightTable pins the whole point of the slice: a
// draft scope reads live rows, a release scope reads that release's archive,
// and neither falls back to the other. Falling back is the defect — it is how
// a release view came to show draft overrides.
func TestScopedReader_ReadsTheRightTable(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := NewPostgresStore(pool)

	const projectID, entityID, version = "ZSCOPE", "ZSCOPEM.1", "1.0.0"

	// Seed: two overrides live, but only ONE archived at 1.0.0 — so the two
	// scopes cannot return the same rows by accident. f1LiveID is the live
	// (never-archived-by-that-id) override that also carries one live ref
	// row, used below to prove a release scope does not fall back to it.
	f1LiveID := seedScopeFixture(t, pool, projectID, entityID, version)

	t.Run("draft reads live", func(t *testing.T) {
		r, err := store.At(auth.Draft())
		if err != nil {
			t.Fatalf("At(Draft): %v", err)
		}
		got, err := r.ListForEntity(ctx, "model", entityID)
		if err != nil {
			t.Fatalf("ListForEntity: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("draft scope returned %d overrides, want 2 (the live rows)", len(got))
		}
	})

	t.Run("a release reads that release's archive", func(t *testing.T) {
		r, err := store.At(auth.Release(version))
		if err != nil {
			t.Fatalf("At(Release): %v", err)
		}
		got, err := r.ListForEntity(ctx, "model", entityID)
		if err != nil {
			t.Fatalf("ListForEntity: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("release scope returned %d overrides, want 1 (only the archived row)", len(got))
		}
	})

	// Review Focus 3: a version that was never released has an empty archive.
	// It must read empty, never fall back to the 2 live rows.
	t.Run("an unreleased version reads empty, it does not fall back", func(t *testing.T) {
		r, err := store.At(auth.Release("9.9.9"))
		if err != nil {
			t.Fatalf("At(Release 9.9.9): %v", err)
		}
		got, err := r.ListForEntity(ctx, "model", entityID)
		if err != nil {
			t.Fatalf("ListForEntity: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("unreleased version returned %d overrides, want 0 — a release scope must never serve draft rows", len(got))
		}
	})

	// Review Focus 1: the zero scope is a caller that forgot.
	t.Run("an invalid scope is refused", func(t *testing.T) {
		var zero auth.ReadScope
		if _, err := store.At(zero); err == nil {
			t.Fatal("At(zero) returned no error — a forgotten scope must fail loudly, not read draft")
		}
	})

	// Review Focus 5: the two archive tables are filled by separate snapshot
	// statements. A read must not assume an archived override has archived
	// refs.
	t.Run("an archived override with no archived refs reads empty refs", func(t *testing.T) {
		r, err := store.At(auth.Release(version))
		if err != nil {
			t.Fatalf("At(Release): %v", err)
		}
		refs, err := r.GetRefs(ctx, archivedOverrideIDWithoutRefs(t, pool, projectID, version))
		if err != nil {
			t.Fatalf("GetRefs must not error when no refs were archived: %v", err)
		}
		if len(refs) != 0 {
			t.Errorf("got %d refs, want 0", len(refs))
		}
	})

	// Review Focus 5 (no-fallback proof): the subtest above shows "no error,
	// empty result" — it cannot show the reader "never falls back to the
	// live refs" because seedScopeFixture archives no refs at all, so a
	// reader that did fall back would also see zero. f1LiveID carries
	// exactly one live ref and zero archived ones, so a release scope that
	// wrongly fell back would return 1, not 0.
	t.Run("a release never falls back to an override's live refs", func(t *testing.T) {
		draftR, err := store.At(auth.Draft())
		if err != nil {
			t.Fatalf("At(Draft): %v", err)
		}
		draftRefs, err := draftR.GetRefs(ctx, f1LiveID)
		if err != nil {
			t.Fatalf("GetRefs (draft): %v", err)
		}
		if len(draftRefs) != 1 {
			t.Fatalf("draft scope returned %d refs, want 1 (the live ref) — fixture broken", len(draftRefs))
		}

		releaseR, err := store.At(auth.Release(version))
		if err != nil {
			t.Fatalf("At(Release): %v", err)
		}
		releaseRefs, err := releaseR.GetRefs(ctx, f1LiveID)
		if err != nil {
			t.Fatalf("GetRefs (release): %v", err)
		}
		if len(releaseRefs) != 0 {
			t.Errorf("release scope returned %d refs for a live-only ref, want 0 — it fell back to the live table", len(releaseRefs))
		}
	})
}

// seedScopeFixture creates a scratch project (with its own owner actor, since
// weave_projects.owner_id is NOT NULL and FK-enforced), two live override
// rows on (model, entityID), one live ref row on the first of those, and
// exactly one archived override row for the same entity at version — so a
// draft read and a release read cannot agree by accident. Registers
// t.Cleanup scoped by projectID; never by version alone, since this
// package's fixtures share one database and several tests reuse version
// "1.0.0". Returns the live id of the first override (f1), which carries
// the one live ref row and is never itself archived under that id.
func seedScopeFixture(t *testing.T, pool *pgxpool.Pool, projectID, entityID, version string) int64 {
	t.Helper()
	ownerID := projectID + "_OWNER"

	mustScopeExec(t, pool, `INSERT INTO weave_actors (id, type, display_name, slug, visibility, created_at, updated_at)
		VALUES ($1, 'organization', $1, $1, 'private', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING`, ownerID)

	uiName, err := json.Marshal(map[string]string{"en": projectID})
	if err != nil {
		t.Fatalf("marshal ui_name: %v", err)
	}
	mustScopeExec(t, pool, `INSERT INTO weave_projects (id, ui_name, description, status, owner_id, visibility, created_at, updated_at)
		VALUES ($1, $2::jsonb, $2::jsonb, 'draft', $3, 'private', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING`, projectID, string(uiName), ownerID)

	// Two live overrides on (model, entityID). f1's id is captured via
	// RETURNING so a live ref row can be attached to it below.
	var f1ID int64
	err = pool.QueryRow(context.Background(), `INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position)
		VALUES ($1 || '.f1', $1, 'model', $2, 0)
		RETURNING id`, projectID, entityID).Scan(&f1ID)
	if err != nil {
		t.Fatalf("insert f1 override: %v", err)
	}
	mustScopeExec(t, pool, `INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position)
		VALUES ($1 || '.f2', $1, 'model', $2, 1)`, projectID, entityID)

	// One live ref on f1, and no archived counterpart anywhere — used to
	// prove a release scope never falls back to the live refs table.
	mustScopeExec(t, pool, `INSERT INTO weave_override_refs
			(override_id, ref_type, target_id, semantic_id, position)
		VALUES ($1, 'resource_model', $2, $2, 0)`, f1ID, entityID)

	// Exactly one archived override for the same entity, at version. Its id
	// comes from the shared weave_field_overrides_id_seq so it can never
	// collide with a live row's id.
	mustScopeExec(t, pool, `INSERT INTO weave_field_overrides_archive
			(id, field_id, project_id, entity_type, entity_id, position, version_number)
		VALUES
			(nextval('weave_field_overrides_id_seq'), $1 || '.f1', $1, 'model', $2, 0, $3)`,
		projectID, entityID, version)

	t.Cleanup(func() {
		bg := context.Background()
		for _, stmt := range []string{
			`DELETE FROM weave_override_refs_archive WHERE project_id=$1`,
			`DELETE FROM weave_field_overrides_archive WHERE project_id=$1`,
			`DELETE FROM weave_override_refs WHERE override_id IN (SELECT id FROM weave_field_overrides WHERE project_id=$1)`,
			`DELETE FROM weave_field_overrides WHERE project_id=$1`,
			`DELETE FROM weave_projects WHERE id=$1`,
			`DELETE FROM weave_actors WHERE id=$1`,
		} {
			arg := projectID
			if stmt == `DELETE FROM weave_actors WHERE id=$1` {
				arg = ownerID
			}
			if _, err := pool.Exec(bg, stmt, arg); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
	})

	return f1ID
}

// archivedOverrideIDWithoutRefs returns the id of the single archived
// override seedScopeFixture wrote for projectID at version — a row with no
// corresponding weave_override_refs_archive rows.
func archivedOverrideIDWithoutRefs(t *testing.T, pool *pgxpool.Pool, projectID, version string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM weave_field_overrides_archive WHERE project_id = $1 AND version_number = $2`,
		projectID, version).Scan(&id)
	if err != nil {
		t.Fatalf("resolve archived override id for %s@%s: %v", projectID, version, err)
	}
	return id
}

func mustScopeExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}
