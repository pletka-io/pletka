package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/override"
)

type versionedModelReader interface {
	GetByIDVersion(ctx context.Context, projectID, id, version string) (*domain.Model, error)
	ListVersion(ctx context.Context, projectID, version string, opts ...domain.QueryOption) ([]*domain.Model, int64, error)
	UsageVersion(ctx context.Context, projectID, modelID, version string) (UsageReport, error)
}

// HierarchyReader provides the project-level ontology coverage check
// used by Create's setup gate.
type HierarchyReader interface {
	ResolvedOntologyVersions(ctx context.Context, projectID string, opts domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error)
}

// EntityNumberer allocates the next sequential semantic-id number.
type EntityNumberer interface {
	AllocateEntityNumber(ctx context.Context, projectID, kind string) (int64, error)
}

type ProjectReader interface {
	GetByID(ctx context.Context, id string) (*domain.Project, error)
}

// ViewReader resolves model field overrides into the model-editor view shape.
// Satisfied by the legacy weave aggregate during the strangler-fig period.
type ViewReader interface {
	ModelView(ctx context.Context, modelID, projectID string) (*domain.ModelView, error)
}

// CategoryReader is the narrow slice of category lookup that Adapt
// (formerly Fork) needs to remap source-project CategoryIDs onto the
// current project's local categories, per the adopt/adapt rollout
// plan. Signatures match domain.WeaveCategoryStore so the legacy
// weave aggregate satisfies it directly without an adapter shim.
type CategoryReader interface {
	GetByID(ctx context.Context, id string) (*domain.Category, error)
	GetByIdentifier(ctx context.Context, identifier string, projectID string) (*domain.Category, error)
}

// AdoptionWriter is the narrow append-receipt slice that Adapt needs to
// record an adoption row when it copies a missing category from a source
// project into the current project. Satisfied by domain.AdoptionStore.
type AdoptionWriter interface {
	ReplaceForContext(ctx context.Context, projectID, contextEntityType, contextEntityID string, items []domain.Adoption) error
	List(ctx context.Context, opts ...domain.QueryOption) ([]domain.Adoption, error)
}

// Service composes Model business logic over Store. The override
// service handles the model's contextual override editing
// (entity_type='model', entity_id=this).
type Service struct {
	store      Store
	overrides  *override.Service
	adoptions  domain.AdoptionStore
	forks      domain.ForkStore
	projects   ProjectReader
	hierarchy  HierarchyReader
	numberer   EntityNumberer
	views      ViewReader
	categories CategoryReader
	log        *slog.Logger
	runner     domain.ChangeLogRunner
}

// NewService constructs a Service. categories may be nil during the
// rollout migration; Adapt falls back to its old "carry source IDs
// through" behaviour in that case, so the dependency lands cleanly
// through the router wiring without making this constructor signature
// a breaking change for callers that don't yet pass it.
func NewService(
	store Store,
	overrides *override.Service,
	adoptions domain.AdoptionStore,
	forks domain.ForkStore,
	projects ProjectReader,
	hierarchy HierarchyReader,
	numberer EntityNumberer,
	views ViewReader,
	categories CategoryReader,
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
		store:      store,
		overrides:  overrides,
		adoptions:  adoptions,
		forks:      forks,
		projects:   projects,
		hierarchy:  hierarchy,
		numberer:   numberer,
		views:      views,
		categories: categories,
		log:        log,
		runner:     runner,
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

type ErrEntityInUse struct {
	ModelID string
	Usage   UsageReport
}

func (e *ErrEntityInUse) Error() string {
	return fmt.Sprintf(
		"model %s is referenced by %d field(s) across %d project(s); deprecate it instead, or remove the references first",
		e.ModelID, e.Usage.FieldCount, len(e.Usage.ProjectIDs),
	)
}

type ErrValidation struct {
	Fields map[string][]string
}

func (e *ErrValidation) Error() string { return "validation error" }

type ErrSetupIncomplete struct {
	ProjectID string
}

func (e *ErrSetupIncomplete) Error() string {
	return fmt.Sprintf("project %s has no ontology coverage", e.ProjectID)
}

var errNotFound = errors.New("model: not found")

func IsNotFound(err error) bool { return errors.Is(err, errNotFound) }

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

type CreateInput struct {
	UIName        domain.Translations
	Description   domain.Translations
	SystemName    string
	OntologyScope domain.PathElement
	ModelType     string
}

// normaliseModelType clamps the supplied value to the domain whitelist.
// Empty string or unrecognised values fall back to the core default
// (curators opt a model DOWN to auxiliary; core is the
// headline default) so callers can pass user input through without
// explicit validation.
func normaliseModelType(t string) string {
	if !domain.IsValidModelType(t) || t == "" {
		return domain.ModelTypeCore
	}
	return t
}

type UpdateInput struct {
	UIName        *domain.Translations
	Description   *domain.Translations
	SystemName    *string
	Status        *domain.Status
	OntologyScope *domain.PathElement
	ModelType     *string
}

// StatsReport is the GET /{modelID}/stats payload.
type StatsReport struct {
	Model *domain.Model `json:"model"`
	Usage UsageReport   `json:"usage"`
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

func (s *Service) List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Model, int64, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, 0, err
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vr, ok := s.store.(versionedModelReader); ok {
			return vr.ListVersion(ctx, projectID, version, opts...)
		}
	}
	opts = append([]domain.QueryOption{domain.WithProjectID(projectID)}, opts...)
	return s.store.List(ctx, opts...)
}

