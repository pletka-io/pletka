package detailview

import (
	"time"

	pkgdomain "github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
)

const (
	ViewModeDetailed = "detailed"
	ViewModeCompact  = "compact"
)

type EntityViewResponse struct {
	Entity       EntityViewMeta         `json:"entity"`
	Capabilities ViewCapabilities       `json:"capabilities"`
	ViewMode     string                 `json:"view_mode"`
	Sections     []ViewSection          `json:"sections"`
	Entries      []ConceptListEntryView `json:"entries,omitempty"`
	Adoptions    []AdoptionItem         `json:"adoptions,omitempty"`
	Refs         ViewRefs               `json:"refs"`
	StatsURL     string                 `json:"stats_url"`
	Release      *ReleaseView           `json:"release,omitempty"`
}

type EntityViewMeta struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Name        pkgdomain.Translations `json:"name"`
	Description pkgdomain.Translations `json:"description,omitempty"`
	SystemName  string                 `json:"system_name"`
	Status      string                 `json:"status"`
	// PublicationState is the live entity's state relative to the project's
	// latest release: draft (no release) | published | modified | new. Derived
	// on read; drives the header badge instead of the raw Status column. Empty
	// in release-view mode (the tier-1 archived status already reads published).
	PublicationState string `json:"publication_state,omitempty"`
	// Deprecated is orthogonal to Status (a draft or published entity can be
	// deprecated). The detail header renders a distinct badge when set.
	Deprecated bool             `json:"deprecated,omitempty"`
	Origin     pkgdomain.Origin `json:"origin"`
	// OriginLabel + OriginHelp are server-resolved LocalizedText so the
	// detail page's provenance badge + tooltip render via tr() with no
	// domain wording in the frontend (schema-driven UI rule). Both walk
	// through h.i18n.Resolve before encode; the wire shape is a
	// Translations map per field — same as every other localized label.
	OriginLabel i18n.LocalizedText `json:"origin_label,omitempty"`
	OriginHelp  i18n.LocalizedText `json:"origin_help,omitempty"`
	Scope       *ViewScopeInfo     `json:"ontology_scope,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`

	OntologyPath      string                        `json:"ontology_path,omitempty"`
	PathElements      []pkgdomain.PathElement       `json:"path_elements,omitempty"`
	ExpectedValueType string                        `json:"expected_value_type,omitempty"`
	SetValue          string                        `json:"set_value,omitempty"`
	CategoryID        string                        `json:"category_id,omitempty"`
	ModelType         string                        `json:"model_type,omitempty"`
	ListType          string                        `json:"list_type,omitempty"`
	ListTypeURI       string                        `json:"list_type_uri,omitempty"`
	ParentTermURI     string                        `json:"parent_term_uri,omitempty"`
	ListTypeLabel     string                        `json:"list_type_label,omitempty"`
	ParentTerm        *pkgdomain.VocabularyEntryRef `json:"parent_term,omitempty"`
	VocabularyID      string                        `json:"vocabulary_id,omitempty"`
	VocabularyLabel   string                        `json:"vocabulary_label,omitempty"`
	SourceVocabulary  *pkgdomain.VocabularyRef      `json:"source_vocabulary,omitempty"`
	EntryCount        int                           `json:"entry_count,omitempty"`
}

type ViewScopeInfo struct {
	LocalName string `json:"local_name"`
	Prefix    string `json:"prefix"`
}

type ViewCapabilities struct {
	Editable     bool               `json:"editable"`
	CanAddFields bool               `json:"can_add_fields"`
	CanReorder   bool               `json:"can_reorder"`
	SaveURL      string             `json:"save_url,omitempty"`
	MetadataURL  string             `json:"metadata_url,omitempty"`
	AdoptURL     string             `json:"adopt_url,omitempty"`
	AdoptLabel   i18n.LocalizedText `json:"adopt_label,omitempty"`
	ForkURL      string             `json:"fork_url,omitempty"`
	// ForkLabel is the verb the curator sees on the button. Stays
	// "fork_*" in the JSON wire shape for backward compat; the i18n
	// value resolves to "Adapt to edit" per the adopt/adapt rename
	// decision (task 2).
	ForkLabel         i18n.LocalizedText `json:"fork_label,omitempty"`
	ForkPending       i18n.LocalizedText `json:"fork_pending,omitempty"`
	SearchFieldsURL   string             `json:"search_fields_url,omitempty"`
	ExamplesSchemaURL string             `json:"examples_schema_url,omitempty"`
	ConceptList       *ConceptListCap    `json:"concept_list,omitempty"`
	Derivatives       *DerivativesCap    `json:"derivatives,omitempty"`
	// ReuseURL is the absolute URL of the field-usage endpoint that
	// powers the Reuse tab on a field item view. Emitted
	// only for field responses today; presence of this URL is what the
	// frontend uses to decide whether to render the Reuse tab.
	ReuseURL string `json:"reuse_url,omitempty"`
	// Lifecycle actions for the detail-page "More actions" kebab. Emitted
	// only for an editable, owned entity, mirroring the list-row actions in
	// each slice's formschema so both surfaces offer the same lifecycle.
	// Exactly one of DeprecateURL / ActivateURL is set per the entity's
	// current status. DeleteURL is destructive — the frontend confirms and
	// redirects to the entity list on success.
	DeleteURL    string `json:"delete_url,omitempty"`
	DeprecateURL string `json:"deprecate_url,omitempty"`
	ActivateURL  string `json:"activate_url,omitempty"`
}

