//go:build integration

package pathaudit_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/pathaudit"
)

// TestAudit_ChildResolvesInheritedOntologies pins that a child project with no
// ontology links of its own resolves path terms through its parent's links,
// the same scope the path builder and term descriptions use.
func TestAudit_ChildResolvesInheritedOntologies(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const child = "PATHCHILD"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id = $1`, child)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_project_inheritance WHERE project_id = $1`, child)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, child)
	})
	var ownerID string
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM weave_projects WHERE id = $1`, testdb.FixtureParent).Scan(&ownerID); err != nil {
		t.Fatalf("fixture parent owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, system_name, ui_name, description, status, owner_id, visibility, parent_project_id) VALUES ($1, 'pathchild', '{"en":"Path child"}', '{}', 'draft', $2, 'private', $3)`, child, ownerID, testdb.FixtureParent); err != nil {
		t.Fatalf("create child project: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, canonical_order, source_mode) VALUES ($1, $2, true, 0, 'draft')`, child, testdb.FixtureParent); err != nil {
		t.Fatalf("create inheritance: %v", err)
	}
	path := `[{"type":"property","prefix":"crm","local_name":"P1_is_identified_by","uri":"http://www.cidoc-crm.org/cidoc-crm/P1_is_identified_by","position":0},{"type":"class","prefix":"crm","local_name":"E41_Appellation","uri":"http://www.cidoc-crm.org/cidoc-crm/E41_Appellation","position":1}]`
	scope := `{"type":"class","prefix":"crm","local_name":"E21_Person","uri":"http://www.cidoc-crm.org/cidoc-crm/E21_Person","position":0}`
	if _, err := pool.Exec(ctx, `INSERT INTO weave_fields (id, semantic_id, system_name, ui_name, description, status, project_id, ontology_scope, ontology_path, path_elements, expected_value_type) VALUES ('PATHCHILDF.1', 'PATHCHILDF.1', 'name', '{"en":"Name"}', '{}', 'draft', $1, $2, '->crm:P1_is_identified_by->crm:E41_Appellation', $3, 'String')`, child, scope, path); err != nil {
		t.Fatalf("create field: %v", err)
	}
	errs, scanned, err := pathaudit.NewService(pool).Audit(ctx, child)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if scanned != 2 {
		t.Fatalf("scanned = %d, want 2", scanned)
	}
	if len(errs) != 0 {
		t.Fatalf("child must resolve its parent's ontologies, got %d errors: %+v", len(errs), errs)
	}
}