// BatchCompositionCounts returns per-model composition aggregates (field/
// category/collection counts) for the given model IDs — the list count badges.
func (s *Service) BatchCompositionCounts(ctx context.Context, projectID string, modelIDs []string) (map[string]domain.CompositionCounts, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.BatchCompositionCounts(ctx, modelIDs)
}

// BatchInUse returns the subset of modelIDs referenced as a value target by a
// field in any project — the per-row delete gate for the model list.
func (s *Service) BatchInUse(ctx context.Context, projectID string, modelIDs []string) (map[string]bool, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.BatchInUse(ctx, modelIDs)
}

// ListConnectedModelIDs returns the transitive closure of model IDs
// reachable from seedIDs via override refs. Drives the Arches
// generator's full-closure walk (Q8 of the adopt/adapt rollout plan).
func (s *Service) ListConnectedModelIDs(ctx context.Context, projectID string, seedIDs []string) ([]string, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListConnectedModelIDs(ctx, seedIDs)
}

// ListAdoptedByReference returns every model from another project that
// the current project's overrides reference as a value target. Drives
// the implicit-by-reference slice of the four-state list rule (adopt/
// adapt rollout plan, task 3b). Caller is expected to dedup against
// own + receipt-adopted IDs — Q4 explicit-dominates rule means a
// receipt-adopted row always wins.
func (s *Service) ListAdoptedByReference(ctx context.Context, projectID string) ([]*domain.Model, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListReferenceAdopted(ctx, projectID)
}

// ListAdoptedExplicit returns every model the project has adopted via an
// explicit receipt (weave_adoptions rows with project_id=current,
// entity_type='model', context_entity_type='project'). Each returned
// model row carries an Origin describing its source. Read-only projection
// — the source row stays in the source project's namespace.
//
// Drives the receipt-adopted slice of the four-state list rule
// (adopt/adapt rollout plan, task 3). Reference-adopted rows arrive
// via a follow-up commit.
func (s *Service) ListAdoptedExplicit(ctx context.Context, projectID string) ([]*domain.Model, map[string]domain.Origin, error) {
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
		domain.WithFilter("entity_type", "model"),
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		opts = append(opts, domain.WithVersion(version))
	}
	adoptions, err := s.adoptions.List(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("list model adoptions: %w", err)
	}
	out := make([]*domain.Model, 0, len(adoptions))
	origins := make(map[string]domain.Origin, len(adoptions))
	seen := map[string]bool{}
	for _, a := range adoptions {
		if seen[a.SourceEntityID] {
			continue
		}
		seen[a.SourceEntityID] = true
		m, err := s.store.GetByID(ctx, a.SourceEntityID)
		if err != nil || m == nil {
			// Source disappeared upstream — receipt is dangling. Skip so
			// the list stays clean; surfacing the dead row would be
			// confusing.
			continue
		}
		out = append(out, m)
		origins[a.SourceEntityID] = domain.AdoptedOrigin(a.SourceProjectID, a.SourceEntityID)
	}
	return out, origins, nil
}

// ScopeClasses returns the distinct ontology scope classes used by
// models in the project. Drives the scope_class filter dropdown.
func (s *Service) ScopeClasses(ctx context.Context, projectID string) ([]string, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ScopeClasses(ctx, projectID)
}

