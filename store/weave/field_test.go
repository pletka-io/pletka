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

func TestFieldStoreNilGuard(t *testing.T) {
	ctx := context.Background()
	var store *fieldStore

	if _, err := store.GetByID(ctx, "TPC", "field-1"); err == nil {
		t.Fatalf("nil GetByID() error = nil; want error")
	}
	if _, err := store.GetByIDVersion(ctx, "TPC", "field-1", "v1"); err == nil {
		t.Fatalf("nil GetByIDVersion() error = nil; want error")
	}
	if _, err := store.List(ctx, "TPC"); err == nil {
		t.Fatalf("nil List() error = nil; want error")
	}
	if _, err := store.ListVersion(ctx, "TPC", "v1"); err == nil {
		t.Fatalf("nil ListVersion() error = nil; want error")
	}
}

func TestFieldFromRow(t *testing.T) {
	createdAt := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	semanticID := "TPC.9"
	systemName := "name"
	expectedValueType := "literal"
	stagingID := int64(99)

	field := fieldFromRow(sqlcgen.WeaveField{
		ID:                "field-1",
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
		SemanticID:        &semanticID,
		SystemName:        &systemName,
		UiName:            mustFieldJSON(t, domain.Translations{"en": "Name"}),
		Description:       mustFieldJSON(t, domain.Translations{"en": "Person name"}),
		Status:            string(domain.StatusPublished),
		ProjectID:         "TPC",
		OntologyScope:     mustFieldJSON(t, domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E21_Person"}),
		PathElements:      mustFieldJSON(t, []domain.PathElement{{Type: "property", Prefix: "crm", LocalName: "P1_is_identified_by"}}),
		ExpectedValueType: &expectedValueType,
		Examples:          mustFieldJSON(t, []domainweave.FieldExample{{Value: "Ada"}}),
		StagingID:         &stagingID,
		Deprecated:        true,
		VersionNumber:     "v1",
	})

	if field.ID != "field-1" || field.SemanticID != "TPC.9" || field.ProjectID != "TPC" {
		t.Fatalf("field identity mapping failed: %#v", field)
	}
	if field.UIName.Get("en") != "Name" || field.Description.Get("en") != "Person name" {
		t.Fatalf("field translations mapping failed: %#v", field)
	}
	if field.OntologyScope.PrefixedName() != "crm:E21_Person" {
		t.Fatalf("OntologyScope = %#v", field.OntologyScope)
	}
	if len(field.PathElements) != 1 || field.OntologyPath() != "->crm:P1_is_identified_by" {
		t.Fatalf("PathElements = %#v; path = %q", field.PathElements, field.OntologyPath())
	}
	if field.ExpectedValueType != "literal" || len(field.Examples) != 1 || field.StagingID == nil || *field.StagingID != 99 {
		t.Fatalf("field payload mapping failed: %#v", field)
	}
	if field.Status != domain.StatusPublished || !field.Deprecated {
		t.Fatalf("field metadata mapping failed: %#v", field)
	}
}

func TestPathElementPrefixedName(t *testing.T) {
	element := domain.PathElement{
		Prefix:    "crm",
		LocalName: "E29_Design_or_Procedure",
		AdditionalTypes: []domain.TypeRef{{
			Prefix:    "crmdig",
			LocalName: "D1_Digital_Object",
		}},
	}
	if got := element.PrefixedName(); got != "crm:E29_Design_or_Procedure/crmdig:D1_Digital_Object" {
		t.Fatalf("PrefixedName() = %q", got)
	}
}

func mustFieldJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return data
}
