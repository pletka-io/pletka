package exportgraph

import (
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/rdfterm"
)

// Identity is what tells one branch's values from another's: the class (or,
// for a URI branch, being an IRI), the value itself when the step has a set
// value, and the fixed values of required one-step set-value fields. It is
// data only; renderers write their own constraints from it.
type Identity struct {
	// Class is the class list of the branch class; empty for a literal or
	// IRI-only branch.
	Class string `json:"class,omitempty"`
	// IRI marks a URI / File branch: any IRI, whatever its class.
	IRI bool `json:"iri,omitempty"`
	// Self is the term of the step's own set value.
	Self string `json:"self,omitempty"`
	// Values maps a property's prefixed name to the terms of its required
	// set values.
	Values map[string][]string `json:"values,omitempty"`
	// Lineage maps each of the class's types to its ancestors; nil when any
	// type's ancestors are unknown.
	Lineage map[string][]string `json:"lineage,omitempty"`
}

// NodeIdentity reads n's identity: its class and the fixed values of every
// required set-value field one step from it. A literal counts when its Min
// is at least 1; an edge ending counts when its own count is (the first
// ending takes the edge's Min, later ones their own).
func NodeIdentity(n *ResourceNode, g *Graph, ns generators.NamespaceSet) Identity {
	id := Identity{Class: rdfterm.ClassList(n.Class), Values: map[string][]string{}}
	add := func(prop, setValue string) {
		v := strings.TrimSpace(setValue)
		if v == "" {
			return
		}
		p, t := prop, rdfterm.Term(v, ns)
		if !slices.Contains(id.Values[p], t) {
			id.Values[p] = append(id.Values[p], t)
		}
	}
	for _, l := range n.Literals {
		if l.Min >= 1 && l.Field.Source != nil {
			add(l.Predicate.PrefixedName(), l.Field.Source.Field.SetValue)
		}
	}
	for _, e := range n.Edges {
		for i, end := range e.Endings {
			min := end.Min
			if i == 0 {
				min = e.Min
			}
			if min < 1 {
				continue
			}
			if f := endingField(g, e.TargetKey, end.FieldID); f != nil {
				add(e.Predicate.PrefixedName(), f.Field.SetValue)
			}
		}
	}
	if len(id.Values) == 0 {
		id.Values = nil
	}
	return id
}

// endingField finds the field that ends on the node with key.
func endingField(g *Graph, key, fieldID string) *generators.FieldNode {
	for _, b := range g.Fields {
		if b.NodeKey == key && b.ID == fieldID {
			return b.Source
		}
	}
	return nil
}

// Distinguishable reports whether no value can match both identities: their
// own set values differ, or they fix one property to sets of values that
// share nothing. Classes alone never tell branches apart (a subclass also
// matches its superclass branch) unless both carry a lineage.
func Distinguishable(a, b Identity) bool {
	if a.Self != "" && b.Self != "" && a.Self != b.Self {
		return true
	}
	if classesUnrelated(a, b) {
		return true
	}
	for p, av := range a.Values {
		bv := b.Values[p]
		if len(av) == 0 || len(bv) == 0 {
			continue
		}
		shared := false
		for _, v := range av {
			if slices.Contains(bv, v) {
				shared = true
				break
			}
		}
		if !shared {
			return true
		}
	}
	return false
}

// classesUnrelated reports two classes neither of which is, or descends
// from, the other, so no single-typed value matches both. It needs both
// lineages; without them classes never tell branches apart.
// ponytail: a value typed with both classes still matches both branches.
func classesUnrelated(a, b Identity) bool {
	if a.Class == "" || b.Class == "" || a.Lineage == nil || b.Lineage == nil {
		return false
	}
	for ta, ancA := range a.Lineage {
		for tb, ancB := range b.Lineage {
			if ta == tb || slices.Contains(ancA, tb) || slices.Contains(ancB, ta) {
				return false
			}
		}
	}
	return true
}

// WithLineage sets the identity's lineage from the class ancestors, when
// every type of its class is known.
func (id Identity) WithLineage(ancestors map[string][]string) Identity {
	if id.Class == "" || ancestors == nil {
		return id
	}
	lin := map[string][]string{}
	for _, t := range strings.Split(id.Class, ", ") {
		anc, ok := ancestors[t]
		if !ok {
			return id
		}
		lin[t] = anc
	}
	id.Lineage = lin
	return id
}
