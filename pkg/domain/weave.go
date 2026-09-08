package domain

import (
	"context"
	"time"
)

// WeaveStore is the data access + resolution layer for the core domain.
// Fields, models, collections, and categories are entangled by the override
// chain — WeaveStore keeps them together.
type WeaveStore interface {
	Models() WeaveModelStore
	Collections() WeaveCollectionStore
	Overrides() OverrideStore
	Adoptions() AdoptionStore
	Forks() ForkStore
	Projects() WeaveProjectStore
	ProjectInheritances() ProjectInheritanceStore
	WeaveFields() WeaveFieldStore
	WeaveCategories() WeaveCategoryStore
	Memberships() MembershipStore
	Auth() AuthStore
	ProjectOntologyVersions() ProjectOntologyVersionStore
	NamespaceBindings() NamespaceBindingStore
	Vocabularies() VocabularyStore
	ConceptLists() ConceptListStore
	ModelView(ctx context.Context, modelID, projectID string) (*ModelView, error)
	CollectionView(ctx context.Context, collectionID, projectID string) ([]ResolvedField, error)

	// AllocateEntityNumber atomically reserves the next sequential number for
	// a (project, kind) pair via the weave_entity_counters table. kind is one
	// of "model", "collection", "field", "category". Concurrent callers each
	// receive a distinct number — the underlying UPSERT serializes within
	// PostgreSQL.
	AllocateEntityNumber(ctx context.Context, projectID, kind string) (int64, error)

	// ReconcileEntityCounters bumps every counter to MAX(existing N) + 1.
	// Call after a bulk import that wrote entities with explicit IDs so
	// subsequent schema-driven creates don't collide with imported numbers.
	ReconcileEntityCounters(ctx context.Context) error
}

// OverrideEntry represents a model/collection field override referencing a category.
type OverrideEntry struct {
	ID         string `json:"id"`
	ParentName string `json:"parent_name"`
	FieldName  string `json:"field_name"`
}

// FieldModelRef represents a model that uses a given field (via model_fields junction).
type FieldModelRef struct {
	ModelID   string `json:"model_id"`
	ModelName string `json:"model_name"`
}

// FieldCollectionRef represents a collection that uses a given field (via collection_fields junction).
type FieldCollectionRef struct {
	CollectionID   string `json:"collection_id"`
	CollectionName string `json:"collection_name"`
}

// ---------------------------------------------------------------------------
// New weave interfaces using domain types. The legacy interfaces above remain
// for backward compatibility during the strangler-fig migration.
// ---------------------------------------------------------------------------

// WeaveCategoryStore handles CRUD and queries for categories using domain types.
type WeaveCategoryStore interface {
	Create(ctx context.Context, category *Category) error
	GetByID(ctx context.Context, id string) (*Category, error)
	Update(ctx context.Context, category *Category) error
	UpdateFields(ctx context.Context, id string, fields map[string]any) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, opts ...QueryOption) ([]*Category, error)
	Count(ctx context.Context, opts ...QueryOption) (int64, error)
	Reorder(ctx context.Context, projectID string, categoryIDs []string) error

	// Domain-specific queries
	GetByIdentifier(ctx context.Context, identifier string, projectID string) (*Category, error)
	ListWithCounts(ctx context.Context, projectID string) ([]WeaveCategoryWithCounts, error)

	// Delete with cascade (reassign fields, clear overrides)
	DeleteWithReassignment(ctx context.Context, id string, reassignTo string, semanticID string) error

	// Stats queries for the category usage modal (single-query joins, no N+1)
	ModelFieldOverrides(ctx context.Context, categoryID string, projectID string) ([]OverrideEntry, error)
	CollectionFieldOverrides(ctx context.Context, semanticID string, projectID string) ([]OverrideEntry, error)
}

