package domain

// Collection represents a group of fields sharing a root ontology context.
// Identity + ontology only. Display properties live in overrides.
type Collection struct {
	Entity

	// OntologyScope is the CRM class that defines this collection's context (e.g., E67_Birth).
	OntologyScope PathElement `json:"ontology_scope"`

	// CollectionNumber is the ordering number within the project.
	CollectionNumber int `json:"collection_number,omitempty"`

	// CanonicalCollectionOrder is the display order.
	CanonicalCollectionOrder int `json:"canonical_collection_order,omitempty"`

	// DefaultCategoryID is the project-level default category for this
	// collection's fields. When the collection is dropped into a model,
	// fields with no explicit category land here. Per-context overrides
	// in the model still win. Nil when not set.
	DefaultCategoryID *string `json:"default_category_id,omitempty"`

	// StagingID links to the import staging record for provenance. Nil for manually created collections.
	StagingID *int64 `json:"staging_id,omitempty"`
}

// CollectionAnchor pairs a collection ID with the URI of its anchor
// CIDOC class — the last class element of the longest common path
// prefix across the collection's fields. Used by snap-level
// generator composition to find candidate collections to graft onto
// stub fields ending at the same class.
type CollectionAnchor struct {
	CollectionID string
	ProjectID    string
	AnchorURI    string
}
