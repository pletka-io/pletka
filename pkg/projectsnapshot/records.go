package projectsnapshot

// Timestamp preserves the loader's explicit timestamp text, including infinities.
// Loaders must use a fixed UTC representation; it is never reinterpreted as policy.
type Timestamp string

// Decimal preserves exact numeric text (including scale and special values).
// It must not pass through float64 in an assembly or restore adapter.
type Decimal string

// Date preserves an exact date, including PostgreSQL infinity values.
type Date string

// AdoptionRecord retains the complete owned weave_adoptions row.
type AdoptionRecord struct {
	ProjectID         string    `json:"project_id"`
	ContextEntityType string    `json:"context_entity_type"`
	ContextEntityID   string    `json:"context_entity_id"`
	EntityType        string    `json:"entity_type"`
	SourceProjectID   string    `json:"source_project_id"`
	SourceEntityID    string    `json:"source_entity_id"`
	SourceVersion     string    `json:"source_version"`
	AdoptedAt         Timestamp `json:"adopted_at"`
	CreatedByID       *string   `json:"created_by_id"`
	VersionNumber     string    `json:"version_number"`
}

// CategoryRecord retains the complete owned weave_categories row.
type CategoryRecord struct {
	ID             string    `json:"id"`
	CreatedAt      Timestamp `json:"created_at"`
	UpdatedAt      Timestamp `json:"updated_at"`
	SemanticID     *string   `json:"semantic_id"`
	SystemName     *string   `json:"system_name"`
	UIName         []byte    `json:"ui_name"`
	Description    []byte    `json:"description"`
	Status         string    `json:"status"`
	ProjectID      string    `json:"project_id"`
	CanonicalOrder int32     `json:"canonical_order"`
	Deprecated     bool      `json:"deprecated"`
	VersionNumber  string    `json:"version_number"`
}

// PlacementRecord retains the complete owned weave_collection_placements row.
type PlacementRecord struct {
	ID            int64     `json:"id"`
	ProjectID     string    `json:"project_id"`
	ModelID       string    `json:"model_id"`
	CategoryID    string    `json:"category_id"`
	CollectionID  string    `json:"collection_id"`
	IsRequired    bool      `json:"is_required"`
	MinOccurs     int32     `json:"min_occurs"`
	MaxOccurs     *int32    `json:"max_occurs"`
	IsHidden      bool      `json:"is_hidden"`
	CreatedAt     Timestamp `json:"created_at"`
	UpdatedAt     Timestamp `json:"updated_at"`
	VersionNumber string    `json:"version_number"`
}

// CollectionRecord retains the complete owned weave_collections row.
type CollectionRecord struct {
	ID                       string    `json:"id"`
	CreatedAt                Timestamp `json:"created_at"`
	UpdatedAt                Timestamp `json:"updated_at"`
	SystemName               *string   `json:"system_name"`
	UIName                   []byte    `json:"ui_name"`
	Description              []byte    `json:"description"`
	Status                   string    `json:"status"`
	ProjectID                string    `json:"project_id"`
	OntologyScope            []byte    `json:"ontology_scope"`
	CollectionNumber         *int32    `json:"collection_number"`
	CanonicalCollectionOrder *int32    `json:"canonical_collection_order"`
	StagingID                *int64    `json:"staging_id"`
	Deprecated               bool      `json:"deprecated"`
	DefaultCategoryID        *string   `json:"default_category_id"`
	VersionNumber            string    `json:"version_number"`
}

// BroaderRecord retains the complete owned weave_concept_broader row.
type BroaderRecord struct {
	ID            string     `json:"id"`
	ConceptID     string     `json:"concept_id"`
	BroaderID     string     `json:"broader_id"`
	SchemeID      string     `json:"scheme_id"`
	Position      int32      `json:"position"`
	VersionNumber string     `json:"version_number"`
	CreatedAt     *Timestamp `json:"created_at"`
}

// ConceptEntryRecord retains the complete owned weave_concept_list_entries row.
type ConceptEntryRecord struct {
	ID                string    `json:"id"`
	ConceptListID     string    `json:"concept_list_id"`
	VocabularyEntryID string    `json:"vocabulary_entry_id"`
	Position          int32     `json:"position"`
	CustomLabel       []byte    `json:"custom_label"`
	CreatedAt         Timestamp `json:"created_at"`
	UpdatedAt         Timestamp `json:"updated_at"`
	VersionNumber     string    `json:"version_number"`
}

