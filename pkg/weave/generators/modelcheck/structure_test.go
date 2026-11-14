package modelcheck

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/views"
)

func collTree(anchor *domain.PathElement, children int) *views.Tree {
	coll := views.TreeNode{Kind: views.NodeCollection, SemanticID: "PC.1", Anchor: anchor}
	for range children {
		coll.Children = append(coll.Children, views.TreeNode{Kind: views.NodeField})
	}
	return &views.Tree{Root: views.TreeNode{Children: []views.TreeNode{{Kind: views.NodeCategory, Children: []views.TreeNode{coll}}}}}
}

func TestC7NoEntryProperty(t *testing.T) {
	snap := snapOf()
	snap.Tree = collTree(nil, 2)
	if got := codes(Model(snap)); len(got) != 1 || got[0] != "C7:" {
		t.Fatalf("%v", got)
	}
	anchor := el("class", "crm:E41_Appellation")
	snap.Tree = collTree(&anchor, 2)
	if got := codes(Model(snap)); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	snap.Tree = collTree(nil, 1) // one field: nothing to share
	if got := codes(Model(snap)); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}

func TestC8UnresolvedCollectionRef(t *testing.T) {
	stub := fn("PF.1", "Collection", 0, nil, el("property", "crm:P1_is_identified_by"), el("class", "crm:E41_Appellation"))
	stub.Field.CollectionModels = []domain.EntityRef{{ID: "c9", SemanticID: "PC.9"}, {ID: "c8", SemanticID: "PC.8"}}
	snap := snapOf(stub)
	snap.Referenced = map[string]generators.ReferencedCollection{"c9": {Missing: "not found"}}
	var msgs []string
	for _, f := range Model(snap) {
		if f.Code == "C8" {
			msgs = append(msgs, f.Message)
		}
	}
	if len(msgs) != 2 || !strings.Contains(msgs[0], "PC.9 not found") || !strings.Contains(msgs[1], "PC.8 not loaded") {
		t.Fatalf("%v", msgs)
	}
}