type ConceptListCap struct {
	SearchEntriesURL string `json:"search_entries_url"`
	AddEntryURL      string `json:"add_entry_url,omitempty"`
	UpdateEntryURL   string `json:"update_entry_url,omitempty"`
	RemoveEntryURL   string `json:"remove_entry_url,omitempty"`
	ReorderURL       string `json:"reorder_url,omitempty"`
	// IsClosed marks a sealed list: membership is locked, so the UI disables
	// adding terms and shows a "sealed" badge (#3599).
	IsClosed bool `json:"is_closed"`
	// CreateTermURL POSTs a hand-authored term ({label, scope_note}) into the
	// project's local vocabulary and links it to this list.
	CreateTermURL string `json:"create_term_url,omitempty"`
	// SealURL PATCHes {is_closed} to lock/unlock membership.
	SealURL string `json:"seal_url,omitempty"`
	// BroaderURLTemplate is the per-term hierarchy endpoint (GET/POST, and
	// DELETE at .../{edgeID}); {conceptID} substitutes the vocabulary entry id.
	BroaderURLTemplate string `json:"broader_url_template,omitempty"`
	// SourceName is the display name of the list's source vocabulary (e.g.
	// "Art & Architecture Thesaurus"), so the UI can label "Add from {name}".
	SourceName string `json:"source_name,omitempty"`
	// HasRemoteSource is true when the source vocabulary is a remote authority
	// (aat/sparql/csv) rather than local/none. When false the add UI goes
	// straight to local term creation instead of a source search (#3599).
	HasRemoteSource bool `json:"has_remote_source,omitempty"`
	// SkosURL downloads the list as a SKOS concept scheme (Turtle). Read-only,
	// available to any viewer of the list (#3599).
	SkosURL string `json:"skos_url,omitempty"`
}

type ReleaseView struct {
	Version  string                `json:"version"`
	DraftURL string                `json:"draft_url,omitempty"`
	Label    pkgdomain.Localizable `json:"label"`
}

// ReuseSection is one relationship's usages, split into same-project buckets
// (Models/Collections — the "This project" sub-tab) and cross-project groups
// (OtherProjects — the "Other projects" sub-tab).
type ReuseSection struct {
	Models        []pkgdomainFieldUsageRef `json:"models"`
	Collections   []pkgdomainFieldUsageRef `json:"collections"`
	OtherProjects []ProjectUsageGroup      `json:"other_projects,omitempty"`
}

// FieldReuseResponse is the payload returned by the Reuse endpoint. It splits
// usages by relationship so the two are never conflated:
//   - IncludedIn: membership — this entity is a member of / bundled into the
//     listed models/collections (fields via overrides, collections via
//     part_of_collection).
//   - ReferencedBy: value-target — a field in the listed models/collections
//     points at this entity as its expected value type (weave_override_refs).
//
// A section is nil when the entity type cannot have that relationship:
// field → IncludedIn only; model → ReferencedBy only; collection → both.
type FieldReuseResponse struct {
	IncludedIn   *ReuseSection `json:"included_in,omitempty"`
	ReferencedBy *ReuseSection `json:"referenced_by,omitempty"`
}

// ProjectUsageGroup is one consuming project's usages of the field, for the
// "Other projects" tab.
type ProjectUsageGroup struct {
	ProjectID   string                   `json:"project_id"`
	ProjectName string                   `json:"project_name,omitempty"`
	Models      []pkgdomainFieldUsageRef `json:"models,omitempty"`
	Collections []pkgdomainFieldUsageRef `json:"collections,omitempty"`
}

