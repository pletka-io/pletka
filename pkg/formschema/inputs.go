package formschema

import "github.com/pletka-io/pletka/pkg/domain"

type ProjectInput struct {
	ID          string
	SystemName  string
	UIName      domain.Translations
	Description domain.Translations
}

type FieldInput struct {
	ID                string
	SystemName        string
	UIName            domain.Translations
	Description       domain.Translations
	OntologyScope     domain.PathElement
	PathElements      []domain.PathElement
	SubfieldPaths     []domain.SubfieldPath // legacy <br><br> read-only paths
	ExpectedValueType string
	Examples          string
}

type FieldOverrideInput struct {
	ID                    int64
	FieldID               string
	ProjectID             string
	EntityType            string
	EntityID              string
	ExpectedValueType     string
	CategoryID            string
	SetValue              string
	ExpectedModelIDs      []string
	ExpectedCollectionIDs []string
}

type ComposedFieldInput struct {
	Field    FieldInput
	Override FieldOverrideInput
}

type OntologyInput struct {
	ID   string
	Name string
}

type ProjectOntologyVersionInput struct {
	ProjectID         string
	OntologyVersionID string
	IsPrimary         bool
	UsageNotes        string
}
