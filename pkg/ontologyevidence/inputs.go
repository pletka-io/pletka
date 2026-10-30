package ontologyevidence

import (
	"fmt"
	"slices"

	"github.com/pletka-io/pletka/pkg/namespace"
)

// InputReference is a normalized semantic input, not a successful assessment.
type InputReference struct {
	EntityKind string
	EntityID   string
	Location   string
	Reference  TermReference
}

// InputReferences uses the same location/normalization rules as retained legacy
// evidence. Unknown or unresolved inputs are not silently removed.
func InputReferences(elements []Element) ([]InputReference, error) {
	result := []InputReference{}
	for _, element := range elements {
		refs, err := elementReferences(element)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			result = append(result, InputReference{EntityKind: ref.EntityKind, EntityID: ref.EntityID, Location: ref.Location, Reference: ref.Reference})
		}
	}
	return result, nil
}

// AssessReference checks one input against only the explicitly supplied sealed
// bodies and aliases. It preserves ambiguity and never selects a newer version.
func AssessReference(ref TermReference, inventory []Body, bindings []namespace.Binding) (Status, []Candidate, error) {
	a, err := NewAssessor(inventory, bindings)
	if err != nil {
		return Invalid, nil, err
	}
	return a.Assess(ref)
}

// Assessor is a detached, decoded-once semantic inventory. It has no lazy reader
// and never changes its selection. Zero cannot perform an assessment.
type Assessor struct {
	inventory []Body
	bodies    []bodyDocument
	bindings  []namespace.Binding
	loaded    bool
}

// NewAssessor validates body bytes once for a batch of semantic locations.
func NewAssessor(inventory []Body, bindings []namespace.Binding) (Assessor, error) {
	bodies := make([]bodyDocument, 0, len(inventory))
	for _, b := range inventory {
		body, err := decodeBody(b)
		if err != nil {
			return Assessor{}, err
		}
		bodies = append(bodies, body)
	}
	// Selection buffers are not needed by matching and are not retained.
	retained := slices.Clone(inventory)
	for i := range retained {
		retained[i].Selection = nil
	}
	return Assessor{inventory: retained, bodies: bodies, bindings: slices.Clone(bindings), loaded: true}, nil
}

// Assess uses only this value's retained bodies and aliases.
func (a Assessor) Assess(ref TermReference) (Status, []Candidate, error) {
	if !a.loaded {
		return Invalid, nil, fmt.Errorf("semantic assessor is uninitialized")
	}
	status, candidates := assess(ref, a.inventory, a.bodies, a.bindings)
	return status, candidates, nil
}
