//go:build integration

package gitmaterializer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// TestWriteProjectTree_ConceptLists pins that a project with a concept list
// materializes concept-lists/<SEMID>.yaml with the canonical keys.
func TestWriteProjectTree_ConceptLists(t *testing.T) {
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)
	const (
		ownerID   = "CLTREE_OWNER_ACTOR"
		projectID = "CLTREE"
		listID    = "CLTREE.CL.1"
	)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_concept_lists WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})
	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{ID: ownerID, Type: "organization", DisplayName: "CL Org", Slug: "cl-org", Visibility: "private"}); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{ID: projectID, UiName: []byte(`{"en":"CL"}`), Description: []byte(`{}`), Status: "draft", OwnerID: ownerID, Visibility: "private"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	sem, sys := listID, "license"
	if _, err := queries.WeaveCreateConceptList(ctx, sqlcgen.WeaveCreateConceptListParams{ID: listID, SemanticID: &sem, SystemName: &sys, UiName: []byte(`{"en":"License"}`), Description: []byte(`{}`), Status: "draft", ProjectID: projectID}); err != nil {
		t.Fatalf("create concept list: %v", err)
	}
	m := NewMaterializer(pool, t.TempDir(), nil)
	dir := t.TempDir()
	if err := m.writeProjectTree(ctx, dir, projectID); err != nil {
		t.Fatalf("writeProjectTree: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "concept-lists", listID+".yaml"))
	if err != nil {
		t.Fatalf("concept list file not written: %v", err)
	}
	for _, want := range []string{"semantic_id: " + listID, "system_name: license", "is_closed: false"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("payload lacks %q:\n%s", want, b)
		}
	}
}
