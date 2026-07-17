package ontology

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
)

// ProjectReader is the cross-slice reader the service uses for path
// validation + autocomplete. Resolves a project's selected ontology
// versions including parent-project inheritance.
type ProjectReader interface {
	ResolvedOntologyVersions(ctx context.Context, projectID string, opts domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error)
}

// ErrNotFound is returned by Service read methods when the requested
// entity does not exist. Handlers translate this to 404.
var ErrNotFound = errors.New("ontology: not found")

// Service composes the ontology slice's business logic over Store plus
// the supporting subpackages (rdf, manifest, autocomplete, io).
//
// Reads + writes + import + autocomplete now wired (Phases A + B + C +
// D). Phase E hooks RebuildFieldRefs into pkg/weave/field on save.
type Service struct {
	store        Store
	projects     ProjectReader
	log          *slog.Logger
	autocomplete *autocomplete.DispatchEngine
	indexCache   *autocomplete.IndexCache
	bus          domain.EventBus
}

type FamilyBrowseItem struct {
	ID               string              `json:"id"`
	Slug             string              `json:"slug"`
	Name             string              `json:"name"`
	Description      domain.Translations `json:"description,omitempty"`
	ParentFamilyID   *string             `json:"parent_family_id,omitempty"`
	ParentFamilyName string              `json:"parent_family_name,omitempty"`
	Icon             string              `json:"icon,omitempty"`
	DisplayOrder     int64               `json:"display_order"`
	OntologyCount    int64               `json:"ontology_count"`
}

type OntologyBrowseItem struct {
	ID                  string              `json:"id"`
	Prefix              string              `json:"prefix"`
	Namespace           string              `json:"namespace"`
	Name                string              `json:"name"`
	Description         domain.Translations `json:"description,omitempty"`
	FamilyID            *string             `json:"family_id,omitempty"`
	FamilyName          string              `json:"family_name,omitempty"`
	OntologyType        domain.OntologyType `json:"ontology_type"`
	ExtendsOntologyID   *string             `json:"extends_ontology_id,omitempty"`
	ExtendsOntologyName string              `json:"extends_ontology_name,omitempty"`
}

type FamilyBrowseInput struct {
	Search  string
	SortBy  string
	Page    int
	PerPage int
}

type OntologyBrowseInput struct {
	Search       string
	FamilyID     string
	OntologyType string
	SortBy       string
	Page         int
	PerPage      int
}

type BrowseResult[T any] struct {
	Items []T
	Total int
}

type ImportVersionOptions struct {
	// VersionNamespaceBinding persists version-level prefix/namespace metadata
	// alongside the imported RDF rows. This covers manifest-declared version
	// prefixes that may not be declared in the RDF itself.
	VersionNamespaceBinding *NamespaceBinding
}

// NewService wires a Service. nil log → slog.Default. projects is
// optional — when nil, autocomplete + path-validation surfaces error.
// Signature unchanged: existing call sites (smoke tests, CLI commands) still
// compile. The default dispatch mode is ModeIndexedWithFallback; call
// WithDispatchConfig after construction to override from app config, and
// WireEventBus to subscribe the index cache to invalidation events.
func NewService(store Store, projects ProjectReader, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{store: store, projects: projects, log: log}
	if projects != nil {
		direct := s.newDirectAutocomplete()
		cache := autocomplete.NewIndexCache(store, projects, log).WithNamespaceBindings(s.autocompleteNamespaceBindings)
		indexed := autocomplete.NewIndexed(cache)
		s.indexCache = cache
		s.autocomplete = autocomplete.NewDispatch(
			indexed,
			direct,
			autocomplete.DispatchConfig{Mode: autocomplete.ModeIndexedWithFallback},
			log,
		)
	}
	return s
}

// WithDispatchConfig replaces the DispatchEngine's mode + per-request-override
// policy. Call once after NewService (typically from the router at boot) to
// apply the mode from app config. No-op when projects was nil at construction.
func (s *Service) WithDispatchConfig(cfg autocomplete.DispatchConfig) *Service {
	if s.indexCache == nil {
		return s
	}
	direct := s.newDirectAutocomplete()
	indexed := autocomplete.NewIndexed(s.indexCache)
	s.autocomplete = autocomplete.NewDispatch(indexed, direct, cfg, s.log)
	return s
}

