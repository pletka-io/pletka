package category

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Service composes Category business logic over a Store. Handlers depend on
// *Service (or its method set), never directly on Store. Different Store
// implementations (postgres, in-memory test stub) plug in via NewService.
//
// The Service owns:
//   - permission checks (AuthSnapshot via context)
//   - validation (system_name uniqueness, parent existence, no cycles)
//   - delete preflight (in-use blocking)
//   - cascade rules (delete+reassign vs deprecate)
//   - audit/changelog emission via ChangeLogRunner (noop during migration;
//     production runner wraps the data write + log flush in a single tx)
//
// HTTP concerns (status codes, JSON, templates) are NOT here — the Handler
// layer maps service errors to responses.
// EntityNumberer allocates monotonic per-project entity numbers used to
// build semantic IDs like "TPECAT.1". Same shape as the model/collection
// service numberer dependency.
type EntityNumberer interface {
	AllocateEntityNumber(ctx context.Context, projectID, kind string) (int64, error)
}

type versionedCategoryReader interface {
	GetByIDVersion(ctx context.Context, projectID, id, version string) (*domain.Category, error)
	ListWithCountsVersion(ctx context.Context, projectID, version string) ([]WithCounts, error)
	ModelFieldOverridesVersion(ctx context.Context, projectID, categoryID, version string) ([]domain.OverrideEntry, error)
	CollectionFieldOverridesVersion(ctx context.Context, projectID, semanticID, version string) ([]domain.OverrideEntry, error)
}

type Service struct {
	store     Store
	adoptions domain.AdoptionStore
	log       *slog.Logger
	runner    domain.ChangeLogRunner
	numberer  EntityNumberer
}

// NewService constructs a Service backed by store.
//   - log:      error/info logger; pass nil for slog.Default
//   - runner:   changelog tx runner; pass nil for the noop runner (mutations
//     happen but audit entries are dropped — useful during the
//     migration period and in tests that don't care about audit)
//   - numberer: per-project entity number allocator. Pass nil to skip
//     semantic-ID generation on Create (legacy behaviour). When set,
//     Create allocates `{ProjectID}CAT.{N}` for fresh categories the
//     same way Models/Collections do, so the override-editor + options
//     dropdowns can render a stable human-readable id instead of a ULID.
func NewService(store Store, adoptions domain.AdoptionStore, log *slog.Logger, runner domain.ChangeLogRunner, numberer EntityNumberer) *Service {
	if log == nil {
		log = slog.Default()
	}
	if runner == nil {
		runner = domain.NoopChangeLogRunner()
	}
	return &Service{store: store, adoptions: adoptions, log: log, runner: runner, numberer: numberer}
}

// ---------------------------------------------------------------------------
// Inputs / Outputs (slice-local DTOs)
// ---------------------------------------------------------------------------

// CreateInput is the payload accepted by Create. Wrapping the input in a
// dedicated type lets handlers JSON-decode without leaking optional fields
// into the domain Category struct.
type CreateInput struct {
	SystemName     string
	UIName         domain.Translations
	Description    domain.Translations
	CanonicalOrder int
	Status         domain.Status // optional; defaults to StatusDraft
}

// UpdateInput captures a partial update. Nil pointers mean "leave alone".
type UpdateInput struct {
	UIName         *domain.Translations
	Description    *domain.Translations
	SystemName     *string
	CanonicalOrder *int
	Status         *domain.Status
}

// DeleteOpts controls Delete behaviour.
type DeleteOpts struct {
	// ReassignTo, when non-empty, switches Delete to the cascade path:
	// fields with this category are reassigned to ReassignTo, overrides
	// referencing this category are cleared, then the category is removed.
	ReassignTo string
}

// ErrEntityInUse is returned by Delete when non-base override rows reference
// the category. Handlers map this to 409 Conflict with a structured payload.
type ErrEntityInUse struct {
	EntityType string // "category"
	EntityID   string
	SemanticID string
}

func (e *ErrEntityInUse) Error() string {
	return fmt.Sprintf("%s %s is in use by overrides", e.EntityType, e.EntityID)
}

// ErrForbidden is returned when the caller lacks the required capability for
// the requested operation. Handlers map this to 403.
type ErrForbidden struct {
	Capability string
	Resource   string
}

func (e *ErrForbidden) Error() string {
	return fmt.Sprintf("forbidden: requires %s on %s", e.Capability, e.Resource)
}

// ErrValidation collects per-field validation errors. Handlers map this to
// 422 with the standard {"errors": {field: [msg]}} shape.
type ErrValidation struct {
	Fields map[string][]string
}

func (e *ErrValidation) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	return "validation: " + strings.Join(keys, ", ")
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// List returns categories within a project. Caller must have read access.
func (s *Service) List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Category, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.List(ctx, projectID, opts...)
}

