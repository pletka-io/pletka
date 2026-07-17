package model

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Store is the data-access contract for models. Override editing
// (entity_type='model' rows in weave_field_overrides) belongs to the
// override slice and is not exposed here — Service consumes
// override.Store directly for those writes.
type Store interface {
	Create(ctx context.Context, m *domain.Model) error
	GetByID(ctx context.Context, id string) (*domain.Model, error)
	GetByIdentifier(ctx context.Context, projectID, identifier string) (*domain.Model, error)
	Update(ctx context.Context, m *domain.Model) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Model, int64, error)
	ListOptions(ctx context.Context, projectID string) ([]domain.EntityOption, error)

	// IsInUse returns true when any field's override-refs point at this
	// model as a value target.
	IsInUse(ctx context.Context, projectID, modelID string) (bool, error)

	// Usage returns counts + sample fields referencing this model as
	// a value target. Used for the 409 payload on blocked delete.
	Usage(ctx context.Context, projectID, modelID string) (UsageReport, error)

	// UsageGlobal reports references to this model across ALL projects
	// (not just its owner), so a cross-project-referenced model is never
	// silently orphaned by delete.
	UsageGlobal(ctx context.Context, modelID string) (UsageReport, error)

	// BatchCompositionCounts returns per-model composition aggregates
	// (field/category/collection counts) for the given model IDs in one
	// query — the model-list count badges.
	BatchCompositionCounts(ctx context.Context, modelIDs []string) (map[string]domain.CompositionCounts, error)

	// BatchInUse returns, for the given model IDs, which are referenced as a
	// value target by a field in any project (weave_override_refs) — the
	// per-row in-use flag that gates delete in the model list. Only in-use
	// models appear in the result (map value always true).
	BatchInUse(ctx context.Context, modelIDs []string) (map[string]bool, error)

	// ListUsage returns the models and collections (across ALL projects) that
	// contain a field targeting this model as a value type — the model Reuse
	// tab. Cross-project by design; the handler partitions by project.
	ListUsage(ctx context.Context, modelID string) (domain.FieldUsageList, error)

	Deprecate(ctx context.Context, modelID string) error
	Activate(ctx context.Context, modelID string) error

	// ScopeClasses returns the distinct prefix:local_name ontology
	// scope classes used by models in the named project. Drives the
	// scope_class filter dropdown on the model list.
	ScopeClasses(ctx context.Context, projectID string) ([]string, error)

	// ListReferenceAdopted returns every model from another project
	// that the current project's overrides reference as a value target
	// (ref_type IN resource_model/collection_model). Drives the
	// "Adopted (by reference)" slice of the list rule (adopt/adapt
	// rollout plan, task 3b).
	ListReferenceAdopted(ctx context.Context, projectID string) ([]*domain.Model, error)

	// ListConnectedModelIDs returns the transitive closure of model IDs
	// reachable from seedIDs via override refs (resource_model targets
	// and collection_model targets contribute their own refs). Drives
	// the Arches generator's full-closure walk (Q8).
	ListConnectedModelIDs(ctx context.Context, seedIDs []string) ([]string, error)

	// ListReceiptModelClosure returns the model IDs in the closure
	// rooted at a single seed entity (the receipt's source). Drives
	// the per-receipt bill-of-materials on the Adoptions tab.
	ListReceiptModelClosure(ctx context.Context, seedID, seedKind string) ([]string, error)

	// ListReceiptModelDirect returns models the seed entity references
	// directly (depth-1). Pairs with the closure query to surface a
	// direct-vs-transitive count split per receipt.
	ListReceiptModelDirect(ctx context.Context, seedID, seedKind string) ([]string, error)
}

// FieldRef identifies a single field referencing a model/collection
// as a value target. Returned in UsageReport samples.
type FieldRef struct {
	FieldID         string `json:"field_id"`
	FieldSemanticID string `json:"field_semantic_id,omitempty"`
	FieldName       string `json:"field_name"`
}

// UsageReport summarises how many fields reference this model as a
// value target.
type UsageReport struct {
	FieldCount   int64      `json:"field_count"`
	FieldSamples []FieldRef `json:"field_samples,omitempty"`
	ProjectIDs   []string   `json:"project_ids,omitempty"` // distinct projects referencing the model (UsageGlobal only)
}

// InUse returns true when at least one field references this model.
func (u UsageReport) InUse() bool { return u.FieldCount > 0 }