func (s *Service) newDirectAutocomplete() *autocomplete.DirectEngine {
	return autocomplete.NewDirectWithNamespaceBindings(s.store, s.projects, s.autocompleteNamespaceBindings)
}

func (s *Service) autocompleteNamespaceBindings(ctx context.Context) ([]autocomplete.NamespaceBinding, error) {
	bindings, err := s.store.NamespaceBindings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]autocomplete.NamespaceBinding, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, autocomplete.NamespaceBinding{
			Prefix:    binding.Prefix,
			Namespace: binding.Namespace,
		})
	}
	return out, nil
}

// NamespaceBindings returns the global namespace bindings used by ontology
// import and qname resolution.
func (s *Service) NamespaceBindings(ctx context.Context) ([]NamespaceBinding, error) {
	return s.store.NamespaceBindings(ctx)
}

// UpsertNamespaceBindings persists global namespace bindings through the
// ontology store so CLI imports, seed commands, and upload imports share the
// same precedence rules.
func (s *Service) UpsertNamespaceBindings(ctx context.Context, bindings []NamespaceBinding) error {
	return s.store.UpsertNamespaceBindings(ctx, bindings)
}

// DeleteNamespaceBindingsBySource removes global namespace bindings for a
// maintenance source such as prefix.cc.
func (s *Service) DeleteNamespaceBindingsBySource(ctx context.Context, source string) error {
	source = strings.TrimSpace(source)
	if source == "" {
		return fmt.Errorf("delete namespace bindings: source required")
	}
	return s.store.DeleteNamespaceBindingsBySource(ctx, source)
}

// PreloadAutocomplete warms the per-project autocomplete index in the
// background. Best-effort: errors are logged inside the cache, not
// returned. No-op when the index cache was not constructed (nil projects
// at service construction time).
func (s *Service) PreloadAutocomplete(projectID string) {
	if s.indexCache == nil {
		return
	}
	s.indexCache.Preload(projectID)
}

// DropAutocompleteCache drops all live entries from the index cache.
// No-op when the index cache was not constructed (nil projects at
// service construction time).
func (s *Service) DropAutocompleteCache() {
	if s.indexCache == nil {
		return
	}
	s.indexCache.Drop()
}

// AutocompleteCacheStats returns a snapshot of the current index cache
// entries. Returns nil when the index cache was not constructed.
func (s *Service) AutocompleteCacheStats() []autocomplete.EntryStat {
	if s.indexCache == nil {
		return nil
	}
	return s.indexCache.Stats()
}

// SeedCacheEntryForTest inserts a synthetic cache entry via the index cache's
// test seam. No-op when the index cache was not constructed.
// Intended for tests only — verifies that Drop actually clears a populated cache.
func (s *Service) SeedCacheEntryForTest(releaseID string) {
	if s.indexCache == nil {
		return
	}
	idx := &autocomplete.Index{
		Versions: []string{"v-seed"},
		Primary:  "v-seed",
		BuiltAt:  s.indexCache.NowForTest(),
		ByQname:  map[string]*autocomplete.Node{},
	}
	s.indexCache.SeedReleaseForTest(releaseID, idx)
}

// WireEventBus sets the bus used for publishing import events and subscribes
// the index cache to invalidation events. Call once at boot after NewService
// and (optionally) WithDispatchConfig. No-op when indexCache is nil.
func (s *Service) WireEventBus(bus domain.EventBus) *Service {
	s.bus = bus
	if s.indexCache != nil {
		s.indexCache.Subscribe(bus)
	}
	return s
}

// ---------------------------------------------------------------------------
// Families
// ---------------------------------------------------------------------------

func (s *Service) GetFamily(ctx context.Context, id string) (*domain.OntologyFamily, error) {
	f, err := s.store.GetFamily(ctx, id)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("%w: family %s", ErrNotFound, id)
	}
	return f, nil
}

func (s *Service) GetFamilyBySlug(ctx context.Context, slug string) (*domain.OntologyFamily, error) {
	f, err := s.store.GetFamilyBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("%w: family slug %s", ErrNotFound, slug)
	}
	return f, nil
}

func (s *Service) ListFamilies(ctx context.Context) ([]*domain.OntologyFamily, error) {
	return s.store.ListFamilies(ctx)
}

