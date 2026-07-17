package collection

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

type versionedCollectionReader interface {
	GetByIDVersion(ctx context.Context, projectID, id, version string) (*domain.Collection, error)
	ListVersion(ctx context.Context, projectID, version string, opts ...domain.QueryOption) ([]*domain.Collection, int64, error)
	UsageVersion(ctx context.Context, projectID, collectionID, version string) (UsageReport, error)
}

type HierarchyReader interface {
	ResolvedOntologyVersions(ctx context.Context, projectID string, opts domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error)
}

type EntityNumberer interface {
	AllocateEntityNumber(ctx context.Context, projectID, kind string) (int64, error)
}

type ProjectReader interface {
	GetByID(ctx context.Context, id string) (*domain.Project, error)
}

// ViewReader resolves collection field overrides into render-ready fields.
// Satisfied by the legacy weave aggregate during the strangler-fig period.
type ViewReader interface {
	CollectionView(ctx context.Context, collectionID, projectID string) ([]domain.ResolvedField, error)
}

// CategoryReader is the narrow slice of category lookup that Adapt
// (formerly Fork) needs to remap source-project CategoryIDs onto the
// current project's local categories. Signatures match
// domain.WeaveCategoryStore so the legacy weave aggregate satisfies it
// directly. Mirrors model.CategoryReader; both slices need the same
// remap on their Adapt paths (adopt/adapt implementation task 3).
type CategoryReader interface {
	GetByID(ctx context.Context, id string) (*domain.Category, error)
	GetByIdentifier(ctx context.Context, identifier string, projectID string) (*domain.Category, error)
}

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
// rollout migration; the Adapt path falls back to its old behaviour
// (source IDs carried through) so tests can construct a Service
// without wiring this slice.
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
	CollectionID string
	Usage        UsageReport
}

func (e *ErrEntityInUse) Error() string {
	return fmt.Sprintf("collection %s is referenced by %d fields", e.CollectionID, e.Usage.FieldCount)
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

var errNotFound = errors.New("collection: not found")

func IsNotFound(err error) bool { return errors.Is(err, errNotFound) }

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

type CreateInput struct {
	UIName            domain.Translations
	Description       domain.Translations
	SystemName        string
	OntologyScope     domain.PathElement
	DefaultCategoryID string
}

type UpdateInput struct {
	UIName                   *domain.Translations
	Description              *domain.Translations
	SystemName               *string
	Status                   *domain.Status
	OntologyScope            *domain.PathElement
	DefaultCategoryID        *string
	CollectionNumber         *int
	CanonicalCollectionOrder *int
}

type StatsReport struct {
	Collection *domain.Collection `json:"collection"`
	Usage      UsageReport        `json:"usage"`
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

func (s *Service) List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Collection, int64, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, 0, err
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vr, ok := s.store.(versionedCollectionReader); ok {
			return vr.ListVersion(ctx, projectID, version, opts...)
		}
	}
	opts = append([]domain.QueryOption{domain.WithProjectID(projectID)}, opts...)
	return s.store.List(ctx, opts...)
}

// BatchCompositionCounts returns per-collection field counts for the given
// collection IDs — the list field-count badge.
func (s *Service) BatchCompositionCounts(ctx context.Context, projectID string, collectionIDs []string) (map[string]domain.CompositionCounts, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.BatchCompositionCounts(ctx, collectionIDs)
}

// ListAdoptedByReference returns every collection from another project
// that the current project's field overrides reference via
// part_of_collection_id. Drives the reference slice of the four-state
// list rule — task 3b.
func (s *Service) ListAdoptedByReference(ctx context.Context, projectID string) ([]*domain.Collection, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListReferenceAdopted(ctx, projectID)
}

