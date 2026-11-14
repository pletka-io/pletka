package modelcheck

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

// URI/File fields are checked as IRIs, whatever class their path ends at:
// two of them under one property are not two same-class branches.
func TestC6IgnoresURIBranches(t *testing.T) {
	snap := snapOf(chained("PF.1", "URI", 0, "", p67, e33), chained("PF.2", "URI", 0, "", p67, e33))
	snap.Fields[1].Path[1].Element.PathNode = "other"
	if got := c6(Model(snap)); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestC6IgnoresMembersHiddenByGrafts(t *testing.T) {
	member := func(sem string) domain.ResolvedField {
		return domain.ResolvedField{SemanticID: sem, ExpectedValueType: "Model", PathElements: []domain.PathElement{p67, e33}}
	}
	snap := snapOf()
	snap.Referenced = map[string]generators.ReferencedCollection{"c": {
		Collection: &domain.Collection{Entity: domain.Entity{ID: "c", SemanticID: "PC.3"}},
		Fields:     []domain.ResolvedField{member("PF.5"), member("PF.6")},
	}}
	snap.Grafts = map[string]domain.GraftOverride{"PF.1/PF.6": {Address: "PF.1/PF.6", IsHidden: true}}
	if got := c6(Model(snap)); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestC6MessageNamesFieldsAndParent(t *testing.T) {
	got := c6(Model(twoBranches(nil, nil)))
	if len(got) != 1 || !strings.Contains(got[0].Message, "PF.1") || !strings.Contains(got[0].Message, "PF.2") || !strings.Contains(got[0].Message, "at the root") {
		t.Fatalf("%+v", got)
	}
}

// the same property and class under two different parents are two
// findings, not one
func TestC6KeepsGroupsUnderDifferentParentsApart(t *testing.T) {
	snap := snapOf()
	for _, parent := range []string{"x", "y"} {
		p4, e52 := el("property", "crm:P4_has_time-span"), el("class", "crm:E52_Time-Span")
		if parent == "y" {
			p4 = el("property", "crm:P9_consists_of")
		}
		for i, sem := range []string{"PF." + parent + "1", "PF." + parent + "2"} {
			f := chained(sem, "Model", 0, "", p4, e52, p67, e33)
			f.Path[3].Element.PathNode = parent + string(rune('a'+i))
			snap.Fields = append(snap.Fields, f)
		}
	}
	if got := c6(Model(snap)); len(got) != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestC6SkipsClassesWithoutAName(t *testing.T) {
	anon := domain.PathElement{Type: "class", URI: "http://example.org/Thing"}
	snap := snapOf(chained("PF.1", "Model", 0, "", p67, anon), chained("PF.2", "Model", 0, "", p67, anon))
	snap.Fields[1].Path[1].Element.PathNode = "other"
	if got := c6(Model(snap)); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}
