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
	norm   func(string) string // set value → comparable form (CURIEs expanded)
	order  []groupKey
	groups map[groupKey][]string          // child node keys, first seen first
	class  map[string]string              // node key → class
	ending map[string][]ending            // node key → each field ending there
	fields map[string][]string            // node key → fields reaching it, for messages
	values map[string]map[string][]string // node key → property → required fixed values one step below
}

// ending is a field whose path ends at a node: its own set value, and for a
// Collection stub the fixed values its collection adds.
type ending struct {
	self  string
	extra map[string][]string
	iri   bool   // a URI/File field: checked as an IRI, not by class
	field string // its semantic id
}

type groupKey struct{ parent, prop string }

func newBranchTree(norm func(string) string) *branchTree {
	return &branchTree{
		norm:   norm,
		groups: map[groupKey][]string{},
		class:  map[string]string{},
		ending: map[string][]ending{},
		fields: map[string][]string{},
		values: map[string]map[string][]string{},
	}
}

// add walks one field's path from the root node (""). extra are the fixed
// values a Collection stub's collection adds to the stub's branch.
func (t *branchTree) add(path []domain.PathElement, f domain.ResolvedField, extra map[string][]string) {
	required := effMin(f.IsRequired, f.MinOccurs) >= 1
	set := strings.TrimSpace(f.SetValue)
	if set != "" {
		set = t.norm(set)
	}
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
				name := next.PrefixedName()
				if name == "" {
					name = next.URI
				}
				key = parent + "/" + prop + "/" + name
			}
			gk := groupKey{parent, prop}
			if _, seen := t.groups[gk]; !seen {
				t.order = append(t.order, gk)
			}
			if !slices.Contains(t.groups[gk], key) {
				t.groups[gk] = append(t.groups[gk], key)
				t.class[key] = next.PrefixedName()
			}
			if !slices.Contains(t.fields[key], f.SemanticID) {
				t.fields[key] = append(t.fields[key], f.SemanticID)
			}
			if j+1 == len(path)-1 {
				t.ending[key] = append(t.ending[key], ending{self: set, extra: extra, iri: iriValue(f), field: f.SemanticID})
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

type branch struct {
	node string
	ending
}

// branches lists a group's branches: one per child node, or one per field
// ending on it when several do.
func (t *branchTree) branches(gk groupKey) []branch {
	var out []branch
	for _, n := range t.groups[gk] {
		if len(t.ending[n]) < 2 {
			var e ending
			if len(t.ending[n]) == 1 {
				e = t.ending[n][0]
			}
			out = append(out, branch{n, e})
			continue
		}
		for _, e := range t.ending[n] {
			out = append(out, branch{n, e})
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
	return valuesApart(mergeValues(t.values[a.node], a.extra), mergeValues(t.values[b.node], b.extra))
}

func mergeValues(a, b map[string][]string) map[string][]string {
	out := map[string][]string{}
	for _, m := range []map[string][]string{a, b} {
		for p, vs := range m {
			for _, v := range vs {
				if !slices.Contains(out[p], v) {
					out[p] = append(out[p], v)
				}
			}
		}
	}
	return out
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
// that are not apart, once per (group, class). URI/File branches are
// checked as IRIs, so they never count as class branches; nor do classes
// without a name. where names the model or collection for the message.
func (c *checker) sameClassTwins(t *branchTree, where, collection string) {
	for _, gk := range t.order {
		bs := t.branches(gk)
		reported := map[string]bool{}
		for i := range bs {
			for j := i + 1; j < len(bs); j++ {
				cls := t.class[bs[i].node]
				if cls == "" || cls != t.class[bs[j].node] || reported[cls] || bs[i].iri || bs[j].iri || t.apart(bs[i], bs[j]) {
					continue
				}
				reported[cls] = true
				c.add("C6", Error, collection, "", "%s: %s %s has %s branches (%s, %s) that no required fixed value tells apart",
					where, t.place(gk.parent), gk.prop, cls, t.label(bs[i]), t.label(bs[j]))
			}
		}
	}
}

// place says where a group sits: at the root, or under a class node.
func (t *branchTree) place(parent string) string {
	if parent == "" {
		return "at the root,"
	}
	return "under " + t.class[parent] + ","
}

// label names a branch by the field ending there, else the fields reaching
// its node.
func (t *branchTree) label(b branch) string {
	if b.field != "" {
		return b.field
	}
	return strings.Join(t.fields[b.node], "+")
}

func iriValue(f domain.ResolvedField) bool {
	switch valueType(f) {
	case "uri", "url", "file/bitstream", "file list":
		return true
	}
	return false
}