// ListAdoptedExplicit returns every collection the project has adopted
// via an explicit receipt. Mirror of model.Service.ListAdoptedExplicit
// from task 3a — same pattern, same dedup, same dangling-receipt skip.
func (s *Service) ListAdoptedExplicit(ctx context.Context, projectID string) ([]*domain.Collection, map[string]domain.Origin, error) {
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
		domain.WithFilter("entity_type", "collection"),
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		opts = append(opts, domain.WithVersion(version))
	}
	adoptions, err := s.adoptions.List(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("list collection adoptions: %w", err)
	}
	out := make([]*domain.Collection, 0, len(adoptions))
	origins := make(map[string]domain.Origin, len(adoptions))
	seen := map[string]bool{}
	for _, a := range adoptions {
		if seen[a.SourceEntityID] {
			continue
		}
		seen[a.SourceEntityID] = true
		c, err := s.store.GetByID(ctx, a.SourceEntityID)
		if err != nil || c == nil {
			continue
		}
		out = append(out, c)
		origins[a.SourceEntityID] = domain.AdoptedOrigin(a.SourceProjectID, a.SourceEntityID)
	}
	return out, origins, nil
}

// ScopeClasses returns the distinct ontology scope classes used by
// collections in the project. Drives the scope_class filter dropdown.
func (s *Service) ScopeClasses(ctx context.Context, projectID string) ([]string, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ScopeClasses(ctx, projectID)
}

// CategoriesUsed returns the distinct categories assigned to
// collections in the project. Drives the category filter dropdown
// on the collection list.
func (s *Service) CategoriesUsed(ctx context.Context, projectID string) ([]CollectionCategoryOption, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.CategoriesUsed(ctx, projectID)
}

func (s *Service) Get(ctx context.Context, projectID, id string) (*domain.Collection, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vr, ok := s.store.(versionedCollectionReader); ok {
			return vr.GetByIDVersion(ctx, projectID, id, version)
		}
	}
	c, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, nil
	}
	if c.ProjectID != projectID {
		return nil, errNotFound
	}
	return c, nil
}

func (s *Service) Stats(ctx context.Context, projectID, id string) (*StatsReport, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	var (
		c     *domain.Collection
		usage UsageReport
		err   error
	)
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vr, ok := s.store.(versionedCollectionReader); ok {
			c, err = vr.GetByIDVersion(ctx, projectID, id, version)
			if err != nil {
				return nil, err
			}
			if c == nil || c.ProjectID != projectID {
				return nil, errNotFound
			}
			usage, err = vr.UsageVersion(ctx, projectID, id, version)
			if err != nil {
				return nil, err
			}
		}
	}
	if c == nil {
		c, err = s.store.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if c == nil || c.ProjectID != projectID {
			return nil, errNotFound
		}
		usage, err = s.store.Usage(ctx, projectID, id)
		if err != nil {
			return nil, err
		}
	}
	return &StatsReport{Collection: c, Usage: usage}, nil
}

func (s *Service) ForkOriginsForProject(ctx context.Context, projectID string) (map[string]domain.Origin, error) {
	if projectID == "" {
		return map[string]domain.Origin{}, nil
	}
	opts := []domain.QueryOption{
		domain.WithProjectID(projectID),
		domain.WithFilter("entity_type", "collection"),
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

// LookupByID returns a collection by its global ID without applying a
// project-scoped ownership check. It is intended for internal cross-slice
// resolution where route/project auth has already been enforced and the caller
// only needs the collection's source project.
func (s *Service) LookupByID(ctx context.Context, id string) (*domain.Collection, error) {
	return s.store.GetByID(ctx, id)
}

// LookupView returns a collection's resolved fields by ID, looking up
// the owning project automatically. Same auth posture as LookupByID:
// no project gate — caller has already been authorised at the
// snapshot-build level. Used by snap-level collection composition to
// fetch the fields of a cross-project anchored collection (e.g. LA's
// Dimension grafted under an OGEE model's timespan_duration stub).
func (s *Service) LookupView(ctx context.Context, id string) ([]domain.ResolvedField, error) {
	c, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("collection %s not found", id)
	}
	if s.views == nil {
		return nil, fmt.Errorf("collection view reader not configured")
	}
	return s.views.CollectionView(ctx, id, c.ProjectID)
}

// AnchorIndex returns every non-deprecated collection paired with its
// anchor CIDOC class URI (last class element of the longest common
// path prefix across the collection's fields). Cross-project — no
// auth gate. Used by snap-level composition.
func (s *Service) AnchorIndex(ctx context.Context) ([]domain.CollectionAnchor, error) {
	return s.store.AnchorIndex(ctx)
}

// View returns collection fields with winning overrides applied. The resolver
// still lives behind ViewReader during the migration; this method is the
// slice-owned read surface generators should consume.
func (s *Service) View(ctx context.Context, projectID, id string) ([]domain.ResolvedField, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if _, err := s.requireOwn(ctx, projectID, id); err != nil {
		return nil, err
	}
	if s.views == nil {
		return nil, fmt.Errorf("collection view reader not configured")
	}
	return s.views.CollectionView(ctx, id, projectID)
}

// ---------------------------------------------------------------------------
// Override editing — delegates to override.Service
// ---------------------------------------------------------------------------

func (s *Service) ListOverrides(ctx context.Context, projectID, collectionID string) ([]domain.FieldOverride, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if _, err := s.requireOwn(ctx, projectID, collectionID); err != nil {
		return nil, err
	}
	return s.overrides.ListForEntity(ctx, projectID, "collection", collectionID)
}

func (s *Service) SaveOverrides(ctx context.Context, projectID, collectionID string, desired []domain.FieldOverride, commitMessage string) ([]domain.FieldOverride, override.Diff, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, override.Diff{}, err
	}
	if _, err := s.requireOwn(ctx, projectID, collectionID); err != nil {
		return nil, override.Diff{}, err
	}
	return s.overrides.SaveForEntity(ctx, projectID, "collection", collectionID, desired, commitMessage)
}

