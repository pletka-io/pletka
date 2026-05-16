package weave

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/pletka-io/pletka/domain"
	domainweave "github.com/pletka-io/pletka/domain/weave"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

func TestFieldOverrideStoreNilGuard(t *testing.T) {
	ctx := context.Background()
	var store *fieldOverrideStore

	if _, err := store.ListForModel(ctx, "TPC", "TPC.1"); err == nil {
		t.Fatalf("nil ListForModel() error = nil; want error")
	}
	if _, err := store.ListForModelVersion(ctx, "TPC", "TPC.1", "v1"); err == nil {
		t.Fatalf("nil ListForModelVersion() error = nil; want error")
	}
	if _, err := store.ListForCollection(ctx, "TPC", "TPC.C1"); err == nil {
		t.Fatalf("nil ListForCollection() error = nil; want error")
	}
	if _, err := store.ListForCollectionVersion(ctx, "TPC", "TPC.C1", "v1"); err == nil {
		t.Fatalf("nil ListForCollectionVersion() error = nil; want error")
	}
	if _, err := store.ListBaseForFields(ctx, "TPC", []string{"TPC.F1"}); err == nil {
		t.Fatalf("nil ListBaseForFields() error = nil; want error")
	}
	if _, err := store.ListBaseForFieldsVersion(ctx, "TPC", "v1", []string{"TPC.F1"}); err == nil {
		t.Fatalf("nil ListBaseForFieldsVersion() error = nil; want error")
	}
	if _, err := store.ListRefsForOverrides(ctx, []int64{1}); err == nil {
		t.Fatalf("nil ListRefsForOverrides() error = nil; want error")
	}
	if _, err := store.ListRefsForOverridesVersion(ctx, "TPC", "v1", []int64{1}); err == nil {
		t.Fatalf("nil ListRefsForOverridesVersion() error = nil; want error")
	}
}

func TestFieldOverrideEmptyInputsDoNotNeedStore(t *testing.T) {
	ctx := context.Background()
	var store *fieldOverrideStore

	base, err := store.ListBaseForFields(ctx, "TPC", nil)
	if err != nil || len(base) != 0 {
		t.Fatalf("empty ListBaseForFields() = %#v, %v", base, err)
	}
	baseVersion, err := store.ListBaseForFieldsVersion(ctx, "TPC", "v1", nil)
	if err != nil || len(baseVersion) != 0 {
		t.Fatalf("empty ListBaseForFieldsVersion() = %#v, %v", baseVersion, err)
	}
	refs, err := store.ListRefsForOverrides(ctx, nil)
	if err != nil || len(refs) != 0 {
		t.Fatalf("empty ListRefsForOverrides() = %#v, %v", refs, err)
	}
	refsVersion, err := store.ListRefsForOverridesVersion(ctx, "TPC", "v1", nil)
	if err != nil || len(refsVersion) != 0 {
		t.Fatalf("empty ListRefsForOverridesVersion() = %#v, %v", refsVersion, err)
	}
}

