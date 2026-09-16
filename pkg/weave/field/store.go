package field

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Store is the data-access contract for fields. Slice-private; cross-slice
// readers consume the Service, not Store, so handlers and other slices
// only depend on the higher-level abstraction.
//
// Field's base override (weave_field_overrides entity_type=”) is NOT
// owned by this Store — the slice's Service writes it via
// override.Store directly, co-located with field writes inside a
// single ChangeLogRunner.Run boundary.
type Store interface {
	// --- CRUD ---

	// Create inserts a new field. Caller populates ID/SemanticID/etc.
	// CreatedAt/UpdatedAt are written back on success.
	Create(ctx context.Context, f *domain.Field) error

	// GetByID returns the field with the given ULID, or (nil, nil) when
	// not found. ID is the field's ULID (the SemanticID like LAF.309 is
	// also accepted by GetByIdentifier).
	GetByID(ctx context.Context, id string) (*domain.Field, error)

	// GetByIDVersion returns the archived field at a specific release
	// version, scoped to projectID, or (nil, nil) when no such row was
	// archived. See versionedFieldReader.
	GetByIDVersion(ctx context.Context, projectID, id, version string) (*domain.Field, error)

	// GetByIdentifier looks up by ULID, semantic_id, or system_name —
	// scoped to project. Same nil/nil convention as GetByID.
	GetByIdentifier(ctx context.Context, projectID, identifier string) (*domain.Field, error)

	// Update writes the full field row. UpdatedAt is set by the store.
	Update(ctx context.Context, f *domain.Field) error

	// Delete removes a field. Caller must run IsInUse preflight first;
	// deleting an in-use field cascades override rows via FK and breaks
	// model/collection references.
	Delete(ctx context.Context, id string) error

	// List returns fields in the project with total count for pagination.
	// opts honours search, sort, limit, offset.
	List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Field, int64, error)

	// ListReferenceAdopted returns every field from another project
	// that the current project's overrides reference via field_id —
	// task 3b reference-adopted slice.
	ListReferenceAdopted(ctx context.Context, projectID string) ([]*domain.Field, error)

	// ListReceiptFieldClosure returns the field IDs in the closure
	// rooted at a single seed entity. Drives the per-receipt
	// bill-of-materials on the Adoptions tab.
	ListReceiptFieldClosure(ctx context.Context, seedID, seedKind string) ([]string, error)

	// ListReceiptFieldDirect returns the field IDs the seed entity
	// references directly (depth-1) — the fields attached to the seed
	// via overrides at entity_type=seedKind, entity_id=seedID. Pairs
	// with the closure query for the direct-vs-transitive split.
	ListReceiptFieldDirect(ctx context.Context, seedID, seedKind string) ([]string, error)

	// --- Path queries ---

	// FindByPathSequence searches by ontology path patterns. n=2 or 3
	// elements; supports anchored, contiguous, subsequence modes.
	FindByPathSequence(ctx context.Context, query domain.PathQuery) ([]*domain.Field, error)

	// --- Cross-references ---

	// GetModels returns models that reference this field via
	// weave_field_overrides (entity_type='model'). Used by the
	// "where is this field used" lookup.
	GetModels(ctx context.Context, fieldID string) ([]domain.FieldModelRef, error)

	// GetCollections returns collections that reference this field
	// via weave_field_overrides (entity_type='collection').
	GetCollections(ctx context.Context, fieldID string) ([]domain.FieldCollectionRef, error)

	// --- Lifecycle + in-use gating ---

	// IsInUse returns true when the field is referenced by any
	// model or collection override. Base overrides (entity_type='')
	// don't count — they're the field's own intrinsic state.
	IsInUse(ctx context.Context, projectID, fieldID string) (bool, error)

	// Usage returns counts + sample model/collection refs for the 409
	// payload when delete is blocked.
	Usage(ctx context.Context, projectID, fieldID string) (UsageReport, error)

	// CountUsage returns numeric adoption counts only. Version-aware via
	// auth context. Powers the field stats endpoint.
	CountUsage(ctx context.Context, fieldID, projectID string) (domain.FieldUsageCounts, error)

	// ListUsage returns the full list of models and collections that
	// reference this field. Drives the Reuse tab. Version-aware.
	ListUsage(ctx context.Context, fieldID, projectID string) (domain.FieldUsageList, error)

	// BatchUsageCounts returns per-field list aggregates (same-project model/
	// collection counts, cross-project distinct-project count, in-use) for the
	// given field IDs in one query — the field-list count badges + delete gate.
	BatchUsageCounts(ctx context.Context, projectID string, fieldIDs []string) (map[string]domain.FieldListCounts, error)

	// BatchUsageRefs returns, for each field ID, the models and collections
	// that place it — the same edges ListUsage walks, but for many fields in
	// one query (project-scale ownership lookups, e.g. hosting-repo modules
	// consumed via the services-out seam, ADR-0008).
	// Reads live rows only; no version-pinned variant. Fields with no
	// placements are absent from the map.
	BatchUsageRefs(ctx context.Context, fieldIDs []string) (map[string]domain.FieldUsageList, error)

	// Deprecate soft-retires a field. Existing references stay intact;
	// pickers exclude the field from new connections.
	Deprecate(ctx context.Context, fieldID string) error

	// Activate reverses Deprecate.
	Activate(ctx context.Context, fieldID string) error

	// --- Category readers (drive the category filter on the field list) ---

	// ListBaseFieldCategories returns the distinct categories assigned to
	// base field overrides in this project. Drives the category filter
	// dropdown on the field list.
	ListBaseFieldCategories(ctx context.Context, projectID string) ([]FieldCategoryOption, error)

	// ListBaseFieldCategoryAssignments returns a map of field_id to
	// category_id for every base override that has a category set. Used
	// to surface the category badge on each row and to post-filter the
	// list response.
	ListBaseFieldCategoryAssignments(ctx context.Context, projectID string) (map[string]string, error)
}

// FieldCategoryOption is one entry in the category filter dropdown.
type FieldCategoryOption struct {
	ID             string
	UIName         domain.Translations
	CanonicalOrder int
}

// UsageReport summarises a field's adoption across models and
// collections — emitted in the 409 payload when delete is blocked
// and surfaced on the field stats endpoint.
type UsageReport struct {
	ModelCount        int64                       `json:"model_count"`
	CollectionCount   int64                       `json:"collection_count"`
	ModelSamples      []domain.FieldModelRef      `json:"model_samples,omitempty"`
	CollectionSamples []domain.FieldCollectionRef `json:"collection_samples,omitempty"`
}

// InUse returns true when the field has any non-base override.
func (u UsageReport) InUse() bool {
	return u.ModelCount+u.CollectionCount > 0
}