// Get returns a single category, or (nil, nil) when not found.
func (s *Service) Get(ctx context.Context, projectID, id string) (*domain.Category, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vr, ok := s.store.(versionedCategoryReader); ok {
			return vr.GetByIDVersion(ctx, projectID, id, version)
		}
	}
	return s.store.GetByID(ctx, projectID, id)
}

// ListWithCounts returns each category with its usage counts and a derived
// in_use boolean. Used by list views that gate the delete button.
func (s *Service) ListWithCounts(ctx context.Context, projectID string) ([]WithCounts, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vr, ok := s.store.(versionedCategoryReader); ok {
			return vr.ListWithCountsVersion(ctx, projectID, version)
		}
	}
	return s.store.ListWithCounts(ctx, projectID)
}

// IsInUse exposes the boolean directly for UI hot-path queries.
func (s *Service) IsInUse(ctx context.Context, projectID, id, semanticID string) (bool, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return false, err
	}
	return s.store.IsInUse(ctx, projectID, id, semanticID)
}

// StatsReport summarises a category's usage. Returned by Stats() and
// rendered in the "category usage" modal. Excludes the per-field listing —
// that's a Field-slice concern and arrives once the Field slice is built.
type StatsReport struct {
	Category                 *domain.Category       `json:"category"`
	ModelFieldOverrides      []domain.OverrideEntry `json:"model_field_overrides"`
	CollectionFieldOverrides []domain.OverrideEntry `json:"collection_field_overrides"`
	TotalUsage               int                    `json:"total_usage"`
}

// Stats returns the category alongside its model + collection field
// override usage. Used by the stats modal in the categories list.
func (s *Service) Stats(ctx context.Context, projectID, id string) (*StatsReport, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}

	var (
		cat            *domain.Category
		err            error
		modelOverrides []domain.OverrideEntry
		collOverrides  []domain.OverrideEntry
	)
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vr, ok := s.store.(versionedCategoryReader); ok {
			cat, err = vr.GetByIDVersion(ctx, projectID, id, version)
			if err != nil {
				return nil, err
			}
			if cat == nil {
				return nil, errNotFound
			}
			modelOverrides, err = vr.ModelFieldOverridesVersion(ctx, projectID, id, version)
			if err != nil {
				return nil, err
			}
			collOverrides, err = vr.CollectionFieldOverridesVersion(ctx, projectID, cat.SemanticID, version)
			if err != nil {
				return nil, err
			}
		}
	}
	if cat == nil {
		cat, err = s.store.GetByID(ctx, projectID, id)
		if err != nil {
			return nil, err
		}
		if cat == nil {
			return nil, errNotFound
		}
		modelOverrides, err = s.store.ModelFieldOverrides(ctx, projectID, id)
		if err != nil {
			return nil, err
		}
		collOverrides, err = s.store.CollectionFieldOverrides(ctx, projectID, cat.SemanticID)
		if err != nil {
			return nil, err
		}
	}

	return &StatsReport{
		Category:                 cat,
		ModelFieldOverrides:      modelOverrides,
		CollectionFieldOverrides: collOverrides,
		TotalUsage:               len(modelOverrides) + len(collOverrides),
	}, nil
}

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

// Create validates the input, builds a fresh Category with StatusDraft (unless
// overridden), persists it, and returns the stored row with timestamps filled.
func (s *Service) Create(ctx context.Context, projectID string, in CreateInput) (*domain.Category, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, err
	}

	if errs := validateCreate(in); len(errs) > 0 {
		return nil, &ErrValidation{Fields: errs}
	}

	status := in.Status
	if status == "" {
		status = domain.StatusDraft
	}

	semanticID := ""
	if s.numberer != nil {
		next, err := s.numberer.AllocateEntityNumber(ctx, projectID, "category")
		if err != nil {
			return nil, fmt.Errorf("allocate category number: %w", err)
		}
		// Match the format the parser expects: "{ProjectID}.CAT.{N}".
		// Models/Collections use "{Project}M.{N}" / "{Project}C.{N}";
		// Categories use the longer dot-CAT form, mirrored by
		// SemanticID.ID() and the Airtable wave importer's category ids.
		semanticID = fmt.Sprintf("%s.CAT.%d", projectID, next)
	}

	cat := &domain.Category{
		Entity: domain.Entity{
			SemanticID:  semanticID,
			SystemName:  in.SystemName,
			UIName:      in.UIName,
			Description: in.Description,
			Status:      status,
			ProjectID:   projectID,
		},
		CanonicalOrder: in.CanonicalOrder,
	}

	err := s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Create(ctx, cat); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType: "category",
			EntityID:   cat.ID,
			Operation:  "create",
			ProjectID:  projectID,
			Payload:    marshalCategory(cat),
		})
	})
	if err != nil {
		return nil, err
	}
	return cat, nil
}

