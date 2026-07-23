package canonical

import (
	"encoding/json"
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
)

// pathElementValue serializes one domain.PathElement to a plain map for
// canonical YAML. The map mirrors the path_elements JSONB contract exactly
// (same keys, same omitempty behavior), so a snapshot round-trips every
// element attribute — type, position, class_code, instance_id, datatype,
// additional_types, sub_property_of, … — not just the qname. Element order
// is the caller's list order and is preserved by YAML sequences.
func pathElementValue(pe domain.PathElement) (map[string]any, error) {
	raw, err := json.Marshal(pe)
	if err != nil {
		return nil, fmt.Errorf("marshal path element %s: %w", pe.URI, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("normalize path element %s: %w", pe.URI, err)
	}
	return m, nil
}

// pathElementValues serializes a path in order.
func pathElementValues(elements []domain.PathElement) ([]any, error) {
	out := make([]any, len(elements))
	for i, pe := range elements {
		m, err := pathElementValue(pe)
		if err != nil {
			return nil, err
		}
		out[i] = m
	}
	return out, nil
}

// subfieldPathValues serializes legacy subfield paths with the same
// element fidelity as the primary path.
func subfieldPathValues(subfields []domain.SubfieldPath) ([]any, error) {
	if len(subfields) == 0 {
		return nil, nil
	}
	out := make([]any, len(subfields))
	for i, sp := range subfields {
		elements, err := pathElementValues(sp.PathElements)
		if err != nil {
			return nil, err
		}
		m := map[string]any{
			"path_elements": elements,
		}
		if sp.ExpectedValueType != "" {
			m["expected_value_type"] = sp.ExpectedValueType
		}
		if sp.Scope != "" {
			m["scope"] = sp.Scope
		}
		if sp.Source != "" {
			m["source"] = sp.Source
		}
		out[i] = m
	}
	return out, nil
}
