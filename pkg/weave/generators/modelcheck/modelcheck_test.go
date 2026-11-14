package modelcheck

import (
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

func el(kind, q string) domain.PathElement {
	p, l, _ := strings.Cut(q, ":")
	return domain.PathElement{Type: kind, URI: q, Prefix: p, LocalName: l}
}

func fn(sem, typ string, min int, max *int, path ...domain.PathElement) generators.FieldNode {
	nodes := make([]generators.PathNode, len(path))
	for i, e := range path {
		nodes[i] = generators.PathNode{Element: e}
	}
	return generators.FieldNode{Field: domain.ResolvedField{SemanticID: sem, ExpectedValueType: typ, MinOccurs: min, MaxOccurs: max, PathElements: path}, Path: nodes}
}

func snapOf(fields ...generators.FieldNode) *generators.Snapshot {
	return &generators.Snapshot{Model: &domain.Model{Entity: domain.Entity{SemanticID: "PM.1"}, OntologyScope: el("class", "crm:E7_Activity")}, Fields: fields}
}

// codes lists code:field for each finding, sorted, leaving out C13 notes
// so tests of other checks need no identity field.
func codes(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		if f.Code == "C13" {
			continue
		}
		out = append(out, f.Code+":"+f.Field)
	}
	sort.Strings(out)
	return out
}

func intp(n int) *int { return &n }

func TestC3MaxBelowMin(t *testing.T) {
	got := codes(Model(snapOf(fn("PF.1", "String", 2, intp(1), el("property", "rdfs:label"), el("literal", "xsd:string")))))
	if diff := cmp.Diff([]string{"C3:PF.1"}, got); diff != "" {
		t.Fatal(diff)
	}
}

func TestC3CollectionPlacement(t *testing.T) {
	f := fn("PF.1", "String", 0, nil, el("property", "crm:P1_is_identified_by"), el("class", "crm:E41_Appellation"), el("property", "crm:P190_has_symbolic_content"), el("literal", "xsd:string"))
	f.CollectionID, f.CollectionSemanticID = "c", "PC.1"
	f.CollectionIsRequired, f.CollectionMaxOccurs = true, intp(0)
	g := f
	g.Field.SemanticID = "PF.2"
	var c3 []Finding
	for _, x := range Model(snapOf(f, g)) {
		if x.Code == "C3" {
			c3 = append(c3, x)
		}
	}
	if len(c3) != 1 || c3[0].Collection != "PC.1" || c3[0].Field != "" {
		t.Fatalf("want one C3 for the placement: %+v", c3)
	}
}

func TestC4(t *testing.T) {
	fs := Model(snapOf(
		fn("PF.1", "String", 0, nil, el("property", "crm:P14_carried_out_by"), el("class", "crm:E39_Actor")),
		fn("PF.2", "Concept", 0, nil, el("property", "crm:P3_has_note"), el("literal", "xsd:string")),
		fn("PF.3", "URI", 0, nil, el("property", "crm:P138i_has_representation"), el("class", "crm:E36_Visual_Item")),
	))
	if diff := cmp.Diff([]string{"C10:PF.2", "C4:PF.1", "C4:PF.2"}, codes(fs)); diff != "" {
		t.Fatal(diff)
	}
}

func TestC4TreatsXSDClassAsLiteral(t *testing.T) {
	fs := Model(snapOf(fn("PF.1", "Date", 0, nil, el("property", "crm:P82a_begin_of_the_begin"), el("class", "xsd:date"))))
	if got := codes(fs); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}

func TestC9C10(t *testing.T) {
	fs := Model(snapOf(
		fn("PF.1", "Model", 0, nil, el("property", "crm:P14_carried_out_by"), el("class", "crm:E39_Actor")),
		fn("PF.2", "Collection", 0, nil, el("property", "crm:P1_is_identified_by"), el("class", "crm:E41_Appellation")),
		fn("PF.3", "Concept", 0, nil, el("property", "crm:P2_has_type"), el("class", "crm:E55_Type")),
	))
	if diff := cmp.Diff([]string{"C10:PF.3", "C9:PF.1", "C9:PF.2"}, codes(fs)); diff != "" {
		t.Fatal(diff)
	}
}

func TestReferencedMembersAreChecked(t *testing.T) {
	snap := snapOf()
	snap.Referenced = map[string]generators.ReferencedCollection{"c": {
		Collection: &domain.Collection{Entity: domain.Entity{ID: "c", SemanticID: "PC.2"}},
		Fields:     []domain.ResolvedField{{SemanticID: "PF.9", ExpectedValueType: "String", MinOccurs: 3, MaxOccurs: intp(1), PathElements: []domain.PathElement{el("property", "rdfs:label"), el("literal", "xsd:string")}}},
	}}
	fs := Model(snap)
	if len(fs) == 0 || fs[0].Code != "C3" || fs[0].Collection != "PC.2" || fs[0].Field != "PF.9" {
		t.Fatalf("%+v", fs)
	}
}

// a collection placed on the model and also named by a stub is checked
// twice; its findings are reported once
func TestFindingsAreReportedOnce(t *testing.T) {
	f := fn("PF.1", "Concept", 0, nil, el("property", "crm:P2_has_type"), el("class", "crm:E55_Type"))
	f.CollectionID, f.CollectionSemanticID = "c", "PC.1"
	snap := snapOf(f)
	snap.Referenced = map[string]generators.ReferencedCollection{"c": {
		Collection: &domain.Collection{Entity: domain.Entity{ID: "c", SemanticID: "PC.1"}},
		Fields:     []domain.ResolvedField{f.Field},
	}}
	if diff := cmp.Diff([]string{"C10:PF.1"}, codes(Model(snap))); diff != "" {
		t.Fatal(diff)
	}
}
