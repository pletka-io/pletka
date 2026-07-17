package collection

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Store is the data-access contract for collections.
type Store interface {
	Create(ctx context.Context, c *domain.Collection) error
	GetByID(ctx context.Context, id string) (*domain.Collection, error)
	GetByIdentifier(ctx context.Context, projectID, identifier string) (*domain.Collection, error)
	Update(ctx context.Context, c *domain.Collection) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Collection, int64, error)
	ListOptions(ctx context.Context, projectID string) ([]domain.EntityOption, error)
	// ScopeClasses returns the distinct prefix:local_name ontology
	// scope classes used by collections in this project. Drives the
	// scope_class filter dropdown on the collection list.
	ScopeClasses(ctx context.Context, projectID string) ([]string, error)
	// CategoriesUsed returns the distinct default_category_id values
	// assigned to collections in this project, joined with the
	// category for label rendering. Drives the category filter
	// dropdown on the collection list.
	CategoriesUsed(ctx context.Context, projectID string) ([]CollectionCategoryOption, error)
	ListUsage(ctx context.Context, collectionID, projectID string) ([]domain.FieldUsageRef, error)

	IsInUse(ctx context.Context, projectID, collectionID string) (bool, error)
	Usage(ctx context.Context, projectID, collectionID string) (UsageReport, error)

	// BatchCompositionCounts returns per-collection field counts for the given
	// collection IDs in one query — the collection-list field-count badge.
	BatchCompositionCounts(ctx context.Context, collectionIDs []string) (map[string]domain.CompositionCounts, error)

	Deprecate(ctx context.Context, collectionID string) error
	Activate(ctx context.Context, collectionID string) error

	// ListReferenceAdopted returns every collection from another project
	// that the current project's field overrides reference via
	// part_of_collection_id — task 3b reference-adopted slice.
	ListReferenceAdopted(ctx context.Context, projectID string) ([]*domain.Collection, error)

	// ListReceiptCollectionClosure returns the collection IDs in the
	// closure rooted at a single seed entity. Drives the per-receipt
	// bill-of-materials on the Adoptions tab.
	ListReceiptCollectionClosure(ctx context.Context, seedID, seedKind string) ([]string, error)

	// AnchorIndex returns every non-deprecated collection across every
	// project paired with the URI of its anchor class — the CIDOC
	// class the collection's fields converge on (the last class element
	// of the longest common path prefix). Used by snap-level
	// collection composition to graft cross-project collections like
	// LA's Dimension under a model's Collection-stub fields whose path
	// terminates at the same class — even when no field on the model
	// belongs to that collection.
	//
	// Computed by SQL aggregation over weave_field_overrides.
	AnchorIndex(ctx context.Context) ([]domain.CollectionAnchor, error)

	// ListReceiptCollectionDirect returns collections the seed
	// references directly (depth-1) via collection_model override
	// refs. Pairs with the closure query for the direct-vs-transitive
	// split.
	ListReceiptCollectionDirect(ctx context.Context, seedID, seedKind string) ([]string, error)
}

// FieldRef identifies a single field referencing a collection as a
// value target. Returned in UsageReport samples.
type FieldRef struct {
	FieldID         string `json:"field_id"`
	FieldSemanticID string `json:"field_semantic_id,omitempty"`
	FieldName       string `json:"field_name"`
}

// CollectionCategoryOption is one entry in the category filter dropdown.
type CollectionCategoryOption struct {
	ID             string
	UIName         domain.Translations
	CanonicalOrder int
}

type UsageReport struct {
	FieldCount   int64      `json:"field_count"`
	FieldSamples []FieldRef `json:"field_samples,omitempty"`
}

func (u UsageReport) InUse() bool { return u.FieldCount > 0 }
