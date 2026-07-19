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
	AsDomainOf      []string            `json:"as_domain_of,omitempty"`
	AsRangeOf       []string            `json:"as_range_of,omitempty"`
	Domains         []string            `json:"domains,omitempty"`
	Ranges          []string            `json:"ranges,omitempty"`
	SuperProperties []string            `json:"super_properties,omitempty"`
	Ontology        string              `json:"ontology"` // Meta.Prefix
	Version         string              `json:"version"`  // Meta.VersionString
}

// DescribeNode flattens a Node into a TermDescription (nil-safe: nil -> nil).
// Edge slices carry qnames, sorted ascending for determinism.
func DescribeNode(n *Node) *TermDescription {
	if n == nil {
		return nil
	}
	return &TermDescription{
		Qname:           n.Qname,
		URI:             n.URI,
		Type:            n.Type,
		Label:           n.Label,
		Comment:         n.Comment,
		PropertyType:    n.PropertyType,
		Superclasses:    edgeQnames(n.Superclasses),
		Subclasses:      edgeQnames(n.Subclasses),
		AsDomainOf:      edgeQnames(n.AsDomainOf),
		AsRangeOf:       edgeQnames(n.AsRangeOf),
		Domains:         edgeQnames(n.Domains),
		Ranges:          edgeQnames(n.Ranges),
		SuperProperties: edgeQnames(n.SuperProps),
		Ontology:        n.Meta.Prefix,
		Version:         n.Meta.VersionString,
	}
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
