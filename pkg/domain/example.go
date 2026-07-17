package domain

import "time"

type ExampleStatus string

const (
	ExampleStatusDraft     ExampleStatus = "draft"
	ExampleStatusValid     ExampleStatus = "valid"
	ExampleStatusHasIssues ExampleStatus = "has_issues"
)

func (s ExampleStatus) Valid() bool {
	switch s {
	case ExampleStatusDraft, ExampleStatusValid, ExampleStatusHasIssues:
		return true
	}
	return false
}

type ExampleEntityType string

const (
	ExampleEntityTypeModel      ExampleEntityType = "model"
	ExampleEntityTypeCollection ExampleEntityType = "collection"
)

func (t ExampleEntityType) Valid() bool {
	switch t {
	case ExampleEntityTypeModel, ExampleEntityTypeCollection:
		return true
	}
	return false
}

type ExampleValueKind string

const (
	ExampleValueKindString     ExampleValueKind = "string"
	ExampleValueKindInteger    ExampleValueKind = "integer"
	ExampleValueKindDate       ExampleValueKind = "date"
	ExampleValueKindURI        ExampleValueKind = "uri"
	ExampleValueKindConcept    ExampleValueKind = "concept"
	ExampleValueKindExampleRef ExampleValueKind = "example_ref"
)

func (k ExampleValueKind) Valid() bool {
	switch k {
	case ExampleValueKindString, ExampleValueKindInteger, ExampleValueKindDate, ExampleValueKindURI, ExampleValueKindConcept, ExampleValueKindExampleRef:
		return true
	}
	return false
}

type Example struct {
	ID            string            `json:"id"`
	ProjectID     string            `json:"project_id"`
	EntityType    ExampleEntityType `json:"entity_type"`
	EntityID      string            `json:"entity_id"`
	Title         Translations      `json:"title,omitempty"`
	Description   Translations      `json:"description,omitempty"`
	Status        ExampleStatus     `json:"status"`
	VersionNumber string            `json:"version_number,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type ExampleValuePayload struct {
	Kind           ExampleValueKind `json:"kind"`
	StringValue    *string          `json:"string_value,omitempty"`
	NumberValue    *float64         `json:"number_value,omitempty"`
	BoolValue      *bool            `json:"bool_value,omitempty"`
	DateValue      *string          `json:"date_value,omitempty"`
	URIValue       *string          `json:"uri_value,omitempty"`
	ConceptURI     *string          `json:"concept_uri,omitempty"`
	ConceptLabel   *string          `json:"concept_label,omitempty"`
	ExampleID      *string          `json:"example_id,omitempty"`
	TargetEntityID *string          `json:"target_entity_id,omitempty"`
	TargetLabel    *string          `json:"target_label,omitempty"`
}

type ExampleValue struct {
	ID                 int64               `json:"id"`
	ExampleID          string              `json:"example_id"`
	OverrideID         int64               `json:"override_id"`
	FieldID            string              `json:"field_id"`
	PartOfCollectionID string              `json:"part_of_collection_id,omitempty"`
	OccurrenceIndex    int                 `json:"occurrence_index"`
	ValueKind          ExampleValueKind    `json:"value_kind"`
	ValuePayload       ExampleValuePayload `json:"value_payload"`
	TextValue          *string             `json:"text_value,omitempty"`
	NumberValue        *float64            `json:"number_value,omitempty"`
	DateValue          *string             `json:"date_value,omitempty"`
	URIValue           *string             `json:"uri_value,omitempty"`
	ConceptURI         *string             `json:"concept_uri,omitempty"`
	LinkedExampleID    *string             `json:"linked_example_id,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

type ExampleIssueSeverity string

const (
	ExampleIssueError   ExampleIssueSeverity = "error"
	ExampleIssueWarning ExampleIssueSeverity = "warning"
)

type ExampleIssue struct {
	Severity        ExampleIssueSeverity `json:"severity"`
	Code            string               `json:"code"`
	FieldID         *string              `json:"field_id,omitempty"`
	OverrideID      *int64               `json:"override_id,omitempty"`
	OccurrenceIndex *int                 `json:"occurrence_index,omitempty"`
	Message         Translations         `json:"message,omitempty"`
}

type ExampleValidationReport struct {
	ExampleID string         `json:"example_id,omitempty"`
	Valid     bool           `json:"valid"`
	Issues    []ExampleIssue `json:"issues,omitempty"`
}