func (s *Service) requireOwn(ctx context.Context, projectID, collectionID string) (*domain.Collection, error) {
	c, err := s.store.GetByID(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	if c == nil || c.ProjectID != projectID {
		return nil, errNotFound
	}
	return c, nil
}

func (s *Service) ForkFromSource(ctx context.Context, projectID, sourceCollectionID string) (*domain.Collection, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sourceCollectionID) == "" {
		return nil, &ErrValidation{Fields: map[string][]string{
			"collection_id": {"collection ID is required"},
		}}
	}
	if s.adoptions == nil || s.forks == nil {
		return nil, fmt.Errorf("collection fork dependencies not configured")
	}

	source, err := s.store.GetByID(ctx, sourceCollectionID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, errNotFound
	}
	if source.ProjectID == projectID {
		return nil, &ErrValidation{Fields: map[string][]string{
			"collection_id": {"collection is already local to this project"},
		}}
	}
	if !s.hasAdoption(ctx, projectID, "collection", source.ProjectID, source.ID) {
		return nil, &ErrValidation{Fields: map[string][]string{
			"collection_id": {"collection must be adopted before it can be forked"},
		}}
	}
	if existing, err := s.existingFork(ctx, projectID, "collection", source.ProjectID, source.ID); err != nil {
		return nil, err
	} else if existing != nil {
		return s.store.GetByID(ctx, existing.ForkEntityID)
	}

	systemName, err := s.ensureAvailableSystemName(ctx, projectID, firstNonEmpty(source.SystemName, slugify(source.UIName.Get("en"))), "collection")
	if err != nil {
		return nil, err
	}
	// Build the category remap upfront so both the new collection's
	// DefaultCategoryID and the cloned overrides' CategoryIDs are
	// rewritten consistently from source-project semantic IDs onto the
	// current project's local equivalents (adopt/adapt implementation
	// task 3).
	srcOverridesForRemap, err := s.overrides.ListForEntity(ctx, projectID, "collection", source.ID)
	if err != nil {
		return nil, fmt.Errorf("load source overrides for remap: %w", err)
	}
	seedCatIDs := make([]string, 0, len(srcOverridesForRemap)+1)
	if source.DefaultCategoryID != nil {
		seedCatIDs = append(seedCatIDs, *source.DefaultCategoryID)
	}
	for _, row := range srcOverridesForRemap {
		seedCatIDs = append(seedCatIDs, row.CategoryID)
	}
	catRemap := s.buildCategoryRemap(ctx, projectID, seedCatIDs)

	in := CreateInput{
		UIName:        source.UIName,
		Description:   source.Description,
		SystemName:    systemName,
		OntologyScope: source.OntologyScope,
	}
	if source.DefaultCategoryID != nil {
		if local, ok := catRemap[*source.DefaultCategoryID]; ok {
			in.DefaultCategoryID = local
		}
		// If the source default exists but has no local equivalent the
		// new collection is created without a default. Curators can pick
		// one inline; leaving a dangling LA pointer here would crash the
		// editor.
	}

	created, err := s.Create(ctx, projectID, in)
	if err != nil {
		return nil, err
	}

	cleanup := func() {
		_ = s.store.Delete(ctx, created.ID)
	}

	cloned, err := s.overrides.CloneFromCollection(ctx, projectID, source.ID, "collection", created.ID, in.DefaultCategoryID)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("clone source collection overrides: %w", err)
	}
	for i := range cloned {
		cloned[i].PartOfCollectionID = created.ID
		cloned[i].CollectionName = created.UIName
		if cloned[i].CategoryID != "" {
			if local, ok := catRemap[cloned[i].CategoryID]; ok && local != "" {
				cloned[i].CategoryID = local
			} else {
				cloned[i].CategoryID = ""
			}
		}
	}
	saved, _, err := s.overrides.SaveForEntity(ctx, projectID, "collection", created.ID, cloned, "fork collection from adopted source")
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("persist forked collection overrides: %w", err)
	}

	sourceOverrides, err := s.overrides.ListForEntity(ctx, projectID, "collection", source.ID)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("load source collection overrides: %w", err)
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

	principal := auth.PrincipalFromContext(ctx)
	var createdByID *string
	if principal != nil && principal.ActorID != "" {
		createdByID = &principal.ActorID
	}
	adoptions := buildAdoptionsFromOverrides(projectID, created.ID, saved, createdByID)
	if err := s.adoptions.ReplaceForContext(ctx, projectID, "collection", created.ID, adoptions); err != nil {
		cleanup()
		return nil, fmt.Errorf("sync collection fork adoptions: %w", err)
	}

	fork := &domain.EntityFork{
		ProjectID:       projectID,
		EntityType:      "collection",
		ForkEntityID:    created.ID,
		SourceProjectID: source.ProjectID,
		SourceEntityID:  source.ID,
		CreatedByID:     createdByID,
	}
	if err := s.forks.Create(ctx, fork); err != nil {
		cleanup()
		return nil, fmt.Errorf("record collection fork: %w", err)
	}
	return created, nil
}

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