func (s *Service) Get(ctx context.Context, projectID, id string) (*domain.Model, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vr, ok := s.store.(versionedModelReader); ok {
			return vr.GetByIDVersion(ctx, projectID, id, version)
		}
	}
	m, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	if m.ProjectID != projectID {
		return nil, errNotFound
	}
	return m, nil
}

func (s *Service) Stats(ctx context.Context, projectID, id string) (*StatsReport, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	var (
		m     *domain.Model
		usage UsageReport
		err   error
	)
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vr, ok := s.store.(versionedModelReader); ok {
			m, err = vr.GetByIDVersion(ctx, projectID, id, version)
			if err != nil {
				return nil, err
			}
			if m == nil || m.ProjectID != projectID {
				return nil, errNotFound
			}
			usage, err = vr.UsageVersion(ctx, projectID, id, version)
			if err != nil {
				return nil, err
			}
		}
	}
	if m == nil {
		m, err = s.store.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if m == nil || m.ProjectID != projectID {
			return nil, errNotFound
		}
		usage, err = s.store.Usage(ctx, projectID, id)
		if err != nil {
			return nil, err
		}
	}
	return &StatsReport{Model: m, Usage: usage}, nil
}

func (s *Service) ForkOriginsForProject(ctx context.Context, projectID string) (map[string]domain.Origin, error) {
	if projectID == "" {
		return map[string]domain.Origin{}, nil
	}
	opts := []domain.QueryOption{
		domain.WithProjectID(projectID),
		domain.WithFilter("entity_type", "model"),
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		opts = append(opts, domain.WithVersion(version))
	}
	forks, err := s.forks.List(ctx, opts...)
	if err != nil {
		return nil, err
	}
	out := make(map[string]domain.Origin, len(forks))
	for _, fork := range forks {
		out[fork.ForkEntityID] = domain.Origin{
			Kind:            domain.OriginForked,
			SourceProjectID: fork.SourceProjectID,
			SourceEntityID:  fork.SourceEntityID,
		}
	}
	return out, nil
}

// View returns the resolved model-editor tree with winning field overrides
// applied. The resolver still lives behind ViewReader during the migration;
// this method is the slice-owned read surface generators should consume.
func (s *Service) View(ctx context.Context, projectID, id string) (*domain.ModelView, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if _, err := s.requireOwn(ctx, projectID, id); err != nil {
		return nil, err
	}
	if s.views == nil {
		return nil, fmt.Errorf("model view reader not configured")
	}
	return s.views.ModelView(ctx, id, projectID)
}

// ---------------------------------------------------------------------------
// Override editing — delegates to override.Service
// ---------------------------------------------------------------------------

// ListOverrides returns the flat list of model-context override rows
// (entity_type='model', entity_id=modelID). Frontend groups into the
// nested category→items shape per the override editor design.
func (s *Service) ListOverrides(ctx context.Context, projectID, modelID string) ([]domain.FieldOverride, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if _, err := s.requireOwn(ctx, projectID, modelID); err != nil {
		return nil, err
	}
	return s.overrides.ListForEntity(ctx, projectID, "model", modelID)
}

// SaveOverrides bulk-replaces the model's override list with desired,
// emitting per-row changelog entries via override.Service.SaveForEntity.
// commitMessage is captured on the change-set when the postgres
// runner replaces NoopChangeLogRunner.
func (s *Service) SaveOverrides(ctx context.Context, projectID, modelID string, desired []domain.FieldOverride, commitMessage string) ([]domain.FieldOverride, override.Diff, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, override.Diff{}, err
	}
	if _, err := s.requireOwn(ctx, projectID, modelID); err != nil {
		return nil, override.Diff{}, err
	}
	saved, diff, err := s.overrides.SaveForEntity(ctx, projectID, "model", modelID, desired, commitMessage)
	if err != nil {
		return nil, override.Diff{}, err
	}
	if s.adoptions != nil {
		adoptions := buildAdoptionsFromOverrides(projectID, modelID, saved, currentActorID(ctx))
		if err := s.adoptions.ReplaceForContext(ctx, projectID, "model", modelID, adoptions); err != nil {
			return nil, override.Diff{}, fmt.Errorf("sync model adoptions: %w", err)
		}
	}
	return saved, diff, nil
}

