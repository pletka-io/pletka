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

	"github.com/pletka-io/pletka/pkg/database/advisorylock"
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

	// GetBaseVersion is the version-aware sibling of GetBase: reads the
	// archived base override for (field, project) at a specific release
	// version, or (nil, nil) if no base row was archived at that version.
	// Backs detailview's entity-view when serving a resolved release.
	GetBaseVersion(ctx context.Context, fieldID, projectID, version string) (*domain.FieldOverride, error)

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

	// ReplaceForEntity atomically makes the overrides on (entityType,
	// entityID) equal to the supplied set, keeping the ids of rows it can
	// match (by id of the same field, else by field/category/collection) so
	// example values anchored to them survive. Each input override has its
	// ID/CreatedAt/UpdatedAt populated on success.
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

	// --- Fingerprint + locking support ---

	// RefsForOverrides bulk-loads the value-target refs for a set of
	// override ids, grouped by override id. Used by EntityFingerprint to
	// hash a whole entity's ref-set in one round trip.
	RefsForOverrides(ctx context.Context, ids []int64) (map[int64][]domain.OverrideRef, error)

	// WithAdvisoryLock runs fn while holding, on the same pooled connection
	// and in this order:
	//
	//  1. a session-level Postgres advisory lock SHARED on projectID (see
	//     ProjectLockKey) — a save never modifies project-wide state
	//     directly, so several saves in one project only need to agree that
	//     none of them is a whole-project operation;
	//  2. a session-level Postgres advisory lock EXCLUSIVE on key — so two
	//     saves of the same entity queue instead of racing.
	//
	// The lock lives on its own pooled connection (not inside a
	// transaction) because a save spans several service calls. The wait to
	// acquire either lock is bounded; a caller that times out gets
	// ErrLockBusy. An implementation that cannot serialize callers must
	// return an error, never a silent no-op.
	//
	// git restore (pkg/service/gitmaterializer) takes the SAME projectID key
	// EXCLUSIVE, transaction-scoped (pg_advisory_xact_lock, released on
	// commit/rollback) for the whole span of its whole-project
	// clear-then-reinsert, so it cannot interleave with a save even though
	// it never takes an entity lock. Ordering is project-then-entity
	// everywhere a caller takes both, and restore only ever takes the
	// project lock, so no deadlock cycle exists between the two lock users.
	//
	// The lock is NOT re-entrant: a call must never be nested inside
	// another call for the same (projectID, key) pair (directly, or by the
	// callback reaching code that locks the same entity again) — the
	// nested call acquires a different session and queues behind the lock
	// its own goroutine holds, deadlocking forever while pinning two
	// pooled connections.
	WithAdvisoryLock(ctx context.Context, projectID, key string, fn func(context.Context) error) error
}

// ErrLockBusy is returned by WithAdvisoryLock (and its Service wrapper,
// WithEntityLock) when the per-entity lock could not be acquired within its
// bounded wait — someone else is already saving this entity. Callers use
// errors.Is(err, ErrLockBusy). Task 3 maps this to a 409.
//
// git restore also joins this exact sentinel onto its own project-lock-busy
// error (pkg/service/gitmaterializer) so a caller that only cares "is
// something else holding this lock" can use the same errors.Is check on
// either side. It lives in pkg/database/advisorylock, not here, and this is
// a re-export: gitmaterializer's production code cannot import this
// package directly without creating an import cycle through this
// package's own integration tests (see advisorylock's doc comment for the
// exact cycle) — advisorylock has no imports of its own, so both sides
// depend on it instead of one depending on the other.
var ErrLockBusy = advisorylock.ErrLockBusy

// ProjectLockKey returns the advisory-lock key for a project-wide lock: the
// SHARED lock WithAdvisoryLock takes before its entity lock, and the
// EXCLUSIVE, transaction-scoped lock git restore takes for its whole
// hydration transaction (pkg/service/gitmaterializer). Both sides call this
// (this is a thin re-export of advisorylock.ProjectLockKey — see ErrLockBusy's
// comment for why the shared definition lives there, not here) so the key
// can never drift between packages — pg_advisory_lock hashes the string
// with hashtext, so a mismatched prefix would silently stop the two sides
// from contending on the same lock at all.
func ProjectLockKey(projectID string) string {
	return advisorylock.ProjectLockKey(projectID)
}
