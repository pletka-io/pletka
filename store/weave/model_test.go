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

func TestModelStoreNilGuard(t *testing.T) {
	ctx := context.Background()
	var store *modelStore

	if _, err := store.GetByID(ctx, "TPC", "model-1"); err == nil {
		t.Fatalf("nil GetByID() error = nil; want error")
	}
	if _, err := store.GetByIDVersion(ctx, "TPC", "model-1", "v1"); err == nil {
		t.Fatalf("nil GetByIDVersion() error = nil; want error")
	}
	if _, err := store.List(ctx, "TPC"); err == nil {
		t.Fatalf("nil List() error = nil; want error")
	}
	if _, err := store.ListVersion(ctx, "TPC", "v1"); err == nil {
		t.Fatalf("nil ListVersion() error = nil; want error")
	}
}

func TestModelFromRow(t *testing.T) {
	createdAt := time.Date(2026, 5, 16, 13, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	systemName := "person"
	stagingID := int64(77)

	model := modelFromRow(sqlcgen.WeaveModel{
		ID:            "TPC.1",
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		SystemName:    &systemName,
		UiName:        mustModelJSON(t, domain.Translations{"en": "Person"}),
		Description:   mustModelJSON(t, domain.Translations{"en": "A person"}),
		Status:        string(domain.StatusPublished),
		ProjectID:     "TPC",
		OntologyScope: mustModelJSON(t, domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E21_Person"}),
		StagingID:     &stagingID,
		Deprecated:    true,
		VersionNumber: "v1",
		ModelType:     domainweave.ModelTypeCore,
	})

	if model.ID != "TPC.1" || model.SemanticID != "TPC.1" || model.ProjectID != "TPC" {
		t.Fatalf("model identity mapping failed: %#v", model)
	}
	if model.UIName.Get("en") != "Person" || model.Description.Get("en") != "A person" {
		t.Fatalf("model translations mapping failed: %#v", model)
	}
	if model.OntologyScope.PrefixedName() != "crm:E21_Person" {
		t.Fatalf("OntologyScope = %#v", model.OntologyScope)
	}
	if model.ModelType != domainweave.ModelTypeCore || model.StagingID == nil || *model.StagingID != 77 {
		t.Fatalf("model payload mapping failed: %#v", model)
	}
	if model.Status != domain.StatusPublished || !model.Deprecated {
		t.Fatalf("model metadata mapping failed: %#v", model)
	}
}

func TestModelTypeValidation(t *testing.T) {
	for _, value := range []string{"", domainweave.ModelTypeCore, domainweave.ModelTypeAuxiliary, domainweave.ModelTypeExample} {
		if !domainweave.IsValidModelType(value) {
			t.Fatalf("IsValidModelType(%q) = false", value)
		}
	}
	if domainweave.IsValidModelType("invalid") {
		t.Fatalf("IsValidModelType(invalid) = true")
	}
}

func mustModelJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return data
}