// requireOwn loads the model and confirms it belongs to the project.
func (s *Service) requireOwn(ctx context.Context, projectID, modelID string) (*domain.Model, error) {
	m, err := s.store.GetByID(ctx, modelID)
	if err != nil {
		return nil, err
	}
	if m == nil || m.ProjectID != projectID {
		return nil, errNotFound
	}
	return m, nil
}

func (s *Service) AdoptSource(ctx context.Context, projectID, sourceModelID string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}
	if strings.TrimSpace(sourceModelID) == "" {
		return &ErrValidation{Fields: map[string][]string{
			"model_id": {"model ID is required"},
		}}
	}
	if s.adoptions == nil {
		return fmt.Errorf("model adoption store not configured")
	}

	source, err := s.store.GetByID(ctx, sourceModelID)
	if err != nil {
		return err
	}
	if source == nil {
		return errNotFound
	}
	if source.ProjectID == projectID {
		return &ErrValidation{Fields: map[string][]string{
			"model_id": {"model is already local to this project"},
		}}
	}
	if s.hasAdoption(ctx, projectID, "model", source.ProjectID, source.ID) {
		return nil
	}

	opts := []domain.QueryOption{
		domain.WithProjectID(projectID),
		domain.WithFilter("context_entity_type", "project"),
		domain.WithFilter("context_entity_id", projectID),
	}
	current, err := s.adoptions.List(ctx, opts...)
	if err != nil {
		return fmt.Errorf("list project adoptions: %w", err)
	}
	adoption := domain.Adoption{
		ProjectID:         projectID,
		ContextEntityType: "project",
		ContextEntityID:   projectID,
		EntityType:        "model",
		SourceProjectID:   source.ProjectID,
		SourceEntityID:    source.ID,
		CreatedByID:       currentActorID(ctx),
	}
	merged := mergeProjectAdoptions(current, adoption)
	if err := s.adoptions.ReplaceForContext(ctx, projectID, "project", projectID, merged); err != nil {
		return fmt.Errorf("store model adoption: %w", err)
	}
	return nil
}

func (s *Service) ForkFromSource(ctx context.Context, projectID, sourceModelID string) (*domain.Model, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sourceModelID) == "" {
		return nil, &ErrValidation{Fields: map[string][]string{
			"model_id": {"model ID is required"},
		}}
	}
	if s.adoptions == nil || s.forks == nil {
		return nil, fmt.Errorf("model fork dependencies not configured")
	}

	source, err := s.store.GetByID(ctx, sourceModelID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, errNotFound
	}
	if source.ProjectID == projectID {
		return nil, &ErrValidation{Fields: map[string][]string{
			"model_id": {"model is already local to this project"},
		}}
	}
	if !s.hasAdoption(ctx, projectID, "model", source.ProjectID, source.ID) {
		return nil, &ErrValidation{Fields: map[string][]string{
			"model_id": {"model must be adopted before it can be forked"},
		}}
	}
	if existing, err := s.existingFork(ctx, projectID, "model", source.ProjectID, source.ID); err != nil {
		return nil, err
	} else if existing != nil {
		return s.store.GetByID(ctx, existing.ForkEntityID)
	}

	systemName, err := s.ensureAvailableSystemName(ctx, projectID, firstNonEmpty(source.SystemName, slugify(source.UIName.Get("en"))), "model")
	if err != nil {
		return nil, err
	}
	created, err := s.Create(ctx, projectID, CreateInput{
		UIName:        source.UIName,
		Description:   source.Description,
		SystemName:    systemName,
		OntologyScope: source.OntologyScope,
	})
	if err != nil {
		return nil, err
	}

	cleanup := func() {
		_ = s.store.Delete(ctx, created.ID)
	}

	cloned, err := s.overrides.ListForEntity(ctx, projectID, "model", source.ID)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("load source model overrides: %w", err)
	}
	// Remap CategoryID from source-project semantic IDs (e.g. LA.CAT.3)
	// onto the current project's local equivalents (e.g. TPZ.CAT.5) by
	// system_name lookup. Most projects already carry every parent's
	// category via the inheritance-copy on project setup, so the lookup
	// hits a local row. When no local category exists with the matching
	// system_name we clear the CategoryID — leaving it as the source ID
	// would produce dangling references in the curator's project
	// (adopt/adapt rollout plan, task 3).
	catRemap := s.buildCategoryRemap(ctx, projectID, cloned)
	for i := range cloned {
		cloned[i].ID = 0
		cloned[i].EntityType = "model"
		cloned[i].EntityID = created.ID
		cloned[i].ProjectID = projectID
		cloned[i].StagingID = nil
		if cloned[i].CategoryID != "" {
			if local, ok := catRemap[cloned[i].CategoryID]; ok && local != "" {
				cloned[i].CategoryID = local
			} else {
				cloned[i].CategoryID = ""
			}
		}
	}
	saved, _, err := s.overrides.SaveForEntity(ctx, projectID, "model", created.ID, cloned, "fork model from adopted source")
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("persist forked model overrides: %w", err)
	}

	sourceOverrides, err := s.overrides.ListForEntity(ctx, projectID, "model", source.ID)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("load source override refs: %w", err)
	}
	for i := range sourceOverrides {
		if i >= len(saved) {
			break
		}
		refs, err := s.overrides.GetRefs(ctx, projectID, sourceOverrides[i].ID)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("load source override refs: %w", err)
		}
		if len(refs) == 0 {
			continue
		}
		if err := s.overrides.SetRefs(ctx, projectID, saved[i].ID, refs); err != nil {
			cleanup()
			return nil, fmt.Errorf("copy override refs: %w", err)
		}
	}

	createdByID := currentActorID(ctx)
	adoptions := buildAdoptionsFromOverrides(projectID, created.ID, saved, createdByID)
	if err := s.adoptions.ReplaceForContext(ctx, projectID, "model", created.ID, adoptions); err != nil {
		cleanup()
		return nil, fmt.Errorf("sync model fork adoptions: %w", err)
	}

	fork := &domain.EntityFork{
		ProjectID:       projectID,
		EntityType:      "model",
		ForkEntityID:    created.ID,
		SourceProjectID: source.ProjectID,
		SourceEntityID:  source.ID,
		CreatedByID:     createdByID,
	}
	if err := s.forks.Create(ctx, fork); err != nil {
		cleanup()
		return nil, fmt.Errorf("record model fork: %w", err)
	}
	return created, nil
}

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

