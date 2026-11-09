package gitmaterializer

import "testing"

func TestPlacementFilePath(t *testing.T) {
	if got := placementFilePath("SIM.19", "SI.CAT.3", "LAC.6"); got != "models/SIM.19/placements/LAC.6@SI.CAT.3.yaml" {
		t.Fatalf("categorized: %s", got)
	}
	if got := placementFilePath("SIM.19", "", "LAC.6"); got != "models/SIM.19/placements/LAC.6@_.yaml" {
		t.Fatalf("uncategorized: %s", got)
	}
}

func TestParsePlacementFileName(t *testing.T) {
	cases := []struct {
		name, coll, cat string
		ok              bool
	}{
		{"LAC.6@SI.CAT.3.yaml", "LAC.6", "SI.CAT.3", true},
		{"LAC.6@_.yaml", "LAC.6", "", true},
		{"LAC.6.yaml", "", "", false},
		{"@SI.CAT.3.yaml", "", "", false},
		{"LAC.6@SI.CAT.3.yml", "", "", false},
	}
	for _, c := range cases {
		coll, cat, ok := parsePlacementFileName(c.name)
		if coll != c.coll || cat != c.cat || ok != c.ok {
			t.Fatalf("%s: got (%q,%q,%v)", c.name, coll, cat, ok)
		}
	}
}

func TestClassifyPlacementFile(t *testing.T) {
	got, ok := classifySnapshotEntityFile("models/SIM.19/placements/LAC.6@SI.CAT.3.yaml")
	if !ok || got.EntityType != entityTypeCollectionPlacement || got.OwnerType != "model" || got.OwnerID != "SIM.19" || got.EntityID != "LAC.6" {
		t.Fatalf("classify: %+v ok=%v", got, ok)
	}
	if _, ok := classifySnapshotEntityFile("models/SIM.19/placements/notes.txt"); ok {
		t.Fatal("non-yaml file classified")
	}
}