func (s *Service) BrowseFamilies(ctx context.Context, in FamilyBrowseInput) (*BrowseResult[FamilyBrowseItem], error) {
	families, err := s.store.ListFamilies(ctx)
	if err != nil {
		return nil, err
	}
	ontologies, err := s.store.ListOntologies(ctx)
	if err != nil {
		return nil, err
	}
	counts := map[string]int64{}
	for _, o := range ontologies {
		if o.FamilyID != nil && *o.FamilyID != "" {
			counts[*o.FamilyID]++
		}
	}
	byID := map[string]*domain.OntologyFamily{}
	for _, f := range families {
		byID[f.ID] = f
	}
	items := make([]FamilyBrowseItem, 0, len(families))
	search := strings.TrimSpace(strings.ToLower(in.Search))
	for _, f := range families {
		parentName := ""
		if f.ParentFamilyID != nil {
			if parent := byID[*f.ParentFamilyID]; parent != nil {
				parentName = parent.Name
			}
		}
		item := FamilyBrowseItem{
			ID:               f.ID,
			Slug:             f.Slug,
			Name:             f.Name,
			Description:      f.Description,
			ParentFamilyID:   f.ParentFamilyID,
			ParentFamilyName: parentName,
			Icon:             f.Icon,
			DisplayOrder:     f.DisplayOrder,
			OntologyCount:    counts[f.ID],
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				item.Name,
				item.Slug,
				item.ParentFamilyName,
			}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		items = append(items, item)
	}
	sortBy := in.SortBy
	if sortBy == "" {
		sortBy = "display_order"
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch sortBy {
		case "name":
			if a.Name == b.Name {
				return a.Slug < b.Slug
			}
			return a.Name < b.Name
		case "slug":
			return a.Slug < b.Slug
		case "ontology_count":
			if a.OntologyCount == b.OntologyCount {
				return a.Name < b.Name
			}
			return a.OntologyCount > b.OntologyCount
		default:
			if a.DisplayOrder == b.DisplayOrder {
				return a.Name < b.Name
			}
			return a.DisplayOrder < b.DisplayOrder
		}
	})
	return paginateBrowse(items, in.Page, in.PerPage), nil
}

func (s *Service) ListRootFamilies(ctx context.Context) ([]*domain.OntologyFamily, error) {
	return s.store.ListRootFamilies(ctx)
}

func (s *Service) ListChildFamilies(ctx context.Context, parentID string) ([]*domain.OntologyFamily, error) {
	return s.store.ListChildFamilies(ctx, parentID)
}

// ---------------------------------------------------------------------------
// Ontologies
// ---------------------------------------------------------------------------

func (s *Service) GetOntology(ctx context.Context, id string) (*domain.Ontology, error) {
	o, err := s.store.GetOntology(ctx, id)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, fmt.Errorf("%w: ontology %s", ErrNotFound, id)
	}
	return o, nil
}

func (s *Service) GetOntologyByPrefix(ctx context.Context, prefix string) (*domain.Ontology, error) {
	o, err := s.store.GetOntologyByPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, fmt.Errorf("%w: ontology prefix %s", ErrNotFound, prefix)
	}
	return o, nil
}

func (s *Service) ListOntologies(ctx context.Context) ([]*domain.Ontology, error) {
	return s.store.ListOntologies(ctx)
}

