//go:build integration

package field_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/field"
)

// TestListAdoptedExplicit_ReadsSourceAtReceiptVersion pins that an adopted
// field is shown as the source release the receipt names, not as the
// source project's current draft.
func TestListAdoptedExplicit_ReadsSourceAtReceiptVersion(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	const (
		src     = "ZADFS"
		dst     = "ZADFD"
		owner   = "ZADFS_OWNER"
		pinned  = "ZADFSF.1"
		follows = "ZADFSF.2"
		version = "1.0.0"
	)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_adoptions WHERE project_id = $1`, dst)
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields_archive WHERE project_id = $1`, src)
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields WHERE project_id = $1`, src)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id IN ($1, $2)`, src, dst)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, owner)
	})
	exec(`INSERT INTO weave_actors (id, display_name, slug) VALUES ($1, $1, $1)`, owner)
	exec(`INSERT INTO weave_projects (id, owner_id) VALUES ($1, $3), ($2, $3)`, src, dst, owner)
	exec(`INSERT INTO weave_fields (id, semantic_id, project_id, system_name, ui_name, status)
		VALUES ($2, $2, $1, 'pinned', '{"en":"Draft Name"}', 'draft'),
		       ($3, $3, $1, 'follows', '{"en":"Followed Draft"}', 'draft')`, src, pinned, follows)
	exec(`INSERT INTO weave_fields_archive (id, semantic_id, project_id, system_name, ui_name, status, version_number)
		VALUES ($2, $2, $1, 'pinned', '{"en":"Released Name"}', 'published', $3)`, src, pinned, version)
	exec(`INSERT INTO weave_adoptions
			(project_id, context_entity_type, context_entity_id, entity_type, source_project_id, source_entity_id, source_version)
		VALUES ($1, 'project', $1, 'field', $2, $3, $4),
		       ($1, 'project', $1, 'field', $2, $5, '')`, dst, src, pinned, version, follows)

	agg := weave.NewPostgresStore(pool)
	svc := field.NewService(field.NewPostgresStore(pool), nil, agg.Adoptions(), agg.Forks(), nil, nil, agg, nil, nil, nil)
	sa := auth.WithSnapshot(ctx, &auth.AuthSnapshot{IsSuperAdmin: true})
	fields, _, err := svc.ListAdoptedExplicit(sa, dst)
	if err != nil {
		t.Fatalf("ListAdoptedExplicit: %v", err)
	}
	got := map[string]string{}
	for _, f := range fields {
		got[f.ID] = f.UIName.Get("en")
	}
	if got[pinned] != "Released Name" {
		t.Errorf("pinned receipt shows %q, want the 1.0.0 name %q", got[pinned], "Released Name")
	}
	if got[follows] != "Followed Draft" {
		t.Errorf("draft-following receipt shows %q, want the draft name", got[follows])
	}
}
