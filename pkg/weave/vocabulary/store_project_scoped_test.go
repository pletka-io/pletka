//go:build integration

package vocabulary

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestListProjectScopedVocabulariesIgnoresTheJoinTable pins the post-change
// contract by going through the production path (Service.ListProjectVocabularies,
// which calls WeaveListProjectScopedVocabularies): a project's vocabularies
// are exactly the rows carrying its project_id. A row reachable only through
// weave_project_vocabularies must NOT appear — that reachability is what
// this work removes.
func TestListProjectScopedVocabulariesIgnoresTheJoinTable(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const (
		ownerID   = "tstvps_owner"
		projectID = "TSTVPS"
		ownedID   = "vocab_tstvps_owned"
		linkedID  = "vocab_tstvps_linked"
		emptyProj = "TSTVPSEMPTY"
	)

	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, display_name, slug) VALUES ($1,$2,$3)
		ON CONFLICT (id) DO NOTHING`, ownerID, "TSTVPS Owner", ownerID); err != nil {
		t.Fatalf("seed owner actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1,$3),($2,$3)
		ON CONFLICT (id) DO NOTHING`, projectID, emptyProj, ownerID); err != nil {
		t.Fatalf("seed projects: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_vocabularies (id, project_id, system_name, connector_type, status)
		VALUES ($1,$2,'tstvps_owned','local','draft') ON CONFLICT (id) DO NOTHING`, ownedID, projectID); err != nil {
		t.Fatalf("seed owned vocabulary: %v", err)
	}
	// A vocabulary owned by nobody, reachable only via the join table.
	if _, err := pool.Exec(ctx, `INSERT INTO weave_vocabularies (id, system_name, connector_type, status)
		VALUES ($1,'tstvps_linked','local','draft') ON CONFLICT (id) DO NOTHING`, linkedID); err != nil {
		t.Fatalf("seed linked vocabulary: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_project_vocabularies (project_id, vocabulary_id, status)
		VALUES ($1,$2,'active') ON CONFLICT (project_id, vocabulary_id) DO NOTHING`, projectID, linkedID); err != nil {
		t.Fatalf("seed join row: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM weave_project_vocabularies WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_vocabularies WHERE id IN ($1,$2)`, ownedID, linkedID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id IN ($1,$2)`, projectID, emptyProj)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	svc := NewService(pool, nil)

	ids := func(projectID string) []string {
		views, err := svc.ListProjectVocabularies(ctx, projectID)
		if err != nil {
			t.Fatalf("ListProjectVocabularies(%s): %v", projectID, err)
		}
		out := make([]string, 0, len(views))
		for _, v := range views {
			out = append(out, v.ID)
		}
		return out
	}

	got := ids(projectID)
	if len(got) != 1 || got[0] != ownedID {
		t.Errorf("project vocabularies = %v, want exactly [%s] — a join-reachable row leaked in", got, ownedID)
	}
	// Review Focus 1: a project with no vocabularies is empty, not an error.
	if got := ids(emptyProj); len(got) != 0 {
		t.Errorf("vocabularies for a project that has none = %v, want empty", got)
	}
}