func (s *Service) BrowseOntologies(ctx context.Context, in OntologyBrowseInput) (*BrowseResult[OntologyBrowseItem], error) {
	ontologies, err := s.store.ListOntologies(ctx)
	if err != nil {
		return nil, err
	}
	families, err := s.store.ListFamilies(ctx)
	if err != nil {
		return nil, err
	}
	familyNames := map[string]string{}
	for _, f := range families {
		familyNames[f.ID] = f.Name
	}
	byID := map[string]*domain.Ontology{}
	for _, o := range ontologies {
		byID[o.ID] = o
	}
	items := make([]OntologyBrowseItem, 0, len(ontologies))
	search := strings.TrimSpace(strings.ToLower(in.Search))
	filterFamily := strings.TrimSpace(in.FamilyID)
	filterType := strings.TrimSpace(strings.ToLower(in.OntologyType))
	for _, o := range ontologies {
		if filterFamily != "" {
			if o.FamilyID == nil || *o.FamilyID != filterFamily {
				continue
			}
		}
		if filterType != "" && strings.ToLower(string(o.OntologyType)) != filterType {
			continue
		}
		item := OntologyBrowseItem{
			ID:                o.ID,
			Prefix:            o.Prefix,
			Namespace:         o.Namespace,
			Name:              o.Name,
			Description:       o.Description,
			FamilyID:          o.FamilyID,
			OntologyType:      o.OntologyType,
			ExtendsOntologyID: o.ExtendsOntologyID,
		}
		if o.FamilyID != nil {
			item.FamilyName = familyNames[*o.FamilyID]
		}
		if o.ExtendsOntologyID != nil {
			if parent := byID[*o.ExtendsOntologyID]; parent != nil {
				item.ExtendsOntologyName = parent.Name
			}
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				item.Prefix,
				item.Namespace,
				item.Name,
				item.FamilyName,
				string(item.OntologyType),
			}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		items = append(items, item)
	}
	sortBy := in.SortBy
	if sortBy == "" {
		sortBy = "prefix"
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch sortBy {
		case "name":
			if a.Name == b.Name {
				return a.Prefix < b.Prefix
			}
			return a.Name < b.Name
		case "family":
			if a.FamilyName == b.FamilyName {
				return a.Prefix < b.Prefix
			}
			return a.FamilyName < b.FamilyName
		case "ontology_type":
			if a.OntologyType == b.OntologyType {
				return a.Prefix < b.Prefix
			}
			return a.OntologyType < b.OntologyType
		default:
			return a.Prefix < b.Prefix
		}
	})
	return paginateBrowse(items, in.Page, in.PerPage), nil
}

func (s *Service) ListOntologiesByFamily(ctx context.Context, familyID string) ([]*domain.Ontology, error) {
	return s.store.ListOntologiesByFamily(ctx, familyID)
}

func (s *Service) ListOntologyExtensions(ctx context.Context, baseOntologyID string) ([]*domain.Ontology, error) {
	return s.store.ListOntologyExtensions(ctx, baseOntologyID)
}

func (s *Service) SearchOntologies(ctx context.Context, query string, limit int) ([]*domain.Ontology, error) {
	return s.store.SearchOntologies(ctx, query, limit)
}

// ---------------------------------------------------------------------------
// Versions
// ---------------------------------------------------------------------------

func (s *Service) GetVersion(ctx context.Context, id string) (*domain.OntologyVersion, error) {
	v, err := s.store.GetVersion(ctx, id)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, fmt.Errorf("%w: version %s", ErrNotFound, id)
	}
	return v, nil
}

func (s *Service) GetActiveVersion(ctx context.Context, ontologyID string) (*domain.OntologyVersion, error) {
	v, err := s.store.GetActiveVersion(ctx, ontologyID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, fmt.Errorf("%w: active version for ontology %s", ErrNotFound, ontologyID)
	}
	return v, nil
}

func (s *Service) ListVersionsByOntology(ctx context.Context, ontologyID string) ([]*domain.OntologyVersion, error) {
	return s.store.ListVersionsByOntology(ctx, ontologyID)
}

// VersionListWithUsage returns versions for one ontology paired with
// project-usage counts, in a single round trip (one list query plus one
// counts query).
func (s *Service) VersionListWithUsage(ctx context.Context, ontologyID string) ([]VersionWithUsage, error) {
	versions, err := s.store.ListVersionsByOntology(ctx, ontologyID)
	if err != nil {
		return nil, err
	}
	counts, err := s.store.VersionUsageCountsForOntology(ctx, ontologyID)
	if err != nil {
		return nil, err
	}
	out := make([]VersionWithUsage, 0, len(versions))
	for _, v := range versions {
		out = append(out, VersionWithUsage{Version: v, ProjectCount: counts[v.ID]})
	}
	return out, nil
}

// VersionWithUsage pairs an OntologyVersion with the count of projects
// that link to it.
type VersionWithUsage struct {
	Version      *domain.OntologyVersion `json:"version"`
	ProjectCount int64                   `json:"project_count"`
}

// ---------------------------------------------------------------------------
// Classes / Properties — read paths used by the ontology explorer + later
// by the autocomplete subpackage.
// ---------------------------------------------------------------------------