func (s *Service) Create(ctx context.Context, projectID string, in CreateInput) (*domain.Model, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, err
	}

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

	next, err := s.numberer.AllocateEntityNumber(ctx, projectID, "model")
	if err != nil {
		return nil, fmt.Errorf("allocate model number: %w", err)
	}
	semanticID := fmt.Sprintf("%sM.%d", projectID, next)

	scope := in.OntologyScope
	if scope.Type == "" {
		scope.Type = "class"
	}

	m := &domain.Model{
		Entity: domain.Entity{
			ID:          semanticID,
			SemanticID:  semanticID,
			SystemName:  systemName,
			UIName:      in.UIName,
			Description: in.Description,
			Status:      domain.StatusDraft,
			ProjectID:   projectID,
		},
		OntologyScope: scope,
		ModelType:     normaliseModelType(in.ModelType),
	}

	if err := s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Create(ctx, m); err != nil {
			return fmt.Errorf("create model: %w", err)
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType: "model",
			EntityID:   m.ID,
			Operation:  "create",
			ProjectID:  projectID,
			Payload:    marshalModel(m),
		})
	}); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) Update(ctx context.Context, projectID, id string, in UpdateInput) (*domain.Model, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, err
	}
	m, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if m == nil || m.ProjectID != projectID {
		return nil, errNotFound
	}
	prev := *m
	if in.UIName != nil {
		m.UIName = *in.UIName
	}
	if in.Description != nil {
		m.Description = *in.Description
	}
	if in.SystemName != nil {
		m.SystemName = *in.SystemName
	}
	if in.Status != nil {
		m.Status = *in.Status
	}
	if in.OntologyScope != nil {
		m.OntologyScope = *in.OntologyScope
	}
	if in.ModelType != nil {
		m.ModelType = normaliseModelType(*in.ModelType)
	}
	if err := s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Update(ctx, m); err != nil {
			return fmt.Errorf("update model: %w", err)
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "model",
			EntityID:        m.ID,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalModel(&prev),
			Payload:         marshalModel(m),
		})
	}); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete removes the model with in-use preflight. To delete an in-use
