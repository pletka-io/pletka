//go:build integration

package weave_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
)

// TestNamespaceBindingStore_ImplementsInterface verifies the compile-time
// interface satisfaction.
func TestNamespaceBindingStore_ImplementsInterface(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	var _ domain.NamespaceBindingStore = store.NamespaceBindings()
}

// TestNamespaceBindingStore_CRUDOnUserRow exercises the full create → get →
// list → update → delete lifecycle on a user-owned row.
func TestNamespaceBindingStore_CRUDOnUserRow(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	nbs := store.NamespaceBindings()
	ctx := context.Background()

	projectID := ids.GenerateULID()
	id := ids.GenerateULID()

	// Idempotent pre-clean.
	_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE prefix LIKE 'test_%'`)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM weave_namespace_bindings WHERE id = $1`, id)
	})

	b := &domain.NamespaceBinding{
		ID:        id,
		ProjectID: projectID,
		Prefix:    "test_ex",
		Namespace: "http://test-example.org/crud/",
		Weight:    10,
		Source:    "user",
	}

	// --- Create ---
	if err := nbs.CreateUser(ctx, b); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// --- Get ---
	got, err := nbs.GetUser(ctx, projectID, id)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got == nil {
		t.Fatal("GetUser returned nil")
	}
	if got.Prefix != "test_ex" {
		t.Errorf("Prefix = %q, want %q", got.Prefix, "test_ex")
	}
	if got.Namespace != "http://test-example.org/crud/" {
		t.Errorf("Namespace = %q, want %q", got.Namespace, "http://test-example.org/crud/")
	}
	if got.Weight != 10 {
		t.Errorf("Weight = %d, want 10", got.Weight)
	}
	if got.Source != "user" {
		t.Errorf("Source = %q, want %q", got.Source, "user")
	}

	// --- List (should include the new row) ---
	list, err := nbs.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, item := range list {
		if item.ID == id {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("List did not return the newly created row (id=%s)", id)
	}

	// --- Update ---
	got.Prefix = "test_ex2"
	got.Namespace = "http://test-example.org/crud/v2/"
	got.Weight = 20
	if err := nbs.UpdateUser(ctx, got); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	got2, err := nbs.GetUser(ctx, projectID, id)
	if err != nil {
		t.Fatalf("GetUser after update: %v", err)
	}
	if got2.Prefix != "test_ex2" {
		t.Errorf("Prefix after update = %q, want %q", got2.Prefix, "test_ex2")
	}
	if got2.Weight != 20 {
		t.Errorf("Weight after update = %d, want 20", got2.Weight)
	}

	// --- Delete ---
	if err := nbs.DeleteUser(ctx, projectID, id); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	list2, err := nbs.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	for _, item := range list2 {
		if item.ID == id {
			t.Errorf("row %s still present after DeleteUser", id)
		}
	}
}

