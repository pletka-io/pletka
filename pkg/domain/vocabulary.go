package domain

import "time"

// Vocabulary is a source of concepts: AAT, ULAN, TGN, a local list, etc.
// Vocabularies are top-level items (not project-scoped) — shared across projects.
type Vocabulary struct {
	Entity
	ConnectorType string `json:"connector_type"`
	BaseURI       string `json:"base_uri,omitempty"`
	Config        []byte `json:"config,omitempty"`
}

// VocabularyEntry is a single concept/term cached from a vocabulary.
type VocabularyEntry struct {
	ID           string               `json:"id"`
	VocabularyID string               `json:"vocabulary_id"`
	URI          string               `json:"uri"`
	Label        Translations         `json:"label"`
	ScopeNote    Translations         `json:"scope_note,omitempty"`
	BroaderURI   string               `json:"broader_uri,omitempty"`
	BroaderPath  []VocabularyEntryRef `json:"broader_path_items,omitempty"`
	ExternalID   string               `json:"external_id,omitempty"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
}

// VocabularyRef is a lightweight reference to a vocabulary source.
type VocabularyRef struct {
	ID         string       `json:"id"`
	SemanticID string       `json:"semantic_id,omitempty"`
	SystemName string       `json:"system_name,omitempty"`
	Name       Translations `json:"name,omitempty"`
	BaseURI    string       `json:"base_uri,omitempty"`
	URL        string       `json:"url,omitempty"`
}

// VocabularyEntryRef is a resolved reference to a vocabulary term. It is used
// both for selected terms and for broader path items so callers do not have to
// parse display strings to recover stable IDs or multilingual labels.
type VocabularyEntryRef struct {
	ID           string               `json:"id,omitempty"`
	VocabularyID string               `json:"vocabulary_id,omitempty"`
	URI          string               `json:"uri,omitempty"`
	Label        Translations         `json:"label,omitempty"`
	ScopeNote    Translations         `json:"scope_note,omitempty"`
	BroaderURI   string               `json:"broader_uri,omitempty"`
	BroaderPath  []VocabularyEntryRef `json:"broader_path,omitempty"`
	ExternalID   string               `json:"external_id,omitempty"`
}

// ConceptList is a curated grouping of vocabulary entries for a specific purpose.
// Project-scoped. SemanticID suffix 'L' means "LAL.1", "LAL.2".
type ConceptList struct {
	Entity
	ListType     *string `json:"list_type,omitempty"`
	VocabularyID *string `json:"vocabulary_id,omitempty"`
	// IsClosed seals the list: its membership is declared complete, so no
	// terms may be added until it is reopened, and a field bound to it treats
	// the list as an exhaustive set (validate values, export as an enum).
	IsClosed bool `json:"is_closed"`
}

// ConceptBroaderEdge is an editable skos:broader relation between two concepts.
// SchemeID scopes the edge to one concept list; a nil SchemeID is a global
// (cross-scheme) edge. Concept and broader are VocabularyEntry IDs.
type ConceptBroaderEdge struct {
	ID        string  `json:"id"`
	ConceptID string  `json:"concept_id"`
	BroaderID string  `json:"broader_id"`
	SchemeID  *string `json:"scheme_id,omitempty"`
	Position  int     `json:"position"`
}

// ConceptListEntry is the junction between ConceptList and VocabularyEntry.
type ConceptListEntry struct {
	ID                string       `json:"id"`
	ConceptListID     string       `json:"concept_list_id"`
	VocabularyEntryID string       `json:"vocabulary_entry_id"`
	Position          int          `json:"position"`
	CustomLabel       Translations `json:"custom_label,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}