// WeaveFieldStore handles CRUD and path queries for fields using domain types.
type WeaveFieldStore interface {
	Create(ctx context.Context, field *Field) error
	GetByID(ctx context.Context, id string) (*Field, error)
	Update(ctx context.Context, field *Field) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, opts ...QueryOption) ([]*Field, int64, error)
	FindByPathSequence(ctx context.Context, query PathQuery) ([]*Field, error)
	// CountUsage returns how many models / collections in the project
	// reference this field, plus the total override-row count (per-model
	// + per-collection rows). Drives the field item view's Stats tab.
	CountUsage(ctx context.Context, fieldID, projectID string) (FieldUsageCounts, error)
	// ListUsage returns the full set of models and collections in the
	// project that reference this field via weave_field_overrides
	// (entity_type IN ('model','collection')). Drives the Reuse tab on
	// the field item view. Same-project only — cross-
	// project usage waits on explicit-adoption infra.
	ListUsage(ctx context.Context, fieldID, projectID string) (FieldUsageList, error)
	// ListReceiptFieldClosure — see WeaveModelStore equivalent.
	ListReceiptFieldClosure(ctx context.Context, seedID, seedKind string) ([]string, error)
	// ListReceiptFieldDirect — see WeaveModelStore equivalent.
	ListReceiptFieldDirect(ctx context.Context, seedID, seedKind string) ([]string, error)
	// ListReferenceAdopted returns fields from another project that
	// the current project's overrides reference via field_id. Used by
	// the list rule + DetailView adopted_reference detection.
	ListReferenceAdopted(ctx context.Context, projectID string) ([]*Field, error)
}

// FieldUsageCounts is the result of WeaveFieldStore.CountUsage.
type FieldUsageCounts struct {
	ModelsUsing      int
	CollectionsUsing int
	OverrideRowCount int
}

// CompositionCounts are the per-entity composition aggregates a model or
// collection list row shows as badges: how many fields compose it, and (models
// only) how many distinct categories and collections those fields span.
// Computed in one batch query per list page, grouped by entity_id over
// weave_field_overrides. entity_id identifies the entity uniquely, so no
// project filter is needed.
type CompositionCounts struct {
	FieldCount      int `json:"field_count"`
	CategoryCount   int `json:"category_count"`
	CollectionCount int `json:"collection_count"`
}

// FieldListCounts are the per-field usage aggregates a list row shows as
// badges: same-project model/collection usage, the number of OTHER projects
// that use the field (cross-project reuse), and an in-use flag (referenced
// anywhere) that gates the delete action. Computed in one batch query per list
// page, not per row.
type FieldListCounts struct {
	ModelCount        int  `json:"model_count"`
	CollectionCount   int  `json:"collection_count"`
	OtherProjectCount int  `json:"other_project_count"`
	InUse             bool `json:"in_use"`
}

// FieldUsageList is the result of WeaveFieldStore.ListUsage.
type FieldUsageList struct {
	Models      []FieldUsageRef
	Collections []FieldUsageRef
}

// FieldUsageRef is a single usage row — a model or collection that
// references the field via weave_field_overrides.
type FieldUsageRef struct {
	ID          string       `json:"id"`
	SemanticID  string       `json:"semantic_id,omitempty"`
	SystemName  string       `json:"system_name,omitempty"`
	Name        Translations `json:"name"`
	Description Translations `json:"description,omitempty"`
	URL         string       `json:"url,omitempty"`
	// ProjectID is the project that OWNS the referencing model/collection —
	// i.e. the consuming project. Equal to the field's project for same-project
	// usage, different for cross-project reuse. ProjectName is resolved for
	// display by the handler (not carried on the store row).
	ProjectID   string `json:"project_id,omitempty"`
	ProjectName string `json:"project_name,omitempty"`
}