type ConceptListEntryView struct {
	ID                string                         `json:"id"`
	VocabularyEntryID string                         `json:"vocabulary_entry_id"`
	URI               string                         `json:"uri,omitempty"`
	Label             pkgdomain.Translations         `json:"label,omitempty"`
	ScopeNote         pkgdomain.Translations         `json:"scope_note,omitempty"`
	ExternalID        string                         `json:"external_id,omitempty"`
	BroaderURI        string                         `json:"broader_uri,omitempty"`
	BroaderPathItems  []pkgdomain.VocabularyEntryRef `json:"broader_path_items,omitempty"`
	CustomLabel       pkgdomain.Translations         `json:"custom_label,omitempty"`
	Position          int                            `json:"position"`
}

// pkgdomainFieldUsageRef is a thin alias so the json shape is owned
// here in the detailview package even though the source struct lives
// in pkg/domain.
type pkgdomainFieldUsageRef = pkgdomain.FieldUsageRef

// DerivativesCap carries the URLs the visualization slice exposes for
// this entity. The frontend's DiagramTab reads these instead of
// constructing the URLs itself, matching the schema-as-contract rule
// (.claude/rules/api-patterns.md).
//
// All five URLs are emitted together when the visualization slice can
// generate the entity's snapshot — currently model, collection, and
// field. Nil on the response when derivatives aren't supported (e.g.
// a future entity type that has no path).
type DerivativesCap struct {
	DiagramURL string `json:"diagram_url,omitempty"`
	TurtleURL  string `json:"turtle_url,omitempty"`
	JSONLDURL  string `json:"jsonld_url,omitempty"`
	SHACLURL   string `json:"shacl_url,omitempty"`
	SPARQLURL  string `json:"sparql_url,omitempty"`
	X3MLAURL   string `json:"x3ml_a_url,omitempty"`
	X3MLBURL   string `json:"x3ml_b_url,omitempty"`
	// X3ML*ZipURL download the mapping plus each linked ontology's raw
	// schema file as a ZIP.
	X3MLAZipURL      string `json:"x3ml_a_zip_url,omitempty"`
	X3MLBZipURL      string `json:"x3ml_b_zip_url,omitempty"`
	ResearchSpaceURL string `json:"researchspace_url,omitempty"`
	// ArchesURL points at the Arches 7.6 Resource Graph JSON export for
	// models. Super-admin only while the contract is under active
	// development. Empty otherwise.
	ArchesURL string `json:"arches_url,omitempty"`
	// SnapshotURL points at the renderer-neutral Snapshot JSON (the tree
	// + path_node/path_node_id identities). Super-admin, models only — a
	// verification surface for the node-identity work.
	SnapshotURL string `json:"snapshot_url,omitempty"`
	// ASCIITreeURL points at a human-readable ASCII tree of the Snapshot
	// keyed by PathNodeID/InstanceID. Super-admin, models only — used by
	// curators to verify how every semantic intermediate is named and how
	// every leaf field is grouped (per review workflow).
	ASCIITreeURL   string `json:"ascii_tree_url,omitempty"`
	ExportGraphURL string `json:"exportgraph_url,omitempty"`
	// CytoscapeURL is the parallel graph format alongside Mermaid.
	// Emitted to all readers (any project reader) so Ekjs and
	// downstream consumers can fetch directly. omitempty until the
	// frontend renderer ships — reserves the contract slot without
	// asking DiagramTab to render anything yet.
	CytoscapeURL string `json:"cytoscape_url,omitempty"`
	// CSVURL is the per-entity CSV download served by pkg/weave/exports.
	// Member-only — empty for non-members so the frontend hides the
	// sub-tab. Lets curators see what an entity looks like in the
	// project verification dump.
	CSVURL string `json:"csv_url,omitempty"`

	// IntegrationActions surfaces per-project integration hub action
	// groups (e.g. "Upload to 3M") on the entity's diagram sub-tab.
	// Each entry bundles one logical action + the list of enabled
	// configs the operator can dispatch it to. The frontend renders a
	// target selector when Targets has >1 entry; falls back to a plain
	// button when single-target. Empty when no integration is enabled
	// for the project.
	IntegrationActions []formschema.ActionGroupSchema `json:"integration_actions,omitempty"`
}

type ViewSection struct {
	Widget         string                 `json:"widget"`
	ID             string                 `json:"id"`
	Name           pkgdomain.Translations `json:"name"`
	CanonicalOrder int                    `json:"canonical_order"`
	Items          []ViewItem             `json:"items"`
}