func (s *Service) Create(ctx context.Context, projectID string, in CreateInput) (*domain.Collection, error) {
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

	next, err := s.numberer.AllocateEntityNumber(ctx, projectID, "collection")
	if err != nil {
		return nil, fmt.Errorf("allocate collection number: %w", err)
	}
	semanticID := fmt.Sprintf("%sC.%d", projectID, next)

	scope := in.OntologyScope
	if scope.Type == "" {
		scope.Type = "class"
	}

	var defaultCat *string
	if in.DefaultCategoryID != "" {
		v := in.DefaultCategoryID
		defaultCat = &v
	}

	c := &domain.Collection{
		Entity: domain.Entity{
			ID:          semanticID,
			SemanticID:  semanticID,
			SystemName:  systemName,
			UIName:      in.UIName,
			Description: in.Description,
			Status:      domain.StatusDraft,
			ProjectID:   projectID,
		},
		OntologyScope:     scope,
		DefaultCategoryID: defaultCat,
	}

	if err := s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Create(ctx, c); err != nil {
			return fmt.Errorf("create collection: %w", err)
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType: "collection",
			EntityID:   c.ID,
			Operation:  "create",
			ProjectID:  projectID,
			Payload:    marshalCollection(c),
		})
	}); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) Update(ctx context.Context, projectID, id string, in UpdateInput) (*domain.Collection, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, err
	}
	c, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil || c.ProjectID != projectID {
		return nil, errNotFound
	}
	prev := *c
	if in.UIName != nil {
		c.UIName = *in.UIName
	}
	if in.Description != nil {
		c.Description = *in.Description
	}
	if in.SystemName != nil {
		c.SystemName = *in.SystemName
	}
	if in.Status != nil {
		c.Status = *in.Status
	}
	if in.OntologyScope != nil {
		c.OntologyScope = *in.OntologyScope
	}
	if in.DefaultCategoryID != nil {
		if *in.DefaultCategoryID == "" {
			c.DefaultCategoryID = nil
		} else {
			v := *in.DefaultCategoryID
			c.DefaultCategoryID = &v
		}
	}
	if in.CollectionNumber != nil {
		c.CollectionNumber = *in.CollectionNumber
	}
	if in.CanonicalCollectionOrder != nil {
		c.CanonicalCollectionOrder = *in.CanonicalCollectionOrder
	}
	if err := s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Update(ctx, c); err != nil {
			return fmt.Errorf("update collection: %w", err)
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "collection",
			EntityID:        c.ID,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalCollection(&prev),
			Payload:         marshalCollection(c),
		})
	}); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) Delete(ctx context.Context, projectID, id string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}
	c, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if c == nil || c.ProjectID != projectID {
		return errNotFound
	}
	usage, err := s.store.Usage(ctx, projectID, id)
	if err != nil {
		return err
	}
	if usage.InUse() {
		return &ErrEntityInUse{CollectionID: id, Usage: usage}
	}
	prev := *c
	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Delete(ctx, id); err != nil {
			return fmt.Errorf("delete collection: %w", err)
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "collection",
			EntityID:        id,
			Operation:       "delete",
			ProjectID:       projectID,
			PreviousPayload: marshalCollection(&prev),
		})
	})
}

