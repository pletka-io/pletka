package field

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/override"
)

type versionedFieldReader interface {
	GetByIDVersion(ctx context.Context, projectID, id, version string) (*domain.Field, error)
	ListVersion(ctx context.Context, projectID, version string, opts ...domain.QueryOption) ([]*domain.Field, int64, error)
	UsageVersion(ctx context.Context, projectID, fieldID, version string) (UsageReport, error)
}

// HierarchyReader is the cross-slice ontology coverage check used by
// Create's setup gate. Satisfied by deps.Weave.Projects() during the
// strangler-fig period.
type HierarchyReader interface {
	ResolvedOntologyVersions(ctx context.Context, projectID string, opts domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error)
}

// EntityNumberer allocates the next sequential semantic-id number for
// a (projectID, kind) pair. Satisfied by deps.Weave (which exposes
// AllocateEntityNumber on the legacy aggregate).
type EntityNumberer interface {
	AllocateEntityNumber(ctx context.Context, projectID, kind string) (int64, error)
}

type ProjectReader interface {
	GetByID(ctx context.Context, id string) (*domain.Project, error)
}

// OntologyRefRebuilder is the cross-slice hook called after Create +
// Update so the materialized weave_field_ontology_refs table tracks
// the field's path_elements. Satisfied by pkg/weave/ontology.Service
// (Phase E). Optional — Service tolerates nil and skips the rebuild
// (used by tests + the importer-side field writer).
type OntologyRefRebuilder interface {
	ReplaceFieldRefs(ctx context.Context, fieldID, projectID string, refs []domain.FieldOntologyRef) error
}

// Service composes Field business logic over Store. Holds an
// override.Store reference so write paths (Create / Update) can write
// the field's base override (entity_type=”) in the same
// ChangeLogRunner.Run boundary as the field write.
type Service struct {
	store        Store
	overrides    override.Store
	adoptions    domain.AdoptionStore
	forks        domain.ForkStore
	projects     ProjectReader
	hierarchy    HierarchyReader
	numberer     EntityNumberer
	ontologyRefs OntologyRefRebuilder
	log          *slog.Logger
	runner       domain.ChangeLogRunner
}

// NewService constructs a Service.
//   - store:        field data layer
//   - overrides:    override Store for the co-located base override write
//   - hierarchy:    setup gate (no ontologies → reject Create)
//   - numberer:     semantic_id sequence allocator (typically deps.Weave)
//   - ontologyRefs: post-save hook for weave_field_ontology_refs (nil OK)
//   - log:          slog.Default if nil
//   - runner:       NoopChangeLogRunner if nil
func NewService(
	store Store,
	overrides override.Store,
	adoptions domain.AdoptionStore,
	forks domain.ForkStore,
	projects ProjectReader,
	hierarchy HierarchyReader,
	numberer EntityNumberer,
	ontologyRefs OntologyRefRebuilder,
	log *slog.Logger,
	runner domain.ChangeLogRunner,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	if runner == nil {
		runner = domain.NoopChangeLogRunner()
	}
	return &Service{
		store:        store,
		overrides:    overrides,
		adoptions:    adoptions,
		forks:        forks,
		projects:     projects,
		hierarchy:    hierarchy,
		numberer:     numberer,
		ontologyRefs: ontologyRefs,
		log:          log,
		runner:       runner,
	}
}

// pathElementsToRefs maps the field's typed path_elements onto
// weave_field_ontology_refs rows. Used by Create + Update to replay
// the field's path into the materialized refs table.
//
// Type detection comes from the path element's own .Type field — the
// path-builder UI sets it explicitly when each element is added. No
// regex heuristics; if .Type is missing we default to "class" (rare,
// but the column is non-NULL so we have to pick something).
func pathElementsToRefs(fieldID, projectID string, elements []domain.PathElement) []domain.FieldOntologyRef {
	out := make([]domain.FieldOntologyRef, 0, len(elements))
	for _, e := range elements {
		if e.Prefix == "" || e.LocalName == "" {
			continue
		}
		kind := e.Type
		if kind != "class" && kind != "property" && kind != "literal" {
			kind = "class"
		}
		out = append(out, domain.FieldOntologyRef{
			FieldID:   fieldID,
			ProjectID: projectID,
			Prefix:    e.Prefix,
			LocalName: e.LocalName,
			Position:  e.Position,
			RefKind:   kind,
		})
	}
	return out
}

