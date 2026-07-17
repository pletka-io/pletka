package domain

// SearchParams defines the parameters for entity search.
type SearchParams struct {
	ProjectID     string
	Type          string // "field" or "collection"
	Query         string // free text search
	Scope         string // "project" (default) or "inherited"
	PathPrefix    string // parsed: ontology prefix (e.g., "crm")
	PathLocalName string // parsed: local name (e.g., "P1_is_identified_by")
	// OntologyClass is the ontology scope class filter (e.g., "E21_Person").
	OntologyClass string
	// OntologyPrefix is the parsed prefix from ontology_class input (e.g., "crm").
	OntologyPrefix string
	// PathStartsWith is a multi-element path prefix filter (e.g., "P1->E33_E41").
	PathStartsWith string
	// PathEndsWith is a terminal element filter.
	PathEndsWith string
	// PathDepth is the exact path element count (0 = any).
	PathDepth int
	// PathFieldIDs holds pre-computed field IDs from path stored functions.
	// When set, acts as an AND filter against the result set.
	PathFieldIDs  []string
	Category      string // category ID filter
	ExpectedValue string // expected_value_type filter
	TargetID      string // model/collection ID for is_linked marking
	Sort          string // "relevance", "adoption", "name"
	Limit         int
	Offset        int
}

// PathSuggestion is a single autocomplete suggestion from path_next_suggestions.
type PathSuggestion struct {
	LocalName  string `json:"local_name"`
	Prefix     string `json:"prefix"`
	Type       string `json:"type"`
	FieldCount int64  `json:"field_count"`
	Display    string `json:"display"` // computed: "crm:P1_is_identified_by"
}

// PathSuggestionsResponse is the response for the path suggestions endpoint.
type PathSuggestionsResponse struct {
	Suggestions []PathSuggestion `json:"suggestions"`
	CurrentPath string           `json:"current_path"`
	Depth       int              `json:"depth"`
}

// SearchResult is a lightweight entity representation for search results.
type SearchResult struct {
	ID                          string        `json:"id"`
	SemanticID                  string        `json:"semantic_id"`
	SystemName                  string        `json:"system_name"`
	UIName                      Translations  `json:"ui_name"`
	Description                 Translations  `json:"description,omitempty"`
	OntologyScope               *PathElement  `json:"ontology_scope,omitempty"`
	OntologyPath                string        `json:"ontology_path"`
	PathElements                []PathElement `json:"path_elements,omitempty"`
	ExpectedValueType           string        `json:"expected_value_type,omitempty"`
	ExpectedResourceModels      []string      `json:"expected_resource_models,omitempty"`
	ExpectedCollectionModels    []string      `json:"expected_collection_models,omitempty"`
	ExpectedConceptLists        []string      `json:"expected_concept_lists,omitempty"`
	ExpectedResourceModelRefs   []EntityRef   `json:"expected_resource_model_refs,omitempty"`
	ExpectedCollectionModelRefs []EntityRef   `json:"expected_collection_model_refs,omitempty"`
	ExpectedConceptListRefs     []EntityRef   `json:"expected_concept_list_refs,omitempty"`
	ProjectID                   string        `json:"project_id"`
	Origin                      Origin        `json:"origin"`
	IsLinked                    bool          `json:"is_linked"`
	AdoptionCount               int           `json:"adoption_count"`
	URL                         string        `json:"url,omitempty"`
}

// SearchResponse is the paginated search API response.
type SearchResponse struct {
	Items      []SearchResult `json:"items"`
	TotalCount int            `json:"total_count"`
}
