package modelcheck

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

// typedCollection is a referenced collection anchored at E33 via P67i whose
// one required member fixes P2 to v.
func typedCollection(id, sem, v string) generators.ReferencedCollection {
	return generators.ReferencedCollection{
		Collection:       &domain.Collection{Entity: domain.Entity{ID: id, SemanticID: sem}},
		SharedPathPrefix: []domain.PathElement{p67, e33},
		Fields: []domain.ResolvedField{{SemanticID: sem + ".t", ExpectedValueType: "Concept", MinOccurs: 1, SetValue: v,
			ConceptLists: []domain.EntityRef{{ID: "l"}},
			PathElements: []domain.PathElement{p67, e33, p2, e55}}},
	}
}

func stubTo(sem, chain, collID string) generators.FieldNode {
	f := chained(sem, "Collection", 0, "", p67, e33)
	f.Path[1].Element.PathNode = chain
	f.Field.CollectionModels = []domain.EntityRef{{ID: collID, SemanticID: collID}}
	return f
}

func TestC6StubsApartByTheirCollections(t *testing.T) {
	snap := snapOf(stubTo("PF.1", "a", "c1"), stubTo("PF.2", "b", "c2"))
	snap.Referenced = map[string]generators.ReferencedCollection{
		"c1": typedCollection("c1", "PC.1", "aat:1"),
		"c2": typedCollection("c2", "PC.2", "aat:2"),
	}
	if got := c6(Model(snap)); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestSetValuesCompareAsIRIs(t *testing.T) {
	snap := twoBranches([]string{"aat:1"}, []string{"http://vocab.getty.edu/aat/1"})
	snap.Namespaces = generators.NamespaceSet{ByPrefix: map[string]string{"aat": "http://vocab.getty.edu/aat/"},
		Bindings: []generators.NamespaceBinding{{Prefix: "aat", Namespace: "http://vocab.getty.edu/aat/"}}}
	if got := c6(Model(snap)); len(got) != 1 {
		t.Fatalf("the same type written two ways tells nothing apart: %+v", got)
	}
}

func TestCollectionFindingsAreReportedOncePerProject(t *testing.T) {
	shared := generators.ReferencedCollection{
		Collection: &domain.Collection{Entity: domain.Entity{ID: "c", SemanticID: "PC.1"}},
		Fields:     []domain.ResolvedField{{SemanticID: "PF.9", ExpectedValueType: "Concept", PathElements: []domain.PathElement{p2, e55}}},
	}
	a, b := model("PM.1", "crm:E7_Activity", "aat:1"), model("PM.2", "crm:E21_Person")
	a.Referenced = map[string]generators.ReferencedCollection{"c": shared}
	b.Referenced = map[string]generators.ReferencedCollection{"c": shared}
	got := byCode(Project([]*generators.Snapshot{a, b}), "C10")
	var fromCollection []Finding
	for _, f := range got {
		if f.Field == "PF.9" {
			fromCollection = append(fromCollection, f)
		}
	}
	if len(fromCollection) != 1 || fromCollection[0].Model != "" || fromCollection[0].Collection != "PC.1" {
		t.Fatalf("%+v", fromCollection)
	}
}

func TestC8InsideReferencedCollection(t *testing.T) {
	snap := snapOf()
	snap.Referenced = map[string]generators.ReferencedCollection{"c": {
		Collection: &domain.Collection{Entity: domain.Entity{ID: "c", SemanticID: "PC.1"}},
		Fields: []domain.ResolvedField{{SemanticID: "PF.12", ExpectedValueType: "Collection",
			CollectionModels: []domain.EntityRef{{ID: "gone", SemanticID: "PC.GONE"}},
			PathElements:     []domain.PathElement{p67, e33}}},
	}}
	got := byCode(Model(snap), "C8")
	if len(got) != 1 || got[0].Collection != "PC.1" || !strings.Contains(got[0].Message, "PC.GONE not loaded") {
		t.Fatalf("%+v", got)
	}
}

func TestSnapshotErrorsAreFindings(t *testing.T) {
	snap := snapOf()
	snap.Report.Errors = []generators.Diagnostic{{Code: "legacy_inverse_path", Message: "inverse step ^P1"}}
	got := byCode(Model(snap), "snapshot")
	if len(got) != 1 || !strings.Contains(got[0].Message, "legacy_inverse_path") {
		t.Fatalf("%+v", got)
	}
}

func TestC5OneFindingPerClass(t *testing.T) {
	fs := Project([]*generators.Snapshot{model("PM.1", "crm:E7_Activity"), model("PM.2", "crm:E7_Activity"), model("PM.3", "crm:E7_Activity")})
	got := byCode(fs, "C5")
	if len(got) != 1 || !strings.Contains(got[0].Message, "PM.1, PM.2, PM.3") {
		t.Fatalf("%+v", got)
	}
}

func TestClassKeyIgnoresTypeOrder(t *testing.T) {
	a, b := el("class", "crmdig:D1_Digital_Object"), el("class", "crmdig:D1_Digital_Object")
	e36 := domain.TypeRef{URI: "crm:E36_Visual_Item", Prefix: "crm", LocalName: "E36_Visual_Item"}
	e73 := domain.TypeRef{URI: "crm:E73_Information_Object", Prefix: "crm", LocalName: "E73_Information_Object"}
	a.AdditionalTypes = []domain.TypeRef{e36, e73}
	b.AdditionalTypes = []domain.TypeRef{e73, e36}
	if classKey(a) != classKey(b) || classKey(a) != "crmdig:D1_Digital_Object, crm:E36_Visual_Item, crm:E73_Information_Object" {
		t.Fatalf("%q vs %q", classKey(a), classKey(b))
	}
}