// VocabularyStore handles CRUD for vocabularies and entries.
type VocabularyStore interface {
	CreateVocabulary(ctx context.Context, vocab *Vocabulary) error
	GetVocabulary(ctx context.Context, id string) (*Vocabulary, error)
	ListVocabularies(ctx context.Context) ([]*Vocabulary, error)

	CreateEntry(ctx context.Context, entry *VocabularyEntry) error
	GetEntry(ctx context.Context, id string) (*VocabularyEntry, error)
	SearchEntries(ctx context.Context, vocabularyID string, query string) ([]*VocabularyEntry, error)
}

// ConceptListStore handles CRUD for concept lists and their entries.
type ConceptListStore interface {
	Create(ctx context.Context, list *ConceptList) error
	GetByID(ctx context.Context, id string) (*ConceptList, error)
	List(ctx context.Context, projectID string) ([]*ConceptList, error)
	FindByListType(ctx context.Context, listTypeID string) ([]*ConceptList, error)

	AddEntry(ctx context.Context, entry *ConceptListEntry) error
	RemoveEntry(ctx context.Context, id string) error
	ListEntries(ctx context.Context, conceptListID string) ([]*ConceptListEntry, error)
	ReorderEntries(ctx context.Context, conceptListID string, entryIDs []string) error
}

// WeaveCategoryWithCounts is a category with usage counts (domain types only).
type WeaveCategoryWithCounts struct {
	Category
	FieldCount           int64 `json:"field_count"`
	ModelFieldCount      int64 `json:"model_field_count"`
	CollectionFieldCount int64 `json:"collection_field_count"`
}

// WeaveFieldWithCounts is a field with usage counts (domain types only).
type WeaveFieldWithCounts struct {
	Field
	ModelCount      int64 `json:"model_count"`
	CollectionCount int64 `json:"collection_count"`
}

// WeaveModelStore handles CRUD for models using domain types.
type WeaveModelStore interface {
	Create(ctx context.Context, model *Model) error
	GetByID(ctx context.Context, id string) (*Model, error)
	Update(ctx context.Context, model *Model) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, opts ...QueryOption) ([]*Model, int64, error)
	GetByIdentifier(ctx context.Context, identifier string, projectID string) (*Model, error)
	// ListOptions returns a lightweight {id, system_name, ui_name, status}
	// view scoped to a project. Used by ref-picker dropdowns. No COUNT
	// query, no full-row payload, no search filter (frontend filters
	// in-memory once the small list is loaded).
	ListOptions(ctx context.Context, projectID string) ([]EntityOption, error)
	// ListConnectedModelIDs returns the transitive closure of model IDs
	// reachable from seedIDs via override refs. Drives the Arches
	// generator's full-closure walk — Q8 of the adopt/adapt rollout.
	ListConnectedModelIDs(ctx context.Context, seedIDs []string) ([]string, error)
	// ListReceiptModelClosure returns models in the closure rooted at
	// a single seed entity. Used by the Adoptions tab's per-receipt
	// bill-of-materials.
	ListReceiptModelClosure(ctx context.Context, seedID, seedKind string) ([]string, error)
	// ListReceiptModelDirect returns models the seed references
	// directly (depth-1). Pairs with the closure variant for the
	// direct-vs-transitive count split.
	ListReceiptModelDirect(ctx context.Context, seedID, seedKind string) ([]string, error)
	// ListReferenceAdopted returns models from another project that
	// the current project's overrides reference as a value target.
	// Drives the four-state list rule (task 3b) and the
	// adopted_reference detection in DetailView.
	ListReferenceAdopted(ctx context.Context, projectID string) ([]*Model, error)

	// ListUsage returns the models/collections (across all projects) that
	// contain a field targeting this model as a value type — the model Reuse
	// tab. Cross-project; the handler partitions by project.
	ListUsage(ctx context.Context, modelID string) (FieldUsageList, error)
}