// rebuildRefs fires the OntologyRefRebuilder hook. Best-effort: if
// the rebuild fails we log + return nil so the field write isn't
// rolled back. The refs table is a denormalization that can be
// repaired by a project-level rebuild sweep (future task) without
// data loss.
func (s *Service) rebuildRefs(ctx context.Context, field *domain.Field) {
	if s.ontologyRefs == nil {
		return
	}
	refs := pathElementsToRefs(field.ID, field.ProjectID, field.PathElements)
	if err := s.ontologyRefs.ReplaceFieldRefs(ctx, field.ID, field.ProjectID, refs); err != nil {
		s.log.Warn("rebuild ontology refs",
			"field_id", field.ID,
			"project_id", field.ProjectID,
			"err", err,
		)
	}
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

type ErrForbidden struct {
	Capability string
	Resource   string
}

func (e *ErrForbidden) Error() string {
	return fmt.Sprintf("forbidden: requires %s on %s", e.Capability, e.Resource)
}

// ErrEntityInUse is returned by Delete when non-base override rows
// reference the field. Handler maps to 409 with usage payload.
type ErrEntityInUse struct {
	FieldID string
	Usage   UsageReport
}

func (e *ErrEntityInUse) Error() string {
	return fmt.Sprintf("field %s is in use by %d models, %d collections",
		e.FieldID, e.Usage.ModelCount, e.Usage.CollectionCount)
}

// ErrValidation collects per-field validation errors.
type ErrValidation struct {
	Fields map[string][]string
}

func (e *ErrValidation) Error() string { return "validation error" }

// ErrSetupIncomplete is returned by Create when the project has no
// ontology coverage (no own + inherited ontology versions). Handler
// maps to 400 with a settings_url hint.
type ErrSetupIncomplete struct {
	ProjectID string
}

func (e *ErrSetupIncomplete) Error() string {
	return fmt.Sprintf("project %s has no ontology coverage; configure at least one ontology", e.ProjectID)
}

var errNotFound = errors.New("field: not found")

// IsNotFound reports whether err signals "not found".
func IsNotFound(err error) bool { return errors.Is(err, errNotFound) }

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// CreateInput is the payload accepted by Create. Includes both
// field-intrinsic state (UIName, OntologyScope, ExpectedValueType,
// etc.) and base-override state (CategoryID, SetValue) — Service
// splits them across the field write and the base override upsert.
type CreateInput struct {
	UIName            domain.Translations
	Description       domain.Translations
	SystemName        string // optional; derived from UIName.en when empty
	SemanticID        string // optional; allocated when empty
	Status            domain.Status
	OntologyScope     domain.PathElement
	OntologyPath      []domain.PathElement
	ExpectedValueType string

	// Override-shaped fields. Written into the base override row
	// (entity_type='') in the same atomic boundary.
	CategoryID            string
	SetValue              string
	ExpectedModelIDs      []string
	ExpectedCollectionIDs []string
}

// UpdateInput captures a partial update. Nil pointers / empty strings
// are "no change" except for OntologyPath where len==0 means "leave
// alone" (explicit clear has its own endpoint).
type UpdateInput struct {
	UIName            *domain.Translations
	Description       *domain.Translations
	SystemName        *string
	Status            *domain.Status
	OntologyScope     *domain.PathElement
	OntologyPath      []domain.PathElement
	ExpectedValueType *string

	// Override-shaped overrides on the base row.
	CategoryID            *string
	SetValue              *string
	ExpectedModelIDs      *[]string
	ExpectedCollectionIDs *[]string
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// List returns fields in projectID with total count. Caller must have
// project read access. Search/sort/limit/offset come through opts.
func (s *Service) List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Field, int64, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, 0, err
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vr, ok := s.store.(versionedFieldReader); ok {
			return vr.ListVersion(ctx, projectID, version, opts...)
		}
	}
	opts = append([]domain.QueryOption{domain.WithProjectID(projectID)}, opts...)
	return s.store.List(ctx, opts...)
}