// Update applies a partial update. Fields with nil pointers are left as-is.
// Returns ErrEntityInUse-equivalent shape only if business rules dictate
// (currently no rules block update).
func (s *Service) Update(ctx context.Context, projectID, id string, in UpdateInput) (*domain.Category, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, err
	}

	cat, err := s.store.GetByID(ctx, projectID, id)
	if err != nil {
		return nil, err
	}
	if cat == nil {
		return nil, errNotFound
	}

	prev := *cat // shallow copy for the previous-payload audit trail
	applyUpdate(cat, in)

	if errs := validateMutable(cat); len(errs) > 0 {
		return nil, &ErrValidation{Fields: errs}
	}

	err = s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Update(ctx, cat); err != nil {
			return err
		}
		if structuralCategoryEdit(prev, in) {
			if err := s.detachProjectCategoryAdoption(ctx, projectID, &prev); err != nil {
				return err
			}
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "category",
			EntityID:        cat.ID,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalCategory(&prev),
			Payload:         marshalCategory(cat),
		})
	})
	if err != nil {
		return nil, err
	}
	return cat, nil
}

// Reorder rewrites canonical_order on each ID to its position in orderedIDs.
// IDs not belonging to projectID are rejected by the store.
func (s *Service) Reorder(ctx context.Context, projectID string, orderedIDs []string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}
	return s.store.Reorder(ctx, projectID, orderedIDs)
}

// Delete removes a category. Pre-flights with IsInUse and returns
// ErrEntityInUse if any non-base override references the category.
//
// To delete an in-use category the caller must either:
//   - call Deprecate first (soft-retire), or
//   - call Delete with DeleteOpts.ReassignTo set (cascade reassign + clear).
func (s *Service) Delete(ctx context.Context, projectID, id string, opts DeleteOpts) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}

	cat, err := s.store.GetByID(ctx, projectID, id)
	if err != nil {
		return err
	}
	if cat == nil {
		return errNotFound
	}

	if opts.ReassignTo != "" {
		return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
			if err := s.store.DeleteWithReassignment(ctx, projectID, id, opts.ReassignTo, cat.SemanticID); err != nil {
				return err
			}
			if err := s.detachProjectCategoryAdoption(ctx, projectID, cat); err != nil {
				return err
			}
			return rec.Record(ctx, domain.ChangeLogEntry{
				EntityType:      "category",
				EntityID:        id,
				Operation:       "delete",
				ProjectID:       projectID,
				PreviousPayload: marshalCategory(cat),
			})
		})
	}

	inUse, err := s.store.IsInUse(ctx, projectID, id, cat.SemanticID)
	if err != nil {
		return err
	}
	if inUse {
		return &ErrEntityInUse{EntityType: "category", EntityID: id, SemanticID: cat.SemanticID}
	}

	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Delete(ctx, projectID, id); err != nil {
			return err
		}
		if err := s.detachProjectCategoryAdoption(ctx, projectID, cat); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "category",
			EntityID:        id,
			Operation:       "delete",
			ProjectID:       projectID,
			PreviousPayload: marshalCategory(cat),
		})
	})
}

// Deprecate soft-retires the category. Existing references stay intact.
// Idempotent: calling on an already-deprecated category is a no-op.
func (s *Service) Deprecate(ctx context.Context, projectID, id string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}

	cat, err := s.store.GetByID(ctx, projectID, id)
	if err != nil {
		return err
	}
	if cat == nil {
		return errNotFound
	}
	if cat.Deprecated {
		return nil
	}

	prev := *cat
	after := *cat
	after.Deprecated = true

	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Deprecate(ctx, projectID, id); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "category",
			EntityID:        id,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalCategory(&prev),
			Payload:         marshalCategory(&after),
		})
	})
}

// Activate reverses Deprecate. Idempotent.
func (s *Service) Activate(ctx context.Context, projectID, id string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}

	cat, err := s.store.GetByID(ctx, projectID, id)
	if err != nil {
		return err
	}
	if cat == nil {
		return errNotFound
	}
	if !cat.Deprecated {
		return nil
	}

	prev := *cat
	after := *cat
	after.Deprecated = false

	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Activate(ctx, projectID, id); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "category",
			EntityID:        id,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalCategory(&prev),
			Payload:         marshalCategory(&after),
		})
	})
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// errNotFound is a sentinel for missing rows. Handlers test for it via
// errors.Is and map to 404.
var errNotFound = errors.New("category not found")

// IsNotFound reports whether err signals "not found". Exposed so callers
// outside this package can map to 404 without importing the sentinel.
func IsNotFound(err error) bool { return errors.Is(err, errNotFound) }

