package generators

import (
	"context"
	"fmt"
	"io"

	"github.com/pletka-io/pletka/pkg/auth"
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
	Get(ctx context.Context, scope auth.ReadScope, projectID, id string) (*domain.Collection, error)
	View(ctx context.Context, projectID, id string) ([]domain.ResolvedField, error)
}

// CollectionLookupByID is an internal escape hatch used by generators when a
// model snapshot references inherited collections. Route auth is still enforced
// on the target project; this lookup only resolves the owning project for
// adopted collections already present in a readable model view.
type CollectionLookupByID interface {
	LookupByID(ctx context.Context, scope auth.ReadScope, id string) (*domain.Collection, error)
}

// CollectionLookupView is an optional capability: a project-agnostic
// counterpart to CollectionReader.View that resolves a collection's fields
// in its owner project. Composition reads referenced collections through
// it; a consuming project may not own them.
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
	referenced, err := s.loadReferencedCollections(ctx, view, opts)
	if err != nil {
		return nil, err
	}

	return s.attachConceptEnums(ctx, BuildModelSnapshot(ModelSnapshotInput{
		Project:     *project,
		Model:       *model,
		View:        *view,
		Collections: collections,
		Referenced:  referenced,
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
	if project == nil {
		return nil, fmt.Errorf("load project %s: not found", projectID)
	}
	collection, err := s.collections.Get(ctx, auth.Draft(), projectID, collectionID) // generators has no scope of its own yet: exports are draft-only
	if err != nil {
		return nil, fmt.Errorf("load collection %s: %w", collectionID, err)
	}
	if collection == nil {
		return nil, fmt.Errorf("load collection %s: not found", collectionID)
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
			collection, err = lookup.LookupByID(ctx, auth.Draft(), id) // generators has no scope of its own yet: exports are draft-only
			if err != nil {
				return nil, fmt.Errorf("lookup collection %s for model snapshot: %w", id, err)
			}
			if collection == nil {
				return nil, fmt.Errorf("lookup collection %s for model snapshot: not found", id)
			}
		} else {
			collection, err = s.collections.Get(ctx, auth.Draft(), projectID, id) // generators has no scope of its own yet: exports are draft-only
			if err != nil {
				return nil, fmt.Errorf("load collection %s for model snapshot: %w", id, err)
			}
		}
		out[id] = collection
	}
	return out, nil
}

// loadReferencedCollections loads every collection a Collection-stub field
// in the view names in its collection_model refs, then the ones their own
// stubs name. Members are resolved in the collection's owner project: a
// consuming project keeps no overrides of an inherited collection, and
// CollectionReader.View refuses a collection the project does not own. A
// ref that resolves to nothing is kept with Missing set, so composition
// can say why; a reader error fails the snapshot. nil when composition is
// off or the reader cannot look collections up by id.
func (s *Service) loadReferencedCollections(ctx context.Context, view *domain.ModelView, opts Options) (map[string]ReferencedCollection, error) {
	if !opts.ComposeCollections || view == nil {
		return nil, nil
	}
	lookup, ok := s.collections.(CollectionLookupByID)
	members, ok2 := s.collections.(CollectionLookupView)
	if !ok || !ok2 {
		return nil, nil
	}
	out := map[string]ReferencedCollection{}
	seen := map[string]bool{}
	var queue []string
	enqueue := func(fields []domain.ResolvedField) {
		for _, f := range fields {
			if !isCollectionStub(f) {
				continue
			}
			for _, ref := range f.CollectionModels {
				if !seen[ref.ID] {
					seen[ref.ID] = true
					queue = append(queue, ref.ID)
				}
			}
		}
	}
	for _, cat := range view.Categories {
		for _, col := range cat.Collections {
			enqueue(col.Fields)
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		coll, err := lookup.LookupByID(ctx, auth.Draft(), id) // generators has no scope of its own yet: exports are draft-only
		if err != nil {
			return nil, fmt.Errorf("lookup referenced collection %s: %w", id, err)
		}
		if coll == nil {
			out[id] = ReferencedCollection{Missing: "not found"}
			continue
		}
		fields, err := members.LookupView(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("resolve referenced collection %s: %w", id, err)
		}
		if len(fields) == 0 {
			out[id] = ReferencedCollection{Collection: coll, Missing: "has no members"}
			continue
		}
		out[id] = ReferencedCollection{Collection: coll, Fields: fields, SharedPathPrefix: graftPrefix(fields)}
		enqueue(fields)
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
			if !f.PathElements[i].SameStep(first[i]) {
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
