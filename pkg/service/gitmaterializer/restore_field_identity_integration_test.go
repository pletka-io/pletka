//go:build integration

package gitmaterializer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// TestHydrateProjectSnapshot_FieldIDIsSemanticID pins the field identity
// convention: a restored field's row id is its semantic id, as the UI and
// the importer create them, so a placement written back by the materializer
// names the field by semantic id and the tree round-trips.
func TestHydrateProjectSnapshot_FieldIDIsSemanticID(t *testing.T) {
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)
	const projectID, ownerID = "TPI", "TPI_OWNER_ACTOR"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_override_refs WHERE override_id IN (SELECT id FROM weave_field_overrides WHERE project_id = $1)`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_entity_counters WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_categories WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_concept_lists WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})
	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{ID: ownerID, Type: "organization", DisplayName: "TPI Org", Slug: "cl-org", Visibility: "private"}); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	root := conceptListFixture(t, projectID, ownerID)
	mustWrite(t, root, domain.FilePath(domain.PathSpec{EntityType: "category", EntityID: projectID + ".CAT.1"}), []byte(
		"canonical_order: 0\ndeprecated: false\ndescription: {}\nsemantic_id: "+projectID+".CAT.1\nstatus: draft\nsystem_name: general\nui_name:\n  en: General\n"))
	mustWrite(t, root, domain.FilePath(domain.PathSpec{EntityType: "model", EntityID: projectID + "M.1"}), []byte(
		"deprecated: false\ndescription: {}\nontology_scope: crm:E21_Person\nsemantic_id: "+projectID+"M.1\nstatus: draft\nsystem_name: person\nui_name:\n  en: Person\n"))
	mustWrite(t, root, domain.FilePath(domain.PathSpec{EntityType: "model_override", OwnerID: projectID + "M.1", FieldID: projectID + "F.1", EntityID: "1"}), []byte(
		"category_id: \"\"\ncollection_name: {}\ncollection_order: 0\ndescription: {}\ndisplay_name:\n  en: Name\nentity_id: "+projectID+"M.1\nentity_type: model\nfield_id: "+projectID+"F.1\nis_hidden: false\nis_required: false\nmax_occurs: null\nmin_occurs: 0\npart_of_collection_id: \"\"\nposition: 0\nrefs: []\nset_value: \"\"\nvisibility: \"\"\n"))

	m := NewMaterializer(pool, t.TempDir(), nil)
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatalf("load: %v", err)
	}
	var id string
	if err := pool.QueryRow(ctx, `SELECT id FROM weave_fields WHERE project_id = $1 AND semantic_id = $2`, projectID, projectID+"F.1").Scan(&id); err != nil {
		t.Fatalf("field not created: %v", err)
	}
	if id != projectID+"F.1" {
		t.Fatalf("field id = %q, want the semantic id", id)
	}
	// Compact path elements are stored with full IRIs, expanded through the
	// manifest's namespace bindings, exactly like UI-built fields.
	var pathJSON []byte
	if err := pool.QueryRow(ctx, `SELECT path_elements FROM weave_fields WHERE id = $1`, projectID+"F.1").Scan(&pathJSON); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pathJSON), "http://www.cidoc-crm.org/cidoc-crm/P1_is_identified_by") || strings.Contains(string(pathJSON), `"crm:P1_is_identified_by"`) {
		t.Fatalf("compact path element not expanded to an IRI: %s", pathJSON)
	}
	// Categories follow the same convention, so an adopted field's base
	// override (which names the category by id) is stable across restores.
	var catID string
	if err := pool.QueryRow(ctx, `SELECT id FROM weave_categories WHERE project_id = $1 AND semantic_id = $2`, projectID, projectID+".CAT.1").Scan(&catID); err != nil {
		t.Fatalf("category not created: %v", err)
	}
	if catID != projectID+".CAT.1" {
		t.Fatalf("category id = %q, want the semantic id", catID)
	}

	// A load that wrote entities with explicit ids must leave the entity
	// counters ahead of them, or the next UI "add field" collides.
	var nextField int64
	if err := pool.QueryRow(ctx, `SELECT next_n FROM weave_entity_counters WHERE project_id = $1 AND kind = 'field'`, projectID).Scan(&nextField); err != nil {
		t.Fatalf("field counter not reconciled after load: %v", err)
	}
	if nextField != 2 {
		t.Fatalf("field counter next_n = %d, want 2", nextField)
	}
	dir := t.TempDir()
	if err := m.writeProjectTree(ctx, dir, projectID); err != nil {
		t.Fatalf("writeProjectTree: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "models", projectID+"M.1", "overrides"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("want one placement file, got %v err %v", entries, err)
	}
	if !strings.HasPrefix(entries[0].Name(), projectID+"F.1@") {
		t.Fatalf("placement file %q must be named by the field's semantic id", entries[0].Name())
	}
	b, _ := os.ReadFile(filepath.Join(dir, "models", projectID+"M.1", "overrides", entries[0].Name()))
	if !strings.Contains(string(b), "field_id: "+projectID+"F.1\n") {
		t.Fatalf("placement must reference the field by semantic id:\n%s", b)
	}
}
