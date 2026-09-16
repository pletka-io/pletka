package domain

// Visibility values for Project.Visibility.
const (
	VisibilityPublic   = "public"
	VisibilityInternal = "internal"
	VisibilityPrivate  = "private"
)

// Project represents a workspace that owns patterns (fields, models, collections).
// ID is the IDPrefix (e.g., "LA", "SRD") — used as PK and in URL routing.
type Project struct {
	Entity

	// Namespace is the RDF namespace URI for this project.
	Namespace string `json:"namespace,omitempty"`

	// ParentProjectID is the IDPrefix of the parent project for inheritance.
	// Nil for root projects.
	ParentProjectID *string `json:"parent_project_id,omitempty"`

	// StagingID links to the import staging record for provenance.
	StagingID *int64 `json:"staging_id,omitempty"`

	// OwnerID is the actor ID (person or org) that owns this project.
	// Mirrors the owner_id column added in migration 017.
	OwnerID string `json:"owner_id,omitempty"`

	// CreatedByID is the actor that originally created the project.
	// Immutable attribution; membership changes do not rewrite it.
	CreatedByID *string `json:"created_by_id,omitempty"`

	// Visibility is "public", "internal", or "private". Originally
	// added as two-tier in migration 017; widened to three-tier in
	// migration 031 alongside the About columns.
	Visibility string `json:"visibility,omitempty"`

	// License is the SPDX-style license identifier (e.g., "CC-BY-4.0",
	// "MIT") declaring how the project's patterns can be reused.
	// Free-form text — validation lives in the schema/UI layer, not
	// the column.
	License string `json:"license,omitempty"`

	// README is the long-form project description shown on the project
	// home. Multilingual: keys are language codes ("en", "nl", …).
	README Translations `json:"readme,omitempty"`

	// Topics are short keyword tags used for filtering and discovery.
	Topics []string `json:"topics,omitempty"`

	// BaseURL overrides the default Pletka entity URI base for this
	// project. Once set and published, changing it invalidates RDF
	// URIs already in the wild — the UI warns about this.
	BaseURL string `json:"base_url,omitempty"`

	// IsCoreWeave marks curated top-level weaves that other
	// projects can adopt as a parent. The parent-project picker on
	// project Settings lists Core Weaves first so they're the
	// natural starting point. Toggle is super-admin-only (enforced
	// in the service layer) so the curated set stays small.
	IsCoreWeave bool `json:"is_core_weave,omitempty"`
}