// model the caller must Deprecate first — there's no force-cascade
// because removing the value-target reference would silently break
// downstream field overrides.
func (s *Service) Delete(ctx context.Context, projectID, id string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}
	m, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if m == nil || m.ProjectID != projectID {
		return errNotFound
	}
	usage, err := s.store.UsageGlobal(ctx, id)
	if err != nil {
		return err
	}
	if usage.InUse() {
		return &ErrEntityInUse{ModelID: id, Usage: usage}
	}
	prev := *m
	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		// The model's own override rows (entity_type='model',
		// entity_id=id) are removed by the FK cascade on weave_models.
		if err := s.store.Delete(ctx, id); err != nil {
			return fmt.Errorf("delete model: %w", err)
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "model",
			EntityID:        id,
			Operation:       "delete",
			ProjectID:       projectID,
			PreviousPayload: marshalModel(&prev),
		})
	})
}

func (s *Service) Deprecate(ctx context.Context, projectID, id string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}
	m, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if m == nil || m.ProjectID != projectID {
		return errNotFound
	}
	if m.Deprecated {
		return nil
	}
	prev := *m
	after := *m
	after.Deprecated = true
	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Deprecate(ctx, id); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "model",
			EntityID:        id,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalModel(&prev),
			Payload:         marshalModel(&after),
		})
	})
}

func (s *Service) Activate(ctx context.Context, projectID, id string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}
	m, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if m == nil || m.ProjectID != projectID {
		return errNotFound
	}
	if !m.Deprecated {
		return nil
	}
	prev := *m
	after := *m
	after.Deprecated = false
	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Activate(ctx, id); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "model",
			EntityID:        id,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalModel(&prev),
			Payload:         marshalModel(&after),
		})
	})
}

// ---------------------------------------------------------------------------
// Validation + helpers
// ---------------------------------------------------------------------------

func validateCreate(in CreateInput) map[string][]string {
	errs := map[string][]string{}
	if in.UIName.IsEmpty() {
		errs["ui_name"] = append(errs["ui_name"], "Name is required")
	}
	if in.OntologyScope.LocalName == "" {
		errs["ontology_scope"] = append(errs["ontology_scope"], "Ontology scope is required")
	}
	return errs
}

