package modelcheck

import (
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// branchTree groups the class nodes field paths reach under each
// (parent node, property), the same way the SHACL renderer builds its
// shapes: nodes merge on their PathNode chain, or on property and class
// where elements carry none (referenced collection members).
type branchTree struct {
	order  []groupKey
	groups map[groupKey][]string          // child node keys, first seen first
	class  map[string]string              // node key → class
	ending map[string][]string            // node key → own set value of each field ending there ("" = none)
	values map[string]map[string][]string // node key → property → required fixed values one step below
}

type groupKey struct{ parent, prop string }

func newBranchTree() *branchTree {
	return &branchTree{
		groups: map[groupKey][]string{},
		class:  map[string]string{},
		ending: map[string][]string{},
		values: map[string]map[string][]string{},
	}
}

// add walks one field's path from the root node ("").
func (t *branchTree) add(path []domain.PathElement, f domain.ResolvedField) {
	required := effMin(f.IsRequired, f.MinOccurs) >= 1
	set := strings.TrimSpace(f.SetValue)
	parent := ""
	for j := 0; j < len(path); j++ {
		p := path[j]
		if p.Type != "property" {
			continue
		}
		prop := p.PrefixedName()
		if j+1 < len(path) && classEnd(path[j+1]) {
			next := path[j+1]
			key := next.PathNode
			if key == "" {
				key = parent + "/" + prop + "/" + next.PrefixedName()
			}
			gk := groupKey{parent, prop}
			if _, seen := t.groups[gk]; !seen {
				t.order = append(t.order, gk)
			}
			if !slices.Contains(t.groups[gk], key) {
				t.groups[gk] = append(t.groups[gk], key)
				t.class[key] = next.PrefixedName()
			}
			if j+1 == len(path)-1 {
				t.ending[key] = append(t.ending[key], set)
				if required && set != "" {
					t.addValue(parent, prop, set)
				}
				return
			}
			parent = key
			j++
			continue
		}
		if required && set != "" {
			t.addValue(parent, prop, set)
		}
		return
	}
}

func (t *branchTree) addValue(node, prop, v string) {
	if t.values[node] == nil {
		t.values[node] = map[string][]string{}
	}
	if !slices.Contains(t.values[node][prop], v) {
		t.values[node][prop] = append(t.values[node][prop], v)
	}
}

type branch struct{ node, self string }

// branches lists a group's branches: one per child node, or one per field
// ending on it when several do.
func (t *branchTree) branches(gk groupKey) []branch {
	var out []branch
	for _, n := range t.groups[gk] {
		if len(t.ending[n]) < 2 {
			self := ""
			if len(t.ending[n]) == 1 {
				self = t.ending[n][0]
			}
			out = append(out, branch{n, self})
			continue
		}
		for _, s := range t.ending[n] {
			out = append(out, branch{n, s})
		}
	}
	return out
}

// apart reports whether no value can match both branches: their own set
// values differ, or they fix a property to value sets that share nothing.
// Classes alone never tell branches apart (rdfs:subClassOf).
func (t *branchTree) apart(a, b branch) bool {
	if a.self != "" && b.self != "" && a.self != b.self {
		return true
	}
	return a.node != b.node && valuesApart(t.values[a.node], t.values[b.node])
}

func valuesApart(a, b map[string][]string) bool {
	for p, av := range a {
		bv := b[p]
		if len(av) == 0 || len(bv) == 0 {
			continue
		}
		if !slices.ContainsFunc(av, func(v string) bool { return slices.Contains(bv, v) }) {
			return true
		}
	}
	return false
}

// sameClassTwins reports C6 for each group with two same-class branches
// that are not apart, once per (group, class). where names the model or
// collection for the message.
func (c *checker) sameClassTwins(t *branchTree, where, collection string) {
	for _, gk := range t.order {
		bs := t.branches(gk)
		reported := map[string]bool{}
		for i := range bs {
			for j := i + 1; j < len(bs); j++ {
				cls := t.class[bs[i].node]
				if cls != t.class[bs[j].node] || reported[cls] || t.apart(bs[i], bs[j]) {
					continue
				}
				reported[cls] = true
				c.add("C6", Error, collection, "", "%s: %s has %s branches that no required fixed value tells apart", where, gk.prop, cls)
			}
		}
	}
}