// BatchUsageCounts returns per-field list aggregates (same-project model/
// collection counts, cross-project distinct-project count, in-use) for the
// given field IDs — the field-list count badges + delete gate.
func (s *Service) BatchUsageCounts(ctx context.Context, projectID string, fieldIDs []string) (map[string]domain.FieldListCounts, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.BatchUsageCounts(ctx, projectID, fieldIDs)
}

// BatchUsageRefs returns, for each field ID, the models and collections that
// place it — project-scale ownership refs (e.g. hosting-repo modules
// consuming it via the services-out seam, ADR-0008), as opposed to
// BatchUsageCounts' aggregate numbers.
// fieldIDs must come from a project-scoped read: the underlying query does not re-check that each field belongs to projectID.
func (s *Service) BatchUsageRefs(ctx context.Context, projectID string, fieldIDs []string) (map[string]domain.FieldUsageList, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.BatchUsageRefs(ctx, fieldIDs)
}

// ListAdoptedByReference returns every field from another project that
// the current project's overrides reference via field_id — task 3b.
func (s *Service) ListAdoptedByReference(ctx context.Context, projectID string) ([]*domain.Field, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListReferenceAdopted(ctx, projectID)
}

// ListAdoptedExplicit returns every field the project has adopted via
// an explicit receipt. Mirror of model.Service.ListAdoptedExplicit
// (task 3a) — same shape, same dedup, same dangling-receipt skip.
func (s *Service) ListAdoptedExplicit(ctx context.Context, projectID string) ([]*domain.Field, map[string]domain.Origin, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, nil, err
	}
	if s.adoptions == nil {
		return nil, nil, nil
	}
	opts := []domain.QueryOption{
		domain.WithProjectID(projectID),
		domain.WithFilter("context_entity_type", "project"),
		domain.WithFilter("context_entity_id", projectID),
		domain.WithFilter("entity_type", "field"),
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		opts = append(opts, domain.WithVersion(version))
	}
	adoptions, err := s.adoptions.List(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("list field adoptions: %w", err)
	}
	out := make([]*domain.Field, 0, len(adoptions))
	origins := make(map[string]domain.Origin, len(adoptions))
	seen := map[string]bool{}
	for _, a := range adoptions {
		if seen[a.SourceEntityID] {
			continue
		}
		seen[a.SourceEntityID] = true
		f, err := s.store.GetByID(ctx, a.SourceEntityID)
		if err != nil || f == nil {
			continue
		}
		out = append(out, f)
		origins[a.SourceEntityID] = domain.AdoptedOrigin(a.SourceProjectID, a.SourceEntityID)
	}
	return out, origins, nil
}

// ListBaseFieldCategories surfaces the categories assigned to base
// overrides in the project. Drives the field-list category filter.
func (s *Service) ListBaseFieldCategories(ctx context.Context, projectID string) ([]FieldCategoryOption, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListBaseFieldCategories(ctx, projectID)
}

// ListBaseFieldCategoryAssignments returns field_id → category_id for
// every base override in the project that has a category set.
func (s *Service) ListBaseFieldCategoryAssignments(ctx context.Context, projectID string) (map[string]string, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListBaseFieldCategoryAssignments(ctx, projectID)
}

// Get returns a single field by ID, or (nil, nil) when not found.
func (s *Service) Get(ctx context.Context, projectID, id string) (*domain.Field, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vr, ok := s.store.(versionedFieldReader); ok {
			return vr.GetByIDVersion(ctx, projectID, id, version)
		}
	}
	f, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, nil
	}
	if f.ProjectID != projectID {
		return nil, errNotFound
	}
	return f, nil
}

// GetByIdentifier accepts ULID, semantic_id, or system_name.
func (s *Service) GetByIdentifier(ctx context.Context, projectID, identifier string) (*domain.Field, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.GetByIdentifier(ctx, projectID, identifier)
}

