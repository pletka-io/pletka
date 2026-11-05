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

// TestHydrateProjectSnapshot_ConceptLists pins: a concept-lists file creates
// the row (id = semantic id), a second load updates it in place, and a base
// override ref to a list that exists nowhere fails naming both.
func TestHydrateProjectSnapshot_ConceptLists(t *testing.T) {
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)
	const projectID, ownerID = "TPL", "TPL_OWNER_ACTOR"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_override_refs WHERE override_id IN (SELECT id FROM weave_field_overrides WHERE project_id = $1)`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_concept_lists WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})
	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{ID: ownerID, Type: "organization", DisplayName: "CL Org", Slug: "cl-org", Visibility: "private"}); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	root := conceptListFixture(t, projectID, ownerID)
	m := NewMaterializer(pool, t.TempDir(), nil)
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatalf("first load: %v", err)
	}
	row, err := queries.WeaveGetProjectConceptList(ctx, sqlcgen.WeaveGetProjectConceptListParams{ProjectID: projectID, ID: "TPL.CL.1"})
	if err != nil {
		t.Fatalf("concept list not created: %v", err)
	}
	if row.ID != "TPL.CL.1" || row.SystemName == nil || *row.SystemName != "license" || !row.IsClosed {
		t.Fatalf("unexpected row %+v", row)
	}

	rewrite(t, filepath.Join(root, "concept-lists", "TPL.CL.1.yaml"), "system_name: license", "system_name: licence")
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatalf("second load: %v", err)
	}
	lists, err := queries.WeaveListConceptListsForTree(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 || lists[0].SystemName == nil || *lists[0].SystemName != "licence" {
		t.Fatalf("want one updated list, got %+v", lists)
	}

	rewrite(t, filepath.Join(root, "fields", "TPLF.1", "base-override.yaml"), "refs: []", "refs:\n  - ref_type: concept_list\n    semantic_id: TPL.CL.9\n    position: 0")
	_, err = m.HydrateProjectSnapshot(ctx, root)
	if err == nil || !strings.Contains(err.Error(), "TPL.CL.9") || !strings.Contains(err.Error(), "base-override.yaml") {
		t.Fatalf("want dangling concept list error naming file and list, got %v", err)
	}
}

func rewrite(t *testing.T, path, old, new string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("%s does not contain %q", path, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(b), old, new, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}
