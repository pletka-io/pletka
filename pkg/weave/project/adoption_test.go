package project

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestBuildAdoptionsFromOverrideCategories(t *testing.T) {
	actorID := "actor-1"
	got := buildAdoptionsFromOverrideCategories("TPC", "model", "TPCM.3", []overrideEditorCategory{
		{
			Items: []overrideEditorItem{
				{
					Widget: "collection-group",
					ID:     "LAC.2",
					Fields: []overrideEditorField{
						{FieldID: "LAF.1"},
					},
				},
				{
					Widget: "field-group",
					Fields: []overrideEditorField{
						{FieldID: "LAF.1"},
						{FieldID: "LAF.1"},
						{FieldID: "TPCF.9"},
						{FieldID: "bad-id"},
					},
				},
			},
		},
	}, &actorID)

	if len(got) != 2 {
		t.Fatalf("expected 2 adoptions, got %d: %#v", len(got), got)
	}

	if got[0].EntityType != "collection" || got[0].SourceProjectID != "LA" || got[0].SourceEntityID != "LAC.2" {
		t.Fatalf("unexpected collection adoption: %#v", got[0])
	}
	if got[1].EntityType != "field" || got[1].SourceProjectID != "LA" || got[1].SourceEntityID != "LAF.1" {
		t.Fatalf("unexpected field adoption: %#v", got[1])
	}
	if got[0].ContextEntityType != "model" || got[0].ContextEntityID != "TPCM.3" {
		t.Fatalf("missing context on adoption: %#v", got[0])
	}
	if got[0].CreatedByID == nil || *got[0].CreatedByID != actorID {
		t.Fatalf("missing created_by_id on adoption: %#v", got[0])
	}
}

func TestAdoptionReceiptKeyStable(t *testing.T) {
	a := domain.Adoption{
		ContextEntityType: "model",
		ContextEntityID:   "TPCM.3",
		EntityType:        "collection",
		SourceProjectID:   "LA",
		SourceEntityID:    "LAC.2",
	}
	b := domain.Adoption{
		ContextEntityType: "model",
		ContextEntityID:   "TPCM.3",
		EntityType:        "collection",
		SourceProjectID:   "LA",
		SourceEntityID:    "LAC.2",
	}
	if adoptionReceiptKey(a) != adoptionReceiptKey(b) {
		t.Fatalf("expected stable adoption receipt key")
	}
}