func (s *Service) Deprecate(ctx context.Context, projectID, id string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}
	c, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if c == nil || c.ProjectID != projectID {
		return errNotFound
	}
	if c.Deprecated {
		return nil
	}
	prev := *c
	after := *c
	after.Deprecated = true
	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Deprecate(ctx, id); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "collection",
			EntityID:        id,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalCollection(&prev),
			Payload:         marshalCollection(&after),
		})
	})
}

func (s *Service) Activate(ctx context.Context, projectID, id string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}
	c, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if c == nil || c.ProjectID != projectID {
		return errNotFound
	}
	if !c.Deprecated {
		return nil
	}
	prev := *c
	after := *c
	after.Deprecated = false
	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Activate(ctx, id); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "collection",
			EntityID:        id,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalCollection(&prev),
			Payload:         marshalCollection(&after),
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
		return "collection"
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
		return "collection"
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
			return "", fmt.Errorf("check collection system name %q: %w", probe, err)
		}
		if existing == nil {
			return probe, nil
		}
	}
	return "", fmt.Errorf("allocate unique collection system name for %q", base)
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

func buildAdoptionsFromOverrides(projectID, contextCollectionID string, overrides []domain.FieldOverride, createdByID *string) []domain.Adoption {
	out := make([]domain.Adoption, 0)
	seen := make(map[string]bool)
	for _, row := range overrides {
		fieldProjectID := domain.ParseSemanticID(row.FieldID).ProjectID
		if fieldProjectID != "" && fieldProjectID != projectID {
			adoption := domain.Adoption{
				ProjectID:         projectID,
				ContextEntityType: "collection",
				ContextEntityID:   contextCollectionID,
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
				ContextEntityType: "collection",
				ContextEntityID:   contextCollectionID,
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

// buildCategoryRemap walks a list of source-project CategoryIDs and
// returns a map from each source ID to its current-project equivalent
// (resolved via system_name). Misses map to "" so the caller can clear
// the dangling reference. nil CategoryReader yields an empty map,
// letting source IDs flow through unchanged — keeps tests that don't
// wire this slice working.
func (s *Service) buildCategoryRemap(ctx context.Context, currentProjectID string, sourceIDs []string) map[string]string {
	out := map[string]string{}
	if s.categories == nil {
		return out
	}
	seen := map[string]bool{}
	for _, id := range sourceIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		sourceCat, err := s.categories.GetByID(ctx, id)
		if err != nil || sourceCat == nil || strings.TrimSpace(sourceCat.SystemName) == "" {
			continue
		}
		localCat, err := s.categories.GetByIdentifier(ctx, sourceCat.SystemName, currentProjectID)
		if err != nil || localCat == nil {
			out[id] = ""
			continue
		}
		out[id] = localCat.ID
	}
	return out
}

func marshalCollection(c *domain.Collection) []byte {
	if c == nil {
		return nil
	}
	b, _ := json.Marshal(c)
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