func structuralCategoryEdit(prev domain.Category, in UpdateInput) bool {
	if in.SystemName != nil && strings.TrimSpace(*in.SystemName) != prev.SystemName {
		return true
	}
	if in.UIName != nil {
		if normalizedTranslations(*in.UIName) != normalizedTranslations(prev.UIName) {
			return true
		}
	}
	if in.Description != nil {
		if normalizedTranslations(*in.Description) != normalizedTranslations(prev.Description) {
			return true
		}
	}
	return false
}

func normalizedTranslations(v domain.Translations) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func (s *Service) detachProjectCategoryAdoption(ctx context.Context, projectID string, category *domain.Category) error {
	if s.adoptions == nil || category == nil || strings.TrimSpace(category.SystemName) == "" {
		return nil
	}
	adoptions, err := s.adoptions.List(ctx,
		domain.WithProjectID(projectID),
		domain.WithFilter("context_entity_type", "project"),
		domain.WithFilter("context_entity_id", projectID),
		domain.WithFilter("entity_type", "category"),
	)
	if err != nil {
		return fmt.Errorf("list project category adoptions: %w", err)
	}
	if len(adoptions) == 0 {
		return nil
	}
	kept := make([]domain.Adoption, 0, len(adoptions))
	detached := false
	for _, adoption := range adoptions {
		source, err := s.store.GetByIdentifier(ctx, adoption.SourceProjectID, adoption.SourceEntityID)
		if err != nil {
			return fmt.Errorf("resolve adopted category source %s/%s: %w", adoption.SourceProjectID, adoption.SourceEntityID, err)
		}
		if source != nil && source.SystemName == category.SystemName {
			detached = true
			continue
		}
		kept = append(kept, adoption)
	}
	if !detached {
		return nil
	}
	if err := s.adoptions.ReplaceForContext(ctx, projectID, "project", projectID, kept); err != nil {
		return fmt.Errorf("detach category adoption: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Permissions (delegates to auth.AuthSnapshot)
// ---------------------------------------------------------------------------

// requireProjectRead enforces that the caller can read the project.
// Categories don't have dedicated capabilities yet; they piggyback on
// ProjectRead. Add CategoryRead etc. to pkg/auth/capabilities.go if a finer
// grain is needed.
func (s *Service) requireProjectRead(ctx context.Context, projectID string) error {
	snap := auth.FromContext(ctx)
	res := auth.ProjectResourceFromContext(ctx)
	if !snap.Can(auth.ProjectRead, res, nil) {
		return &ErrForbidden{Capability: string(auth.ProjectRead), Resource: "project:" + projectID}
	}
	return nil
}

// requireProjectWrite enforces that the caller can mutate categories.
// Uses ProjectEdit (same rationale as requireProjectRead).
func (s *Service) requireProjectWrite(ctx context.Context, projectID string) error {
	snap := auth.FromContext(ctx)
	res := auth.ProjectResourceFromContext(ctx)
	if !snap.Can(auth.ProjectEdit, res, nil) {
		return &ErrForbidden{Capability: string(auth.ProjectEdit), Resource: "project:" + projectID}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Validation + helpers
// ---------------------------------------------------------------------------

// validateCreate runs synchronous checks on Create input. Returns a
// field-keyed error map; empty result means valid.
func validateCreate(in CreateInput) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(in.SystemName) == "" {
		errs["system_name"] = append(errs["system_name"], "required")
	}
	if in.Status != "" && !in.Status.Valid() {
		errs["status"] = append(errs["status"], "must be 'draft' or 'published'")
	}
	return errs
}

// validateMutable checks the post-update Category is still well-formed.
func validateMutable(c *domain.Category) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(c.SystemName) == "" {
		errs["system_name"] = append(errs["system_name"], "required")
	}
	if c.Status != "" && !c.Status.Valid() {
		errs["status"] = append(errs["status"], "must be 'draft' or 'published'")
	}
	return errs
}

// marshalCategory serialises a category for changelog payloads. Returns
// nil on nil input or marshal error (audit row records empty bytes — the
// failure is logged at flush time).
func marshalCategory(c *domain.Category) []byte {
	if c == nil {
		return nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return nil
	}
	return b
}

// applyUpdate mutates c in place with non-nil fields from in.
func applyUpdate(c *domain.Category, in UpdateInput) {
	if in.UIName != nil {
		c.UIName = *in.UIName
	}
	if in.Description != nil {
		c.Description = *in.Description
	}
	if in.SystemName != nil {
		c.SystemName = *in.SystemName
	}
	if in.CanonicalOrder != nil {
		c.CanonicalOrder = *in.CanonicalOrder
	}
	if in.Status != nil {
		c.Status = *in.Status
	}
}
