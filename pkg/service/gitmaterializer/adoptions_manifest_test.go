package gitmaterializer

import (
	"strings"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestGroupProjectAdoptions_GroupsBySourceEntity(t *testing.T) {
	adoptions := []domain.Adoption{
		{
			EntityType:        "field",
			SourceProjectID:   "LA",
			SourceEntityID:    "LAF.11",
			ContextEntityType: "model",
			ContextEntityID:   "TPCM.3",
			AdoptedAt:         time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC),
		},
		{
			EntityType:        "field",
			SourceProjectID:   "LA",
			SourceEntityID:    "LAF.11",
			ContextEntityType: "collection",
			ContextEntityID:   "TPCC.4",
			AdoptedAt:         time.Date(2026, 5, 10, 11, 0, 0, 0, time.UTC),
		},
		{
			EntityType:        "model",
			SourceProjectID:   "LA",
			SourceEntityID:    "LAM.13",
			ContextEntityType: "project",
			ContextEntityID:   "TPC",
		},
	}

	grouped := groupProjectAdoptions(adoptions)
	if len(grouped) != 2 {
		t.Fatalf("expected 2 grouped receipts, got %d", len(grouped))
	}
	if grouped[0].EntityType != "field" || grouped[0].EntityID != "LAF.11" {
		t.Fatalf("unexpected first group: %#v", grouped[0])
	}
	if len(grouped[0].Contexts) != 2 {
		t.Fatalf("expected grouped field receipt to have 2 contexts, got %d", len(grouped[0].Contexts))
	}
	if grouped[1].EntityType != "model" || grouped[1].EntityID != "LAM.13" {
		t.Fatalf("unexpected second group: %#v", grouped[1])
	}
}

func TestEncodeAdoptionReceiptManifest_Deterministic(t *testing.T) {
	manifest := adoptionReceiptManifest{
		SchemaVersion: 1,
		Adoption: adoptionReceiptRecord{
			EntityType: "field",
			EntityID:   "LAF.11",
			Source: adoptionReceiptSource{
				ProjectID: "LA",
				EntityID:  "LAF.11",
			},
			Contexts: []adoptionReceiptContext{
				{
					ContextEntityType: "collection",
					ContextEntityID:   "TPCC.4",
					AdoptedAt:         "2026-05-10T11:00:00Z",
					AdoptedBy: &manifestActor{
						ActorID:     "ACT2",
						Type:        "person",
						Slug:        "sjoerd_siebinga",
						DisplayName: "Sjoerd Siebinga",
					},
				},
				{
					ContextEntityType: "model",
					ContextEntityID:   "TPCM.3",
					AdoptedAt:         "2026-05-10T10:00:00Z",
				},
			},
		},
	}

	got, err := encodeAdoptionReceiptManifest(manifest)
	if err != nil {
		t.Fatalf("encodeAdoptionReceiptManifest: %v", err)
	}
	gotAgain, err := encodeAdoptionReceiptManifest(manifest)
	if err != nil {
		t.Fatalf("encodeAdoptionReceiptManifest second pass: %v", err)
	}
	if string(got) != string(gotAgain) {
		t.Fatal("encodeAdoptionReceiptManifest is not deterministic")
	}

	want := strings.TrimLeft(`
adoption:
  contexts:
    - adopted_at: "2026-05-10T11:00:00Z"
      adopted_by:
        actor_id: ACT2
        display_name: Sjoerd Siebinga
        slug: sjoerd_siebinga
        type: person
      context_entity_id: TPCC.4
      context_entity_type: collection
    - adopted_at: "2026-05-10T10:00:00Z"
      context_entity_id: TPCM.3
      context_entity_type: model
  entity_id: LAF.11
  entity_type: field
  source:
    entity_id: LAF.11
    project_id: LA
schema_version: 1
`, "\n")

	if string(got) != want {
		t.Fatalf("unexpected yaml:\n%s", got)
	}
}
