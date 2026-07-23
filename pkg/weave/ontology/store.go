package ontology

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Store is the data-access contract for the master ontology slice.
// Read-only at this stage (Phase A step 3); writes land in Phase B.
//
// The interface is chunky on purpose — many of the read methods are
// version-scoped and a single Service call typically composes several
// (e.g. "render version detail" needs version + classes + properties).
// Splitting into per-entity sub-stores adds plumbing without buying
// anything; one interface per slice matches every other vertical slice
// in pkg/weave/.
//
// The Phase D autocomplete subpackage takes Store and runs its own
// recursive CTEs over weave_ontology_relations via pgxpool, since
// sqlc's parser doesn't tolerate the recursive arms of those queries.
type Store interface {
	// ---- Families ----

	GetFamily(ctx context.Context, id string) (*domain.OntologyFamily, error)
	GetFamilyBySlug(ctx context.Context, slug string) (*domain.OntologyFamily, error)
	ListFamilies(ctx context.Context) ([]*domain.OntologyFamily, error)
	ListRootFamilies(ctx context.Context) ([]*domain.OntologyFamily, error)
	ListChildFamilies(ctx context.Context, parentID string) ([]*domain.OntologyFamily, error)

	// ---- Ontologies ----

	GetOntology(ctx context.Context, id string) (*domain.Ontology, error)
	GetOntologyByPrefix(ctx context.Context, prefix string) (*domain.Ontology, error)
	ListOntologies(ctx context.Context) ([]*domain.Ontology, error)
	ListOntologiesByFamily(ctx context.Context, familyID string) ([]*domain.Ontology, error)
	ListOntologiesByType(ctx context.Context, ontologyType domain.OntologyType) ([]*domain.Ontology, error)
	ListOntologyExtensions(ctx context.Context, baseOntologyID string) ([]*domain.Ontology, error)
	SearchOntologies(ctx context.Context, query string, limit int) ([]*domain.Ontology, error)

	// NamespaceBindings returns (prefix, namespace) pairs ordered by namespace
	// length DESC. Used by the importer + path validator to resolve URIs
	// to qnames; longer namespaces first ensures the most-specific
	// binding wins.
	NamespaceBindings(ctx context.Context) ([]NamespaceBinding, error)

	// UpsertNamespaceBindings persists global namespace bindings using the
	// same precedence rules as RDF import: existing higher-weight bindings
	// are preserved, equal/lower-weight bindings are refreshed.
	UpsertNamespaceBindings(ctx context.Context, bindings []NamespaceBinding) error

	// DeleteNamespaceBindingsBySource removes global namespace bindings for a
	// maintenance source such as prefix.cc.
	DeleteNamespaceBindingsBySource(ctx context.Context, source string) error

	// ---- Versions ----

	GetVersion(ctx context.Context, id string) (*domain.OntologyVersion, error)
	GetVersionByOntologyAndString(ctx context.Context, ontologyID, versionString string) (*domain.OntologyVersion, error)
	GetActiveVersion(ctx context.Context, ontologyID string) (*domain.OntologyVersion, error)
	ListVersionsByOntology(ctx context.Context, ontologyID string) ([]*domain.OntologyVersion, error)

	// VersionUsageCount returns the number of projects linked to the version.
	VersionUsageCount(ctx context.Context, versionID string) (int64, error)
	ListProjectsUsingVersion(ctx context.Context, versionID string, limit int) ([]VersionProjectUsage, error)

	// VersionUsageCountsForOntology returns project-count per version for
	// one ontology in a single query — drives the version list view's
	// "linked to N projects" badge.
	VersionUsageCountsForOntology(ctx context.Context, ontologyID string) (map[string]int64, error)

	// ---- Classes ----

	GetClass(ctx context.Context, id string) (*domain.OntologyClass, error)
	GetClassByURI(ctx context.Context, versionID, uri string) (*domain.OntologyClass, error)
	GetClassByQname(ctx context.Context, versionID, qname string) (*domain.OntologyClass, error)
	ListClassesByVersion(ctx context.Context, versionID string) ([]*domain.OntologyClass, error)
	ListClassesByQnames(ctx context.Context, versionID string, qnames []string) ([]*domain.OntologyClass, error)
	// ListSubclassesByQname returns the direct subclasses of parentQname
	// in this version. Caller does breadth-first descent for transitive
	// coverage. Used by autocomplete to expand range suggestions —
	// P1's range is E41_Appellation, dropdown should also offer
	// E33_E41_Linguistic_Appellation since it subclasses E41.
	ListSubclassesByQname(ctx context.Context, versionID, parentQname string) ([]*domain.OntologyClass, error)
	SearchClasses(ctx context.Context, versionID, query string, limit int) ([]*domain.OntologyClass, error)

	// ---- Properties ----

	GetProperty(ctx context.Context, id string) (*domain.OntologyProperty, error)
	GetPropertyByURI(ctx context.Context, versionID, uri string) (*domain.OntologyProperty, error)
	GetPropertyByQname(ctx context.Context, versionID, qname string) (*domain.OntologyProperty, error)
	ListPropertiesByVersion(ctx context.Context, versionID string) ([]*domain.OntologyProperty, error)
	ListPropertiesByQnames(ctx context.Context, versionID string, qnames []string) ([]*domain.OntologyProperty, error)
	SearchProperties(ctx context.Context, versionID, query string, limit int) ([]*domain.OntologyProperty, error)
	PropertiesForDomainQname(ctx context.Context, versionID, qname string) ([]*domain.OntologyProperty, error)
	PropertiesForRangeQname(ctx context.Context, versionID, qname string) ([]*domain.OntologyProperty, error)
	GetInverseProperty(ctx context.Context, versionID, propertyID string) (*domain.OntologyProperty, error)

	// ---- Relations (read) ----

	ListRelationsForSource(ctx context.Context, sourceID, sourceKind string) ([]*domain.OntologyRelation, error)
	ListRelationsForSourceByType(ctx context.Context, sourceID, sourceKind, relType string) ([]*domain.OntologyRelation, error)
	ListRelationsForTarget(ctx context.Context, targetQname, relType string) ([]*domain.OntologyRelation, error)

	// ---- Bulk reads (by version-id set; feed the per-project autocomplete index) ----

	// ListClassesByVersions returns all classes across the supplied version IDs.
	ListClassesByVersions(ctx context.Context, versionIDs []string) ([]*domain.OntologyClass, error)
	// ListPropertiesByVersions returns all properties across the supplied version IDs.
	ListPropertiesByVersions(ctx context.Context, versionIDs []string) ([]*domain.OntologyProperty, error)
	// ListRelationsByVersionsAndTypes returns relations of the requested types
	// for sources belonging to the supplied version IDs. Each row includes the
	// source qname so the index builder can wire edges without a second lookup.
	ListRelationsByVersionsAndTypes(ctx context.Context, versionIDs, relTypes []string) ([]*domain.OntologyRelationWithSource, error)

	// ---- Field ontology refs (read) ----

	ListFieldRefs(ctx context.Context, fieldID string) ([]*domain.FieldOntologyRef, error)
	FindFieldsUsingQname(ctx context.Context, projectID, qname string, limit int) ([]string, error)
	CountFieldsUsingQname(ctx context.Context, projectID, qname string) (int64, error)
	CountFieldsUsingVersion(ctx context.Context, projectID, versionID string) (int64, error)
	SampleFieldsUsingVersion(ctx context.Context, projectID, versionID string, limit int) ([]FieldVersionRef, error)

	// ---- Writes (Phase C; admin CRUD calls land in Phase B against this surface) ----

	CreateFamily(ctx context.Context, in CreateFamilyInput) (*domain.OntologyFamily, error)
	UpdateFamily(ctx context.Context, id string, in UpdateFamilyInput) (*domain.OntologyFamily, error)
	DeleteFamily(ctx context.Context, id string) error
	CountOntologiesInFamily(ctx context.Context, familyID string) (int64, error)

	CreateOntology(ctx context.Context, in CreateOntologyInput) (*domain.Ontology, error)
	UpdateOntology(ctx context.Context, id string, in UpdateOntologyInput) (*domain.Ontology, error)
	DeleteOntology(ctx context.Context, id string) error
	CountOntologyVersions(ctx context.Context, ontologyID string) (int64, error)

	CreateVersion(ctx context.Context, in CreateVersionInput) (*domain.OntologyVersion, error)
	UpdateVersionMetadata(ctx context.Context, id string, in UpdateVersionMetadataInput) (*domain.OntologyVersion, error)
	UpdateVersionCounts(ctx context.Context, id string, classCount, propertyCount int64) error
	SetActiveVersion(ctx context.Context, ontologyID, versionID string) error
	DeleteVersion(ctx context.Context, id string) error

	// ImportVersion is the high-level transactional entry for the importer.
	// Atomic: writes the version row, replaces classes/properties for that
	// version, and replaces relations for those classes/properties. On
	// error the entire transaction rolls back.
	ImportVersion(ctx context.Context, in ImportVersionInput) (*domain.OntologyVersion, error)

	// ReplaceFieldRefs bulk-replaces the ontology-ref rows for one field.
	// Called by pkg/weave/field on save (Phase E).
	ReplaceFieldRefs(ctx context.Context, fieldID, projectID string, refs []domain.FieldOntologyRef) error
}

