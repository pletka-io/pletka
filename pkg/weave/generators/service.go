package generators

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// ProjectReader is the project slice surface generators need.
type ProjectReader interface {
	GetByID(ctx context.Context, id string) (*domain.Project, error)
	ResolvedOntologyVersions(ctx context.Context, projectID string, opts domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error)
}

// ModelReader is the model slice surface generators need.
type ModelReader interface {
	Get(ctx context.Context, projectID, id string) (*domain.Model, error)
	View(ctx context.Context, projectID, id string) (*domain.ModelView, error)
}

// CollectionReader is the collection slice surface generators need.
type CollectionReader interface {
	Get(ctx context.Context, projectID, id string) (*domain.Collection, error)
	View(ctx context.Context, projectID, id string) ([]domain.ResolvedField, error)
}

// CollectionLookupByID is an internal escape hatch used by generators when a
// model snapshot references inherited collections. Route auth is still enforced
// on the target project; this lookup only resolves the owning project for
// adopted collections already present in a readable model view.
type CollectionLookupByID interface {
	LookupByID(ctx context.Context, id string) (*domain.Collection, error)
}

// CollectionAnchorIndexer is an optional capability — when the
// underlying CollectionReader implements it, the snap builder uses it
// to find collections anchored at a CIDOC class even when no field on
// the current model belongs to them (cross-anchor composition).
type CollectionAnchorIndexer interface {
	AnchorIndex(ctx context.Context) ([]domain.CollectionAnchor, error)
}

// CollectionLookupView is an optional capability: a project-agnostic
// counterpart to CollectionReader.View, used to fetch resolved fields
// for collections discovered via the anchor index that the current
// caller has no project-scoped access to.
type CollectionLookupView interface {
	LookupView(ctx context.Context, id string) ([]domain.ResolvedField, error)
}

// FieldReader is the field slice surface generators need.
type FieldReader interface {
	Get(ctx context.Context, projectID, id string) (*domain.Field, error)
	Resolved(ctx context.Context, projectID, id string) (*domain.ResolvedField, error)
}

// NamespaceReader is the namespace-binding slice surface generators need.
type NamespaceReader interface {
	ListForProject(ctx context.Context, projectID string) ([]*domain.NamespaceBinding, error)
}

// ConceptEnumReader returns the member concept URIs of the given concept lists
// (union, de-duplicated). Optional — wired by the host so generators can emit
// value enums for fields bound to sealed lists (#3599); nil in core-only
// builds and tests, in which case no enum is attached.
type ConceptEnumReader interface {
	ConceptListMemberURIs(ctx context.Context, projectID string, listIDs []string) ([]string, error)
}

// Service builds generator snapshots from narrow cross-slice readers.
type Service struct {
	projects    ProjectReader
	models      ModelReader
	collections CollectionReader
	fields      FieldReader
	namespaces  NamespaceReader
	registry    *Registry
	conceptEnum ConceptEnumReader // optional; set via SetConceptEnumReader
}

// SetConceptEnumReader wires the optional reader used to attach sealed-list
// value enums to snapshot fields (#3599). Host builds call this after
// construction; leaving it unset disables enum attachment.
func (s *Service) SetConceptEnumReader(r ConceptEnumReader) { s.conceptEnum = r }

// attachConceptEnums fills FieldNode.ConceptEnum for fields bound to a sealed
// concept list, from the optional reader. Best-effort: a reader/query error
// leaves the field's enum empty rather than failing the whole snapshot.
func (s *Service) attachConceptEnums(ctx context.Context, snap *Snapshot) *Snapshot {
	if s.conceptEnum == nil || snap == nil {
		return snap
	}
	for i := range snap.Fields {
		var sealed []string
		for _, ref := range snap.Fields[i].Field.ConceptLists {
			if ref.IsClosed && ref.ID != "" {
				sealed = append(sealed, ref.ID)
			}
		}
		if len(sealed) == 0 {
			continue
		}
		uris, err := s.conceptEnum.ConceptListMemberURIs(ctx, snap.Project.ID, sealed)
		if err == nil && len(uris) > 0 {
			snap.Fields[i].ConceptEnum = uris
		}
	}
	return snap
}

func NewService(
	projects ProjectReader,
	models ModelReader,
	collections CollectionReader,
	fields FieldReader,
	namespaces NamespaceReader,
	registry *Registry,
) *Service {
	return &Service{
		projects:    projects,
		models:      models,
		collections: collections,
		fields:      fields,
		namespaces:  namespaces,
		registry:    registry,
	}
}