// ConceptListRecord retains the complete owned weave_concept_lists row.
type ConceptListRecord struct {
	ID            string    `json:"id"`
	CreatedAt     Timestamp `json:"created_at"`
	UpdatedAt     Timestamp `json:"updated_at"`
	SemanticID    *string   `json:"semantic_id"`
	SystemName    *string   `json:"system_name"`
	UIName        []byte    `json:"ui_name"`
	Description   []byte    `json:"description"`
	Status        string    `json:"status"`
	ProjectID     string    `json:"project_id"`
	ListType      *string   `json:"list_type"`
	VocabularyID  *string   `json:"vocabulary_id"`
	VersionNumber string    `json:"version_number"`
	IsClosed      bool      `json:"is_closed"`
}

// ForkRecord retains the complete owned weave_entity_forks row.
type ForkRecord struct {
	ProjectID       string    `json:"project_id"`
	EntityType      string    `json:"entity_type"`
	ForkEntityID    string    `json:"fork_entity_id"`
	SourceProjectID string    `json:"source_project_id"`
	SourceEntityID  string    `json:"source_entity_id"`
	SourceVersion   string    `json:"source_version"`
	ForkedAt        Timestamp `json:"forked_at"`
	CreatedByID     *string   `json:"created_by_id"`
	VersionNumber   string    `json:"version_number"`
}

// ExampleValueRecord retains the complete owned weave_example_values row.
type ExampleValueRecord struct {
	ID                 int64     `json:"id"`
	ExampleID          string    `json:"example_id"`
	OverrideID         int64     `json:"override_id"`
	FieldID            string    `json:"field_id"`
	PartOfCollectionID *string   `json:"part_of_collection_id"`
	OccurrenceIndex    int32     `json:"occurrence_index"`
	ValueKind          string    `json:"value_kind"`
	ValuePayload       []byte    `json:"value_payload"`
	TextValue          *string   `json:"text_value"`
	NumberValue        *Decimal  `json:"number_value"`
	DateValue          *Date     `json:"date_value"`
	URIValue           *string   `json:"uri_value"`
	ConceptURI         *string   `json:"concept_uri"`
	LinkedExampleID    *string   `json:"linked_example_id"`
	SlotPath           string    `json:"slot_path"`
	CreatedAt          Timestamp `json:"created_at"`
	UpdatedAt          Timestamp `json:"updated_at"`
	VersionNumber      string    `json:"version_number"`
}

// ExampleRecord retains the complete owned weave_examples row.
type ExampleRecord struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	EntityType    string    `json:"entity_type"`
	EntityID      string    `json:"entity_id"`
	Title         []byte    `json:"title"`
	Description   []byte    `json:"description"`
	Status        string    `json:"status"`
	CreatedAt     Timestamp `json:"created_at"`
	UpdatedAt     Timestamp `json:"updated_at"`
	VersionNumber string    `json:"version_number"`
}

// OverrideRecord retains the complete owned weave_field_overrides row.
type OverrideRecord struct {
	ID                 int64     `json:"id"`
	FieldID            string    `json:"field_id"`
	ProjectID          string    `json:"project_id"`
	EntityType         string    `json:"entity_type"`
	EntityID           string    `json:"entity_id"`
	Position           int32     `json:"position"`
	CollectionOrder    int32     `json:"collection_order"`
	DisplayName        []byte    `json:"display_name"`
	Description        []byte    `json:"description"`
	CollectionName     []byte    `json:"collection_name"`
	CategoryID         *string   `json:"category_id"`
	PartOfCollectionID *string   `json:"part_of_collection_id"`
	ExpectedValueType  *string   `json:"expected_value_type"`
	SetValue           *string   `json:"set_value"`
	IsRequired         *bool     `json:"is_required"`
	MinOccurs          *int32    `json:"min_occurs"`
	MaxOccurs          *int32    `json:"max_occurs"`
	IsHidden           *bool     `json:"is_hidden"`
	Visibility         *string   `json:"visibility"`
	StagingID          *int64    `json:"staging_id"`
	CreatedAt          Timestamp `json:"created_at"`
	UpdatedAt          Timestamp `json:"updated_at"`
	ContentHash        *string   `json:"content_hash"`
	VersionNumber      string    `json:"version_number"`
	SetValueEntryID    *string   `json:"set_value_entry_id"`
}

// FieldRecord retains the complete owned weave_fields row.
type FieldRecord struct {
	ID                string    `json:"id"`
	CreatedAt         Timestamp `json:"created_at"`
	UpdatedAt         Timestamp `json:"updated_at"`
	SemanticID        *string   `json:"semantic_id"`
	SystemName        *string   `json:"system_name"`
	UIName            []byte    `json:"ui_name"`
	Description       []byte    `json:"description"`
	Status            string    `json:"status"`
	ProjectID         string    `json:"project_id"`
	OntologyScope     []byte    `json:"ontology_scope"`
	OntologyPath      *string   `json:"ontology_path"`
	PathElements      []byte    `json:"path_elements"`
	ExpectedValueType *string   `json:"expected_value_type"`
	Examples          []byte    `json:"examples"`
	StagingID         *int64    `json:"staging_id"`
	Deprecated        bool      `json:"deprecated"`
	VersionNumber     string    `json:"version_number"`
	SubfieldPaths     []byte    `json:"subfield_paths"`
}