// WeaveCollectionStore handles CRUD for collections using domain types.
type WeaveCollectionStore interface {
	Create(ctx context.Context, collection *Collection) error
	GetByID(ctx context.Context, id string) (*Collection, error)
	Update(ctx context.Context, collection *Collection) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, opts ...QueryOption) ([]*Collection, int64, error)
	GetByIdentifier(ctx context.Context, identifier string, projectID string) (*Collection, error)
	// ListOptions — see ModelStore.ListOptions.
	ListOptions(ctx context.Context, projectID string) ([]EntityOption, error)
	// ListUsage returns the models in projectID that include this
	// collection (i.e. weave_field_overrides rows where entity_type =
	// 'model' and part_of_collection_id = collectionID). Drives the
	// Reuse tab on the collection item view. Same-project only;
	// cross-project reuse waits on explicit-adoption.
	ListUsage(ctx context.Context, collectionID, projectID string) ([]FieldUsageRef, error)
	// ListReceiptCollectionClosure — see WeaveModelStore equivalent.
	ListReceiptCollectionClosure(ctx context.Context, seedID, seedKind string) ([]string, error)
	// ListReceiptCollectionDirect — see WeaveModelStore equivalent.
	ListReceiptCollectionDirect(ctx context.Context, seedID, seedKind string) ([]string, error)
	// ListReferenceAdopted returns collections from another project
	// that the current project's field overrides reference via
	// part_of_collection_id. Used by the list rule + DetailView
	// adopted_reference detection.
	ListReferenceAdopted(ctx context.Context, projectID string) ([]*Collection, error)
}

// EntityOption is the lightweight payload returned by ListOptions for
// dropdown rendering. Excludes description, ontology scope, timestamps,
// staging metadata, and project_id (the caller already knows it).
type EntityOption struct {
	ID         string       `json:"id"`
	SemanticID string       `json:"semantic_id,omitempty"`
	SystemName string       `json:"system_name,omitempty"`
	UIName     Translations `json:"ui_name,omitempty"`
	Status     string       `json:"status,omitempty"`
}

// WeaveProjectStats holds entity counts for a single project.
type WeaveProjectStats struct {
	FieldCount      int64
	ModelCount      int64
	CollectionCount int64
	CategoryCount   int64
}

// ProjectActor is a minimal view of an actor linked to a project.
type ProjectActor struct {
	ID          string // actor id (slug)
	Type        string // institution, person, consortium
	DisplayName string
	Role        string // owner, funder, adopter, contributor, author
}

// ProjectAboutUpdate carries the four About fields editable from the
// project settings About section. Lives on domain so both the slice
// store and the WeaveStore impl can speak the same shape.
type ProjectAboutUpdate struct {
	License string       `json:"license"`
	README  Translations `json:"readme"`
	Topics  []string     `json:"topics"`
	BaseURL string       `json:"base_url"`
}

// WeaveProjectStore handles CRUD for projects using domain types.
type WeaveProjectStore interface {
	Create(ctx context.Context, project *Project) error
	GetByID(ctx context.Context, id string) (*Project, error)
	Update(ctx context.Context, project *Project) error

	// UpdateAbout writes only the four About columns (license, readme,
	// topics, base_url) plus updated_at. Identity and access-control
	// fields are untouched. Returns the updated project.
	UpdateAbout(ctx context.Context, projectID string, in ProjectAboutUpdate) (*Project, error)

	Delete(ctx context.Context, id string) error
	List(ctx context.Context, opts ...QueryOption) ([]*Project, int64, error)
	ListChildren(ctx context.Context, parentID string) ([]*Project, error)
	StatsForProjects(ctx context.Context, projectIDs []string) (map[string]*WeaveProjectStats, error)
	// OwnersForProjects returns the owning institution actor (if any) per project ID.
	OwnersForProjects(ctx context.Context, projectIDs []string) (map[string]*ProjectActor, error)
	// ListOwnerInstitutions returns all institutions that own at least one project, sorted by display name.
	ListOwnerInstitutions(ctx context.Context) ([]*ProjectActor, error)
	// LinkedOntologies removed. Canonical reader lives at
	// pkg/weave/projectontologyversion.Service.LinkedOntologies — single
	// source of truth for "what ontology versions does this project have
	// linked", reading from weave_ontology_versions. The legacy SQL on
	// this store hit the empty-post-029 ontology_versions table.

	// ResolvedOntologyVersions walks weave_projects.parent_project_id up the
	// chain (bounded depth 10) and returns all ontology-version links,
	// deduped by ontology_version_id (the current project's row wins over
	// ancestors). Use OnlyInherited=true to exclude own rows.
	ResolvedOntologyVersions(ctx context.Context, projectID string, opts ResolvedOntologyVersionOpts) ([]ResolvedOntologyVersion, error)
	// GetParentID returns the parent project's ID for cycle detection.
	// Returns (nil, nil) when the project has no parent.
	GetParentID(ctx context.Context, projectID string) (*string, error)
	// ProjectChain returns the inheritance chain starting with projectID
	// itself followed by its ancestors in BFS order (parent, grandparent,
	// …). Bounded depth 10. Drives any option/list call that needs to
	// surface inherited entities.
	ProjectChain(ctx context.Context, projectID string) ([]string, error)
}

