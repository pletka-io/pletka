package i18n

import (
	"html/template"
	"time"
)

// Translation represents a single translation entry
type Translation struct {
	Key         string
	Value       string
	Language    string
	Context     string
	IsHTML      bool
	IsMarkdown  bool
	Status      TranslationStatus
	Translator  string // human, ai, imported
	UpdatedAt   time.Time
	UpdatedBy   string
	Version     int
	Variables   []Variable
	PluralForms map[PluralForm]string
	Comment     string // for translators
	MaxLength   int    // UI constraint
	Examples    []Example
}

// TranslationStatus represents the approval status
type TranslationStatus string

const (
	StatusDraft      TranslationStatus = "draft"
	StatusFuzzy      TranslationStatus = "fuzzy"
	StatusApproved   TranslationStatus = "approved"
	StatusDeprecated TranslationStatus = "deprecated"
)

// Variable represents a placeholder variable in a translation
type Variable struct {
	Name     string
	Type     string // string, number, date
	Example  any
	Required bool
}

// Example shows usage context for translators
type Example struct {
	Context      string
	Variables    map[string]any
	RenderedText string
}

// PluralForm represents different plural forms
type PluralForm string

const (
	PluralZero  PluralForm = "zero"
	PluralOne   PluralForm = "one"
	PluralTwo   PluralForm = "two"
	PluralFew   PluralForm = "few"
	PluralMany  PluralForm = "many"
	PluralOther PluralForm = "other"
)

// TranslationSet represents all translations for a specific key
type TranslationSet struct {
	Key          string
	Context      string
	Metadata     TranslationMetadata
	Translations map[string]*Translation // keyed by language code
}

// TranslationMetadata contains metadata about a translation key
type TranslationMetadata struct {
	Description   string
	Category      string
	Tags          []string
	UILocation    string
	ScreenshotURL string
	AddedAt       time.Time
	AddedBy       string
}

// GetSafe returns the translation value as template.HTML
func (t *Translation) GetSafe() template.HTML {
	return template.HTML(t.Value)
}

// HasVariable checks if the translation contains a specific variable
func (t *Translation) HasVariable(name string) bool {
	for _, v := range t.Variables {
		if v.Name == name {
			return true
		}
	}
	return false
}

// QueryFilters for searching translations
type QueryFilters struct {
	Language     string
	Status       TranslationStatus
	Context      string
	Category     string
	Search       string
	UpdatedAfter time.Time
	UpdatedBy    string
	Tags         []string
	HasVariables bool
	PageSize     int
	PageNumber   int
}

// BulkUpdate represents a bulk translation update
type BulkUpdate struct {
	Updates []TranslationUpdate
	Comment string
	User    string
}

// TranslationUpdate represents a single translation update
type TranslationUpdate struct {
	Key      string
	Language string
	Value    string
	Status   TranslationStatus
}