func (s *Service) ListClasses(ctx context.Context, versionID string) ([]*domain.OntologyClass, error) {
	return s.store.ListClassesByVersion(ctx, versionID)
}

func (s *Service) GetClassByQname(ctx context.Context, versionID, qname string) (*domain.OntologyClass, error) {
	c, err := s.store.GetClassByQname(ctx, versionID, qname)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("%w: class %s in version %s", ErrNotFound, qname, versionID)
	}
	return c, nil
}

func (s *Service) SearchClasses(ctx context.Context, versionID, query string, limit int) ([]*domain.OntologyClass, error) {
	return s.store.SearchClasses(ctx, versionID, query, limit)
}

func (s *Service) ListProperties(ctx context.Context, versionID string) ([]*domain.OntologyProperty, error) {
	return s.store.ListPropertiesByVersion(ctx, versionID)
}

func (s *Service) GetPropertyByQname(ctx context.Context, versionID, qname string) (*domain.OntologyProperty, error) {
	p, err := s.store.GetPropertyByQname(ctx, versionID, qname)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, fmt.Errorf("%w: property %s in version %s", ErrNotFound, qname, versionID)
	}
	return p, nil
}

func (s *Service) SearchProperties(ctx context.Context, versionID, query string, limit int) ([]*domain.OntologyProperty, error) {
	return s.store.SearchProperties(ctx, versionID, query, limit)
}

// ---------------------------------------------------------------------------
// Family writes (Phase B / Phase C importer)
// ---------------------------------------------------------------------------

// ErrInUse is returned when a delete is blocked because dependent rows
// still exist (e.g. a family with attached ontologies, an ontology with
// versions, a version linked to projects).
var ErrInUse = errors.New("ontology: in use")

// CreateFamily creates a new family. Caller supplies slug + name; ID is
// derived deterministically from the slug via id_helpers.
func (s *Service) CreateFamily(ctx context.Context, in CreateFamilyInput) (*domain.OntologyFamily, error) {
	if in.ID == "" {
		in.ID = GenerateFamilyID(in.Slug)
	}
	return s.store.CreateFamily(ctx, in)
}

func (s *Service) UpdateFamily(ctx context.Context, id string, in UpdateFamilyInput) (*domain.OntologyFamily, error) {
	return s.store.UpdateFamily(ctx, id, in)
}

func (s *Service) DeleteFamily(ctx context.Context, id string) error {
	count, err := s.store.CountOntologiesInFamily(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: family %s has %d ontologies attached", ErrInUse, id, count)
	}
	return s.store.DeleteFamily(ctx, id)
}

func paginateBrowse[T any](items []T, page, perPage int) *BrowseResult[T] {
	total := len(items)
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	start := (page - 1) * perPage
	if start >= total {
		return &BrowseResult[T]{Items: []T{}, Total: total}
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return &BrowseResult[T]{
		Items: items[start:end],
		Total: total,
	}
}

// UpsertFamily looks up by slug; creates if missing, updates otherwise.
// Used by the importer to apply manifest changes idempotently.
func (s *Service) UpsertFamily(ctx context.Context, in CreateFamilyInput) (*domain.OntologyFamily, error) {
	if in.Slug == "" {
		return nil, fmt.Errorf("upsert family: slug required")
	}
	existing, err := s.store.GetFamilyBySlug(ctx, in.Slug)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return s.CreateFamily(ctx, in)
	}
	return s.store.UpdateFamily(ctx, existing.ID, UpdateFamilyInput{
		Slug:           in.Slug,
		Name:           in.Name,
		Description:    in.Description,
		ParentFamilyID: in.ParentFamilyID,
		HomepageURL:    in.HomepageURL,
		Icon:           in.Icon,
		DisplayOrder:   in.DisplayOrder,
	})
}

// ---------------------------------------------------------------------------
// Ontology writes (Phase B / Phase C importer)
// ---------------------------------------------------------------------------

func (s *Service) CreateOntology(ctx context.Context, in CreateOntologyInput) (*domain.Ontology, error) {
	if in.ID == "" {
		in.ID = GenerateOntologyID(in.Prefix)
	}
	if in.OntologyType == "" {
		in.OntologyType = domain.OntologyTypeBase
	}
	return s.store.CreateOntology(ctx, in)
}

