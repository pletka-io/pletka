package canonical

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestPlacementEncodesSortedKeysAndNullMax(t *testing.T) {
	got, err := Placement(&domain.CollectionPlacement{
		ID: 7, ProjectID: "SI", ModelID: "SIM.19", CategoryID: "SI.CAT.3", CollectionID: "LAC.6",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "category_id: SI.CAT.3\ncollection_id: LAC.6\nis_hidden: false\nis_required: false\nmax_occurs: null\nmin_occurs: 0\nmodel_id: SIM.19\n"
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	one := 1
	got, _ = Placement(&domain.CollectionPlacement{ModelID: "SIM.19", CollectionID: "LAC.6", MaxOccurs: &one, MinOccurs: 1, IsRequired: true})
	want = "category_id: \"\"\ncollection_id: LAC.6\nis_hidden: false\nis_required: true\nmax_occurs: 1\nmin_occurs: 1\nmodel_id: SIM.19\n"
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}