// ModelRecord retains the complete owned weave_models row.
type ModelRecord struct {
	ID            string    `json:"id"`
	CreatedAt     Timestamp `json:"created_at"`
	UpdatedAt     Timestamp `json:"updated_at"`
	SystemName    *string   `json:"system_name"`
	UIName        []byte    `json:"ui_name"`
	Description   []byte    `json:"description"`
	Status        string    `json:"status"`
	ProjectID     string    `json:"project_id"`
	OntologyScope []byte    `json:"ontology_scope"`
	StagingID     *int64    `json:"staging_id"`
	Deprecated    bool      `json:"deprecated"`
	VersionNumber string    `json:"version_number"`
	ModelType     string    `json:"model_type"`
}

// NamespaceRecord retains the complete owned weave_namespace_bindings row.
type NamespaceRecord struct {
	ReleaseProjectID string    `json:"release_project_id"`
	VersionNumber    string    `json:"version_number"`
	ID               string    `json:"id"`
	CreatedAt        Timestamp `json:"created_at"`
	UpdatedAt        Timestamp `json:"updated_at"`
	AirtableRaw      []byte    `json:"airtable_raw"`
	AirtableContext  []byte    `json:"airtable_context"`
	AirtableRef      *string   `json:"airtable_ref"`
	SemanticID       *string   `json:"semantic_id"`
	LongSemanticID   *string   `json:"long_semantic_id"`
	SystemName       *string   `json:"system_name"`
	UIName           []byte    `json:"ui_name"`
	Description      []byte    `json:"description"`
	URI              *string   `json:"uri"`
	ProjectID        *string   `json:"project_id"`
	Prefix           string    `json:"prefix"`
	Namespace        string    `json:"namespace"`
	Weight           int64     `json:"weight"`
	Source           string    `json:"source"`
	OntologyID       *string   `json:"ontology_id"`
}

// OverrideRefRecord retains the complete owned weave_override_refs row.
type OverrideRefRecord struct {
	OverrideID    int64  `json:"override_id"`
	RefType       string `json:"ref_type"`
	TargetID      string `json:"target_id"`
	SemanticID    string `json:"semantic_id"`
	Position      int32  `json:"position"`
	ProjectID     string `json:"project_id"`
	VersionNumber string `json:"version_number"`
}

// ProjectActorRecord retains the complete owned weave_project_actors row.
type ProjectActorRecord struct {
	ProjectID     string    `json:"project_id"`
	ActorID       string    `json:"actor_id"`
	Role          string    `json:"role"`
	CreatedAt     Timestamp `json:"created_at"`
	VersionNumber string    `json:"version_number"`
}

// AttributionRecord retains the complete owned weave_project_attributions row.
type AttributionRecord struct {
	ProjectID     string    `json:"project_id"`
	ActorID       string    `json:"actor_id"`
	Kind          string    `json:"kind"`
	Position      int32     `json:"position"`
	Note          string    `json:"note"`
	CreatedAt     Timestamp `json:"created_at"`
	VersionNumber string    `json:"version_number"`
}

// InheritanceRecord retains the complete owned weave_project_inheritance row.
type InheritanceRecord struct {
	ProjectID       string    `json:"project_id"`
	ParentProjectID string    `json:"parent_project_id"`
	IsPrimary       bool      `json:"is_primary"`
	CanonicalOrder  int32     `json:"canonical_order"`
	AdoptedAt       Timestamp `json:"adopted_at"`
	VersionNumber   string    `json:"version_number"`
	SourceMode      string    `json:"source_mode"`
	SourceVersion   *string   `json:"source_version"`
}

// OntologyLinkRecord retains the complete owned weave_project_ontology_versions row.
type OntologyLinkRecord struct {
	ProjectID         string    `json:"project_id"`
	OntologyVersionID string    `json:"ontology_version_id"`
	AddedAt           Timestamp `json:"added_at"`
	AddedByID         *string   `json:"added_by_id"`
	IsPrimary         *bool     `json:"is_primary"`
	UsageNotes        *string   `json:"usage_notes"`
	VersionNumber     string    `json:"version_number"`
}

