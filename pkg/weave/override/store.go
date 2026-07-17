// Package override implements the field-override vertical slice. Unlike
// Category and NamespaceBinding, Override is an internal data layer — it
// does not register HTTP routes of its own. Override mutations always
// arrive through a parent entity (model, collection, or field), so the
// owning slice (e.g. pkg/weave/model when it lands) drives the user-facing
// flow and calls into override.Service for the heavy lifting.
//
// Files:
//   - store.go         — Store interface (CRUD + bulk replace + ref-set)
//   - store_postgres.go — pgx + sqlc implementation
//   - service.go        — Business logic: validation, ChangeLogRunner-bound
//     atomic mutation paths, helper compositions
//
// No handler.go / formschema.go / routes.go — by design.
package override

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Store is the data-access contract for field overrides. Implementations
// may be pgx/sqlc-backed (production) or in-memory (tests).
//
// Override IDs are int64 (bigserial) rather than ULIDs because the rows
// are a high-volume junction surface and we want sequential primary keys
// for natural ordering and cheap insert performance.
//
// Three-row "kinds" coexist in weave_field_overrides:
//   - EntityType="" : the field's BASE override (defaults that apply
//     everywhere unless a model/collection overrides them).
//   - EntityType="model"      : per-model override.
//   - EntityType="collection" : per-collection override.
//
// The override resolver (pkg/weave/resolve.go) layers these in priority
// order (collection > model > base) when materialising a model view.
type Store interface {
	// --- CRUD ---

	// Create inserts an override. The DB assigns ID via bigserial; the
	// generated id and timestamps are written back onto override.
	Create(ctx context.Context, override *domain.FieldOverride) error

	// GetByID returns the override with the given ID, or (nil, nil) if no
	// row matches.
	GetByID(ctx context.Context, id int64) (*domain.FieldOverride, error)

	// GetBase returns the base (EntityType="") override for a (field,
	// project) pair, or (nil, nil) if none exists. Hot path for the
	// resolver — the base layer is consulted for every field render.
	GetBase(ctx context.Context, fieldID, projectID string) (*domain.FieldOverride, error)

	// Update writes the full row. Caller must populate every mutable
	// field. Returns "not found" if the row has been deleted.
	Update(ctx context.Context, override *domain.FieldOverride) error

	// Delete removes the override. Refs in weave_override_refs are
	// removed by foreign-key cascade.
	Delete(ctx context.Context, id int64) error

	// --- Queries ---

	// ListForEntity returns every override row attached to an entity
	// (model or collection), ordered by position.
	ListForEntity(ctx context.Context, entityType, entityID string) ([]domain.FieldOverride, error)

	// ListForField returns every override row across all entity types
	// that references fieldID. Used by the field-usage modal.
	ListForField(ctx context.Context, fieldID string) ([]domain.FieldOverride, error)

	// ListByProjectAndType returns every override of the given entity
	// type within a project, sorted by entity_id then position. Used by
	// resolver bulk-load paths and override-debug endpoints.
	ListByProjectAndType(ctx context.Context, projectID, entityType string) ([]domain.FieldOverride, error)

	// --- Atomic bulk mutations ---

	// ReplaceForEntity atomically deletes all overrides on (entityType,
	// entityID) and re-inserts the supplied set. Each input override has
	// its ID/CreatedAt/UpdatedAt populated on success.
	ReplaceForEntity(ctx context.Context, entityType, entityID string, overrides []domain.FieldOverride) error

	// SetRefs atomically replaces an override's expected-value-target
	// references in weave_override_refs. Used when an override's expected
	// value type is "resource" or "collection" and a list of permitted
	// targets must be persisted.
	SetRefs(ctx context.Context, overrideID int64, refs []domain.OverrideRef) error

	// GetRefs returns all refs attached to overrideID, ordered by
	// ref_type then position.
	GetRefs(ctx context.Context, overrideID int64) ([]domain.OverrideRef, error)

	// --- Collection placements ---
	// Per-(model, category, collection) constraints for a collection group
	// inside a model. Absence of a row = optional, 0..unbounded, visible.

	// UpsertPlacement inserts or updates the placement keyed by
	// (ModelID, CategoryID, CollectionID); ID is written back.
	UpsertPlacement(ctx context.Context, p *domain.CollectionPlacement) error

	// ListPlacements returns all placements for a model, ordered by
	// category then collection.
	ListPlacements(ctx context.Context, modelID string) ([]domain.CollectionPlacement, error)

	// DeletePlacement removes one placement; missing rows are a no-op.
	DeletePlacement(ctx context.Context, modelID, categoryID, collectionID string) error
}