// Resolved returns a single field with its base override applied. Model and
// collection context overrides are intentionally not layered here; those are
// exposed through model.View and collection.View.
func (s *Service) Resolved(ctx context.Context, projectID, id string) (*domain.ResolvedField, error) {
	field, err := s.Get(ctx, projectID, id)
	if err != nil {
		return nil, err
	}
	if field == nil {
		return nil, nil
	}

	resolved := &domain.ResolvedField{
		ID:                field.ID,
		SemanticID:        field.SemanticID,
		SystemName:        field.SystemName,
		OntologyPath:      field.OntologyPath(),
		PathElements:      append([]domain.PathElement(nil), field.PathElements...),
		DisplayName:       field.UIName,
		Description:       field.Description,
		ExpectedValueType: field.ExpectedValueType,
		OverrideSource:    "field",
	}

	if s.overrides == nil {
		return resolved, nil
	}
	base, err := s.overrides.GetBase(ctx, field.ID, projectID)
	if err != nil {
		return nil, fmt.Errorf("load base override: %w", err)
	}
	if base == nil {
		return resolved, nil
	}

	resolved.OverrideSource = "base"
	resolved.OverrideID = base.ID
	resolved.Position = base.Position
	resolved.DisplayName = base.DisplayName
	resolved.Description = base.Description
	resolved.CategoryID = base.CategoryID
	resolved.CollectionOrder = base.CollectionOrder
	resolved.CollectionName = base.CollectionName
	resolved.SetValue = base.SetValue
	resolved.IsRequired = base.IsRequired
	resolved.IsHidden = base.IsHidden
	if base.SemanticID != "" {
		resolved.SemanticID = base.SemanticID
	}
	return resolved, nil
}

// GetModels returns models that reference this field via overrides.
func (s *Service) GetModels(ctx context.Context, projectID, fieldID string) ([]domain.FieldModelRef, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.GetModels(ctx, fieldID)
}

// GetCollections returns collections that reference this field via overrides.
func (s *Service) GetCollections(ctx context.Context, projectID, fieldID string) ([]domain.FieldCollectionRef, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.GetCollections(ctx, fieldID)
}

// FindByPathSequence returns fields matching the path query.
func (s *Service) FindByPathSequence(ctx context.Context, projectID string, query domain.PathQuery) ([]*domain.Field, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.FindByPathSequence(ctx, query)
}

// IsInUse exposes the boolean directly for UI hot-path queries.
func (s *Service) IsInUse(ctx context.Context, projectID, fieldID string) (bool, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return false, err
	}
	return s.store.IsInUse(ctx, projectID, fieldID)
}

// Stats returns the field with usage counts + samples. Used by the
// stats modal in the field list.
type StatsReport struct {
	Field *domain.Field `json:"field"`
	Usage UsageReport   `json:"usage"`
}

// Stats handles GET /{fieldID}/stats payload assembly.
func (s *Service) Stats(ctx context.Context, projectID, fieldID string) (*StatsReport, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	var (
		f     *domain.Field
		usage UsageReport
		err   error
	)
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vr, ok := s.store.(versionedFieldReader); ok {
			f, err = vr.GetByIDVersion(ctx, projectID, fieldID, version)
			if err != nil {
				return nil, err
			}
			if f == nil || f.ProjectID != projectID {
				return nil, errNotFound
			}
			usage, err = vr.UsageVersion(ctx, projectID, fieldID, version)
			if err != nil {
				return nil, err
			}
		}
	}
	if f == nil {
		f, err = s.store.GetByID(ctx, fieldID)
		if err != nil {
			return nil, err
		}
		if f == nil || f.ProjectID != projectID {
			return nil, errNotFound
		}
		usage, err = s.store.Usage(ctx, projectID, fieldID)
		if err != nil {
			return nil, err
		}
	}
	return &StatsReport{Field: f, Usage: usage}, nil
}

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

