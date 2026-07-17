package project

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestBuildAdoptedCollectionItemFlattensIntoSingleGroup(t *testing.T) {
	collection := &domain.Collection{
		Entity: domain.Entity{
			ID:         "collection-1",
			SemanticID: "LAC.1",
			UIName:     domain.Translations{"en": "Name"},
		},
	}
	sharedPrefix := []domain.PathElement{
		{Prefix: "crm", LocalName: "P1_is_identified_by", URI: "crm:P1_is_identified_by"},
	}
	source := []overrideEditorCategory{
		{
			CategoryID: "cat-a",
			Items: []overrideEditorItem{{
				Widget: "field-group",
				ID:     directFieldsID,
				Fields: []overrideEditorField{
					{
						FieldID:                "field-1",
						OverrideID:             11,
						Position:               1,
						DisplayName:            domain.Translations{"en": "Label"},
						CategoryID:             "cat-a",
						PartOfCollectionID:     "",
						ExpectedValueType:      "Model",
						ExpectedResourceModels: []string{"model-1"},
						ExpectedConceptLists:   []string{"list-1"},
						ExpectedResourceModelRefs: []domain.EntityRef{{
							ID:         "model-1",
							SemanticID: "TPEM.1",
							Name:       domain.Translations{"en": "Person"},
						}},
						ExpectedConceptListRefs: []domain.EntityRef{{
							ID:         "list-1",
							SemanticID: "TPEC.1",
							Name:       domain.Translations{"en": "Materials"},
						}},
					},
				},
			}},
		},
		{
			CategoryID: "cat-b",
			Items: []overrideEditorItem{{
				Widget: "field-group",
				ID:     directFieldsID,
				Fields: []overrideEditorField{
					{
						FieldID:                  "field-2",
						OverrideID:               12,
						Position:                 1,
						DisplayName:              domain.Translations{"en": "Type"},
						CategoryID:               "cat-b",
						ExpectedValueType:        "Collection",
						ExpectedCollectionModels: []string{"collection-2"},
						ExpectedCollectionModelRefs: []domain.EntityRef{{
							ID:         "collection-2",
							SemanticID: "LAC.2",
							Name:       domain.Translations{"en": "Identifier"},
						}},
					},
				},
			}},
		},
	}

	item := buildAdoptedCollectionItem(collection, "target-cat", source, sharedPrefix)

	if item.Widget != "collection-group" {
		t.Fatalf("widget = %q, want collection-group", item.Widget)
	}
	if item.ID != collection.ID {
		t.Fatalf("id = %q, want %q", item.ID, collection.ID)
	}
	if item.SemanticID != collection.SemanticID {
		t.Fatalf("semantic_id = %q, want %q", item.SemanticID, collection.SemanticID)
	}
	if got := item.Name.Get("en", ""); got != "Name" {
		t.Fatalf("name.en = %q, want Name", got)
	}
	if len(item.SharedPathPrefix) != 1 || item.SharedPathPrefix[0].URI != "crm:P1_is_identified_by" {
		t.Fatalf("shared prefix = %#v", item.SharedPathPrefix)
	}
	if item.FieldCount != 2 {
		t.Fatalf("field_count = %d, want 2", item.FieldCount)
	}
	if len(item.Fields) != 2 {
		t.Fatalf("len(fields) = %d, want 2", len(item.Fields))
	}

	first := item.Fields[0]
	if first.OverrideID != 0 {
		t.Fatalf("first override_id = %d, want 0", first.OverrideID)
	}
	if first.Position != 1 {
		t.Fatalf("first position = %d, want 1", first.Position)
	}
	if first.CategoryID != "target-cat" {
		t.Fatalf("first category_id = %q, want target-cat", first.CategoryID)
	}
	if first.PartOfCollectionID != collection.ID {
		t.Fatalf("first part_of_collection_id = %q, want %q", first.PartOfCollectionID, collection.ID)
	}
	if len(first.ExpectedResourceModelRefs) != 1 || first.ExpectedResourceModelRefs[0].ID != "model-1" {
		t.Fatalf("first resource refs = %#v", first.ExpectedResourceModelRefs)
	}
	if len(first.ExpectedConceptListRefs) != 1 || first.ExpectedConceptListRefs[0].ID != "list-1" {
		t.Fatalf("first concept-list refs = %#v", first.ExpectedConceptListRefs)
	}

	second := item.Fields[1]
	if second.Position != 2 {
		t.Fatalf("second position = %d, want 2", second.Position)
	}
	if second.CategoryID != "target-cat" {
		t.Fatalf("second category_id = %q, want target-cat", second.CategoryID)
	}
	if len(second.ExpectedCollectionModelRefs) != 1 || second.ExpectedCollectionModelRefs[0].ID != "collection-2" {
		t.Fatalf("second collection refs = %#v", second.ExpectedCollectionModelRefs)
	}
}

func TestNormalizeEditorCategoryID(t *testing.T) {
	if got := normalizeEditorCategoryID(overrideEditorUncategorizedID); got != "" {
		t.Fatalf("normalize uncategorized = %q, want empty", got)
	}
	if got := denormalizeEditorCategoryID(""); got != overrideEditorUncategorizedID {
		t.Fatalf("denormalize empty = %q, want %q", got, overrideEditorUncategorizedID)
	}
	if got := normalizeEditorCategoryID("cat-1"); got != "cat-1" {
		t.Fatalf("normalize cat-1 = %q, want cat-1", got)
	}
}

func TestBuildOverrideRefsIncludesConceptLists(t *testing.T) {
	refs := buildOverrideRefs(overrideEditorField{
		ExpectedResourceModels:   []string{"model-1"},
		ExpectedCollectionModels: []string{"collection-1"},
		ExpectedConceptLists:     []string{"concept-list-1"},
	})
	if len(refs) != 3 {
		t.Fatalf("len(refs) = %d, want 3", len(refs))
	}
	if refs[0].RefType != "resource_model" || refs[0].TargetID != "model-1" {
		t.Fatalf("resource ref = %#v", refs[0])
	}
	if refs[1].RefType != "collection_model" || refs[1].TargetID != "collection-1" {
		t.Fatalf("collection ref = %#v", refs[1])
	}
	if refs[2].RefType != "concept_list" || refs[2].TargetID != "concept-list-1" {
		t.Fatalf("concept-list ref = %#v", refs[2])
	}
}