func slugify(name string) string {
	if name == "" {
		return "model"
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
		return "model"
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Service) ensureAvailableSystemName(ctx context.Context, projectID, base, fallback string) (string, error) {
	base = slugify(base)
	if strings.TrimSpace(base) == "" {
		base = fallback
	}
	candidate := base + "_fork"
	if candidate == "_fork" {
		candidate = fallback + "_fork"
	}
	for i := 1; i <= 1000; i++ {
		probe := candidate
		if i > 1 {
			probe = fmt.Sprintf("%s_%d", candidate, i)
		}
		existing, err := s.store.GetByIdentifier(ctx, projectID, probe)
		if err != nil {
			return "", fmt.Errorf("check model system name %q: %w", probe, err)
		}
		if existing == nil {
			return probe, nil
		}
	}
	return "", fmt.Errorf("allocate unique model system name for %q", base)
}

func (s *Service) hasAdoption(ctx context.Context, projectID, entityType, sourceProjectID, sourceEntityID string) bool {
	adoptions, err := s.adoptions.List(ctx,
		domain.WithProjectID(projectID),
		domain.WithFilter("entity_type", entityType),
		domain.WithFilter("source_project_id", sourceProjectID),
		domain.WithFilter("source_entity_id", sourceEntityID),
	)
	return err == nil && len(adoptions) > 0
}

func (s *Service) existingFork(ctx context.Context, projectID, entityType, sourceProjectID, sourceEntityID string) (*domain.EntityFork, error) {
	forks, err := s.forks.List(ctx,
		domain.WithProjectID(projectID),
		domain.WithFilter("entity_type", entityType),
		domain.WithFilter("source_project_id", sourceProjectID),
		domain.WithFilter("source_entity_id", sourceEntityID),
	)
	if err != nil {
		return nil, fmt.Errorf("list existing forks: %w", err)
	}
	if len(forks) == 0 {
		return nil, nil
	}
	return &forks[0], nil
}

func currentActorID(ctx context.Context) *string {
	principal := auth.PrincipalFromContext(ctx)
	if principal == nil || strings.TrimSpace(principal.ActorID) == "" {
		return nil
	}
	return &principal.ActorID
}

// buildCategoryRemap walks the source overrides, collects every
// distinct non-empty CategoryID, and returns a map from each source ID
// to its current-project equivalent. The lookup goes source ID → source
// category row (project-agnostic GetByID) → system_name → current
// project's GetByIdentifier. Misses (no local category with that
// system_name) map to "" so the caller can clear the dangling
// reference instead of preserving an unreachable pointer.
//
// Returns an empty map when no CategoryReader is wired (which
// preserves the old behaviour of letting source IDs flow through
// unchanged) so tests that don't need this slice can construct a
// Service with nil categories.
func (s *Service) buildCategoryRemap(ctx context.Context, currentProjectID string, overrides []domain.FieldOverride) map[string]string {
	out := map[string]string{}
	if s.categories == nil {
		return out
	}
	seen := map[string]bool{}
	for _, row := range overrides {
		if row.CategoryID == "" || seen[row.CategoryID] {
			continue
		}
		seen[row.CategoryID] = true
		sourceCat, err := s.categories.GetByID(ctx, row.CategoryID)
		if err != nil || sourceCat == nil || strings.TrimSpace(sourceCat.SystemName) == "" {
			continue
		}
		localCat, err := s.categories.GetByIdentifier(ctx, sourceCat.SystemName, currentProjectID)
		if err != nil || localCat == nil {
			out[row.CategoryID] = ""
			continue
		}
		out[row.CategoryID] = localCat.ID
	}
	return out
}

func buildAdoptionsFromOverrides(projectID, contextModelID string, overrides []domain.FieldOverride, createdByID *string) []domain.Adoption {
	out := make([]domain.Adoption, 0)
	seen := make(map[string]bool)
	for _, row := range overrides {
		fieldProjectID := domain.ParseSemanticID(row.FieldID).ProjectID
		if fieldProjectID != "" && fieldProjectID != projectID {
			adoption := domain.Adoption{
				ProjectID:         projectID,
				ContextEntityType: "model",
				ContextEntityID:   contextModelID,
				EntityType:        "field",
				SourceProjectID:   fieldProjectID,
				SourceEntityID:    row.FieldID,
				CreatedByID:       createdByID,
			}
			key := adoption.SourceProjectID + "|" + adoption.EntityType + "|" + adoption.SourceEntityID
			if !seen[key] {
				seen[key] = true
				out = append(out, adoption)
			}
		}
		collectionProjectID := domain.ParseSemanticID(row.PartOfCollectionID).ProjectID
		if row.PartOfCollectionID != "" && collectionProjectID != "" && collectionProjectID != projectID {
			adoption := domain.Adoption{
				ProjectID:         projectID,
				ContextEntityType: "model",
				ContextEntityID:   contextModelID,
				EntityType:        "collection",
				SourceProjectID:   collectionProjectID,
				SourceEntityID:    row.PartOfCollectionID,
				CreatedByID:       createdByID,
			}
			key := adoption.SourceProjectID + "|" + adoption.EntityType + "|" + adoption.SourceEntityID
			if !seen[key] {
				seen[key] = true
				out = append(out, adoption)
			}
		}
	}
	return out
}

func mergeProjectAdoptions(existing []domain.Adoption, extra domain.Adoption) []domain.Adoption {
	merged := make([]domain.Adoption, 0, len(existing)+1)
	seen := make(map[string]bool, len(existing)+1)
	for _, adoption := range existing {
		key := adoptionReceiptKey(adoption)
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, adoption)
	}
	key := adoptionReceiptKey(extra)
	if !seen[key] {
		merged = append(merged, extra)
	}
	return merged
}

func adoptionReceiptKey(a domain.Adoption) string {
	return strings.Join([]string{
		a.ContextEntityType,
		a.ContextEntityID,
		a.EntityType,
		a.SourceProjectID,
		a.SourceEntityID,
		a.SourceVersion,
	}, "|")
}

func marshalModel(m *domain.Model) []byte {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	return b
}

// ---------------------------------------------------------------------------
// Permissions
// ---------------------------------------------------------------------------

func (s *Service) requireProjectRead(ctx context.Context, projectID string) error {
	visibility := "public"
	orgID := ""
	if s.projects != nil {
		project, err := s.projects.GetByID(ctx, projectID)
		if err != nil {
			return fmt.Errorf("load project %s for auth: %w", projectID, err)
		}
		if project == nil {
			return errNotFound
		}
		if project.Visibility != "" {
			visibility = project.Visibility
		}
		orgID = project.OwnerID
	}
	snap := auth.FromContext(ctx)
	res := auth.Resource{ScopeType: "project", ID: projectID, OrgID: orgID, Visibility: visibility}
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