const (
	NamespaceBindingSourceRDFDeclared = "rdf-declared"
	NamespaceBindingWeightRDFDeclared = int64(80)
	// Version-derived bindings come from import metadata rather than RDF
	// declarations. They are weaker than curator/RDF declarations but stronger
	// than generic seed data.
	NamespaceBindingSourceVersionDerived = "version-derived"
	NamespaceBindingWeightVersionDerived = int64(40)
)

// NamespaceBinding is one (prefix, namespace) pair available to ontology
// import and qname resolution. Store.NamespaceBindings returns these ordered
// by namespace length DESC.
type NamespaceBinding struct {
	Prefix     string
	Namespace  string
	Weight     int64
	Source     string
	OntologyID *string
}

// FieldVersionRef pairs a field id with the qname that links it to an
// ontology version, for unlink-preflight 409 payloads.
type FieldVersionRef struct {
	FieldID string `json:"field_id"`
	Qname   string `json:"qname"`
}

// VersionProjectUsage is one project link to an ontology version.
type VersionProjectUsage struct {
	ProjectID   string
	ProjectName string
	AddedAt     time.Time
	IsPrimary   bool
}

// ---------------------------------------------------------------------------
// Write inputs — value-typed so the Store interface is decoupled from
// postgres specifics.
// ---------------------------------------------------------------------------

