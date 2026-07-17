package domain

import "strings"

// Field is an atomic semantic unit — one ontological path from a scope class
// to a point where a user enters real content (a name, a date, a reference).
//
// Override-shaped properties (CategoryID, SetValue, expected-value targets)
// live on weave_field_overrides — the base override row (entity_type=”) is
// the canonical home for the field's defaults; model/collection rows
// override them. Migration 027 dropped the duplicate columns from
// weave_fields. The resolver in pkg/weave/resolve.go layers base ↦ model
// ↦ collection overrides when materialising views, with one caveat: when
// the BASE override has a non-empty SetValue, that value is non-overridable
// (model/collection SetValue overrides are ignored). See pkg/weave/resolve.go.
type Field struct {
	Entity

	// Ontology path
	OntologyScope PathElement   `json:"ontology_scope"`
	PathElements  []PathElement `json:"path_elements,omitempty"`

	// ExpectedValueType stays on the field — it's a structural ontology
	// concept (Reference Model / Reference Collection / Date / Text / …)
	// that defines what kind of value this field accepts. Overrides can
	// narrow the concrete expected targets via OverrideRef rows, but they
	// do not change the type itself.
	ExpectedValueType string `json:"expected_value_type,omitempty"`

	// SubfieldPaths holds legacy additional ontology paths that were encoded in
	// the same Airtable cell, separated by <br><br>. Empty for non-legacy
	// fields. Read-only — no new subfields can be authored.
	SubfieldPaths []SubfieldPath `json:"subfield_paths,omitempty"`

	// Content
	DefaultValue string         `json:"default_value,omitempty"`
	Examples     []FieldExample `json:"examples,omitempty"`
}

// SubfieldPath is one legacy <br><br>-split path sharing the parent field's
// metadata but carrying its own terminal type and scope hint.
type SubfieldPath struct {
	PathElements      []PathElement `json:"path_elements"`
	ExpectedValueType string        `json:"expected_value_type,omitempty"`
	Scope             string        `json:"scope,omitempty"`
	Source            string        `json:"source,omitempty"`
}

// AllPaths returns the primary path followed by any legacy subfield paths, in
// source order. Generators, validators and exporters iterate this so subfields
// are treated first-class. Each entry is a slice of PathElements.
func (f *Field) AllPaths() [][]PathElement {
	paths := make([][]PathElement, 0, 1+len(f.SubfieldPaths))
	if len(f.PathElements) > 0 {
		paths = append(paths, f.PathElements)
	}
	for _, sf := range f.SubfieldPaths {
		if len(sf.PathElements) > 0 {
			paths = append(paths, sf.PathElements)
		}
	}
	return paths
}

// OntologyPath returns the string representation derived from PathElements.
// This is the computed form — PathElements is the source of truth.
func (f *Field) OntologyPath() string {
	if len(f.PathElements) == 0 {
		return ""
	}
	var b strings.Builder
	for _, pe := range f.PathElements {
		b.WriteString("->")
		b.WriteString(pe.PrefixedName())
	}
	return b.String()
}

// FieldExample represents a structured usage example for a field.
type FieldExample struct {
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Language    string `json:"language,omitempty"`
}