// LinkedOntology holds display data for a project's linked ontology version.
//
// ClassesUsed and PropertiesUsed are the count of distinct qnames from
// this ontology version that the project's fields actually reference.
// Computed via weave_field_ontology_refs in a single round-trip per
// version. Drives the "Deployed Ontologies" card on the overview tab
// — "4/20 classes | 10/50 properties" instead of a
// lumped "20 terms".
type LinkedOntology struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	URI            string `json:"uri"`
	ClassCount     int    `json:"class_count"`
	PropertyCount  int    `json:"property_count"`
	ClassesUsed    int    `json:"classes_used"`
	PropertiesUsed int    `json:"properties_used"`
	// SourceProjectID is the ID of the project that originally linked
	// this ontology version. Empty when the current project links it
	// directly; set to the ancestor's ID when the link was inherited via
	// the parent_project_id chain. Frontend can render "Inherited from
	// LA" badges using this field.
	SourceProjectID string `json:"source_project_id,omitempty"`
	Origin          Origin `json:"origin"`
}

// Membership is the authorization record binding an actor to a scope (org
// or project) with a role.
type Membership struct {
	ActorID   string
	ScopeType string // "org" | "project"
	ScopeID   string
	Role      string
	CreatedAt time.Time
}

// SnapshotRow is the flattened row returned by SnapshotForActor. Each row
// is either an explicit membership or a "kind = owned" project-owner row.
type SnapshotRow struct {
	Kind      string // "membership" | "owned"
	ScopeType string
	ScopeID   string
	Role      string
}

// MembershipStore accesses the weave_memberships table.
type MembershipStore interface {
	Upsert(ctx context.Context, m Membership) error
	Delete(ctx context.Context, actorID, scopeType, scopeID string) error
	ListByActor(ctx context.Context, actorID string) ([]Membership, error)
	ListByScope(ctx context.Context, scopeType, scopeID string) ([]Membership, error)
	SnapshotForActor(ctx context.Context, actorID string) ([]SnapshotRow, error)
}

