//go:build integration

package vocabulary_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

// TestGetProjectConceptList_VersionAware proves a pinned release version reads
// the archived snapshot (its sealed state), not the live draft.
func TestGetProjectConceptList_VersionAware(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	mustExec(t, pool, `INSERT INTO weave_actors (id, display_name, slug) VALUES ('own4','O4','own4')`)
	mustExec(t, pool, `INSERT INTO weave_projects (id, owner_id) VALUES ('VERP','own4')`)
	// Live list: not sealed.
	mustExec(t, pool, `INSERT INTO weave_concept_lists (id, project_id, is_closed) VALUES ('CLVER','VERP',false)`)
	// Archived snapshot at 1.0.0: sealed.
	mustExec(t, pool, `INSERT INTO weave_concept_lists_archive (id, project_id, status, is_closed, version_number) VALUES ('CLVER','VERP','published',true,'1.0.0')`)

	svc := vocabulary.NewService(pool, nil)

	live, err := svc.GetProjectConceptList(ctx, "VERP", "CLVER")
	if err != nil || live == nil {
		t.Fatalf("live get: %v", err)
	}
	if live.IsClosed {
		t.Fatal("live list should not be sealed")
	}

	released, err := svc.GetProjectConceptList(auth.WithProjectVersion(ctx, "1.0.0"), "VERP", "CLVER")
	if err != nil || released == nil {
		t.Fatalf("versioned get: %v", err)
	}
	if !released.IsClosed {
		t.Fatal("versioned read did not honor the archived (sealed) snapshot")
	}
}
