package generators

import (
	"context"
	"testing"
)

// fakeLineage returns E1 as every class's only ancestor, plus the class itself
// (as the ontology walk does), and records the classes asked for.
type fakeLineage struct{ asked map[string]bool }

func (f *fakeLineage) ClassAncestors(_ context.Context, _ []string, qname string) ([]string, error) {
	f.asked[qname] = true
	if qname == "crm:E1_CRM_Entity" {
		return nil, nil
	}
	return []string{"crm:E1_CRM_Entity"}, nil
}

func TestSnapshotCarriesClassAncestors(t *testing.T) {
	svc := serviceWithNestedModel()
	lin := &fakeLineage{asked: map[string]bool{}}
	svc.SetClassLineageReader(lin)
	snap, err := svc.SnapshotForModel(context.Background(), "OG", "m-1", Options{IncludeReferenced: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.ClassAncestors) == 0 {
		t.Fatal("no class ancestors on the snapshot")
	}
	for cls, anc := range snap.ClassAncestors {
		if len(anc) != 1 || anc[0] != "crm:E1_CRM_Entity" {
			t.Fatalf("%s ancestors = %v", cls, anc)
		}
	}
	// every class a member of a referenced collection uses is covered
	for _, rc := range snap.Referenced {
		for _, f := range rc.Fields {
			for _, e := range f.PathElements {
				if e.Type == "class" && e.LocalName != "" {
					if _, ok := snap.ClassAncestors[e.Prefix+":"+e.LocalName]; !ok {
						t.Fatalf("referenced class %s missing", e.Prefix+":"+e.LocalName)
					}
				}
			}
		}
	}
}

func TestSnapshotWithoutLineageHasNoAncestors(t *testing.T) {
	snap, err := serviceWithNestedModel().SnapshotForModel(context.Background(), "OG", "m-1", Options{IncludeReferenced: true})
	if err != nil {
		t.Fatal(err)
	}
	if snap.ClassAncestors != nil {
		t.Fatalf("ancestors without a reader: %v", snap.ClassAncestors)
	}
	svc := serviceWithNestedModel()
	svc.SetClassLineageReader(&fakeLineage{asked: map[string]bool{}})
	snap, err = svc.SnapshotForModel(context.Background(), "OG", "m-1", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if snap.ClassAncestors != nil {
		t.Fatalf("ancestors without IncludeReferenced: %v", snap.ClassAncestors)
	}
}