// AuthRecord represents one row in weave_auth.
type AuthRecord struct {
	ActorID                string
	PasswordHash           string
	EmailVerifiedAt        *time.Time
	PasswordResetToken     *string
	PasswordResetExpiresAt *time.Time
	LastLoginAt            *time.Time
	PermsVersion           int32
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// AuthWithActor is an AuthRecord joined with the enclosing actor's display
// fields. Used for login where both are needed in one query.
type AuthWithActor struct {
	AuthRecord
	Email       string
	Slug        string
	DisplayName string
	Role        string // weave_actors.role: contributor | admin | super_admin
}

// RegisterPersonParams carries the fields needed to create a new person
// actor alongside their auth row.
type RegisterPersonParams struct {
	ActorID      string // caller-generated ULID
	DisplayName  string
	Slug         string // caller-validated; store trusts this
	Email        string
	PasswordHash string
}

// AuthStore accesses the weave_auth table plus related joined queries.
type AuthStore interface {
	// RegisterPerson inserts a new person into weave_actors AND their
	// password row into weave_auth within a single transaction.
	// Returns pgconn.PgError with code 23505 on unique-violation (email
	// or slug collision) — wrap-check with errors.As in the handler.
	RegisterPerson(ctx context.Context, p RegisterPersonParams) (*AuthWithActor, error)

	Create(ctx context.Context, actorID, passwordHash string, emailVerifiedAt *time.Time) (*AuthRecord, error)
	GetByActorID(ctx context.Context, actorID string) (*AuthRecord, error)
	GetByEmail(ctx context.Context, email string) (*AuthWithActor, error)
	// GetByEmailOrSlug matches on either the actor's email or slug. Used
	// by Login so users can sign in with their username.
	GetByEmailOrSlug(ctx context.Context, loginIdentifier string) (*AuthWithActor, error)
	// GetProfileByActorID joins auth with the enclosing actor row so the Me
	// handler can return profile fields without touching weave_actors directly.
	GetProfileByActorID(ctx context.Context, actorID string) (*AuthWithActor, error)
	UpdatePassword(ctx context.Context, actorID, hash string) error
	MarkLogin(ctx context.Context, actorID string) error
	SetResetToken(ctx context.Context, actorID, token string, expires time.Time) error
	GetByResetToken(ctx context.Context, token string) (*AuthRecord, error)
	ClearResetToken(ctx context.Context, actorID string) error
	Delete(ctx context.Context, actorID string) error
	BumpPermsVersion(ctx context.Context, actorID string) error
}

// OverrideStore handles CRUD and queries for field overrides.
type OverrideStore interface {
	Create(ctx context.Context, override *FieldOverride) error
	Update(ctx context.Context, override *FieldOverride) error
	Delete(ctx context.Context, id int64) error
	GetByID(ctx context.Context, id int64) (*FieldOverride, error)
	GetBase(ctx context.Context, fieldID, projectID string) (*FieldOverride, error)
	ListForEntity(ctx context.Context, entityType, entityID string) ([]FieldOverride, error)
	ListForField(ctx context.Context, fieldID string) ([]FieldOverride, error)
	ListByProjectAndType(ctx context.Context, projectID, entityType string) ([]FieldOverride, error)
	ReplaceForEntity(ctx context.Context, entityType, entityID string, overrides []FieldOverride) error
	SetRefs(ctx context.Context, overrideID int64, refs []OverrideRef) error
	GetRefs(ctx context.Context, overrideID int64) ([]OverrideRef, error)
}

// NamespaceBinding is a project-scoped mapping from a namespace prefix to a
// full namespace URI. Rows with Source="system" are read-only; mutation
// methods return ErrReadOnly when called against them.
type NamespaceBinding struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id,omitempty"`
	Prefix    string `json:"prefix"`
	Namespace string `json:"namespace"`
	Weight    int64  `json:"weight"`
	Source    string `json:"source"`
}

// ProjectOntologyVersionStore manages project to ontology-version links.
//
// The store is the typed surface over the weave_project_ontology_versions
// junction table. "Base vs extension" is not a column on this table — it is
// derived by joining against ontology_versions.compatible_base_versions at
// read time (handled in Tasks 17/19).
type ProjectOntologyVersionStore interface {
	List(ctx context.Context, projectID string) ([]*ProjectOntologyVersion, error)
	ListWithCounts(ctx context.Context, projectID string) ([]ProjectOntologyVersionWithCounts, error)
	ListGrouped(ctx context.Context, projectID string) ([]LinkedOntologyGroup, error)
	Get(ctx context.Context, projectID, versionID string) (*ProjectOntologyVersion, error)
	Create(ctx context.Context, link *ProjectOntologyVersion) error
	Update(ctx context.Context, link *ProjectOntologyVersion) error
	Delete(ctx context.Context, projectID, versionID string) error
	SetPrimary(ctx context.Context, projectID, versionID string) error
	CountPathElementUsage(ctx context.Context, projectID, versionID string) (int64, error)
	SamplePathElementFields(ctx context.Context, projectID, versionID string, limit int) ([]FieldUsageSample, error)
}

