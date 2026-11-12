//go:build integration

package model_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestListAdoptedExplicit_ReadsSourceAtReceiptVersion pins that an adopted
// model is shown as the source release the receipt names, not as the
// source project's current draft: the source renamed the model after the
// adoption, and the adopter must keep seeing the release it took.
func TestListAdoptedExplicit_ReadsSourceAtReceiptVersion(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	const (
		src     = "ZADVS"
		dst     = "ZADVD"
		owner   = "ZADVS_OWNER"
		pinned  = "ZADVSM.1"
		follows = "ZADVSM.2"
		version = "1.0.0"
	)
	t.Cleanup(func() {
		bg := context.Background()
		mustCleanup(pool, bg, `DELETE FROM weave_adoptions WHERE project_id = $1`, dst)
		mustCleanup(pool, bg, `DELETE FROM weave_models_archive WHERE project_id = $1`, src)
		mustCleanup(pool, bg, `DELETE FROM weave_models WHERE project_id = $1`, src)
		mustCleanup(pool, bg, `DELETE FROM weave_projects WHERE id IN ($1, $2)`, src, dst)
		mustCleanup(pool, bg, `DELETE FROM weave_actors WHERE id = $1`, owner)
	})
	mustFork(t, pool, ctx, `INSERT INTO weave_actors (id, display_name, slug) VALUES ($1, $1, $1)`, owner)
	mustFork(t, pool, ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1, $3), ($2, $3)`, src, dst, owner)
	mustFork(t, pool, ctx, `INSERT INTO weave_models (id, project_id, system_name, ui_name, status)
		VALUES ($2, $1, 'pinned', '{"en":"Draft Name"}', 'draft'),
		       ($3, $1, 'follows', '{"en":"Followed Draft"}', 'draft')`, src, pinned, follows)
	mustFork(t, pool, ctx, `INSERT INTO weave_models_archive (id, project_id, system_name, ui_name, status, version_number)
		VALUES ($2, $1, 'pinned', '{"en":"Released Name"}', 'published', $3)`, src, pinned, version)
	mustFork(t, pool, ctx, `INSERT INTO weave_adoptions
			(project_id, context_entity_type, context_entity_id, entity_type, source_project_id, source_entity_id, source_version)
		VALUES ($1, 'project', $1, 'model', $2, $3, $4),
		       ($1, 'project', $1, 'model', $2, $5, '')`, dst, src, pinned, version, follows)

	models, _, err := newForkTestService(pool).ListAdoptedExplicit(forkTestContext(t), dst)
	if err != nil {
		t.Fatalf("ListAdoptedExplicit: %v", err)
	}
	got := map[string]string{}
	for _, m := range models {
		got[m.ID] = m.UIName.Get("en")
	}
	if got[pinned] != "Released Name" {
		t.Errorf("pinned receipt shows %q, want the 1.0.0 name %q", got[pinned], "Released Name")
	}
	if got[follows] != "Followed Draft" {
		t.Errorf("draft-following receipt shows %q, want the draft name", got[follows])
	}
}