func (s *Service) UpdateOntology(ctx context.Context, id string, in UpdateOntologyInput) (*domain.Ontology, error) {
	if in.OntologyType == "" {
		in.OntologyType = domain.OntologyTypeBase
	}
	return s.store.UpdateOntology(ctx, id, in)
}

func (s *Service) DeleteOntology(ctx context.Context, id string) error {
	count, err := s.store.CountOntologyVersions(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: ontology %s has %d versions", ErrInUse, id, count)
	}
	return s.store.DeleteOntology(ctx, id)
}

// UpsertOntology looks up by prefix; creates if missing, updates otherwise.
// Used by the importer.
func (s *Service) UpsertOntology(ctx context.Context, in CreateOntologyInput) (*domain.Ontology, error) {
	if in.Prefix == "" {
		return nil, fmt.Errorf("upsert ontology: prefix required")
	}
	existing, err := s.store.GetOntologyByPrefix(ctx, in.Prefix)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return s.CreateOntology(ctx, in)
	}
	return s.UpdateOntology(ctx, existing.ID, UpdateOntologyInput{
		Prefix:            in.Prefix,
		Namespace:         in.Namespace,
		Name:              in.Name,
		Description:       in.Description,
		FamilyID:          in.FamilyID,
		OntologyType:      in.OntologyType,
		ExtendsOntologyID: in.ExtendsOntologyID,
		HomepageURL:       in.HomepageURL,
		SourceURL:         in.SourceURL,
	})
}

// ---------------------------------------------------------------------------
// Version writes (Phase B admin CRUD + Phase C importer)
// ---------------------------------------------------------------------------

func (s *Service) UpdateVersionMetadata(ctx context.Context, id string, in UpdateVersionMetadataInput) (*domain.OntologyVersion, error) {
	return s.store.UpdateVersionMetadata(ctx, id, in)
}

func (s *Service) SetActiveVersion(ctx context.Context, ontologyID, versionID string) error {
	return s.store.SetActiveVersion(ctx, ontologyID, versionID)
}

// DeleteVersion blocks if any project references the version. Callers
// that hit ErrInUse should surface a 409 with the project list.
func (s *Service) DeleteVersion(ctx context.Context, id string) error {
	count, err := s.store.VersionUsageCount(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: version %s linked to %d projects", ErrInUse, id, count)
	}
	return s.store.DeleteVersion(ctx, id)
}

// ImportVersion delegates to the Store's transactional bulk-import.
// Callers (importer CLI, admin UI) pre-compute classes / properties /
// relations from a parsed RDF file and supply them as the input.
//
// The Store atomically: deletes the existing version row (FK-cascading
// classes / properties / relations), inserts the new version, bulk-
// inserts classes / properties / relations, updates counts, and
// optionally marks the version active.
//
// On success, publishes EventOntologyVersionImported so the IndexCache
// can evict stale live entries for the imported version.
func (s *Service) ImportVersion(ctx context.Context, in ImportVersionInput) (*domain.OntologyVersion, error) {
	return s.ImportVersionWithOptions(ctx, in, ImportVersionOptions{})
}

// ImportVersionWithOptions imports one ontology version and applies additional
// import policy before the store transaction starts.
func (s *Service) ImportVersionWithOptions(ctx context.Context, in ImportVersionInput, opts ImportVersionOptions) (*domain.OntologyVersion, error) {
	if in.Version.ID == "" {
		return nil, fmt.Errorf("import version: version ID required")
	}
	if in.Version.OntologyID == "" {
		return nil, fmt.Errorf("import version: ontology ID required")
	}
	if in.Version.VersionString == "" {
		return nil, fmt.Errorf("import version: version_string required")
	}
	if opts.VersionNamespaceBinding != nil {
		binding, err := normalizeVersionDerivedNamespaceBinding(*opts.VersionNamespaceBinding)
		if err != nil {
			return nil, err
		}
		in.NamespaceBindings = append(cloneNamespaceBindings(in.NamespaceBindings), binding)
	}
	out, err := s.store.ImportVersion(ctx, in)
	if err != nil {
		return nil, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, domain.Event{
			Type:     domain.EventOntologyVersionImported,
			EntityID: out.ID,
		})
	}
	return out, nil
}