func (s *Service) SnapshotForModel(ctx context.Context, projectID, modelID string, opts Options) (*Snapshot, error) {
	if err := s.requireReaders(s.projects, s.models, s.collections, s.namespaces); err != nil {
		return nil, err
	}
	opts = normalizeOptions(opts)

	project, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load project %s: %w", projectID, err)
	}
	model, err := s.models.Get(ctx, projectID, modelID)
	if err != nil {
		return nil, fmt.Errorf("load model %s: %w", modelID, err)
	}
	view, err := s.models.View(ctx, projectID, modelID)
	if err != nil {
		return nil, fmt.Errorf("build model view %s: %w", modelID, err)
	}

	namespaces, err := s.loadNamespaces(ctx, *project)
	if err != nil {
		return nil, err
	}
	ontologies, err := s.loadOntologies(ctx, projectID, opts)
	if err != nil {
		return nil, err
	}
	collections, err := s.loadCollections(ctx, projectID, view)
	if err != nil {
		return nil, err
	}

	// When the underlying collection reader exposes a global anchor
	// index, pull collections that anchor at any of the model's
	// Collection-stub terminal classes — even when no field on the
	// model belongs to them. This is what closes the OGEE Activity
	// Duration gap: timespan_duration stubs at E54_Dimension graft
	// LA's Dimension collection. Failures fall back to the un-graft
	// behaviour with a logged warning, so a misconfigured DB never
	// breaks snapshot building.
	extraAnchored, err := s.loadExtraAnchoredCollections(ctx, view, collections, opts)
	if err != nil {
		return nil, err
	}

	return s.attachConceptEnums(ctx, BuildModelSnapshot(ModelSnapshotInput{
		Project:       *project,
		Model:         *model,
		View:          *view,
		Collections:   collections,
		ExtraAnchored: extraAnchored,
		Namespaces:  namespaces,
		Ontologies:  ontologies,
		Options:     opts,
	})), nil
}

func (s *Service) SnapshotForCollection(ctx context.Context, projectID, collectionID string, opts Options) (*Snapshot, error) {
	if err := s.requireReaders(s.projects, s.collections, s.namespaces); err != nil {
		return nil, err
	}
	opts = normalizeOptions(opts)

	project, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load project %s: %w", projectID, err)
	}
	collection, err := s.collections.Get(ctx, projectID, collectionID)
	if err != nil {
		return nil, fmt.Errorf("load collection %s: %w", collectionID, err)
	}
	fields, err := s.collections.View(ctx, projectID, collectionID)
	if err != nil {
		return nil, fmt.Errorf("build collection view %s: %w", collectionID, err)
	}

	namespaces, err := s.loadNamespaces(ctx, *project)
	if err != nil {
		return nil, err
	}
	ontologies, err := s.loadOntologies(ctx, projectID, opts)
	if err != nil {
		return nil, err
	}

	return s.attachConceptEnums(ctx, BuildCollectionSnapshot(CollectionSnapshotInput{
		Project:    *project,
		Collection: *collection,
		Fields:     fields,
		Namespaces: namespaces,
		Ontologies: ontologies,
		Options:    opts,
	})), nil
}

func (s *Service) SnapshotForField(ctx context.Context, projectID, fieldID string, opts Options) (*Snapshot, error) {
	if err := s.requireReaders(s.projects, s.fields, s.namespaces); err != nil {
		return nil, err
	}
	opts = normalizeOptions(opts)

	project, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load project %s: %w", projectID, err)
	}
	field, err := s.fields.Get(ctx, projectID, fieldID)
	if err != nil {
		return nil, fmt.Errorf("load field %s: %w", fieldID, err)
	}
	resolved, err := s.fields.Resolved(ctx, projectID, fieldID)
	if err != nil {
		return nil, fmt.Errorf("resolve field %s: %w", fieldID, err)
	}
	namespaces, err := s.loadNamespaces(ctx, *project)
	if err != nil {
		return nil, err
	}
	ontologies, err := s.loadOntologies(ctx, projectID, opts)
	if err != nil {
		return nil, err
	}

	return s.attachConceptEnums(ctx, BuildFieldSnapshot(FieldSnapshotInput{
		Project:    *project,
		Field:      *field,
		Resolved:   *resolved,
		Namespaces: namespaces,
		Ontologies: ontologies,
		Options:    opts,
	})), nil
}

func (s *Service) GenerateModel(ctx context.Context, projectID, modelID string, format Format, w io.Writer, opts Options) error {
	snapshot, err := s.SnapshotForModel(ctx, projectID, modelID, opts)
	if err != nil {
		return err
	}
	return s.render(ctx, format, snapshot, w)
}

