//go:build integration

package override

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// TestReleaseViewDoesNotServeDraftOverrides is #3582 as a regression test:
// an override edited after a release must not appear in that release's
// view. #3582 was closed on a fix that covered the list paths and Get but
// not the overrides the reporter was actually looking at — this is why the
// override slice went first. A count assertion cannot prove this (a reader
// serving the wrong table would still return the right number of rows), so
// the fixture makes the live and archived rows share an id and differ in
// content, and the assertion is on the value.
func TestReleaseViewDoesNotServeDraftOverrides(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := serviceTestContext(t, "Z3582")
	svc := NewService(NewPostgresStore(pool), nil, nil)

	const projectID, entityID, version = "Z3582", "Z3582M.1", "1.0.0"

	// One override archived at 1.0.0 with display_name "as released", and the
	// SAME override edited live to "edited after the release".
	seedEditedAfterRelease(t, pool, projectID, entityID, version)

	got, err := svc.ListForEntity(ctx, auth.Release(version), projectID, "model", entityID)
	if err != nil {
		t.Fatalf("release read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d overrides, want 1", len(got))
	}
	if name := displayName(got[0]); name != "as released" {
		t.Errorf("release view shows %q, want %q — the draft edit leaked into the release", name, "as released")
	}
}

// displayName reads the "en" translation off an override's DisplayName, the
// same way the package's other fixtures read translated jsonb fields back
// (see replace_integration_test.go).
func displayName(o domain.FieldOverride) string {
	return o.DisplayName.Get("en")
}

// seedEditedAfterRelease creates a scratch project (with its own owner
// actor) and a single override that shares one id across two tables: the
// live row (weave_field_overrides) carries "edited after the release", and
// the archived row (weave_field_overrides_archive) at version carries "as
// released" — the state the row was in when it was released, before the
// draft edit landed. Sharing the id is the point: a reader that serves the
// wrong table still returns exactly one row, so only a value assertion can
// catch it.
//
// Mirrors seedScopeFixture's seeding pattern and cleanup discipline
// (scoped_reader_integration_test.go): cleanup is scoped by projectID, never
// by version alone, since this package's fixtures share one database and
// several tests reuse version "1.0.0".
func seedEditedAfterRelease(t *testing.T, pool *pgxpool.Pool, projectID, entityID, version string) {
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

	liveName, err := json.Marshal(map[string]string{"en": "edited after the release"})
	if err != nil {
		t.Fatalf("marshal live display_name: %v", err)
	}
	archivedName, err := json.Marshal(map[string]string{"en": "as released"})
	if err != nil {
		t.Fatalf("marshal archived display_name: %v", err)
	}

	var overrideID int64
	err = pool.QueryRow(context.Background(), `INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position, display_name)
		VALUES ($1 || '.f1', $1, 'model', $2, 0, $3::jsonb)
		RETURNING id`,
		projectID, entityID, string(liveName)).Scan(&overrideID)
	if err != nil {
		t.Fatalf("insert live override: %v", err)
	}

	// The archived row reuses the live row's id — the fixture's whole point —
	// instead of drawing a fresh one from the shared sequence.
	mustScopeExec(t, pool, `INSERT INTO weave_field_overrides_archive
			(id, field_id, project_id, entity_type, entity_id, position, version_number, display_name)
		VALUES ($1, $2 || '.f1', $2, 'model', $3, 0, $4, $5::jsonb)`,
		overrideID, projectID, entityID, version, string(archivedName))

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
}
