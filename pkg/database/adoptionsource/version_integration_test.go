//go:build integration

package adoptionsource_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/adoptionsource"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// TestVersion_SourceReachedThroughPinnedParent pins that a source the
// consumer reaches through a pinned parent is recorded at the release that
// parent's pinned release took it at, not at the source's newest release.
// A source reached only through draft links keeps the newest release that
// holds the entity.
func TestVersion_SourceReachedThroughPinnedParent(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := sqlcgen.New(pool)

	const (
		owner   = "ZASV_OWNER"
		pinned  = "ZASVC" // consumer pinned to its parent
		draft   = "ZASVD" // consumer following its parent's draft
		parent  = "ZASVP"
		parent2 = "ZASVQ"
		source  = "ZASVG"
		fieldID = "ZASVGF.1"
	)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}
	t.Cleanup(func() {
		bg := context.Background()
		for _, sql := range []string{
			`DELETE FROM weave_project_inheritance_archive WHERE project_id LIKE 'ZASV%'`,
			`DELETE FROM weave_project_inheritance WHERE project_id LIKE 'ZASV%'`,
			`DELETE FROM weave_fields_archive WHERE project_id LIKE 'ZASV%'`,
			`DELETE FROM weave_releases WHERE project_id LIKE 'ZASV%'`,
			`DELETE FROM weave_projects WHERE id LIKE 'ZASV%'`,
			`DELETE FROM weave_actors WHERE id = 'ZASV_OWNER'`,
		} {
			_, _ = pool.Exec(bg, sql)
		}
	})
	exec(`INSERT INTO weave_actors (id, display_name, slug) VALUES ($1, $1, $1)`, owner)
	exec(`INSERT INTO weave_projects (id, owner_id) VALUES ($1, $6), ($2, $6), ($3, $6), ($4, $6), ($5, $6)`,
		pinned, draft, parent, parent2, source, owner)
	exec(`INSERT INTO weave_releases (project_id, version, created_by_id, created_at)
		VALUES ($1, '1.0.0', $2, now() - interval '1 day'), ($1, '3.0.0', $2, now())`, source, owner)
	exec(`INSERT INTO weave_fields_archive (id, semantic_id, project_id, system_name, status, version_number)
		VALUES ($1, $1, $2, 'f', 'published', '1.0.0'), ($1, $1, $2, 'f', 'published', '3.0.0')`, fieldID, source)
	// pinned -> parent@2.0.0, whose 2.0.0 release pinned source@1.0.0.
	exec(`INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, source_mode, source_version)
		VALUES ($1, $2, true, 'release', '2.0.0')`, pinned, parent)
	exec(`INSERT INTO weave_project_inheritance_archive (project_id, parent_project_id, is_primary, version_number, source_mode, source_version)
		VALUES ($1, $2, true, '2.0.0', 'release', '1.0.0')`, parent, source)
	// draft -> parent2 (draft) -> source (draft).
	exec(`INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, source_mode)
		VALUES ($1, $2, true, 'draft'), ($2, $3, true, 'draft')`, draft, parent2, source)

	for _, tc := range []struct {
		name, consumer, want string
	}{
		{"through pinned parent", pinned, "1.0.0"},
		{"through draft parents", draft, "3.0.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := adoptionsource.Version(ctx, q, tc.consumer, source, "field", fieldID)
			if err != nil {
				t.Fatalf("Version: %v", err)
			}
			if got != tc.want {
				t.Errorf("Version = %q, want %q", got, tc.want)
			}
		})
	}
}