func TestFieldOverrideFromRow(t *testing.T) {
	createdAt := time.Date(2026, 5, 16, 14, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	categoryID := "cat-1"
	collectionID := "col-1"
	expectedType := "string"
	setValue := "literal"
	required := true
	minOccurs := int32(1)
	maxOccurs := int32(3)
	hidden := false
	visibility := "public"
	stagingID := int64(99)
	contentHash := "abc123"
	setValueEntryID := "entry-1"

	override := fieldOverrideFromRow(sqlcgen.WeaveFieldOverride{
		ID:                 42,
		FieldID:            "TPC.F1",
		ProjectID:          "TPC",
		EntityType:         domainweave.FieldOverrideEntityModel,
		EntityID:           "TPC.M1",
		Position:           7,
		CollectionOrder:    2,
		DisplayName:        mustFieldOverrideJSON(t, domain.Translations{"en": "Title"}),
		Description:        mustFieldOverrideJSON(t, domain.Translations{"en": "A title"}),
		CollectionName:     mustFieldOverrideJSON(t, domain.Translations{"en": "Names"}),
		CategoryID:         &categoryID,
		PartOfCollectionID: &collectionID,
		ExpectedValueType:  &expectedType,
		SetValue:           &setValue,
		IsRequired:         &required,
		MinOccurs:          &minOccurs,
		MaxOccurs:          &maxOccurs,
		IsHidden:           &hidden,
		Visibility:         &visibility,
		StagingID:          &stagingID,
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
		ContentHash:        &contentHash,
		VersionNumber:      "draft",
		SetValueEntryID:    &setValueEntryID,
	})

	if override.ID != 42 || override.FieldID != "TPC.F1" || override.EntityID != "TPC.M1" {
		t.Fatalf("override identity mapping failed: %#v", override)
	}
	if override.Position != 7 || override.CollectionOrder != 2 {
		t.Fatalf("override order mapping failed: %#v", override)
	}
	if override.DisplayName.Get("en") != "Title" || override.Description.Get("en") != "A title" || override.CollectionName.Get("en") != "Names" {
		t.Fatalf("override translations mapping failed: %#v", override)
	}
	if override.MinOccurs == nil || *override.MinOccurs != 1 || override.MaxOccurs == nil || *override.MaxOccurs != 3 {
		t.Fatalf("override occurrence mapping failed: %#v", override)
	}
	if override.SetValueEntryID == nil || *override.SetValueEntryID != "entry-1" {
		t.Fatalf("override set value entry mapping failed: %#v", override)
	}
}

func TestResolvedFieldOverrideFromValues(t *testing.T) {
	systemName := "title"
	semanticID := "TPC.F1"
	expectedType := "string"
	ontologyPath := "->crm:P1_is_identified_by"

	resolved := resolvedFieldOverrideFromValues(resolvedFieldValues{
		Override: fieldOverrideValues{
			ID:            10,
			FieldID:       "field-row-1",
			ProjectID:     "TPC",
			EntityType:    domainweave.FieldOverrideEntityCollection,
			EntityID:      "TPC.C1",
			VersionNumber: "v1",
		},
		FieldSemanticID:        &semanticID,
		FieldID:                "field-row-1",
		FieldSystemName:        &systemName,
		FieldOntologyScope:     mustFieldOverrideJSON(t, domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E33_E41_Linguistic_Appellation"}),
		FieldOntologyPath:      &ontologyPath,
		FieldExpectedValueType: &expectedType,
		FieldPathElements: mustFieldOverrideJSON(t, []domain.PathElement{
			{Type: "property", Prefix: "crm", LocalName: "P1_is_identified_by", Position: 1},
		}),
	})

	if resolved.Override.ID != 10 || resolved.Override.EntityType != domainweave.FieldOverrideEntityCollection {
		t.Fatalf("resolved override mapping failed: %#v", resolved)
	}
	if resolved.Field.ID != "field-row-1" || resolved.Field.SemanticID != "TPC.F1" || resolved.Field.SystemName != "title" {
		t.Fatalf("resolved field identity mapping failed: %#v", resolved.Field)
	}
	if resolved.Field.OntologyScope.PrefixedName() != "crm:E33_E41_Linguistic_Appellation" {
		t.Fatalf("resolved field scope = %#v", resolved.Field.OntologyScope)
	}
	if len(resolved.Field.PathElements) != 1 || resolved.Field.PathElements[0].PrefixedName() != "crm:P1_is_identified_by" {
		t.Fatalf("resolved path elements = %#v", resolved.Field.PathElements)
	}
}

func TestGroupOverrideRefsByOverrideID(t *testing.T) {
	grouped := groupOverrideRefsByOverrideID([]domainweave.OverrideRef{
		{OverrideID: 2, RefType: "set_value", Position: 1, SemanticID: "B"},
		{OverrideID: 1, RefType: "ontology", Position: 1, SemanticID: "A"},
		{OverrideID: 2, RefType: "set_value", Position: 2, SemanticID: "C"},
	})

	if len(grouped) != 2 || len(grouped[2]) != 2 || grouped[2][1].SemanticID != "C" {
		t.Fatalf("grouped refs = %#v", grouped)
	}
}

func TestIntPtrFromInt32Ptr(t *testing.T) {
	if intPtrFromInt32Ptr(nil) != nil {
		t.Fatalf("nil conversion should stay nil")
	}
	value := int32(4)
	converted := intPtrFromInt32Ptr(&value)
	if converted == nil || *converted != 4 {
		t.Fatalf("converted = %#v", converted)
	}
}

func mustFieldOverrideJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return data
}