func normalizeVersionDerivedNamespaceBinding(binding NamespaceBinding) (NamespaceBinding, error) {
	binding.Prefix = strings.TrimSpace(binding.Prefix)
	binding.Namespace = strings.TrimSpace(binding.Namespace)
	if binding.Prefix == "" || binding.Namespace == "" {
		return NamespaceBinding{}, fmt.Errorf("import version: version namespace binding requires prefix and namespace")
	}
	if binding.Weight == 0 {
		binding.Weight = NamespaceBindingWeightVersionDerived
	}
	if strings.TrimSpace(binding.Source) == "" {
		binding.Source = NamespaceBindingSourceVersionDerived
	}
	return binding, nil
}

func cloneNamespaceBindings(bindings []NamespaceBinding) []NamespaceBinding {
	if len(bindings) == 0 {
		return nil
	}
	out := make([]NamespaceBinding, len(bindings))
	copy(out, bindings)
	return out
}

// ---------------------------------------------------------------------------
// Autocomplete (Phase D)
// ---------------------------------------------------------------------------

// ErrAutocompleteUnavailable is returned by autocomplete + path
// methods when the Service was constructed without a ProjectReader
// (slice mounted standalone or tests that don't need cross-slice
// reads).
var ErrAutocompleteUnavailable = errors.New("ontology: autocomplete unavailable")

// GetSuggestions returns path-builder suggestions for the project.
// Backed by the DispatchEngine; engine mode comes from app config.
func (s *Service) GetSuggestions(ctx context.Context, req autocomplete.Request) ([]autocomplete.Suggestion, error) {
	if s.autocomplete == nil {
		return nil, ErrAutocompleteUnavailable
	}
	return s.autocomplete.GetSuggestions(ctx, req)
}

// OntologyLabels returns label maps for every class + property the
// project's selected versions cover. Used by the frontend's
// schema-driven renderers (path display, breadcrumbs, …).
// Always uses the DirectEngine — label lookups are not cached in the index.
func (s *Service) OntologyLabels(ctx context.Context, projectID, lang string) (*autocomplete.OntologyLabelsResult, error) {
	if s.autocomplete == nil {
		return nil, ErrAutocompleteUnavailable
	}
	return s.autocomplete.Direct().OntologyLabels(ctx, projectID, lang)
}

// NewScopeResolver builds a project-scoped fuzzy scope resolver. Used
// by the import tooling to translate free-text "Scope" descriptors
// ("E18 Physical Thing", "E18", …) into structured PathElements during
// staging→canonical promotion.
//
// The resolver caches all classes + properties for the project's
// resolved versions in memory — caller should reuse one resolver
// across all per-field lookups in an import session.
func (s *Service) NewScopeResolver(ctx context.Context, projectID string) (*autocomplete.ScopeResolver, error) {
	if s.autocomplete == nil {
		return nil, ErrAutocompleteUnavailable
	}
	// We pass the same Store + ProjectReader the autocomplete engine
	// uses (constructed with them in NewService). The resolver doesn't
	// share state with Engine — it builds its own indexes — but the
	// dependency wiring is the same shape.
	return autocomplete.NewScopeResolver(ctx, s.store, projectReaderFromService(s), projectID)
}

// projectReaderFromService extracts the ProjectReader passed at
// construction time. Stored explicitly on Service.projects so
// NewScopeResolver can access it without the DispatchEngine exposing
// its internal reader.
func projectReaderFromService(s *Service) autocomplete.ProjectReader {
	return s.projects
}

// IsVersionInUseByProject + VersionUsageInProject still TODO — the
// Store has CountFieldsUsingVersion + SampleFieldsUsingVersion ready
// for the projectontologyversion slice's Delete preflight.

// ---------------------------------------------------------------------------
// Field-ref maintenance (Phase E)
// ---------------------------------------------------------------------------

// ReplaceFieldRefs atomically replaces the ontology-ref rows for one
// field. Called by pkg/weave/field's Service after Create + Update so
// the materialized weave_field_ontology_refs table tracks the
// canonical path_elements.
//
// refs may be nil/empty — the existing rows for fieldID are still
// cleared (mirrors the field having an empty path).
func (s *Service) ReplaceFieldRefs(ctx context.Context, fieldID, projectID string, refs []domain.FieldOntologyRef) error {
	return s.store.ReplaceFieldRefs(ctx, fieldID, projectID, refs)
}