func (s *Service) GenerateCollection(ctx context.Context, projectID, collectionID string, format Format, w io.Writer, opts Options) error {
	snapshot, err := s.SnapshotForCollection(ctx, projectID, collectionID, opts)
	if err != nil {
		return err
	}
	return s.render(ctx, format, snapshot, w)
}

func (s *Service) GenerateField(ctx context.Context, projectID, fieldID string, format Format, w io.Writer, opts Options) error {
	snapshot, err := s.SnapshotForField(ctx, projectID, fieldID, opts)
	if err != nil {
		return err
	}
	return s.render(ctx, format, snapshot, w)
}

func (s *Service) RenderSnapshot(ctx context.Context, format Format, snapshot *Snapshot, w io.Writer) error {
	return s.render(ctx, format, snapshot, w)
}

func (s *Service) render(ctx context.Context, format Format, snapshot *Snapshot, w io.Writer) error {
	if s == nil || s.registry == nil {
		return fmt.Errorf("generator service: registry is nil")
	}
	renderer, ok := s.registry.Renderer(format)
	if !ok {
		return fmt.Errorf("generator renderer %q: not registered", format)
	}
	return renderer.Render(ctx, snapshot, w)
}

func (s *Service) loadNamespaces(ctx context.Context, project domain.Project) (NamespaceSet, error) {
	bindings, err := s.namespaces.ListForProject(ctx, project.ID)
	if err != nil {
		return NamespaceSet{}, fmt.Errorf("load namespace bindings for project %s: %w", project.ID, err)
	}
	return BuildNamespaceSet(project, bindings), nil
}

func (s *Service) loadOntologies(ctx context.Context, projectID string, opts Options) ([]domain.ResolvedOntologyVersion, error) {
	out, err := s.projects.ResolvedOntologyVersions(ctx, projectID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		return nil, fmt.Errorf("load resolved ontologies for project %s: %w", projectID, err)
	}
	if opts.IncludeInheritedOntologies {
		return out, nil
	}

	own := out[:0]
	for _, version := range out {
		if version.SourceProjectID == "" || version.SourceProjectID == projectID {
			own = append(own, version)
		}
	}
	return own, nil
}

func (s *Service) loadCollections(ctx context.Context, projectID string, view *domain.ModelView) (map[string]*domain.Collection, error) {
	ids := make(map[string]struct{})
	if view != nil {
		for _, category := range view.Categories {
			for _, collection := range category.Collections {
				if collection.ID != "" && collection.ID != directCollectionID {
					ids[collection.ID] = struct{}{}
				}
			}
		}
	}

	out := make(map[string]*domain.Collection, len(ids))
	lookup, hasLookup := s.collections.(CollectionLookupByID)
	for id := range ids {
		var (
			collection *domain.Collection
			err        error
		)
		if hasLookup {
			collection, err = lookup.LookupByID(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("lookup collection %s for model snapshot: %w", id, err)
			}
			if collection == nil {
				return nil, fmt.Errorf("lookup collection %s for model snapshot: not found", id)
			}
		} else {
			collection, err = s.collections.Get(ctx, projectID, id)
			if err != nil {
				return nil, fmt.Errorf("load collection %s for model snapshot: %w", id, err)
			}
		}
		out[id] = collection
	}
	return out, nil
}

