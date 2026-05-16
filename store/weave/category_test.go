package weave

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/pletka-io/pletka/domain"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

func TestCategoryStoreNilGuard(t *testing.T) {
	ctx := context.Background()
	var store *categoryStore

	if _, err := store.GetByID(ctx, "TPC", "cat-1"); err == nil {
		t.Fatalf("nil GetByID() error = nil; want error")
	}
	if _, err := store.GetByIDVersion(ctx, "TPC", "cat-1", "v1"); err == nil {
		t.Fatalf("nil GetByIDVersion() error = nil; want error")
	}
	if _, err := store.List(ctx, "TPC"); err == nil {
		t.Fatalf("nil List() error = nil; want error")
	}
	if _, err := store.ListVersion(ctx, "TPC", "v1"); err == nil {
		t.Fatalf("nil ListVersion() error = nil; want error")
	}
}

func TestCategoryFromRow(t *testing.T) {
	createdAt := time.Date(2026, 5, 16, 11, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	semanticID := "TPC.12"
	systemName := "identity"

	category := categoryFromRow(sqlcgen.WeaveCategory{
		ID:             "cat-1",
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		SemanticID:     &semanticID,
		SystemName:     &systemName,
		UiName:         mustCategoryJSON(t, domain.Translations{"en": "Identity"}),
		Description:    mustCategoryJSON(t, domain.Translations{"en": "Identity fields"}),
		Status:         string(domain.StatusPublished),
		ProjectID:      "TPC",
		CanonicalOrder: 7,
		Deprecated:     true,
		VersionNumber:  "v1",
	})

	if category.ID != "cat-1" || category.SemanticID != "TPC.12" || category.ProjectID != "TPC" {
		t.Fatalf("category identity mapping failed: %#v", category)
	}
	if category.UIName.Get("en") != "Identity" || category.Description.Get("en") != "Identity fields" {
		t.Fatalf("category translations mapping failed: %#v", category)
	}
	if category.Status != domain.StatusPublished || category.CanonicalOrder != 7 || !category.Deprecated {
		t.Fatalf("category metadata mapping failed: %#v", category)
	}
	if category.Origin.Kind != domain.OriginOwn {
		t.Fatalf("category origin = %#v; want own", category.Origin)
	}
}

func TestSemanticProjectID(t *testing.T) {
	if got := semanticProjectID(nil); got != "" {
		t.Fatalf("semanticProjectID(nil) = %q", got)
	}
	value := "LA.5"
	if got := semanticProjectID(&value); got != "LA" {
		t.Fatalf("semanticProjectID(LA.5) = %q", got)
	}
	bad := "not-semantic"
	if got := semanticProjectID(&bad); got != "" {
		t.Fatalf("semanticProjectID(bad) = %q", got)
	}
}

func mustCategoryJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return data
}
