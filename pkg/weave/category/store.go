package category

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Store is the data-access contract for Category. Implementations:
//   - postgresStore (this package)  — pgx + sqlc, production
//   - in-memory test stubs           — defined in service_test.go / handler_test.go
//
// All methods take a projectID where the operation is project-scoped. Even on
// primary-key lookups (GetByID), passing projectID lets the implementation
// enforce row-level access without a second query.
type Store interface {
	// --- CRUD ---

	// Create inserts a new category. The caller is responsible for setting
	// c.ID (ULID), c.ProjectID, and any required fields. Returns an error if
	// the system_name conflicts with an existing category in the same project.
	Create(ctx context.Context, c *domain.Category) error

	// GetByID returns the category with the given ULID, scoped to the project.
	// Returns (nil, nil) when no row matches — callers should treat that as a
	// 404 rather than an error.
	GetByID(ctx context.Context, projectID, id string) (*domain.Category, error)

	// GetByIdentifier looks up by the human-readable identifier (system_name
	// or semantic_id), scoped to project. Same nil/nil convention as GetByID.
	GetByIdentifier(ctx context.Context, projectID, identifier string) (*domain.Category, error)

	// Update writes the full row. Caller must populate every field; for
	// partial updates use UpdateFields.
	Update(ctx context.Context, c *domain.Category) error

	// UpdateFields applies a partial update via a map of column → value. Used
	// by inline-edit endpoints that touch a single field at a time.
	UpdateFields(ctx context.Context, id string, fields map[string]any) error

	// Delete removes a category. Children that reference this category via
	// model/collection field overrides are NOT touched — use
	// DeleteWithReassignment for cascade semantics.
	Delete(ctx context.Context, projectID, id string) error

	// DeleteWithReassignment removes the category, points overriding fields
	// at reassignTo, and updates any references that store the category's
	// semantic_id (kept as a column for human-readable joins).
	// Pass reassignTo="" to clear the override instead of reassigning.
	DeleteWithReassignment(ctx context.Context, projectID, id, reassignTo, semanticID string) error

	// --- Queries ---

	// List returns categories within a project, applying optional filter,
	// sort, pagination, and search options.
	List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Category, error)

	// Count returns the total number of categories matching the same options
	// as List, ignoring limit/offset. Used by paginated views.
	Count(ctx context.Context, projectID string, opts ...domain.QueryOption) (int64, error)

	// ListWithCounts returns every category in the project paired with its
	// usage counts (fields, model-field overrides, collection-field
	// overrides). Single-query join, no N+1.
	ListWithCounts(ctx context.Context, projectID string) ([]WithCounts, error)

	// --- Mutations ---

	// Reorder rewrites canonical_order on every supplied ID to match its
	// position in the slice. IDs not in the slice are untouched.
	Reorder(ctx context.Context, projectID string, orderedIDs []string) error

	// --- Cross-entity reads (used by the "category usage" modal) ---

	// ModelFieldOverrides returns model-field rows whose category override
	// matches categoryID, scoped to the project. Used to show "this category
	// is used by these fields in these models".
	ModelFieldOverrides(ctx context.Context, projectID, categoryID string) ([]domain.OverrideEntry, error)

	// CollectionFieldOverrides returns collection-field rows whose category
	// override matches semanticID, scoped to the project. Collections key
	// off semantic_id rather than category ULID for legacy reasons.
	CollectionFieldOverrides(ctx context.Context, projectID, semanticID string) ([]domain.OverrideEntry, error)

	// --- Lifecycle ---

	// Deprecate soft-retires the category. Existing references stay intact;
	// pickers stop offering it for new connections. UI badges as deprecated.
	// The service layer is expected to also emit a changelog entry.
	Deprecate(ctx context.Context, projectID, id string) error

	// Activate reverses Deprecate.
	Activate(ctx context.Context, projectID, id string) error

	// IsInUse returns true if the category is referenced by any field
	// override (base, model-level, or collection-level). Single-table EXISTS
	// query over weave_field_overrides, matching either ULID or semantic_id
	// in the override row's category_id column. Hot path: used both as a
	// service-level delete preflight AND as a per-row signal in list
	// responses to gate the UI delete button.
	IsInUse(ctx context.Context, projectID, id, semanticID string) (bool, error)
}

// WithCounts pairs a Category with its usage counts. Slice-local because
// it's a list-view DTO, not a cross-cutting domain type. Embeds Category so
// JSON output keeps the flat shape ListWithCounts callers already expect.
//
// InUse is a derived boolean computed in the same query (any override row
// matches the category's ULID or semantic_id). Lets the UI gate the delete
// button without a per-row follow-up call.
type WithCounts struct {
	domain.Category
	FieldCount           int64 `json:"field_count"`
	ModelFieldCount      int64 `json:"model_field_count"`
	CollectionFieldCount int64 `json:"collection_field_count"`
	InUse                bool  `json:"in_use"`
	OriginLabel          string `json:"origin_label,omitempty"`
}
