package autocomplete

import (
	"sort"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TermDescription is the flat, serialization-safe projection of a Node.
// It carries only strings/slices/Translations (no *Node) so it can cross a
// package boundary — or a jsonschema inference pass — without exposing the
// pointer graph.
type TermDescription struct {
	Qname           string              `json:"qname"`
	URI             string              `json:"uri"`
	Type            string              `json:"type"` // "class" | "property"
	Label           domain.Translations `json:"label,omitempty"`
	Comment         domain.Translations `json:"comment,omitempty"` // the scope note
	PropertyType    string              `json:"property_type,omitempty"`
	Superclasses    []string            `json:"superclasses,omitempty"`
	Subclasses      []string            `json:"subclasses,omitempty"`
	AsDomainOf      []string            `json:"as_domain_of,omitempty"` // properties declared directly on this class
	AsRangeOf       []string            `json:"as_range_of,omitempty"`
	Domains         []string            `json:"domains,omitempty"`
	Ranges          []string            `json:"ranges,omitempty"`
	SuperProperties []string            `json:"super_properties,omitempty"`
	// InheritedProperties are properties a class gains from its superclass
	// lineage (ancestors' AsDomainOf), excluding those already in AsDomainOf.
	// Empty for property terms.
	InheritedProperties []string `json:"inherited_properties,omitempty"`
	// Labels maps every qname referenced in the edge slices above (and this
	// term itself) to its label, so a consumer can render related terms by
	// name instead of an opaque qname (e.g. obo:BFO_0000001 -> "entity").
	// Only qnames with a non-empty label appear.
	Labels   map[string]domain.Translations `json:"labels,omitempty"`
	Ontology string                         `json:"ontology"` // Meta.Prefix
	Version  string                         `json:"version"`  // Meta.VersionString
}

// DescribeNode flattens a Node into a TermDescription (nil-safe: nil -> nil).
// Edge slices carry qnames, sorted ascending for determinism. For class nodes
// it also derives InheritedProperties from the superclass lineage and a Labels
// map so related terms can be rendered by name.
func DescribeNode(n *Node) *TermDescription {
	if n == nil {
		return nil
	}
	inherited := inheritedProperties(n)
	td := &TermDescription{
		Qname:               n.Qname,
		URI:                 n.URI,
		Type:                n.Type,
		Label:               n.Label,
		Comment:             n.Comment,
		PropertyType:        n.PropertyType,
		Superclasses:        edgeQnames(n.Superclasses),
		Subclasses:          edgeQnames(n.Subclasses),
		AsDomainOf:          edgeQnames(n.AsDomainOf),
		AsRangeOf:           edgeQnames(n.AsRangeOf),
		Domains:             edgeQnames(n.Domains),
		Ranges:              edgeQnames(n.Ranges),
		SuperProperties:     edgeQnames(n.SuperProps),
		InheritedProperties: edgeQnames(inherited),
		Ontology:            n.Meta.Prefix,
		Version:             n.Meta.VersionString,
	}
	td.Labels = collectLabels(n, inherited)
	return td
}

// inheritedProperties returns the property nodes a class gains from its
// superclass lineage (each ancestor's AsDomainOf), excluding properties
// declared directly on the class and any ghost. Returns nil for a property
// node. Mirrors how IndexedEngine.suggestPropertiesForClass unions the lineage.
func inheritedProperties(n *Node) []*Node {
	if n.Type != "class" {
		return nil
	}
	direct := map[*Node]struct{}{}
	for _, p := range n.AsDomainOf {
		direct[p] = struct{}{}
	}
	seen := map[*Node]struct{}{}
	out := make([]*Node, 0)
	for _, anc := range closure(n, func(x *Node) []*Node { return x.Superclasses }, ClosureOpts{}) {
		for _, p := range anc.AsDomainOf {
			if p.Ghost {
				continue
			}
			if _, isDirect := direct[p]; isDirect {
				continue
			}
			if _, dup := seen[p]; dup {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}

// collectLabels gathers the label of the node and every node it references
// through its edge slices (plus the supplied inherited set), keyed by qname.
// Only non-empty labels are included.
func collectLabels(n *Node, inherited []*Node) map[string]domain.Translations {
	labels := map[string]domain.Translations{}
	add := func(m *Node) {
		if m == nil || m.Ghost || m.Qname == "" || len(m.Label) == 0 {
			return
		}
		labels[m.Qname] = m.Label
	}
	add(n)
	for _, group := range [][]*Node{
		n.Superclasses, n.Subclasses, n.AsDomainOf, n.AsRangeOf,
		n.Domains, n.Ranges, n.SuperProps, inherited,
	} {
		for _, m := range group {
			add(m)
		}
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}

// edgeQnames extracts the Qname of every node in nodes, sorted ascending for
// deterministic output. Nil-safe: a nil or empty slice returns nil. Named
// distinctly from the package-internal test helper `qnames` in
// closure_test.go, which this would otherwise collide with.
func edgeQnames(nodes []*Node) []string {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		out = append(out, n.Qname)
	}
	sort.Strings(out)
	return out
}