// NamespaceBindingStore manages project-scoped namespace prefix bindings.
//
// Rows with source='system' are read-only; mutation methods return
// ErrReadOnly when called against them. Global bindings (project_id IS NULL)
// are always visible, and updates are routed exclusively through CreateUser /
// UpdateUser / DeleteUser so system rows stay untouched.
type NamespaceBindingStore interface {
	List(ctx context.Context, projectID string) ([]*NamespaceBinding, error)
	GetUser(ctx context.Context, projectID string, id string) (*NamespaceBinding, error)
	CreateUser(ctx context.Context, b *NamespaceBinding) error
	UpdateUser(ctx context.Context, b *NamespaceBinding) error
	DeleteUser(ctx context.Context, projectID string, id string) error
	// ExistsByPrefixAndNamespace returns true when another row (excluding id)
	// already uses the given (prefix, namespace) pair. Pass id="" on create.
	ExistsByPrefixAndNamespace(ctx context.Context, prefix, namespace, excludeID string) (bool, error)
}

// ProjectOntologyVersionWithCounts carries the link row plus the number of
// path elements in the project that reference it.
type ProjectOntologyVersionWithCounts struct {
	Link       *ProjectOntologyVersion `json:"link"`
	UsageCount int64                   `json:"usage_count"`
	// OntologyName is the item's own ontology display name, so the list
	// view can label each row (base and extension) individually rather
	// than reusing the group's base label.
	OntologyName string `json:"ontology_name,omitempty"`
}

// LinkedOntologyGroup is one group of linked ontologies (one per base version
// the project uses, plus one per inherited parent project). Items include the
// base itself and any extensions of that base linked in this project.
type LinkedOntologyGroup struct {
	BaseVersionID string                             `json:"base_version_id"`
	BaseLabel     string                             `json:"base_label"`
	Primary       bool                               `json:"primary"`
	Items         []ProjectOntologyVersionWithCounts `json:"items"`
	Source        string                             `json:"source"`                   // "own" or "inherited"
	SourceProject string                             `json:"source_project,omitempty"` // set when Source="inherited"
}

// FieldUsageSample identifies a field referencing an ontology version. Used
// to populate 409 responses when a delete is blocked.
type FieldUsageSample struct {
	ID         string       `json:"id"`
	SemanticID string       `json:"semantic_id"`
	SystemName string       `json:"system_name"`
	UIName     Translations `json:"ui_name"`
}

// ResolvedOntologyVersionOpts controls ResolvedOntologyVersions traversal.
type ResolvedOntologyVersionOpts struct {
	// OnlyInherited skips links owned by the project itself and returns
	// only rows contributed by ancestor projects.
	OnlyInherited bool
}

// ResolvedOntologyVersion is a link with provenance: SourceProjectID is the
// ancestor whose row contributed the link, or "" when the project owns it.
//
// OntologyName + VersionString are the human-readable display values
// resolved by the store via JOIN to ontologies + ontology_versions, so
// callers can render labels without orchestrating extra reads.
type ResolvedOntologyVersion struct {
	Link            *ProjectOntologyVersion `json:"link"`
	SourceProjectID string                  `json:"source_project_id,omitempty"`
	Origin          Origin                  `json:"origin"`
	OntologyName    string                  `json:"ontology_name,omitempty"`
	OntologyPrefix  string                  `json:"ontology_prefix,omitempty"`
	VersionString   string                  `json:"version_string,omitempty"`
}
