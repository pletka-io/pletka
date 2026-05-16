package weave

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/pletka-io/pletka/domain"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

func TestCollectionStoreNilGuard(t *testing.T) {
	ctx := context.Background()
	var store *collectionStore

	if _, err := store.GetByID(ctx, "TPC", "collection-1"); err == nil {
		t.Fatalf("nil GetByID() error = nil; want error")
	}
	if _, err := store.GetByIDVersion(ctx, "TPC", "collection-1", "v1"); err == nil {
		t.Fatalf("nil GetByIDVersion() error = nil; want error")
	}
	if _, err := store.List(ctx, "TPC"); err == nil {
		t.Fatalf("nil List() error = nil; want error")
	}
	if _, err := store.ListVersion(ctx, "TPC", "v1"); err == nil {
		t.Fatalf("nil ListVersion() error = nil; want error")
	}
}

func TestCollectionFromRow(t *testing.T) {
	createdAt := time.Date(2026, 5, 16, 14, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	systemName := "birth"
	collectionNumber := int32(3)
	order := int32(9)
	stagingID := int64(88)
	defaultCategoryID := "cat-1"

	collection := collectionFromRow(sqlcgen.WeaveCollection{
		ID:                       "TPC.3",
		CreatedAt:                createdAt,
		UpdatedAt:                updatedAt,
		SystemName:               &systemName,
		UiName:                   mustCollectionJSON(t, domain.Translations{"en": "Birth"}),
		Description:              mustCollectionJSON(t, domain.Translations{"en": "Birth event"}),
		Status:                   string(domain.StatusPublished),
		ProjectID:                "TPC",
		OntologyScope:            mustCollectionJSON(t, domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E67_Birth"}),
		CollectionNumber:         &collectionNumber,
		CanonicalCollectionOrder: &order,
		StagingID:                &stagingID,
		Deprecated:               true,
		DefaultCategoryID:        &defaultCategoryID,
		VersionNumber:            "v1",
	})

	if collection.ID != "TPC.3" || collection.SemanticID != "TPC.3" || collection.ProjectID != "TPC" {
		t.Fatalf("collection identity mapping failed: %#v", collection)
	}
	if collection.UIName.Get("en") != "Birth" || collection.Description.Get("en") != "Birth event" {
		t.Fatalf("collection translations mapping failed: %#v", collection)
	}
	if collection.OntologyScope.PrefixedName() != "crm:E67_Birth" {
		t.Fatalf("OntologyScope = %#v", collection.OntologyScope)
	}
	if collection.CollectionNumber != 3 || collection.CanonicalCollectionOrder != 9 {
		t.Fatalf("collection order mapping failed: %#v", collection)
	}
	if collection.DefaultCategoryID == nil || *collection.DefaultCategoryID != "cat-1" || collection.StagingID == nil || *collection.StagingID != 88 {
		t.Fatalf("collection payload mapping failed: %#v", collection)
	}
	if collection.Status != domain.StatusPublished || !collection.Deprecated {
		t.Fatalf("collection metadata mapping failed: %#v", collection)
	}
}

func mustCollectionJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return data
}
