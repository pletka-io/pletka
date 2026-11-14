package modelcheck

import (
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/views"
)

// entryChecks reports C7: a placed collection whose fields share no entry
// property (the tree gives such a collection no anchor).
func (c *checker) entryChecks(tree *views.Tree) {
	if tree == nil {
		return
	}
	var walk func(views.TreeNode)
	walk = func(n views.TreeNode) {
		if n.Kind == views.NodeCollection && n.Anchor == nil && len(n.Children) > 1 {
			c.add("C7", Error, n.SemanticID, "", "fields of %s share no entry property", n.SemanticID)
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(tree.Root)
}

// refChecks reports C8: a Collection field naming a collection that did not
// resolve. Only meaningful on snapshots that include referenced
// collections; without them nothing is reported.
func (c *checker) refChecks(snap *generators.Snapshot) {
	if snap.Referenced == nil {
		return
	}
	for _, f := range snap.Fields {
		c.unresolvedRefs(snap, f.Field, f.CollectionSemanticID)
	}
}

// unresolvedRefs reports C8 for one field's collection_model refs.
func (c *checker) unresolvedRefs(snap *generators.Snapshot, f domain.ResolvedField, collection string) {
	if snap.Referenced == nil {
		return
	}
	ids, names := refIDs(f.CollectionModels)
	for i, id := range ids {
		rc, ok := snap.Referenced[id]
		switch {
		case !ok:
			c.add("C8", Error, collection, f.SemanticID, "%s: collection_model %s not loaded", f.SemanticID, names[i])
		case rc.Missing != "":
			c.add("C8", Error, collection, f.SemanticID, "%s: collection_model %s %s", f.SemanticID, names[i], rc.Missing)
		}
	}
}
