package weave

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/pletka-io/pletka/domain"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

func TestProjectStoreNilGuard(t *testing.T) {
	ctx := context.Background()
	var store *projectStore

	if _, err := store.GetByID(ctx, "TPC"); err == nil {
		t.Fatalf("nil GetByID() error = nil; want error")
	}
	if _, err := store.GetByIDVersion(ctx, "TPC", "v1"); err == nil {
		t.Fatalf("nil GetByIDVersion() error = nil; want error")
	}
}

func TestProjectFromRow(t *testing.T) {
	createdAt := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	systemName := "test-project"
	namespace := "https://example.test/ns/"
	parentID := "LA"
	stagingID := int64(42)
	createdByID := "actor-1"

	project := projectFromRow(sqlcgen.WeaveProject{
		ID:              "TPC",
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		SystemName:      &systemName,
		UiName:          mustJSON(t, domain.Translations{"en": "Test Project"}),
		Description:     mustJSON(t, domain.Translations{"en": "Description"}),
		Status:          string(domain.StatusPublished),
		Namespace:       &namespace,
		ParentProjectID: &parentID,
		StagingID:       &stagingID,
		OwnerID:         "owner-1",
		Visibility:      "public",
		License:         "CC-BY-4.0",
		Readme:          mustJSON(t, domain.Translations{"en": "README"}),
		Topics:          []string{"test", "project"},
		BaseUrl:         "https://example.test/",
		CreatedByID:     &createdByID,
		VersionNumber:   "draft",
		IsMasterWeave:   true,
	})

	if project.ID != "TPC" || project.SemanticID != "TPC" || project.SystemName != "test-project" {
		t.Fatalf("project identity mapping failed: %#v", project)
	}
	if project.UIName.Get("en") != "Test Project" || project.README.Get("en") != "README" {
		t.Fatalf("project translations mapping failed: %#v", project)
	}
	if project.ParentProjectID == nil || *project.ParentProjectID != "LA" {
		t.Fatalf("ParentProjectID = %#v", project.ParentProjectID)
	}
	if project.StagingID == nil || *project.StagingID != 42 {
		t.Fatalf("StagingID = %#v", project.StagingID)
	}
	if project.Status != domain.StatusPublished || !project.IsMasterWeave {
		t.Fatalf("project status/master mapping failed: %#v", project)
	}
}

func TestTranslationsFromJSON(t *testing.T) {
	if translationsFromJSON(nil) != nil {
		t.Fatalf("nil translations should stay nil")
	}
	if translationsFromJSON([]byte("{")) != nil {
		t.Fatalf("invalid translations should return nil")
	}
	got := translationsFromJSON([]byte(`{"en":"Value"}`))
	if got.Get("en") != "Value" {
		t.Fatalf("translationsFromJSON() = %#v", got)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return data
}
