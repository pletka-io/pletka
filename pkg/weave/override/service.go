package override

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Service composes override business logic over a Store. Unlike Category
// or NamespaceBinding, this Service is not consumed by a slice handler in
// this package — it is called from the parent slice (model, collection)
// when those slices land. The flow:
//
//  1. Parent slice handler decodes PUT /…/overrides payload.
//  2. Hands off to Service.SaveForEntity with the desired set + commit msg.
//  3. Service computes a Diff against the current set, runs ReplaceForEntity
//     inside ChangeLogRunner.Run, records per-row changelog entries.
//
// Permission gate: ProjectEdit on the override's project. Refining to
// finer-grained capabilities (ModelEdit, CollectionEdit) is a TODO once
// those slices define their own.
type Service struct {
	store  Store
	log    *slog.Logger
	runner domain.ChangeLogRunner
}

// NewService constructs a Service backed by store. nil log → slog.Default;
// nil runner → NoopChangeLogRunner (during slice migration).
func NewService(store Store, log *slog.Logger, runner domain.ChangeLogRunner) *Service {
	if log == nil {
		log = slog.Default()
	}
	if runner == nil {
		runner = domain.NoopChangeLogRunner()
	}
	return &Service{store: store, log: log, runner: runner}
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ErrForbidden is returned when the caller lacks the required capability.
type ErrForbidden struct {
	Capability string
	Resource   string
}

func (e *ErrForbidden) Error() string {
	return fmt.Sprintf("forbidden: requires %s on %s", e.Capability, e.Resource)
}

// ErrValidation collects per-field validation errors.
type ErrValidation struct {
	Fields map[string][]string
}

func (e *ErrValidation) Error() string { return "validation error" }

var errNotFound = errors.New("override: not found")

// IsNotFound reports whether err signals "not found".
func IsNotFound(err error) bool { return errors.Is(err, errNotFound) }

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// ListForEntity returns the ordered override set attached to a parent
// (model or collection). Hot path for editor load.
func (s *Service) ListForEntity(ctx context.Context, projectID, entityType, entityID string) ([]domain.FieldOverride, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListForEntity(ctx, entityType, entityID)
}

// GetByID returns a single override or (nil, nil) when not found.
func (s *Service) GetByID(ctx context.Context, projectID string, id int64) (*domain.FieldOverride, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	o, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, nil
	}
	if o.ProjectID != projectID {
		return nil, errNotFound // don't leak cross-project rows
	}
	return o, nil
}

// ListForField returns every override that references fieldID across all
// entity types. Used by the field-usage modal.
func (s *Service) ListForField(ctx context.Context, projectID, fieldID string) ([]domain.FieldOverride, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListForField(ctx, fieldID)
}

// GetRefs returns the ref-set attached to overrideID.
func (s *Service) GetRefs(ctx context.Context, projectID string, overrideID int64) ([]domain.OverrideRef, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.GetRefs(ctx, overrideID)
}

