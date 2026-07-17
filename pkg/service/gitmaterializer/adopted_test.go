package gitmaterializer

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestAdoptedIDsFromAdoptions_ExplicitOnly(t *testing.T) {
	adoptions := []domain.Adoption{
		{
			EntityType:      "field",
			SourceProjectID: "LA",
			SourceEntityID:  "LAF.11",
		},
		{
			EntityType:      "model",
			SourceProjectID: "LA",
			SourceEntityID:  "LAM.13",
		},
		{
			EntityType:      "collection",
			SourceProjectID: "LA",
			SourceEntityID:  "LAC.4",
		},
		{
			EntityType:      "category",
			SourceProjectID: "LA",
			SourceEntityID:  "LA.CAT.1",
		},
		{
			EntityType:      "field",
			SourceProjectID: "TPC",
			SourceEntityID:  "TPCF.1",
		},
		{
			EntityType:      "field",
			SourceProjectID: "",
			SourceEntityID:  "LAF.22",
		},
	}

	got := adoptedIDsFromAdoptions(adoptions, "TPC")

	if len(got.fieldIDs) != 1 || got.fieldIDs["LAF.11"] != "LA" {
		t.Fatalf("unexpected field adoptions: %#v", got.fieldIDs)
	}
	if len(got.modelIDs) != 1 || got.modelIDs["LAM.13"] != "LA" {
		t.Fatalf("unexpected model adoptions: %#v", got.modelIDs)
	}
	if len(got.collectionIDs) != 1 || got.collectionIDs["LAC.4"] != "LA" {
		t.Fatalf("unexpected collection adoptions: %#v", got.collectionIDs)
	}
}
