package domain

import (
	"fmt"
	"strings"
)

// Valid checks that a Field has all required data for database insertion.
// Returns nil when valid, or a slice of errors describing every problem.
//
// Reference-target validation (expected_value_id when ExpectedValueType is
// a reference) lives at the override layer after migration 027 — the field
// no longer carries the ID. The override slice's validation enforces the
// rule when the base override is created/updated.
func (f *Field) Valid() []error {
	var errs []error

	errs = append(errs, f.Entity.valid()...)

	if f.OntologyScope.URI == "" {
		errs = append(errs, fmt.Errorf("ontology_scope is required"))
	}
	errs = append(errs, validateAdditionalTypes(f.OntologyScope)...)
	if len(f.PathElements) == 0 {
		errs = append(errs, fmt.Errorf("path_elements is required"))
	}
	if f.ExpectedValueType == "" {
		errs = append(errs, fmt.Errorf("expected_value_type is required"))
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// Valid checks that a Model has all required data for database insertion.
func (m *Model) Valid() []error {
	var errs []error
	errs = append(errs, m.Entity.valid()...)

	if m.OntologyScope.URI == "" {
		errs = append(errs, fmt.Errorf("ontology_scope is required"))
	}
	errs = append(errs, validateAdditionalTypes(m.OntologyScope)...)

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// Valid checks that a Collection has all required data for database insertion.
func (c *Collection) Valid() []error {
	var errs []error
	errs = append(errs, c.Entity.valid()...)

	if c.OntologyScope.URI == "" {
		errs = append(errs, fmt.Errorf("ontology_scope is required"))
	}
	errs = append(errs, validateAdditionalTypes(c.OntologyScope)...)

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// validateAdditionalTypes checks that every additional type on a scope
// has a non-empty URI. Empty primary scope is reported separately.
func validateAdditionalTypes(scope PathElement) []error {
	var errs []error
	for i, t := range scope.AdditionalTypes {
		if t.URI == "" {
			errs = append(errs, fmt.Errorf("ontology_scope.additional_types[%d].uri is required", i))
		}
	}
	return errs
}

// Valid checks that a Category has all required data for database insertion.
func (c *Category) Valid() []error {
	errs := c.Entity.valid()
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// valid checks shared Entity-level required fields.
func (e *Entity) valid() []error {
	var errs []error

	if e.ID == "" {
		errs = append(errs, fmt.Errorf("id is required"))
	}
	if e.ProjectID == "" {
		errs = append(errs, fmt.Errorf("project_id is required"))
	}
	if e.UIName.IsEmpty() {
		errs = append(errs, fmt.Errorf("ui_name is required"))
	}
	if e.Description.IsEmpty() {
		errs = append(errs, fmt.Errorf("description is required"))
	}

	return errs
}

// isReferenceType returns true for value types that require an expected_value_id.
func isReferenceType(vt string) bool {
	lower := strings.ToLower(vt)
	return strings.Contains(lower, "reference model") || strings.Contains(lower, "reference collection")
}