// Create inserts a new field with its co-located base override. The
// project must have at least one ontology version (own or inherited)
// — otherwise ErrSetupIncomplete. Validation errors collect into
// ErrValidation. The whole operation runs inside ChangeLogRunner.Run
// so the field row + base override row + changelog entries are
// atomic.
//
// CategoryID + SetValue from CreateInput land on the base override
// row, NOT on the field — they're override-shaped (post-migration
// 027). The field gets ExpectedValueType + OntologyScope/Path/etc.
func (s *Service) Create(ctx context.Context, projectID string, in CreateInput) (*domain.Field, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, err
	}

	// Setup gate — refuse to create fields if no ontology coverage.
	if s.hierarchy != nil {
		resolved, err := s.hierarchy.ResolvedOntologyVersions(ctx, projectID, domain.ResolvedOntologyVersionOpts{})
		if err != nil {
			s.log.Warn("resolve ontology coverage", "project_id", projectID, "err", err)
		} else if len(resolved) == 0 {
			return nil, &ErrSetupIncomplete{ProjectID: projectID}
		}
	}

	if errs := validateCreate(in); len(errs) > 0 {
		return nil, &ErrValidation{Fields: errs}
	}

	systemName := in.SystemName
	if systemName == "" {
		systemName = slugify(in.UIName.Get("en"))
	}

	semanticID := in.SemanticID
	if semanticID == "" {
		next, err := s.numberer.AllocateEntityNumber(ctx, projectID, "field")
		if err != nil {
			return nil, fmt.Errorf("allocate field number: %w", err)
		}
		semanticID = fmt.Sprintf("%sF.%d", projectID, next)
	}

	status := in.Status
	if status == "" {
		status = domain.StatusDraft
	}

	scope := in.OntologyScope
	if scope.Type == "" {
		scope.Type = "class"
	}

	field := &domain.Field{
		Entity: domain.Entity{
			ID:          semanticID,
			SemanticID:  semanticID,
			SystemName:  systemName,
			UIName:      in.UIName,
			Description: in.Description,
			Status:      status,
			ProjectID:   projectID,
		},
		OntologyScope:     scope,
		PathElements:      in.OntologyPath,
		ExpectedValueType: in.ExpectedValueType,
	}

	if err := s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Create(ctx, field); err != nil {
			return fmt.Errorf("create field: %w", err)
		}
		if err := rec.Record(ctx, domain.ChangeLogEntry{
			EntityType: "field",
			EntityID:   field.ID,
			Operation:  "create",
			ProjectID:  projectID,
			Payload:    marshalField(field),
		}); err != nil {
			return err
		}

		// Co-located base override write.
		base := &domain.FieldOverride{
			FieldID:     field.ID,
			ProjectID:   projectID,
			EntityType:  "",
			EntityID:    "",
			Position:    0,
			DisplayName: in.UIName,
			Description: in.Description,
			CategoryID:  in.CategoryID,
			SetValue:    in.SetValue,
			SemanticID:  field.SemanticID,
		}
		if err := s.overrides.Create(ctx, base); err != nil {
			return fmt.Errorf("create base override: %w", err)
		}
		if err := s.overrides.SetRefs(ctx, base.ID, buildBaseOverrideRefs(in.ExpectedModelIDs, in.ExpectedCollectionIDs)); err != nil {
			return fmt.Errorf("set base override refs: %w", err)
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType: "override",
			EntityID:   fmt.Sprintf("%d", base.ID),
			Operation:  "create",
			ProjectID:  projectID,
			Payload:    marshalOverride(base),
		})
	}); err != nil {
		return nil, err
	}
	// Refresh the materialized ontology-refs table for this field.
	// Best-effort post-commit hook (see rebuildRefs).
	s.rebuildRefs(ctx, field)
	return field, nil
}

