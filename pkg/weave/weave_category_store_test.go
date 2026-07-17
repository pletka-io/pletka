package weave_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
)

func TestPostgresStore_ImplementsWeaveCategoryStore(t *testing.T) {
	// Compile-time check is in weave_category_store.go; this verifies it at run time.
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	var _ domain.WeaveCategoryStore = store.WeaveCategories()
}

func TestWeaveCategoryStore_CRUD(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	cats := store.WeaveCategories()
	ctx := context.Background()

	projectID := ids.GenerateULID()

	// --- Create ---
	cat := &domain.Category{
		Entity: domain.Entity{
			ID:          ids.GenerateULID(),
			SemanticID:  "WCT.1",
			SystemName:  "wct_category",
			UIName:      domain.Translations{"en": "WCT Category", "nl": "WCT Categorie"},
			Description: domain.Translations{"en": "A weave category test"},
			ProjectID:   projectID,
		},
		CanonicalOrder: 1,
	}

	if err := cats.Create(ctx, cat); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_ = cats.Delete(context.Background(), cat.ID)
	})

	if cat.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be populated after Create")
	}

	// --- GetByID ---
	got, err := cats.GetByID(ctx, cat.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if got == nil {
		t.Fatal("GetByID returned nil")
	}

	if got.SystemName != "wct_category" {
		t.Errorf("SystemName = %q, want %q", got.SystemName, "wct_category")
	}

	if got.UIName.Get("en") != "WCT Category" {
		t.Errorf("UIName[en] = %q, want %q", got.UIName.Get("en"), "WCT Category")
	}

	// --- Update ---
	got.UIName.Set("en", "Updated WCT Category")

	if err := cats.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got2, _ := cats.GetByID(ctx, cat.ID)
	if got2.UIName.Get("en") != "Updated WCT Category" {
		t.Errorf("UIName[en] after update = %q, want %q", got2.UIName.Get("en"), "Updated WCT Category")
	}

	// --- UpdateFields ---
	if err := cats.UpdateFields(ctx, cat.ID, map[string]any{
		"system_name":     "wct_renamed",
		"canonical_order": 5,
	}); err != nil {
		t.Fatalf("UpdateFields: %v", err)
	}

	afterFields, _ := cats.GetByID(ctx, cat.ID)
	if afterFields.SystemName != "wct_renamed" {
		t.Errorf("SystemName after UpdateFields = %q, want %q", afterFields.SystemName, "wct_renamed")
	}
	if afterFields.CanonicalOrder != 5 {
		t.Errorf("CanonicalOrder after UpdateFields = %d, want 5", afterFields.CanonicalOrder)
	}

	// --- List ---
	list, err := cats.List(ctx, domain.WithProjectID(projectID))
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(list) != 1 {
		t.Errorf("List returned %d items, want 1", len(list))
	}

	// --- Count ---
	count, err := cats.Count(ctx, domain.WithProjectID(projectID))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	if count != 1 {
		t.Errorf("Count = %d, want 1", count)
	}

	// --- GetByIdentifier ---
	byIdent, err := cats.GetByIdentifier(ctx, "WCT.1", projectID)
	if err != nil {
		t.Fatalf("GetByIdentifier: %v", err)
	}

	if byIdent == nil {
		t.Fatal("GetByIdentifier returned nil")
	}

	if byIdent.ID != cat.ID {
		t.Errorf("GetByIdentifier ID = %q, want %q", byIdent.ID, cat.ID)
	}

	// --- ListWithCounts ---
	withCounts, err := cats.ListWithCounts(ctx, projectID)
	if err != nil {
		t.Fatalf("ListWithCounts: %v", err)
	}

	if len(withCounts) != 1 {
		t.Errorf("ListWithCounts returned %d items, want 1", len(withCounts))
	}

	// --- Reorder ---
	cat2 := &domain.Category{
		Entity: domain.Entity{
			ID:         ids.GenerateULID(),
			SemanticID: "WCT.2",
			SystemName: "wct_category_2",
			UIName:     domain.Translations{"en": "Second WCT Category"},
			ProjectID:  projectID,
		},
		CanonicalOrder: 2,
	}
	if err := cats.Create(ctx, cat2); err != nil {
		t.Fatalf("Create cat2: %v", err)
	}
	t.Cleanup(func() {
		_ = cats.Delete(context.Background(), cat2.ID)
	})

	// Reverse the order: cat2 first, cat first
	if err := cats.Reorder(ctx, projectID, []string{cat2.ID, cat.ID}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}

	reordered1, _ := cats.GetByID(ctx, cat2.ID)
	reordered2, _ := cats.GetByID(ctx, cat.ID)
	if reordered1.CanonicalOrder != 1 {
		t.Errorf("cat2 CanonicalOrder = %d, want 1", reordered1.CanonicalOrder)
	}
	if reordered2.CanonicalOrder != 2 {
		t.Errorf("cat CanonicalOrder = %d, want 2", reordered2.CanonicalOrder)
	}

	// Reorder with wrong project should fail
	if err := cats.Reorder(ctx, "01WRONGPROJECTID0000000000", []string{cat.ID}); err == nil {
		t.Error("expected error for wrong project, got nil")
	}

	// Clean up cat2 before the main Delete test
	_ = cats.Delete(ctx, cat2.ID)

	// --- Delete ---
	if err := cats.Delete(ctx, cat.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	deleted, err := cats.GetByID(ctx, cat.ID)
	if err != nil {
		t.Fatalf("GetByID after delete: %v", err)
	}

	if deleted != nil {
		t.Error("expected nil after delete")
	}
}

func TestWeaveCategoryStore_GetByID_NotFound(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	cats := store.WeaveCategories()

	got, err := cats.GetByID(context.Background(), "01NONEXISTENT0000000000000")
	if err != nil {
		t.Fatalf("expected nil error for not-found, got: %v", err)
	}

	if got != nil {
		t.Error("expected nil result for not-found")
	}
}
