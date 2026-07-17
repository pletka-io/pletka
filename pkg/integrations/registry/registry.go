package registry

import (
	"fmt"
)

// Registry holds the Integrations available to the hub. Construct via
// NewRegistry; the result is read-only.
type Registry struct {
	byID map[string]Integration
}

// NewRegistry validates and indexes the provided integrations by ID.
// Nil entries are skipped silently. Empty IDs and duplicates fail.
func NewRegistry(integrations ...Integration) (*Registry, error) {
	r := &Registry{byID: make(map[string]Integration, len(integrations))}
	for _, integ := range integrations {
		if integ == nil {
			continue
		}
		id := integ.ID()
		if id == "" {
			return nil, fmt.Errorf("register integration: empty id")
		}
		if _, exists := r.byID[id]; exists {
			return nil, fmt.Errorf("register integration %q: duplicate id", id)
		}
		r.byID[id] = integ
	}
	return r, nil
}

// Integration returns the integration with the given ID, or (nil, false)
// if none is registered or the receiver is nil.
func (r *Registry) Integration(id string) (Integration, bool) {
	if r == nil {
		return nil, false
	}
	integ, ok := r.byID[id]
	return integ, ok
}

// All returns every registered integration. Order is unspecified.
func (r *Registry) All() []Integration {
	if r == nil {
		return nil
	}
	out := make([]Integration, 0, len(r.byID))
	for _, integ := range r.byID {
		out = append(out, integ)
	}
	return out
}

// ForFormat returns integrations whose AppliesTo set contains the given
// format. The hub uses this to surface format-specific action buttons
// (e.g. "Upload to 3M" on the X3ML B sub-tab).
func (r *Registry) ForFormat(format Format) []Integration {
	if r == nil || format == "" {
		return nil
	}
	out := make([]Integration, 0)
	for _, integ := range r.byID {
		for _, f := range integ.AppliesTo() {
			if f == format {
				out = append(out, integ)
				break
			}
		}
	}
	return out
}
