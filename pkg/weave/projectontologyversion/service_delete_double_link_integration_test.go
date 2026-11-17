//go:build integration

package projectontologyversion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	pov "github.com/pletka-io/pletka/pkg/weave/projectontologyversion"
)

// TestDelete_SecondVersionOfALinkedBase covers a project that links two
// versions of one base ontology. Removing one of them must leave the base's
// extensions linked, must not count a term the other version still provides
// as in use, and must still refuse when a field uses a term only the removed
// version holds.
func TestDelete_SecondVersionOfALinkedBase(t *testing.T) {
	ctx := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	pool := testdb.Pool(t)
	seedExtendsHierarchy(t, pool)
	svc := newServiceForTest(pool, weaveontology.NewPostgresStore(pool))

	projectID := seedEmptyProject(t, pool)
	crmOntID, crmVer, _ := activeOntologyVersion(t, pool, "crm")
	aaaoVer := activeVersionIDForPrefix(t, pool, "aaao")

	var shared string
	if err := pool.QueryRow(ctx, `SELECT local_name FROM weave_ontology_classes WHERE ontology_version_id = $1 ORDER BY local_name LIMIT 1`, crmVer).Scan(&shared); err != nil {
		t.Fatalf("pick a crm class: %v", err)
	}

	const oldVer, sharedField, oldOnlyField = "ZCHAIN_CRM_OLD", "ZCHAIN_F_SHARED", "ZCHAIN_F_OLD"
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}
	cleanup := func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields WHERE id IN ($1, $2)`, sharedField, oldOnlyField)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_ontology_versions WHERE ontology_version_id = $1`, oldVer)
		_, _ = pool.Exec(bg, `DELETE FROM weave_ontology_versions WHERE id = $1`, oldVer)
	}
	cleanup()
	t.Cleanup(cleanup)

	// An older crm version: holds the shared class and one of its own.
	exec(`INSERT INTO weave_ontology_versions (id, ontology_id, version_string) VALUES ($1, $2, '0.0-old')`, oldVer, crmOntID)
	exec(`INSERT INTO weave_ontology_classes (id, ontology_version_id, prefix, local_name, uri) VALUES
		('ZCHAIN_C1', $1, 'crm', $2, 'urn:zchain:shared'), ('ZCHAIN_C2', $1, 'crm', 'EZ_Only_In_Old', 'urn:zchain:old')`, oldVer, shared)
	exec(`INSERT INTO weave_project_ontology_versions (project_id, ontology_version_id, added_at) VALUES
		($1, $2, now()), ($1, $3, now()), ($1, $4, now())`, projectID, crmVer, oldVer, aaaoVer)
	field := func(id, semantic, localName string) {
		exec(`INSERT INTO weave_fields (id, project_id, status, semantic_id, system_name, ui_name, path_elements)
			VALUES ($1, $2, 'draft', $3, $4, '{"en":"f"}'::jsonb, jsonb_build_array(jsonb_build_object('position', 1, 'type', 'class', 'prefix', 'crm', 'local_name', $5::text)))`,
			id, projectID, semantic, id, localName)
	}

	// A term only the old version holds blocks the removal.
	field(sharedField, "ZCHAIN.F.1", shared)
	field(oldOnlyField, "ZCHAIN.F.2", "EZ_Only_In_Old")
	var inUse *pov.ErrInUse
	if err := svc.Delete(ctx, projectID, oldVer); !errors.As(err, &inUse) {
		t.Fatalf("Delete with a term only the old version holds: got %v, want ErrInUse", err)
	}
	if inUse.TotalUsage != 1 {
		t.Errorf("usage = %d, want 1: the shared term is still provided by the other version", inUse.TotalUsage)
	}

	// Without that field the shared term alone does not block.
	exec(`DELETE FROM weave_fields WHERE id = $1`, oldOnlyField)
	if err := svc.Delete(ctx, projectID, oldVer); err != nil {
		t.Fatalf("Delete of the second crm version: %v", err)
	}
	linked := linkedVersionIDs(t, pool, projectID)
	if contains(linked, oldVer) {
		t.Error("the old crm version is still linked")
	}
	if !contains(linked, crmVer) || !contains(linked, aaaoVer) {
		t.Errorf("crm and its extension must stay linked, got %v", linked)
	}

	// The last version of the base is in use by the same field.
	if err := svc.Delete(ctx, projectID, crmVer); !errors.As(err, &inUse) {
		t.Fatalf("Delete of the last crm version: got %v, want ErrInUse", err)
	}
	// And once nothing uses it, it takes its extensions along.
	exec(`DELETE FROM weave_fields WHERE id = $1`, sharedField)
	if err := svc.Delete(ctx, projectID, crmVer); err != nil {
		t.Fatalf("Delete of the last crm version, unused: %v", err)
	}
	if linked := linkedVersionIDs(t, pool, projectID); len(linked) != 0 {
		t.Errorf("removing the last base version must remove its extensions, still linked: %v", linked)
	}
}
