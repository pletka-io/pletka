//go:build integration

package field

import (
	"context"
	"testing"
)

// TestGetByIdentifierVersionReadsTheArchive is the live-DB gate for Redmine
// #3610: a single-entity read ignored an explicit version and returned the
// draft.
//
// The service resolved a pinned version for Get but not for GetByIdentifier,
// and the MCP surface reaches a field through GetByIdentifier — so an agent
// asking about release 0.1.0 was handed today's working state, with nothing
// in the response to say so. Verified on production against LA's LAF.107,
// whose expected value type changed from String to Model two months after the
// release it was asked for.
//
// Three properties, and the second and third are why this resolves against
// the archive rather than resolving live and then reading the archived row:
// a renamed field must be findable by the name the RELEASE carried, and a
// field deleted since the release must still be findable in it.
func TestGetByIdentifierVersionReadsTheArchive(t *testing.T) {
	pool := batchUsageRefsTestPool(t)
	ctx := context.Background()
	const (
		projectID = "IDVER"
		version   = "1.0.0"
	)

	seed := [][]any{
		// A field whose definition changed after the release: the archive says
		// String, the live row says Model.
		{`INSERT INTO weave_fields (id, project_id, semantic_id, system_name, ui_name, expected_value_type, status)
		  VALUES ('idver_changed', $1, 'IDVERF.1', 'changed_field', '{"en":"Changed"}'::jsonb, 'Model', 'published')`, projectID},
		{`INSERT INTO weave_fields_archive (id, project_id, semantic_id, system_name, ui_name, expected_value_type, status, version_number)
		  VALUES ('idver_changed', $1, 'IDVERF.1', 'changed_field', '{"en":"Changed"}'::jsonb, 'String', 'published', $2)`, projectID, version},
		// A field RENAMED since the release: the release carried
		// 'old_system_name', the live row carries 'new_system_name'.
		{`INSERT INTO weave_fields (id, project_id, semantic_id, system_name, ui_name, expected_value_type, status)
		  VALUES ('idver_renamed', $1, 'IDVERF.2', 'new_system_name', '{"en":"Renamed"}'::jsonb, 'String', 'published')`, projectID},
		{`INSERT INTO weave_fields_archive (id, project_id, semantic_id, system_name, ui_name, expected_value_type, status, version_number)
		  VALUES ('idver_renamed', $1, 'IDVERF.2', 'old_system_name', '{"en":"Renamed"}'::jsonb, 'String', 'published', $2)`, projectID, version},
		// A field DELETED since the release: in the archive only.
		{`INSERT INTO weave_fields_archive (id, project_id, semantic_id, system_name, ui_name, expected_value_type, status, version_number)
		  VALUES ('idver_deleted', $1, 'IDVERF.3', 'deleted_field', '{"en":"Deleted"}'::jsonb, 'String', 'published', $2)`, projectID, version},
	}
	for _, stmt := range seed {
		if _, err := pool.Exec(ctx, stmt[0].(string), stmt[1:]...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields_archive WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
	})

	base := NewPostgresStore(pool)
	// Reached by assertion, the same way Service does it: the versioned reads
	// are not on the Store interface, they are an optional capability a store
	// may implement.
	store, ok := base.(versionedFieldReader)
	if !ok {
		t.Fatal("the postgres store does not implement versionedFieldReader")
	}

	t.Run("the archived definition wins over the live one", func(t *testing.T) {
		got, err := store.GetByIdentifierVersion(ctx, projectID, "IDVERF.1", version)
		if err != nil {
			t.Fatalf("GetByIdentifierVersion: %v", err)
		}
		if got == nil {
			t.Fatal("field not found at the pinned version")
		}
		if got.ExpectedValueType != "String" {
			t.Errorf("expected_value_type = %q, want %q — the live row leaked through", got.ExpectedValueType, "String")
		}

		// Guard the other direction: without a version the live row is still
		// what comes back, so this change narrows nothing it should not.
		live, err := base.GetByIdentifier(ctx, projectID, "IDVERF.1")
		if err != nil {
			t.Fatalf("GetByIdentifier: %v", err)
		}
		if live == nil || live.ExpectedValueType != "Model" {
			t.Errorf("unversioned read = %v, want the live Model row", live)
		}
	})

	t.Run("a field is found by the name the release carried", func(t *testing.T) {
		got, err := store.GetByIdentifierVersion(ctx, projectID, "old_system_name", version)
		if err != nil {
			t.Fatalf("GetByIdentifierVersion: %v", err)
		}
		if got == nil {
			t.Fatal("the release's own system name did not resolve — resolving live first would miss this")
		}
		if got.ID != "idver_renamed" {
			t.Errorf("id = %q, want idver_renamed", got.ID)
		}

		// And the CURRENT name must not resolve at that version: it did not
		// exist then.
		if cur, err := store.GetByIdentifierVersion(ctx, projectID, "new_system_name", version); err != nil {
			t.Fatalf("GetByIdentifierVersion: %v", err)
		} else if cur != nil {
			t.Errorf("the post-release name resolved at %s, want nothing", version)
		}
	})

	t.Run("a field deleted since the release is still in it", func(t *testing.T) {
		got, err := store.GetByIdentifierVersion(ctx, projectID, "IDVERF.3", version)
		if err != nil {
			t.Fatalf("GetByIdentifierVersion: %v", err)
		}
		if got == nil {
			t.Fatal("a field present only in the archive did not resolve — resolving live first would miss this")
		}
		if got.SystemName != "deleted_field" {
			t.Errorf("system_name = %q, want deleted_field", got.SystemName)
		}
	})

	t.Run("an unknown identifier is absent, not an error", func(t *testing.T) {
		got, err := store.GetByIdentifierVersion(ctx, projectID, "no_such_field", version)
		if err != nil {
			t.Fatalf("GetByIdentifierVersion: %v", err)
		}
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}
