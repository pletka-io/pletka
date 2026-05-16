package weave

import (
	"context"
	"strings"

	"github.com/pletka-io/pletka/domain"
)

// Field is an atomic semantic unit.
type Field struct {
	domain.Entity

	OntologyScope     domain.PathElement   `json:"ontology_scope"`
	PathElements      []domain.PathElement `json:"path_elements,omitempty"`
	ExpectedValueType string               `json:"expected_value_type,omitempty"`
	DefaultValue      string               `json:"default_value,omitempty"`
	Examples          []FieldExample       `json:"examples,omitempty"`
	StagingID         *int64               `json:"staging_id,omitempty"`
}

// OntologyPath returns the string representation derived from PathElements.
func (f *Field) OntologyPath() string {
	if len(f.PathElements) == 0 {
		return ""
	}
	var b strings.Builder
	for _, element := range f.PathElements {
		b.WriteString("->")
		b.WriteString(element.PrefixedName())
	}
	return b.String()
}

// FieldExample represents a structured usage example for a field.
type FieldExample struct {
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Language    string `json:"language,omitempty"`
}

// FieldStore reads project-scoped fields for materialization.
type FieldStore interface {
	GetByID(ctx context.Context, projectID, id string) (*Field, error)
	GetByIDVersion(ctx context.Context, projectID, id, version string) (*Field, error)
	List(ctx context.Context, projectID string) ([]*Field, error)
	ListVersion(ctx context.Context, projectID, version string) ([]*Field, error)
}
