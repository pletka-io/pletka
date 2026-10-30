package namespace

import (
	"fmt"
	"slices"
	"strings"
)

// Registry retains all namespace declarations, including conflicting aliases.
// It is evidence, not a usable one-per-prefix output map. Its zero value is unloaded.
type Registry struct {
	bindings []Binding
	loaded   bool
}

// NewRegistry retains an independent copy, including weights, order and provenance.
// An explicitly supplied empty inventory is different from an unloaded registry.
func NewRegistry(bindings []Binding) Registry {
	return Registry{bindings: slices.Clone(bindings), loaded: true}
}

// Bindings returns an independent copy of the complete retained inventory.
func (r Registry) Bindings() ([]Binding, error) {
	if !r.loaded {
		return nil, fmt.Errorf("namespace registry is not loaded")
	}
	return append([]Binding{}, r.bindings...), nil
}

// EffectiveBindings is a checked output projection. It cannot be constructed
// from an unchecked map, and its zero value cannot resolve a prefix.
type EffectiveBindings struct {
	bindings []Binding
	loaded   bool
}

// Effective applies the weighted policy without modifying or collapsing evidence.
// Invalid declarations and tied conflicting aliases fail without a partial map.
func (r Registry) Effective() (EffectiveBindings, error) {
	if !r.loaded {
		return EffectiveBindings{}, fmt.Errorf("namespace registry is not loaded")
	}
	for _, b := range r.bindings {
		if strings.TrimSpace(b.Prefix) == "" || strings.TrimSpace(b.Namespace) == "" {
			return EffectiveBindings{}, fmt.Errorf("namespace declaration requires a prefix and namespace")
		}
	}
	bindings, err := Effective(r.bindings)
	if err != nil {
		return EffectiveBindings{}, err
	}
	return EffectiveBindings{bindings: bindings, loaded: true}, nil
}

// Bindings returns a detached, stable prefix-ordered output projection.
func (e EffectiveBindings) Bindings() ([]Binding, error) {
	if !e.loaded {
		return nil, fmt.Errorf("effective namespace bindings are not checked")
	}
	return append([]Binding{}, e.bindings...), nil
}

// Expand resolves only a checked prefix. Missing declarations never consult
// defaults, a manager, another project's context or current policy.
func (e EffectiveBindings) Expand(prefix, localName string) (string, error) {
	if !e.loaded || strings.TrimSpace(localName) == "" {
		return "", fmt.Errorf("namespace expansion requires checked bindings and a local name")
	}
	for _, b := range e.bindings {
		if b.Prefix == prefix {
			return b.Namespace + localName, nil
		}
	}
	return "", fmt.Errorf("prefix %q is not in checked namespace bindings", prefix)
}