// loadExtraAnchoredCollections asks the underlying CollectionReader
// (when it supports CollectionAnchorIndexer + CollectionLookupView)
// for collections whose anchor class matches at least one
// Collection-stub field in the view but whose ID isn't already in
// the loaded collection set. Returns an empty map when either
// capability is missing or when the index turns up no additional
// matches.
//
// Errors from the lookup are tolerated (logged-equivalent — folded
// into a soft error) so a misconfigured DB never breaks snapshot
// building. Returns (nil, nil) when the reader doesn't implement
// the optional interfaces.
//
// Short-circuits when opts.ComposeCollections is off — the only
// consumer of the returned map is composeCollections, which itself
// no-ops in that case. Skipping the AnchorIndex SQL + view walk per
// snapshot is the difference between ~2s and ~0s of overhead on a
// 23-graph pack build (regression introduced when cross-anchor
// loading landed on 2026-06-08).
func (s *Service) loadExtraAnchoredCollections(ctx context.Context, view *domain.ModelView, alreadyLoaded map[string]*domain.Collection, opts Options) (map[string]ExtraAnchoredCollection, error) {
	if !opts.ComposeCollections {
		return nil, nil
	}
	indexer, indexable := s.collections.(CollectionAnchorIndexer)
	viewer, viewable := s.collections.(CollectionLookupView)
	if !indexable || !viewable {
		return nil, nil
	}

	// 1. Discover stub anchors in the view.
	stubAnchors := map[string]struct{}{}
	for _, cat := range view.Categories {
		for _, col := range cat.Collections {
			for _, f := range col.Fields {
				switch strings.ToLower(strings.TrimSpace(f.ExpectedValueType)) {
				case "collection", "collection model":
					if u := terminalClassURI(f.PathElements); u != "" {
						stubAnchors[u] = struct{}{}
					}
				}
			}
		}
	}
	if len(stubAnchors) == 0 {
		return nil, nil
	}

	// 2. Fetch the global anchor index.
	idx, err := indexer.AnchorIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("collection anchor index: %w", err)
	}

	// 3. Skip anchors view already covers (else every project's
	// sibling Name/Statement at same anchor explodes grafts).
	coveredAnchors := map[string]struct{}{}
	for _, cat := range view.Categories {
		for _, col := range cat.Collections {
			if len(col.SharedPathPrefix) == 0 {
				continue
			}
			if a := col.SharedPathPrefix[len(col.SharedPathPrefix)-1].URI; a != "" {
				coveredAnchors[a] = struct{}{}
			}
		}
	}

	// 4. Group candidates per uncovered anchor.
	loadedIDs := map[string]struct{}{}
	for id := range alreadyLoaded {
		loadedIDs[id] = struct{}{}
	}
	// Track which projects the already-loaded collections come from
	// so the tie-break for cross-anchor candidates can prefer the
	// "host" project's neighbours. OGEE's view loads LAC.* (project
	// LA); when picking a Dimension among GLBC.5 / LAC.8 / SRDC.5,
	// LA wins because it's where the rest of the model's collections
	// already live.
	preferredProjects := map[string]int{}
	for _, c := range alreadyLoaded {
		if c != nil && c.ProjectID != "" {
			preferredProjects[c.ProjectID]++
		}
	}

	candidatesByAnchor := map[string][]domain.CollectionAnchor{}
	for _, ca := range idx {
		if _, want := stubAnchors[ca.AnchorURI]; !want {
			continue
		}
		if _, covered := coveredAnchors[ca.AnchorURI]; covered {
			continue
		}
		if _, dup := loadedIDs[ca.CollectionID]; dup {
			continue
		}
		candidatesByAnchor[ca.AnchorURI] = append(candidatesByAnchor[ca.AnchorURI], ca)
	}

	// 5. Per uncovered anchor, pick exactly one collection
	// (lex-smallest ID for determinism). One graft per anchor —
	// curator-like, not exhaustive multi-graft.
	out := map[string]ExtraAnchoredCollection{}
	for _, candidates := range candidatesByAnchor {
		pick := candidates[0]
		pickScore := preferredProjects[pick.ProjectID]
		for _, c := range candidates[1:] {
			cScore := preferredProjects[c.ProjectID]
			// 1. Higher preferred-project count wins.
			// 2. Tie: lex-smallest collection ID for determinism.
			better := cScore > pickScore || (cScore == pickScore && c.CollectionID < pick.CollectionID)
			if better {
				pick = c
				pickScore = cScore
			}
		}
		coll, err := s.collections.(CollectionLookupByID).LookupByID(ctx, pick.CollectionID)
		if err != nil || coll == nil {
			continue
		}
		fields, err := viewer.LookupView(ctx, pick.CollectionID)
		if err != nil || len(fields) == 0 {
			continue
		}
		out[pick.CollectionID] = ExtraAnchoredCollection{
			Collection:       coll,
			Fields:           fields,
			SharedPathPrefix: computeSharedPathPrefix(fields),
		}
	}
	return out, nil
}

// computeSharedPathPrefix mirrors weave/resolve.ComputeSharedPathPrefix
// without taking a dependency back on the weave package (cycle).
func computeSharedPathPrefix(fields []domain.ResolvedField) []domain.PathElement {
	if len(fields) == 0 {
		return nil
	}
	first := fields[0].PathElements
	if len(first) == 0 {
		return nil
	}
	prefixLen := len(first)
	for _, f := range fields[1:] {
		if len(f.PathElements) < prefixLen {
			prefixLen = len(f.PathElements)
		}
		for i := 0; i < prefixLen; i++ {
			if f.PathElements[i].URI != first[i].URI {
				prefixLen = i
				break
			}
		}
		if prefixLen == 0 {
			return nil
		}
	}
	if prefixLen == 0 {
		return nil
	}
	return append([]domain.PathElement(nil), first[:prefixLen]...)
}

func (s *Service) requireReaders(readers ...any) error {
	if s == nil {
		return fmt.Errorf("generator service: nil service")
	}
	for _, reader := range readers {
		if reader == nil {
			return fmt.Errorf("generator service: required reader is nil")
		}
	}
	return nil
}