type ViewItem struct {
	Widget        string                  `json:"widget"`
	ID            string                  `json:"id"`
	Name          pkgdomain.Translations  `json:"name,omitempty"`
	OntologyScope string                  `json:"ontology_scope,omitempty"`
	FieldCount    int                     `json:"field_count"`
	Fields        []ViewField             `json:"fields"`
	SharedPrefix  []pkgdomain.PathElement `json:"shared_path_prefix,omitempty"`
	// Placement carries collection-group constraints for badge rendering.
	// Hidden groups never reach the read-only contract.
	Placement *pkgdomain.CollectionPlacement `json:"placement,omitempty"`
	// SourceURL is the canonical URL of the underlying entity (e.g. the
	// collection's standalone detail page). Populated for collection
	// groups so the frontend can render a "go to source" link in the
	// header without constructing the URL itself (api-patterns.md —
	// schema is the contract). Empty for the synthetic 'Direct Fields'
	// group whose ID is __direct__.
	SourceURL string `json:"source_url,omitempty"`
}

type ViewField struct {
	Widget              string                  `json:"widget"`
	OverrideID          int64                   `json:"override_id"`
	FieldID             string                  `json:"field_id"`
	FieldSemanticID     string                  `json:"field_semantic_id"`
	FieldSystemName     string                  `json:"field_system_name"`
	Origin              pkgdomain.Origin        `json:"origin"`
	Position            int                     `json:"position"`
	DisplayName         pkgdomain.Translations  `json:"display_name"`
	Description         pkgdomain.Translations  `json:"description,omitempty"`
	ExpectedValueType   string                  `json:"expected_value_type,omitempty"`
	SetValue            string                  `json:"set_value,omitempty"`
	IsRequired          bool                    `json:"is_required"`
	IsHidden            bool                    `json:"is_hidden"`
	OntologyPath        string                  `json:"ontology_path,omitempty"`
	PathElements        []pkgdomain.PathElement `json:"path_elements,omitempty"`
	ResourceModelRefs   []pkgdomain.EntityRef   `json:"resource_model_refs,omitempty"`
	CollectionModelRefs []pkgdomain.EntityRef   `json:"collection_model_refs,omitempty"`
	ConceptListRefs     []pkgdomain.EntityRef   `json:"concept_list_refs,omitempty"`
	CategoryID          string                  `json:"category_id,omitempty"`
	CollectionID        string                  `json:"collection_id,omitempty"`
	// SourceFieldURL is the standalone field detail page URL for the
	// underlying field (e.g. /projects/LA/fields/LAF.10). Lets the
	// frontend render the field semantic-ID chip as a click-through to
	// the source without constructing URLs itself. Honours inheritance:
	// when the field is inherited from another project the URL points
	// to the source project.
	SourceFieldURL string `json:"source_field_url,omitempty"`
	// SourceCollectionURL is the standalone collection detail page URL
	// for the wrapping collection (when CollectionID is set). Drives
	// the "from {collection}" link on field rows.
	SourceCollectionURL string `json:"source_collection_url,omitempty"`
}

type ViewRefs struct {
	Categories  map[string]ViewRefEntry `json:"categories,omitempty"`
	Collections map[string]ViewRefEntry `json:"collections,omitempty"`
	Models      map[string]ViewRefEntry `json:"models,omitempty"`
}

type ViewRefEntry struct {
	Name   pkgdomain.Translations `json:"name"`
	Scope  string                 `json:"scope,omitempty"`
	Order  int                    `json:"order,omitempty"`
	URL    string                 `json:"url,omitempty"`
	Origin pkgdomain.Origin       `json:"origin,omitempty"`
}

type AdoptionItem struct {
	EntityType      string                 `json:"entity_type"`
	SourceProjectID string                 `json:"source_project_id"`
	SourceEntityID  string                 `json:"source_entity_id"`
	SourceURL       string                 `json:"source_url,omitempty"`
	AdoptedAt       time.Time              `json:"adopted_at"`
	CreatedBy       pkgdomain.ActorRef     `json:"created_by,omitempty"`
	Origin          pkgdomain.Origin       `json:"origin"`
	Label           pkgdomain.Translations `json:"label"`
}

type Response = EntityViewResponse
type Meta = EntityViewMeta
type Capabilities = ViewCapabilities
type Section = ViewSection
type Item = ViewItem
type Field = ViewField
type Refs = ViewRefs
type RefEntry = ViewRefEntry
type ScopeInfo = ViewScopeInfo
type Release = ReleaseView
