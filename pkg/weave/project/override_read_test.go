package project

import (
	"context"
	"testing"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// TestOverrideCapabilitiesCanHideFieldsMirrorsEditPermission is the
// regression test for (collection hide checkbox never
// rendered): CollectionOverrides hard-coded CanHideFields: false while
// ModelOverrides correctly derived it from CanEdit. Both editors must
// mirror the caller's edit permission for CanHideFields, exactly like
// CanAddField/CanReorder/CanEditOverrides already do. h has a nil svc —
// Service.CanEdit reads only auth.FromContext(ctx), never the receiver,
// so this doesn't need a DB.
func TestOverrideCapabilitiesCanHideFieldsMirrorsEditPermission(t *testing.T) {
	tests := []struct {
		name                  string
		snap                  *weaveauth.AuthSnapshot
		supportsAddCollection bool // true for ModelOverrides, false for CollectionOverrides
	}{
		{
			name:                  "editor can edit: collection editor (no add-collection support)",
			snap:                  &weaveauth.AuthSnapshot{IsSuperAdmin: true},
			supportsAddCollection: false,
		},
		{
			name:                  "editor can edit: model editor (supports add-collection)",
			snap:                  &weaveauth.AuthSnapshot{IsSuperAdmin: true},
			supportsAddCollection: true,
		},
		{
			name:                  "anonymous cannot edit",
			snap:                  &weaveauth.AuthSnapshot{IsAnonymous: true},
			supportsAddCollection: false,
		},
	}

	h := &Handler{}
	p := &domain.Project{Entity: domain.Entity{ID: "P1"}, Visibility: "private"}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := weaveauth.WithSnapshot(context.Background(), tc.snap)
			canEdit := h.svc.CanEdit(ctx, p)

			caps := h.overrideCapabilities(ctx, p, tc.supportsAddCollection)

			if caps.CanHideFields != canEdit {
				t.Fatalf("CanHideFields = %v, want %v (mirrors CanEdit)", caps.CanHideFields, canEdit)
			}
			if caps.CanHideFields != caps.CanEditOverrides {
				t.Fatalf("CanHideFields (%v) diverges from CanEditOverrides (%v)", caps.CanHideFields, caps.CanEditOverrides)
			}
			wantCanAddCollection := tc.supportsAddCollection && canEdit
			if caps.CanAddCollection != wantCanAddCollection {
				t.Fatalf("CanAddCollection = %v, want %v", caps.CanAddCollection, wantCanAddCollection)
			}
		})
	}
}

func TestRefsFromCategoriesIncludesCanonicalOrderAndFallbackName(t *testing.T) {
	refs := refsFromCategories([]*domain.Category{
		{
			Entity: domain.Entity{
				ID:         "cat-1",
				SystemName: "name",
			},
			CanonicalOrder: 7,
		},
	})

	if len(refs) != 2 {
		t.Fatalf("len(refs) = %d, want 2", len(refs))
	}
	if refs[0].CanonicalOrder != 7 {
		t.Fatalf("canonical_order = %d, want 7", refs[0].CanonicalOrder)
	}
	if got := refs[0].Name.Get("en", ""); got != "name" {
		t.Fatalf("name.en = %q, want name", got)
	}
	if refs[1].ID != overrideEditorUncategorizedID {
		t.Fatalf("refs[1].id = %q, want %q", refs[1].ID, overrideEditorUncategorizedID)
	}
	if refs[1].CanonicalOrder != 8 {
		t.Fatalf("uncategorized canonical_order = %d, want 8", refs[1].CanonicalOrder)
	}
	if got := refs[1].Name.Get("en", ""); got != "Uncategorized" {
		t.Fatalf("uncategorized name.en = %q, want Uncategorized", got)
	}
}

// TestCollectionOverrideCategoriesNeverHoistsSharedPathPrefix is the
// regression test for the follow-up: a category subsection
// ("field-group", ID directFieldsID) must never get a SharedPathPrefix, even
// when its fields happen to share a path root. Direct fields share only a
// display category, not an ontology root — hoisting the shared prefix (here,
// a single field's whole path) causes EditableFieldRow to slice it off the
// row and render an empty field. Mirrors the fix already applied to
// resolve.go's buildModelView/buildModelViewVersion for the "__direct__"
// collection bucket (commit 402e7b27).
func TestCollectionOverrideCategoriesNeverHoistsSharedPathPrefix(t *testing.T) {
	h := &Handler{}
	sharedPath := []domain.PathElement{
		{Type: "class", URI: "crm:E21_Person", LocalName: "E21_Person", Position: 0},
		{Type: "property", URI: "crm:P1_is_identified_by", LocalName: "P1_is_identified_by", Position: 1},
	}
	categories := h.collectionOverrideCategories(context.Background(), "P1", []domain.ResolvedField{
		{
			ID:           "field-1",
			DisplayName:  domain.Translations{"en": "Name"},
			CategoryID:   "cat-1",
			Position:     1,
			OverrideID:   10,
			Description:  domain.Translations{"en": "Description"},
			OntologyPath: "crm:E21_Person/crm:P1_is_identified_by",
			PathElements: sharedPath,
		},
	}, nil)

	if len(categories) != 1 || len(categories[0].Items) != 1 {
		t.Fatalf("categories = %#v", categories)
	}
	item := categories[0].Items[0]
	if item.ID != directFieldsID {
		t.Fatalf("item.ID = %q, want %q", item.ID, directFieldsID)
	}
	if len(item.SharedPathPrefix) != 0 {
		t.Fatalf("SharedPathPrefix = %#v, want nil/empty (direct fields share a category, not an ontology root)", item.SharedPathPrefix)
	}
	if len(item.Fields) != 1 || len(item.Fields[0].PathElements) != len(sharedPath) {
		t.Fatalf("field row path_elements truncated: %#v", item.Fields)
	}
}

func TestCollectionOverrideCategoriesNormalizesUncategorized(t *testing.T) {
	h := &Handler{}
	categories := h.collectionOverrideCategories(context.Background(), "P1", []domain.ResolvedField{
		{
			ID:           "field-1",
			DisplayName:  domain.Translations{"en": "Name"},
			CategoryID:   "",
			Position:     1,
			OverrideID:   10,
			Description:  domain.Translations{"en": "Description"},
			OntologyPath: "crm:P1",
		},
	}, nil)

	if len(categories) != 1 {
		t.Fatalf("len(categories) = %d, want 1", len(categories))
	}
	if categories[0].CategoryID != overrideEditorUncategorizedID {
		t.Fatalf("category_id = %q, want %q", categories[0].CategoryID, overrideEditorUncategorizedID)
	}
	if got := categories[0].CategoryName.Get("en", ""); got != "Uncategorized" {
		t.Fatalf("category_name.en = %q, want Uncategorized", got)
	}
	if len(categories[0].Items) != 1 || categories[0].Items[0].ID != directFieldsID {
		t.Fatalf("items = %#v", categories[0].Items)
	}
	if len(categories[0].Items[0].Fields) != 1 {
		t.Fatalf("len(fields) = %d, want 1", len(categories[0].Items[0].Fields))
	}
	if categories[0].Items[0].Fields[0].CategoryID != overrideEditorUncategorizedID {
		t.Fatalf("field.category_id = %q, want %q", categories[0].Items[0].Fields[0].CategoryID, overrideEditorUncategorizedID)
	}
}
