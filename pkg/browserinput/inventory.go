package browserinput

import (
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// InventoryQuery applies to the union of owned, explicitly adopted and referenced rows.
// It never accepts source project versions; the selected dependency graph owns those.
type InventoryQuery struct {
	Page, PerPage                                              int
	Search, SortBy, Origin, Scope, Category, ModelType, Status string
	Descending                                                 bool
}

// InventoryRow is detached browser display data. Counts refer to the consumer project.
type InventoryRow struct {
	ID                       string               `json:"id"`
	SemanticID               string               `json:"semantic_id,omitempty"`
	SystemName               string               `json:"system_name,omitempty"`
	UIName                   domain.Translations  `json:"ui_name,omitempty"`
	Description              domain.Translations  `json:"description,omitempty"`
	Status                   string               `json:"status"`
	ProjectID                string               `json:"project_id"`
	SourceVersion            string               `json:"source_version,omitempty"`
	OriginalProjectID        string               `json:"original_project_id,omitempty"`
	Deprecated               bool                 `json:"deprecated"`
	Owned                    bool                 `json:"owned"`
	CanDeprecate             bool                 `json:"can_deprecate"`
	CanActivate              bool                 `json:"can_activate"`
	CreatedAt                time.Time            `json:"created_at"`
	UpdatedAt                time.Time            `json:"updated_at"`
	OntologyScope            string               `json:"ontology_scope,omitempty"`
	ScopeClassCode           string               `json:"scope_class_code,omitempty"`
	ModelType                string               `json:"model_type,omitempty"`
	ExpectedValueType        string               `json:"expected_value_type,omitempty"`
	PathElements             []domain.PathElement `json:"path_elements,omitempty"`
	CategoryID               string               `json:"category_id,omitempty"`
	DefaultCategoryID        string               `json:"default_category_id,omitempty"`
	CollectionNumber         int                  `json:"collection_number,omitempty"`
	CanonicalCollectionOrder int                  `json:"canonical_collection_order,omitempty"`
	ModelCount               int                  `json:"model_count,omitempty"`
	CollectionCount          int                  `json:"collection_count,omitempty"`
	FieldCount               int                  `json:"field_count,omitempty"`
	CategoryCount            int                  `json:"category_count,omitempty"`
	InUse                    bool                 `json:"in_use,omitempty"`
	Origin                   domain.Origin        `json:"origin"`
	OriginKind               string               `json:"origin_kind"`
	NotReferenced            bool                 `json:"-"`
	CurrentURL               string               `json:"current_url"`
	SourceURL                string               `json:"source_url"`
}

// Inventory is one globally ordered page with exact filtered totals and scoped facets.
type Inventory struct {
	ProjectID, Version, Kind string
	Items                    []InventoryRow
	Total                    int64
	Page, PerPage            int
	Scopes                   []string
	Categories               map[string]domain.Translations
}

// Adoption is an explicit receipt with its selected source's bill of materials.
type Adoption struct {
	Receipt         projectsnapshot.AdoptionRecord
	Actor           domain.ActorRef
	Root            InventoryRow
	Direct, Closure map[string][]InventoryRow
}

// Adoptions is detached content for the adoption tab and its closure drilldowns.
type Adoptions struct {
	ProjectID, Version string
	Items              []Adoption
}