// Update applies a partial update to the field row + base override.
// Same atomic boundary as Create. Override-shaped fields
// (CategoryID, SetValue) update the base override row in place.
func (s *Service) Update(ctx context.Context, projectID, id string, in UpdateInput) (*domain.Field, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, err
	}

	field, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if field == nil || field.ProjectID != projectID {
		return nil, errNotFound
	}
	prev := *field

	if in.UIName != nil {
		field.UIName = *in.UIName
	}
	if in.Description != nil {
		field.Description = *in.Description
	}
	if in.SystemName != nil {
		field.SystemName = *in.SystemName
	}
	if in.Status != nil {
		field.Status = *in.Status
	}
	if in.ExpectedValueType != nil {
		field.ExpectedValueType = *in.ExpectedValueType
	}
	if in.OntologyScope != nil {
		field.OntologyScope = *in.OntologyScope
	}
	if len(in.OntologyPath) > 0 {
		field.PathElements = in.OntologyPath
	}

	if err := s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Update(ctx, field); err != nil {
			return fmt.Errorf("update field: %w", err)
		}
		if err := rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "field",
			EntityID:        field.ID,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalField(&prev),
			Payload:         marshalField(field),
		}); err != nil {
			return err
		}

		// Touch the base override only when override-shaped fields
		// actually change. Saves a write + changelog row when the
		// caller is updating only field-intrinsic state.
		if in.CategoryID == nil && in.SetValue == nil &&
			in.ExpectedModelIDs == nil && in.ExpectedCollectionIDs == nil &&
			in.UIName == nil && in.Description == nil {
			return nil
		}
		base, err := s.overrides.GetBase(ctx, field.ID, projectID)
		if err != nil {
			return fmt.Errorf("load base override: %w", err)
		}
		if base == nil {
			// Defensive: the base override should exist (Create
			// always writes one). Insert if missing.
			base = &domain.FieldOverride{
				FieldID:     field.ID,
				ProjectID:   projectID,
				EntityType:  "",
				EntityID:    "",
				DisplayName: field.UIName,
				Description: field.Description,
				SemanticID:  field.SemanticID,
			}
		}
		prevBase := *base
		if in.CategoryID != nil {
			base.CategoryID = *in.CategoryID
		}
		if in.SetValue != nil {
			base.SetValue = *in.SetValue
		}
		if in.UIName != nil {
			base.DisplayName = *in.UIName
		}
		if in.Description != nil {
			base.Description = *in.Description
		}
		if base.ID == 0 {
			if err := s.overrides.Create(ctx, base); err != nil {
				return fmt.Errorf("create missing base override: %w", err)
			}
			if err := s.overrides.SetRefs(ctx, base.ID, buildBaseOverrideRefs(dbutil.SliceOrEmpty(in.ExpectedModelIDs), dbutil.SliceOrEmpty(in.ExpectedCollectionIDs))); err != nil {
				return fmt.Errorf("set refs on missing base override: %w", err)
			}
			return rec.Record(ctx, domain.ChangeLogEntry{
				EntityType: "override",
				EntityID:   fmt.Sprintf("%d", base.ID),
				Operation:  "create",
				ProjectID:  projectID,
				Payload:    marshalOverride(base),
			})
		}
		if err := s.overrides.Update(ctx, base); err != nil {
			return fmt.Errorf("update base override: %w", err)
		}
		if in.ExpectedModelIDs != nil || in.ExpectedCollectionIDs != nil {
			if err := s.overrides.SetRefs(ctx, base.ID, buildBaseOverrideRefs(dbutil.SliceOrEmpty(in.ExpectedModelIDs), dbutil.SliceOrEmpty(in.ExpectedCollectionIDs))); err != nil {
				return fmt.Errorf("update base override refs: %w", err)
			}
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "override",
			EntityID:        fmt.Sprintf("%d", base.ID),
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalOverride(&prevBase),
			Payload:         marshalOverride(base),
		})
	}); err != nil {
		return nil, err
	}
	// Refresh the materialized ontology-refs table for this field.
	// Best-effort post-commit hook (see rebuildRefs).
	s.rebuildRefs(ctx, field)
	return field, nil
}

// Delete removes a field after preflight in-use check. To delete an
// in-use field the caller must Deprecate first (soft-retire) — there
// is no force-cascade option for fields because removing override
// references would silently break model/collection compositions.
func (s *Service) Delete(ctx context.Context, projectID, fieldID string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}

	field, err := s.store.GetByID(ctx, fieldID)
	if err != nil {
		return err
	}
	if field == nil || field.ProjectID != projectID {
		return errNotFound
	}

	usage, err := s.store.Usage(ctx, projectID, fieldID)
	if err != nil {
		return err
	}
	if usage.InUse() {
		return &ErrEntityInUse{FieldID: fieldID, Usage: usage}
	}

	prev := *field
	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Delete(ctx, fieldID); err != nil {
			return fmt.Errorf("delete field: %w", err)
		}
		// Base override is removed by FK cascade on weave_fields.
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "field",
			EntityID:        fieldID,
			Operation:       "delete",
			ProjectID:       projectID,
			PreviousPayload: marshalField(&prev),
		})
	})
}

