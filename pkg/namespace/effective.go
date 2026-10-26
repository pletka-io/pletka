package namespace

import (
	"fmt"
	"sort"
	"strings"
)

// Binding is a weighted namespace declaration, independent of its storage owner.
type Binding struct {
	Prefix    string `json:"prefix"`
	Namespace string `json:"namespace"`
	Weight    int64  `json:"weight,omitempty"`
	Source    string `json:"source,omitempty"`
}

// Effective selects the highest weight per prefix, rejecting different URIs
// tied at that weight. It preserves the input and returns stable prefix order.
func Effective(bindings []Binding) ([]Binding, error) {
	candidates := make([]Binding, 0, len(bindings))
	for _, b := range bindings {
		b.Prefix = strings.TrimSpace(b.Prefix)
		b.Namespace = strings.TrimSpace(b.Namespace)
		if b.Prefix != "" && b.Namespace != "" {
			candidates = append(candidates, b)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Prefix != b.Prefix {
			return a.Prefix < b.Prefix
		}
		if a.Weight != b.Weight {
			return a.Weight > b.Weight
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Source < b.Source
	})
	out := make([]Binding, 0, len(candidates))
	for _, b := range candidates {
		if len(out) == 0 || out[len(out)-1].Prefix != b.Prefix {
			out = append(out, b)
			continue
		}
		prev := out[len(out)-1]
		if prev.Weight == b.Weight && prev.Namespace != b.Namespace {
			return nil, fmt.Errorf("namespace prefix %q has conflicting bindings at weight %d: %q and %q", b.Prefix, b.Weight, prev.Namespace, b.Namespace)
		}
	}
	return out, nil
}