// ProjectRecord retains the complete owned weave_projects row.
type ProjectRecord struct {
	ID                        string    `json:"id"`
	CreatedAt                 Timestamp `json:"created_at"`
	UpdatedAt                 Timestamp `json:"updated_at"`
	SystemName                *string   `json:"system_name"`
	UIName                    []byte    `json:"ui_name"`
	Description               []byte    `json:"description"`
	Status                    string    `json:"status"`
	Namespace                 *string   `json:"namespace"`
	ParentProjectID           *string   `json:"parent_project_id"`
	StagingID                 *int64    `json:"staging_id"`
	OwnerID                   string    `json:"owner_id"`
	Visibility                string    `json:"visibility"`
	Deprecated                bool      `json:"deprecated"`
	License                   string    `json:"license"`
	README                    []byte    `json:"readme"`
	Topics                    []string  `json:"topics"`
	BaseURL                   string    `json:"base_url"`
	CreatedByID               *string   `json:"created_by_id"`
	VersionNumber             string    `json:"version_number"`
	ConceptNamespace          *string   `json:"concept_namespace"`
	EffectiveConceptNamespace *string   `json:"effective_concept_namespace"`
	EnforceConceptLists       *bool     `json:"enforce_concept_lists"`
}

// VocabularyRecord retains the complete owned weave_vocabularies row.
type VocabularyRecord struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	SemanticID    *string   `json:"semantic_id"`
	SystemName    *string   `json:"system_name"`
	UIName        []byte    `json:"ui_name"`
	Description   []byte    `json:"description"`
	Status        string    `json:"status"`
	ConnectorType string    `json:"connector_type"`
	BaseURI       *string   `json:"base_uri"`
	Config        []byte    `json:"config"`
	Deprecated    bool      `json:"deprecated"`
	CreatedAt     Timestamp `json:"created_at"`
	UpdatedAt     Timestamp `json:"updated_at"`
	VersionNumber string    `json:"version_number"`
}

// VocabularyEntryRecord retains the complete owned weave_vocabulary_entries row.
type VocabularyEntryRecord struct {
	ID               string    `json:"id"`
	VocabularyID     string    `json:"vocabulary_id"`
	URI              string    `json:"uri"`
	Label            []byte    `json:"label"`
	ScopeNote        []byte    `json:"scope_note"`
	BroaderURI       *string   `json:"broader_uri"`
	BroaderPath      []byte    `json:"broader_path"`
	BroaderPathItems []byte    `json:"broader_path_items"`
	ExternalID       *string   `json:"external_id"`
	CreatedAt        Timestamp `json:"created_at"`
	UpdatedAt        Timestamp `json:"updated_at"`
	VersionNumber    string    `json:"version_number"`
}

// Content retains every owned captured column. Byte slices contain original JSON
// bytes, not a lossy domain projection. Nil sections mean unloaded; an explicit
// empty slice means successfully loaded empty. This is not a portable wire format.
type Content struct {
	Adoptions         []AdoptionRecord        `json:"weave_adoptions_archive"`
	Categories        []CategoryRecord        `json:"weave_categories_archive"`
	Placements        []PlacementRecord       `json:"weave_collection_placements_archive"`
	Collections       []CollectionRecord      `json:"weave_collections_archive"`
	Broader           []BroaderRecord         `json:"weave_concept_broader_archive"`
	ConceptEntries    []ConceptEntryRecord    `json:"weave_concept_list_entries_archive"`
	ConceptLists      []ConceptListRecord     `json:"weave_concept_lists_archive"`
	Forks             []ForkRecord            `json:"weave_entity_forks_archive"`
	ExampleValues     []ExampleValueRecord    `json:"weave_example_values_archive"`
	Examples          []ExampleRecord         `json:"weave_examples_archive"`
	Overrides         []OverrideRecord        `json:"weave_field_overrides_archive"`
	Fields            []FieldRecord           `json:"weave_fields_archive"`
	Models            []ModelRecord           `json:"weave_models_archive"`
	Namespaces        []NamespaceRecord       `json:"weave_release_namespace_bindings_archive"`
	OverrideRefs      []OverrideRefRecord     `json:"weave_override_refs_archive"`
	ProjectActors     []ProjectActorRecord    `json:"weave_project_actors_archive"`
	Attributions      []AttributionRecord     `json:"weave_project_attributions_archive"`
	Inheritance       []InheritanceRecord     `json:"weave_project_inheritance_archive"`
	OntologyLinks     []OntologyLinkRecord    `json:"weave_project_ontology_versions_archive"`
	Projects          []ProjectRecord         `json:"weave_projects_archive"`
	Vocabularies      []VocabularyRecord      `json:"weave_vocabularies_archive"`
	VocabularyEntries []VocabularyEntryRecord `json:"weave_vocabulary_entries_archive"`
}
