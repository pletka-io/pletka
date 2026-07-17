package gitmaterializer

import (
	"strings"
	"testing"
)

func TestEncodeForkReceiptManifest_Deterministic(t *testing.T) {
	manifest := forkReceiptManifest{
		SchemaVersion: 1,
		Fork: forkReceiptRecord{
			EntityType: "model",
			EntityID:   "TPCM.8",
			Source: forkReceiptSource{
				ProjectID: "LA",
				EntityID:  "LAM.13",
				Version:   "0.1.5-test",
			},
			ForkedAt: "2026-05-10T12:00:00Z",
			CreatedBy: &manifestActor{
				ActorID:     "ACT2",
				Type:        "person",
				Slug:        "sjoerd_siebinga",
				DisplayName: "Sjoerd Siebinga",
			},
		},
	}

	got, err := encodeForkReceiptManifest(manifest)
	if err != nil {
		t.Fatalf("encodeForkReceiptManifest: %v", err)
	}
	gotAgain, err := encodeForkReceiptManifest(manifest)
	if err != nil {
		t.Fatalf("encodeForkReceiptManifest second pass: %v", err)
	}
	if string(got) != string(gotAgain) {
		t.Fatal("encodeForkReceiptManifest is not deterministic")
	}

	want := strings.TrimLeft(`
fork:
  created_by:
    actor_id: ACT2
    display_name: Sjoerd Siebinga
    slug: sjoerd_siebinga
    type: person
  entity_id: TPCM.8
  entity_type: model
  forked_at: "2026-05-10T12:00:00Z"
  source:
    entity_id: LAM.13
    project_id: LA
    version: 0.1.5-test
schema_version: 1
`, "\n")

	if string(got) != want {
		t.Fatalf("unexpected yaml:\n%s", got)
	}
}
