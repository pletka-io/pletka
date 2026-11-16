package exportgraph

import (
	"sort"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

// BuildMembers builds the graph of a referenced collection as it sits under
// a stub at addr. Its root is the collection's anchor class (the last class
// of the shared path prefix, else the collection's ontology scope); members
// are placed with that prefix stripped, hidden members are dropped, and the
// graft overrides below addr (hidden, max occurs, display name) are applied.
// Each member field's Address is addr + "/" + its semantic id.
func BuildMembers(rc generators.ReferencedCollection, addr string, grafts map[string]domain.GraftOverride) *Graph {
	var coll domain.Collection
	if rc.Collection != nil {
		coll = *rc.Collection
	}
	anchor := coll.OntologyScope
	for _, e := range rc.SharedPathPrefix {
		if e.Type == elemClass {
			anchor = e
		}
	}
	coll.OntologyScope = anchor

	fields := make([]generators.FieldNode, 0, len(rc.Fields))
	for _, rf := range rc.Fields {
		if rf.IsHidden {
			continue
		}
		f := generators.FieldNode{Field: rf, Address: addr + "/" + rf.SemanticID}
		if g, ok := grafts[f.Address]; ok {
			if g.IsHidden {
				continue
			}
			if g.MaxOccursSet {
				f.Field.MaxOccurs = g.MaxOccurs
			}
			if len(g.DisplayName) > 0 {
				f.Field.DisplayName = g.DisplayName
			}
		}
		elems := stripPrefix(f.Field.PathElements, rc.SharedPathPrefix)
		f.Path = make([]generators.PathNode, len(elems))
		for i, el := range elems {
			f.Path[i] = generators.PathNode{Element: el, Index: i}
		}
		fields = append(fields, f)
	}
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].Field.Position < fields[j].Field.Position })

	g := Build(&generators.Snapshot{RootKind: generators.EntityCollection, Collection: &coll, Fields: fields})
	if rc.Collection != nil {
		if root := g.nodeByKey(g.RootKey); root != nil {
			root.Collection = &CollectionAnchor{SemanticID: coll.SemanticID, Label: coll.UIName, Address: addr}
		}
	}
	return g
}

func (g *Graph) nodeByKey(key string) *ResourceNode {
	for _, n := range g.Nodes {
		if n.Key == key {
			return n
		}
	}
	return nil
}

// stripPrefix drops the graft prefix a referenced collection's members share.
func stripPrefix(path, prefix []domain.PathElement) []domain.PathElement {
	if len(prefix) == 0 || len(path) <= len(prefix) {
		return path
	}
	for i := range prefix {
		if !path[i].SameStep(prefix[i]) {
			return path
		}
	}
	return path[len(prefix):]
}