// CloneFromCollection returns a draft slice of overrides for use when
// adding a collection's fields to a model (or any target context).
//
// Reads all override rows for entity_type='collection', entity_id=
// collectionID, and produces copies retargeted to (targetEntityType,
// targetEntityID). The copies are NOT persisted — caller merges them
// into the desired list and submits via SaveForEntity. Each cloned row
// has ID=0, so SaveForEntity's diff reports it as Added — unless the
// target already has a row with the same (field, category, collection)
// key, in which case matchOverrides reuses that row's id and it's
// reported as Changed instead. Either way the clone lands somewhere;
// which changelog bucket it lands in depends on what's already there.
//
// fallbackCategoryID is used when a source row's category_id is empty.
// Pass the collection's default_category_id (read from
// weave_collections by the caller) — the user can still re-categorize
// per-row before saving. Empty fallbackCategoryID means "leave empty".
//
// staging_id and content_hash on cloned rows are nil/empty: they're
// brand-new rows from the user's perspective, not import-derived.
func (s *Service) CloneFromCollection(
	ctx context.Context,
	projectID, collectionID, targetEntityType, targetEntityID, fallbackCategoryID string,
) ([]domain.FieldOverride, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if collectionID == "" {
		return nil, &ErrValidation{Fields: map[string][]string{
			"collection_id": {"required"},
		}}
	}
	if targetEntityType != "model" && targetEntityType != "collection" {
		return nil, &ErrValidation{Fields: map[string][]string{
			"target_entity_type": {"must be 'model' or 'collection'"},
		}}
	}
	if targetEntityID == "" {
		return nil, &ErrValidation{Fields: map[string][]string{
			"target_entity_id": {"required"},
		}}
	}

	src, err := s.store.ListForEntity(ctx, "collection", collectionID)
	if err != nil {
		return nil, fmt.Errorf("load collection overrides: %w", err)
	}

	out := make([]domain.FieldOverride, 0, len(src))
	for _, row := range src {
		clone := row
		clone.ID = 0 // new row — SaveForEntity assigns a fresh bigserial
		clone.EntityType = targetEntityType
		clone.EntityID = targetEntityID
		clone.ProjectID = projectID
		clone.StagingID = nil
		if clone.CategoryID == "" && fallbackCategoryID != "" {
			clone.CategoryID = fallbackCategoryID
		}
		out = append(out, clone)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

// SaveForEntity replaces the override set on (entityType, entityID) with
// `desired`, atomically with one changelog entry per Added/Removed/Changed
// row. commitMessage is captured on the enclosing change set; see the
// changelog runner TODO for how it'll be wired through once the postgres
// runner replaces NoopChangeLogRunner.
//
// Returns the resulting override set with IDs/timestamps populated.
//
// Caller (parent slice handler) is responsible for verifying that
// entityType/entityID exist and live in projectID before calling.
func (s *Service) SaveForEntity(
	ctx context.Context,
	projectID, entityType, entityID string,
	desired []domain.FieldOverride,
	commitMessage string,
) ([]domain.FieldOverride, Diff, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, Diff{}, err
	}
	if entityType != "model" && entityType != "collection" {
		return nil, Diff{}, &ErrValidation{Fields: map[string][]string{
			"entity_type": {"must be 'model' or 'collection'"},
		}}
	}
	if entityID == "" {
		return nil, Diff{}, &ErrValidation{Fields: map[string][]string{
			"entity_id": {"required"},
		}}
	}

	// Force the scope onto every desired row so caller payloads can't
	// smuggle a row into another (entityType, entityID, projectID).
	for i := range desired {
		desired[i].EntityType = entityType
		desired[i].EntityID = entityID
		desired[i].ProjectID = projectID
	}

	existing, err := s.store.ListForEntity(ctx, entityType, entityID)
	if err != nil {
		return nil, Diff{}, fmt.Errorf("load existing overrides: %w", err)
	}

	// Match desired rows against existing ones the same way ReplaceForEntity
	// will, and stamp the matched ids onto desired before diffing. Without
	// this, a caller that resends a kept row without its id — the raw PUT
	// …/overrides routes decoding straight from client JSON, or an ops/MCP/
	// CLI writer — would diff as a Removed+Added pair instead of a Changed:
	// matchOverrides is the only place that knows a row with ID==0 can
	// still be the same placement as an existing row with the same (field,
	// category, collection) key. See stampMatchedIDs' doc comment for the
	// concurrency note on `existing` being a pre-transaction snapshot.
	plan := matchOverrides(existing, desired)
	stampMatchedIDs(desired, plan)

	diff := ComputeDiff(existing, desired)

	if err := s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.ReplaceForEntity(ctx, entityType, entityID, desired); err != nil {
			return fmt.Errorf("replace overrides: %w", err)
		}
		// ReplaceForEntity just assigned real ids to the rows it inserted.
		// diff.Added was built before that ran, from copies of desired that
		// still carried ID==0 (or an id ComputeDiff didn't accept as a
		// match) — patch those copies now, using diff.AddedIdx (the
		// desired-slice index ComputeDiff itself recorded for each Added
		// entry) so the changelog's "create" entries carry the row's
		// actual id instead of a placeholder.
		for j, i := range diff.AddedIdx {
			diff.Added[j].ID = desired[i].ID
		}
		return s.recordDiff(ctx, rec, projectID, diff)
	}); err != nil {
		return nil, Diff{}, err
	}

	s.log.Info("override.SaveForEntity",
		"project_id", projectID,
		"entity_type", entityType,
		"entity_id", entityID,
		"added", len(diff.Added),
		"removed", len(diff.Removed),
		"changed", len(diff.Changed),
		"commit_message", commitMessage,
	)

	// `desired` now has IDs/timestamps populated by ReplaceForEntity.
	return desired, diff, nil
}

// SetRefs atomically replaces the expected-value-target refs on an
// override. Call site is the same parent-slice handler, after a Save
// pass when an override's expected_value_type is "resource"/"collection".
func (s *Service) SetRefs(ctx context.Context, projectID string, overrideID int64, refs []domain.OverrideRef) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}

	// Confirm override belongs to the project before mutating.
	o, err := s.store.GetByID(ctx, overrideID)
	if err != nil {
		return err
	}
	if o == nil {
		return errNotFound
	}
	if o.ProjectID != projectID {
		return errNotFound
	}

	for i := range refs {
		refs[i].OverrideID = overrideID
	}

	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.SetRefs(ctx, overrideID, refs); err != nil {
			return fmt.Errorf("set refs: %w", err)
		}
		// SetRefs is recorded as a single update on the parent override:
		// previous and new payloads reflect the override row plus its
		// ref-set so a future audit reader sees the ref change.
		payload := marshalOverrideWithRefs(o, refs)
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "override",
			EntityID:        fmt.Sprintf("%d", overrideID),
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalOverride(o), // refs not loaded — accept the small audit gap
			Payload:         payload,
		})
	})
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// recordDiff emits one ChangeLogEntry per Added/Removed/Changed override.
// Per-entry Operation is "create"/"delete"/"update" mapped from the diff
// bucket. EntityType is "override" so the audit reader can distinguish
// override changes from parent-entity changes.
func (s *Service) recordDiff(
	ctx context.Context,
	rec domain.ChangeLogRecorder,
	projectID string,
	diff Diff,
) error {
	for i := range diff.Added {
		o := diff.Added[i]
		if err := rec.Record(ctx, domain.ChangeLogEntry{
			EntityType: "override",
			EntityID:   fmt.Sprintf("%d", o.ID),
			Operation:  "create",
			ProjectID:  projectID,
			Payload:    marshalOverride(&o),
		}); err != nil {
			return err
		}
	}
	for i := range diff.Removed {
		o := diff.Removed[i]
		if err := rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "override",
			EntityID:        fmt.Sprintf("%d", o.ID),
			Operation:       "delete",
			ProjectID:       projectID,
			PreviousPayload: marshalOverride(&o),
		}); err != nil {
			return err
		}
	}
	for i := range diff.Changed {
		pair := diff.Changed[i]
		if err := rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "override",
			EntityID:        fmt.Sprintf("%d", pair.After.ID),
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalOverride(&pair.Before),
			Payload:         marshalOverride(&pair.After),
		}); err != nil {
			return err
		}
	}
	return nil
}