// Deprecate soft-retires a field. Existing model/collection overrides
// keep referencing it; pickers exclude it from new connections.
// Idempotent.
func (s *Service) Deprecate(ctx context.Context, projectID, fieldID string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}
	field, err := s.store.GetByID(ctx, fieldID)
	if err != nil {
		return err
	}
	if field == nil || field.ProjectID != projectID {
		return errNotFound
	}
	if field.Deprecated {
		return nil
	}
	prev := *field
	after := *field
	after.Deprecated = true
	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Deprecate(ctx, fieldID); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "field",
			EntityID:        fieldID,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalField(&prev),
			Payload:         marshalField(&after),
		})
	})
}

// Activate reverses Deprecate. Idempotent.
func (s *Service) Activate(ctx context.Context, projectID, fieldID string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}
	field, err := s.store.GetByID(ctx, fieldID)
	if err != nil {
		return err
	}
	if field == nil || field.ProjectID != projectID {
		return errNotFound
	}
	if !field.Deprecated {
		return nil
	}
	prev := *field
	after := *field
	after.Deprecated = false
	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Activate(ctx, fieldID); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "field",
			EntityID:        fieldID,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalField(&prev),
			Payload:         marshalField(&after),
		})
	})
}

// ---------------------------------------------------------------------------
// Validation + helpers
// ---------------------------------------------------------------------------

func validateCreate(in CreateInput) map[string][]string {
	errs := map[string][]string{}
	if in.UIName.IsEmpty() {
		errs["ui_name"] = append(errs["ui_name"], "Field name is required")
	}
	if in.OntologyScope.LocalName == "" {
		errs["ontology_scope"] = append(errs["ontology_scope"],
			"Ontology scope (root class) is required")
	}
	if len(in.OntologyPath) == 0 {
		errs["ontology_path"] = append(errs["ontology_path"],
			"Ontology path is required — chain of properties starting from the scope")
	} else if t := in.OntologyPath[0].Type; t != "" && !isPropertyType(t) {
		errs["ontology_path"] = append(errs["ontology_path"],
			"Ontology path must start with a property element, not a class")
	}
	if strings.TrimSpace(in.CategoryID) == "" {
		errs["category_id"] = append(errs["category_id"], "Category is required")
	}
	return errs
}

func buildBaseOverrideRefs(modelIDs, collectionIDs []string) []domain.OverrideRef {
	refs := make([]domain.OverrideRef, 0, len(modelIDs)+len(collectionIDs))
	for i, id := range modelIDs {
		refs = append(refs, domain.OverrideRef{
			RefType:    "resource_model",
			TargetID:   id,
			SemanticID: id,
			Position:   i + 1,
		})
	}
	for i, id := range collectionIDs {
		refs = append(refs, domain.OverrideRef{
			RefType:    "collection_model",
			TargetID:   id,
			SemanticID: id,
			Position:   i + 1,
		})
	}
	return refs
}

func isPropertyType(t string) bool {
	t = strings.ToLower(t)
	return t == "property" || t == "edge"
}

// slugify produces a snake_case identifier from a display name. Same
// algorithm as the legacy slugifyName helper (lowercase, non-alnum→_,
// trim).
func slugify(name string) string {
	if name == "" {
		return "field"
	}
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "field"
	}
	return out
}

func marshalField(f *domain.Field) []byte {
	if f == nil {
		return nil
	}
	b, _ := json.Marshal(f)
	return b
}

func marshalOverride(o *domain.FieldOverride) []byte {
	if o == nil {
		return nil
	}
	b, _ := json.Marshal(o)
	return b
}

// ---------------------------------------------------------------------------
// Permissions
// ---------------------------------------------------------------------------

func (s *Service) requireProjectRead(ctx context.Context, projectID string) error {
	if s.projects == nil {
		return nil
	}
	project, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("load project %s for auth: %w", projectID, err)
	}
	if project == nil {
		return errNotFound
	}
	if !auth.FromContext(ctx).Can(auth.ProjectRead, auth.ProjectResource(project), nil) {
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
