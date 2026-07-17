package domain

// Model represents an entity type definition — a scope class with fields.
// Identity + ontology only. Display properties live in overrides.
type Model struct {
	Entity

	// OntologyScope is the CRM class that defines this model's scope (e.g., E21_Person).
	OntologyScope PathElement `json:"ontology_scope"`

	// ModelType classifies the model within the project. Whitelist:
	// "core"      — headline model (sorts to top of the list).
	// "auxiliary" — supporting / secondary model (default).
	// "example"   — demonstration / template model.
	// The whitelist lives in the service layer so new categories can
	// land without a migration.
	ModelType string `json:"model_type"`

	// StagingID links to the import staging record for provenance. Nil for manually created models.
	StagingID *int64 `json:"staging_id,omitempty"`
}

// Recognised model_type values. Service layer rejects anything outside this set.
const (
	ModelTypeCore      = "core"
	ModelTypeAuxiliary = "auxiliary"
	ModelTypeExample   = "example"
)

// ModelTypes returns the canonical list of allowed model_type values.
// Add new categories here — no DB migration needed.
func ModelTypes() []string {
	return []string{ModelTypeCore, ModelTypeAuxiliary, ModelTypeExample}
}

// IsValidModelType reports whether t is one of the recognised model
// categories. Empty string maps to auxiliary (the default).
func IsValidModelType(t string) bool {
	switch t {
	case "", ModelTypeCore, ModelTypeAuxiliary, ModelTypeExample:
		return true
	}
	return false
}