func marshalOverride(o *domain.FieldOverride) []byte {
	b, _ := json.Marshal(o)
	return b
}

func marshalOverrideWithRefs(o *domain.FieldOverride, refs []domain.OverrideRef) []byte {
	b, _ := json.Marshal(struct {
		*domain.FieldOverride
		Refs []domain.OverrideRef `json:"refs"`
	}{o, refs})
	return b
}

// ---------------------------------------------------------------------------
// Permissions
// ---------------------------------------------------------------------------

func (s *Service) requireProjectRead(ctx context.Context, projectID string) error {
	snap := auth.FromContext(ctx)
	res := auth.ProjectResourceFromContext(ctx)
	if !snap.Can(auth.ProjectRead, res, nil) {
		return &ErrForbidden{Capability: string(auth.ProjectRead), Resource: "project:" + projectID}
	}
	return nil
}

func (s *Service) requireProjectWrite(ctx context.Context, projectID string) error {
	snap := auth.FromContext(ctx)
	res := auth.ProjectResourceFromContext(ctx)
	if !snap.Can(auth.ProjectEdit, res, nil) {
		return &ErrForbidden{Capability: string(auth.ProjectEdit), Resource: "project:" + projectID}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Fingerprint + locking
// ---------------------------------------------------------------------------

// EntityFingerprint is the fingerprint of the entity's current pattern: its
// override rows, their value-target refs, and (for a model) its collection
// placements. A later API/MCP writer calls this directly, so it takes no
// projectID and performs no permission check — the caller has already
// authorized the read.
func (s *Service) EntityFingerprint(ctx context.Context, entityType, entityID string) (string, error) {
	rows, err := s.store.ListForEntity(ctx, entityType, entityID)
	if err != nil {
		return "", fmt.Errorf("load overrides: %w", err)
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	refs, err := s.store.RefsForOverrides(ctx, ids)
	if err != nil {
		return "", fmt.Errorf("load refs: %w", err)
	}
	var placements []domain.CollectionPlacement
	if entityType == "model" {
		placements, err = s.store.ListPlacements(ctx, entityID)
		if err != nil {
			return "", fmt.Errorf("load placements: %w", err)
		}
	}
	return Fingerprint(rows, refs, placements), nil
}

// WithEntityLock serializes work on one entity's pattern behind a
// session-level Postgres advisory lock, so two saves of the same model or
// collection queue instead of racing. It first takes projectID's shared
// project lock, then the entity's exclusive lock, both on the same
// connection (see Store.WithAdvisoryLock) — two saves of different
// entities in the same project still run concurrently, since they only
// contend on the shared project lock. A save spans several service calls,
// not one transaction, which is why this isn't a plain DB transaction lock.
// The wait to acquire either lock is bounded; a caller that could not
// acquire it in time gets ErrLockBusy back — "someone else is already
// saving this entity" (or restoring this project), not a real failure. Not
// re-entrant: a call must not be nested inside another call (directly, or
// via the callback) for the same (projectID, entityType, entityID) — it
// would deadlock against its own goroutine.
func (s *Service) WithEntityLock(ctx context.Context, projectID, entityType, entityID string, fn func(context.Context) error) error {
	return s.store.WithAdvisoryLock(ctx, projectID, entityType+":"+entityID, fn)
}

// ---------------------------------------------------------------------------
// Collection placements
// ---------------------------------------------------------------------------

// ListPlacements returns all collection placements for a model.
func (s *Service) ListPlacements(ctx context.Context, modelID string) ([]domain.CollectionPlacement, error) {
	return s.store.ListPlacements(ctx, modelID)
}

// UpsertPlacement inserts or updates one collection placement.
func (s *Service) UpsertPlacement(ctx context.Context, p *domain.CollectionPlacement) error {
	return s.store.UpsertPlacement(ctx, p)
}

// DeletePlacement removes one collection placement; missing rows no-op.
func (s *Service) DeletePlacement(ctx context.Context, modelID, categoryID, collectionID string) error {
	return s.store.DeletePlacement(ctx, modelID, categoryID, collectionID)
}