type CreateFamilyInput struct {
	ID             string
	Slug           string
	Name           string
	Description    domain.Translations
	ParentFamilyID *string
	HomepageURL    string
	Icon           string
	DisplayOrder   int64
}

type UpdateFamilyInput struct {
	Slug           string
	Name           string
	Description    domain.Translations
	ParentFamilyID *string
	HomepageURL    string
	Icon           string
	DisplayOrder   int64
}

type CreateOntologyInput struct {
	ID                string
	Prefix            string
	Namespace         string
	Name              string
	Description       domain.Translations
	FamilyID          *string
	OntologyType      domain.OntologyType
	ExtendsOntologyID *string
	HomepageURL       string
	SourceURL         string
	CreatedByID       string
}

type UpdateOntologyInput struct {
	Prefix            string
	Namespace         string
	Name              string
	Description       domain.Translations
	FamilyID          *string
	OntologyType      domain.OntologyType
	ExtendsOntologyID *string
	HomepageURL       string
	SourceURL         string
}

type CreateVersionInput struct {
	ID                     string
	OntologyID             string
	VersionString          string
	IsActive               bool
	CompatibleBaseVersions []string
	RDFContent             string
	ParsedAt               *time.Time
	OriginalFilename       string
	FileSize               int64
	FileMD5                string
	OntologyURI            string
	VersionIRI             string
	VersionInfo            domain.Translations
	ImportedOntologies     []string
	OntologyLabel          domain.Translations
	OntologyComment        domain.Translations
	OntologyMetadata       json.RawMessage
	ClassCount             int64
	PropertyCount          int64
}

type UpdateVersionMetadataInput struct {
	VersionString          string
	CompatibleBaseVersions []string
	OntologyURI            string
	VersionIRI             string
	VersionInfo            domain.Translations
	ImportedOntologies     []string
	OntologyLabel          domain.Translations
	OntologyComment        domain.Translations
	OntologyMetadata       json.RawMessage
}

// ImportVersionInput carries the full payload for one ImportVersion call.
// Classes / Properties / Relations are bulk-written; the version row is
// upserted in the same transaction.
type ImportVersionInput struct {
	Version           CreateVersionInput
	SetActive         bool
	NamespaceBindings []NamespaceBinding
	Classes           []ImportClass
	Properties        []ImportProperty
	// Relations covers all polymorphic edges between classes/properties
	// in this version (subclass_of, subproperty_of, equivalent_*,
	// disjoint_*, domain, range, inverse_property).
	Relations []domain.OntologyRelation
	// CompanionFiles carries the raw bytes of every companion source the
	// version was parsed from (e.g. CIDOC-CRM's PC module). Persisted to
	// weave_ontology_version_companions so self-contained snapshots can
	// vendor every source file, not just the primary rdf_content.
	CompanionFiles []CompanionFileInput
}

// CompanionFileInput is one companion RDF source to persist verbatim.
type CompanionFileInput struct {
	Filename    string
	Description string
	Content     string
}

// ImportClass is the input shape for one class row inside ImportVersion.
// Identity (ID + URI + LocalName + Prefix) is supplied by the caller —
// the importer derives the deterministic ID via id_helpers and resolves
// prefix from the namespace bindings before calling.
type ImportClass struct {
	ID        string
	URI       string
	Prefix    string
	LocalName string
	Label     domain.Translations
	Comment   domain.Translations
	// SourceModule names the companion file this class came from, or is
	// empty when the class belongs to the version's base file.
	SourceModule string
}

// ImportProperty is the input shape for one property row inside
// ImportVersion. Mirrors ImportClass + property-specific OWL flags.
type ImportProperty struct {
	ID                  string
	URI                 string
	Prefix              string
	LocalName           string
	Label               domain.Translations
	Comment             domain.Translations
	PropertyType        string
	IsFunctional        bool
	IsInverseFunctional bool
	IsTransitive        bool
	IsSymmetric         bool
	IsAsymmetric        bool
	IsReflexive         bool
	IsIrreflexive       bool
	InversePropertyURI  string
	// SourceModule names the companion file this property came from, or
	// is empty when the property belongs to the version's base file.
	SourceModule string
}
