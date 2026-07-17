package domain

import (
	"encoding/json"
	"time"
)

// OntologyFamily groups related ontologies (e.g. "CIDOC CRM Family",
// "Schema.org Family"). Families may nest via ParentFamilyID.
type OntologyFamily struct {
	ID             string       `json:"id"`
	Slug           string       `json:"slug"`
	Name           string       `json:"name"`
	Description    Translations `json:"description,omitempty"`
	ParentFamilyID *string      `json:"parent_family_id,omitempty"`
	HomepageURL    string       `json:"homepage_url,omitempty"`
	Icon           string       `json:"icon,omitempty"`
	DisplayOrder   int64        `json:"display_order"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// OntologyType marks an ontology as a base or extension. Extensions
// declare ExtendsOntologyID to chain off a base ontology.
type OntologyType string

const (
	OntologyTypeBase      OntologyType = "base"
	OntologyTypeExtension OntologyType = "extension"
)

// Ontology is a registered semantic ontology (CRM, SKOS, custom
// extensions). The (Prefix, Namespace) pair is the key bridge between
// RDF and weave: RDF refers to URIs; weave queries by qname; the
// ontology row stores the conversion.
type Ontology struct {
	ID                string       `json:"id"`
	Prefix            string       `json:"prefix"`
	Namespace         string       `json:"namespace"`
	Name              string       `json:"name"`
	Description       Translations `json:"description,omitempty"`
	FamilyID          *string      `json:"family_id,omitempty"`
	OntologyType      OntologyType `json:"ontology_type"`
	ExtendsOntologyID *string      `json:"extends_ontology_id,omitempty"`
	HomepageURL       string       `json:"homepage_url,omitempty"`
	SourceURL         string       `json:"source_url,omitempty"`
	CreatedByID       string       `json:"created_by_id,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

// IsBase reports whether the ontology is a base (non-extension). The
// empty zero value is treated as base for legacy compatibility.
func (o *Ontology) IsBase() bool {
	return o.OntologyType == OntologyTypeBase || o.OntologyType == ""
}

// IsExtension reports whether the ontology extends a base ontology.
func (o *Ontology) IsExtension() bool {
	return o.OntologyType == OntologyTypeExtension
}

// OntologyVersion is one importable snapshot of an Ontology. Multiple
// versions may exist; at most one is active per ontology (enforced by a
// partial unique index in migration 029).
type OntologyVersion struct {
	ID            string `json:"id"`
	OntologyID    string `json:"ontology_id"`
	VersionString string `json:"version_string"`
	IsActive      bool   `json:"is_active"`

	// CompatibleBaseVersions records which base-ontology versions an
	// extension claims compatibility with. Empty for base ontologies.
	CompatibleBaseVersions []string `json:"compatible_base_versions,omitempty"`

	// RDFContent is the raw imported RDF source (Turtle/RDF-XML).
	// Optional — may be empty for fully-derived versions.
	RDFContent string     `json:"rdf_content,omitempty"`
	ParsedAt   *time.Time `json:"parsed_at,omitempty"`

	OriginalFilename string `json:"original_filename,omitempty"`
	FileSize         int64  `json:"file_size,omitempty"`
	FileMD5          string `json:"file_md5,omitempty"`

	OntologyURI        string       `json:"ontology_uri,omitempty"`
	VersionIRI         string       `json:"version_iri,omitempty"`
	VersionInfo        Translations `json:"version_info,omitempty"`
	ImportedOntologies []string     `json:"imported_ontologies,omitempty"`

	OntologyLabel    Translations    `json:"ontology_label,omitempty"`
	OntologyComment  Translations    `json:"ontology_comment,omitempty"`
	OntologyMetadata json.RawMessage `json:"ontology_metadata,omitempty"`

	ClassCount    int64 `json:"class_count"`
	PropertyCount int64 `json:"property_count"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OntologyClass is one parsed class within an OntologyVersion. Hierarchy
// (subclass_of, equivalent_class, disjoint_class) is in the polymorphic
// weave_ontology_relations table — read via the slice's Store, not a
// field on this struct.
type OntologyClass struct {
	ID                string       `json:"id"`
	OntologyVersionID string       `json:"ontology_version_id"`
	Prefix            string       `json:"prefix"`
	LocalName         string       `json:"local_name"`
	URI               string       `json:"uri"`
	Qname             string       `json:"qname"`
	Label             Translations `json:"label,omitempty"`
	Comment           Translations `json:"comment,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

// OntologyProperty is one parsed property within an OntologyVersion.
// Domain/range/inverse/sub/equivalent edges live in weave_ontology_relations.
type OntologyProperty struct {
	ID                string       `json:"id"`
	OntologyVersionID string       `json:"ontology_version_id"`
	Prefix            string       `json:"prefix"`
	LocalName         string       `json:"local_name"`
	URI               string       `json:"uri"`
	Qname             string       `json:"qname"`
	Label             Translations `json:"label,omitempty"`
	Comment           Translations `json:"comment,omitempty"`
	PropertyType      string       `json:"property_type,omitempty"`

	IsFunctional        bool `json:"is_functional"`
	IsInverseFunctional bool `json:"is_inverse_functional"`
	IsTransitive        bool `json:"is_transitive"`
	IsSymmetric         bool `json:"is_symmetric"`
	IsAsymmetric        bool `json:"is_asymmetric"`
	IsReflexive         bool `json:"is_reflexive"`
	IsIrreflexive       bool `json:"is_irreflexive"`

	InversePropertyURI string `json:"inverse_property_uri,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OntologyRelation is one edge in the polymorphic relations table.
// Covers: subclass_of, subproperty_of, equivalent_class,
// equivalent_property, disjoint_class, disjoint_property, domain, range,
// inverse_property. Future RDF axioms slot in as new RelType values
// without schema changes.
type OntologyRelation struct {
	SourceID    string `json:"source_id"`
	SourceKind  string `json:"source_kind"` // "class" | "property"
	RelType     string `json:"rel_type"`
	TargetQname string `json:"target_qname"`
	Position    int    `json:"position"`
}

// OntologyRelationWithSource is an OntologyRelation plus the source's qname,
// returned by bulk relation reads so the index builder can wire edges
// without a follow-up lookup.
type OntologyRelationWithSource struct {
	OntologyRelation
	SourceQname string `json:"source_qname"`
}

// ProjectOntologyVersion is the link row joining a project to one
// ontology_version. Composite primary key (ProjectID, OntologyVersionID)
// — there is no surrogate ID. AddedBy is denormalised onto the row at
// link time so callers can render "added by X" without a separate
// actor lookup.
type ProjectOntologyVersion struct {
	ProjectID         string     `json:"project_id"`
	OntologyVersionID string     `json:"ontology_version_id"`
	AddedAt           time.Time  `json:"added_at"`
	AddedByID         *string    `json:"added_by_id,omitempty"`
	IsPrimary         bool       `json:"is_primary"`
	UsageNotes        string     `json:"usage_notes,omitempty"`

	// OntologyVersion is the joined version row when the caller asked
	// for it. nil when the link is fetched without the join.
	OntologyVersion *OntologyVersion `json:"ontology_version,omitempty"`
}

// OntologyBundleEntry is one ontology a project links, paired with its
// raw schema content. It feeds the X3ML <target> blocks and the
// downloadable ZIP bundle. RDFContent and
// OriginalFilename are empty for ontologies authored in Pletka that
// have no imported schema file.
type OntologyBundleEntry struct {
	Prefix           string
	Namespace        string
	Name             string
	VersionString    string
	OriginalFilename string
	RDFContent       string
}

// FieldOntologyRef materializes one path-element-to-ontology-qname link
// from a field's path_elements JSONB. Populated by the field slice on
// save; consumed by unlink-preflight + reverse "fields using qname"
// lookup.
type FieldOntologyRef struct {
	FieldID    string `json:"field_id"`
	ProjectID  string `json:"project_id"`
	Prefix     string `json:"prefix"`
	LocalName  string `json:"local_name"`
	Qname      string `json:"qname"`
	Position   int    `json:"position"`
	RefKind    string `json:"ref_kind"` // "class" | "property" | "literal"
}
