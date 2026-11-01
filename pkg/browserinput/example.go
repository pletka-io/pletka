package browserinput

import (
	"encoding/json"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// ExampleView preserves recorded values without revalidating them against another state.
type ExampleView struct {
	ProjectID   string              `json:"project_id"`
	Version     string              `json:"version"`
	ID          string              `json:"id"`
	Title       domain.Translations `json:"title"`
	Description domain.Translations `json:"description"`
	Status      string              `json:"status"`
	TargetID    string              `json:"target_id"`
	TargetName  domain.Translations `json:"target_name"`
	TargetURL   string              `json:"target_url"`
	Values      []ExampleValueView  `json:"values"`
}

// ExampleValueView retains a saved slot, including stale references and legacy scalar columns.
type ExampleValueView struct {
	TextValue       *string                  `json:"text_value,omitempty"`
	NumberValue     *projectsnapshot.Decimal `json:"number_value,omitempty"`
	DateValue       *projectsnapshot.Date    `json:"date_value,omitempty"`
	URIValue        *string                  `json:"uri_value,omitempty"`
	ConceptURI      *string                  `json:"concept_uri,omitempty"`
	LinkedExampleID *string                  `json:"linked_example_id,omitempty"`
	ID              int64                    `json:"id"`
	FieldID         string                   `json:"field_id"`
	Label           domain.Translations      `json:"label"`
	SlotPath        string                   `json:"slot_path"`
	OccurrenceIndex int32                    `json:"occurrence_index"`
	Kind            string                   `json:"kind"`
	Unresolved      bool                     `json:"unresolved"`
	Payload         json.RawMessage          `json:"payload"`
	LinkedURL       string                   `json:"linked_url,omitempty"`
}