// TestNamespaceBindingStore_ListIncludesGlobalRows verifies that List returns
// both global rows (project_id IS NULL, source='system') and project-scoped
// rows together.
func TestNamespaceBindingStore_ListIncludesGlobalRows(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	nbs := store.NamespaceBindings()
	ctx := context.Background()

	projectID := ids.GenerateULID()
	globalID := ids.GenerateULID()
	projectRowID := ids.GenerateULID()

	// Idempotent pre-clean.
	_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE prefix LIKE 'test_%'`)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM weave_namespace_bindings WHERE id IN ($1, $2)`, globalID, projectRowID)
	})

	// Seed a global system row.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_namespace_bindings (id, project_id, prefix, namespace, weight, source, created_at, updated_at)
		 VALUES ($1, NULL, 'test_global_skos', 'http://www.w3.org/2004/02/skos/core#', 0, 'system', now(), now())`,
		globalID,
	); err != nil {
		t.Fatalf("seed global row: %v", err)
	}

	// Seed a project-scoped user row.
	if err := nbs.CreateUser(ctx, &domain.NamespaceBinding{
		ID:        projectRowID,
		ProjectID: projectID,
		Prefix:    "test_local",
		Namespace: "http://test-local.org/",
		Weight:    5,
	}); err != nil {
		t.Fatalf("CreateUser project row: %v", err)
	}

	list, err := nbs.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	foundGlobal := false
	foundProject := false
	for _, item := range list {
		if item.ID == globalID {
			foundGlobal = true
		}
		if item.ID == projectRowID {
			foundProject = true
		}
	}
	if !foundGlobal {
		t.Error("List did not include global row")
	}
	if !foundProject {
		t.Error("List did not include project-scoped row")
	}
}

// TestNamespaceBindingStore_BlockedOnSystemRow seeds a system row and asserts
// that UpdateUser and DeleteUser return domain.ErrReadOnly.
func TestNamespaceBindingStore_BlockedOnSystemRow(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	nbs := store.NamespaceBindings()
	ctx := context.Background()

	projectID := ids.GenerateULID()
	id := ids.GenerateULID()

	// Idempotent pre-clean.
	_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE prefix LIKE 'test_%'`)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM weave_namespace_bindings WHERE id = $1`, id)
	})

	// Seed a system row owned by the test project.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_namespace_bindings (id, project_id, prefix, namespace, weight, source, created_at, updated_at)
		 VALUES ($1, $2, 'test_sys', 'http://test-system.org/', 99, 'system', now(), now())`,
		id, projectID,
	); err != nil {
		t.Fatalf("seed system row: %v", err)
	}

	// UpdateUser should return ErrReadOnly.
	updateErr := nbs.UpdateUser(ctx, &domain.NamespaceBinding{
		ID:        id,
		ProjectID: projectID,
		Prefix:    "test_sys_updated",
		Namespace: "http://test-system.org/",
		Weight:    99,
	})
	if !errors.Is(updateErr, domain.ErrReadOnly) {
		t.Errorf("UpdateUser on system row: got %v, want errors.Is(err, domain.ErrReadOnly)", updateErr)
	}

	// DeleteUser should return ErrReadOnly.
	deleteErr := nbs.DeleteUser(ctx, projectID, id)
	if !errors.Is(deleteErr, domain.ErrReadOnly) {
		t.Errorf("DeleteUser on system row: got %v, want errors.Is(err, domain.ErrReadOnly)", deleteErr)
	}
}

// TestNamespaceBindingStore_DuplicatePrefixNamespace verifies that
// ExistsByPrefixAndNamespace returns the correct boolean and respects excludeID.
func TestNamespaceBindingStore_DuplicatePrefixNamespace(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	nbs := store.NamespaceBindings()
	ctx := context.Background()

	projectID := ids.GenerateULID()
	id := ids.GenerateULID()

	// Idempotent pre-clean.
	_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE prefix LIKE 'test_%'`)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM weave_namespace_bindings WHERE id = $1`, id)
	})

	// Before creation: should not exist.
	exists, err := nbs.ExistsByPrefixAndNamespace(ctx, "test_ex_dup", "http://example.org/dup/", "")
	if err != nil {
		t.Fatalf("ExistsByPrefixAndNamespace before create: %v", err)
	}
	if exists {
		t.Error("ExistsByPrefixAndNamespace before create: got true, want false")
	}

	// Create the row.
	if err := nbs.CreateUser(ctx, &domain.NamespaceBinding{
		ID:        id,
		ProjectID: projectID,
		Prefix:    "test_ex_dup",
		Namespace: "http://example.org/dup/",
		Weight:    1,
	}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// After creation without excludeID: should exist.
	exists2, err := nbs.ExistsByPrefixAndNamespace(ctx, "test_ex_dup", "http://example.org/dup/", "")
	if err != nil {
		t.Fatalf("ExistsByPrefixAndNamespace after create: %v", err)
	}
	if !exists2 {
		t.Error("ExistsByPrefixAndNamespace after create: got false, want true")
	}

	// After creation with the row's own ID excluded (self-update case): should not exist.
	exists3, err := nbs.ExistsByPrefixAndNamespace(ctx, "test_ex_dup", "http://example.org/dup/", id)
	if err != nil {
		t.Fatalf("ExistsByPrefixAndNamespace with excludeID: %v", err)
	}
	if exists3 {
		t.Error("ExistsByPrefixAndNamespace with excludeID matching own row: got true, want false")
	}
}
