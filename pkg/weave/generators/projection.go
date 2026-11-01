package generators

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

type constraintKey struct {
	project  string
	override int64
}
type fieldConstraint struct {
	uris   []string
	closed bool
}

// Selection identifies the consumer and source without a database capability.
type Selection struct {
	ProjectID       string `json:"project_id"`
	Version         string `json:"version"`
	Draft           bool   `json:"draft"`
	Kind            string `json:"kind"`
	ID              string `json:"id"`
	SourceProjectID string `json:"source_project_id"`
	SourceVersion   string `json:"source_version"`
}

// BuildSnapshot is the pure generator boundary. Both Go renderers and future
// script adapters consume this detached projection, never a reader or session.
// Missing semantic evidence and constraints fail before rendering begins.
func BuildSnapshot(fragment projectsnapshot.EntityFragment, opts Options) (*Snapshot, error) {
	p, err := newProjection(fragment)
	if err != nil {
		return nil, err
	}
	view, err := fragment.Compose()
	if err != nil {
		return nil, err
	}
	project, err := projectEntity(p.content[p.consumer].Projects[0])
	if err != nil {
		return nil, err
	}
	groups, collections, enums, err := p.groups(view)
	if err != nil {
		return nil, err
	}
	opts = normalizeOptions(opts)
	if !opts.IncludeInheritedOntologies {
		p.ontologies = slices.DeleteFunc(p.ontologies, func(v domain.ResolvedOntologyVersion) bool { return v.SourceProjectID != p.consumer.ProjectID() })
	}
	// Ontology targets come from this value, never independently supplied live data.
	opts.X3MLTargets = p.targets
	var snap *Snapshot
	switch p.root.Kind {
	case string(EntityModel):
		extra, err := p.grafts(fragment, view, collections, enums, opts)
		if err != nil {
			return nil, err
		}
		model, err := p.model(p.root)
		if err != nil {
			return nil, err
		}
		snap = BuildModelSnapshot(ModelSnapshotInput{Project: project, Model: model, View: domain.ModelView{ModelID: model.ID, ProjectID: p.consumer.ProjectID(), Categories: groups}, Collections: collections, ExtraAnchored: extra, Namespaces: p.namespaces, Ontologies: p.ontologies, Options: opts})
	case string(EntityCollection):
		coll, err := p.collection(p.root)
		if err != nil {
			return nil, err
		}
		snap = BuildCollectionSnapshot(CollectionSnapshotInput{Project: project, Collection: coll, Fields: flattenGroups(groups), Namespaces: p.namespaces, Ontologies: p.ontologies, Options: opts})
	case string(EntityField):
		field, err := p.field(p.root)
		if err != nil {
			return nil, err
		}
		fields := flattenGroups(groups)
		if len(fields) != 1 {
			return nil, fmt.Errorf("field generator requires exactly one consumer base override, found %d", len(fields))
		}
		snap = BuildFieldSnapshot(FieldSnapshotInput{Project: project, Field: field, Resolved: fields[0], Namespaces: p.namespaces, Ontologies: p.ontologies, Options: opts})
	default:
		return nil, fmt.Errorf("unsupported generator entity %q", p.root.Kind)
	}
	for i := range snap.Fields {
		constraint := enums[constraintKey{snap.Fields[i].Field.OverrideProjectID, snap.Fields[i].Field.OverrideID}]
		snap.Fields[i].ConceptEnum = slices.Clone(constraint.uris)
		snap.Fields[i].ConceptEnumClosed = constraint.closed
	}
	snap.SchemaVersion = 1
	snap.Selection = Selection{ProjectID: p.consumer.ProjectID(), Version: p.consumer.Version(), Draft: p.consumer.IsDraft(), Kind: p.root.Kind, ID: p.root.ID, SourceProjectID: p.root.Source.ProjectID(), SourceVersion: p.root.Source.Version()}
	snap.OntologyBundle = p.bundle
	return snap, nil
}

func flattenGroups(groups []domain.CategoryGroup) []domain.ResolvedField {
	out := []domain.ResolvedField{}
	for _, g := range groups {
		for _, c := range g.Collections {
			out = append(out, c.Fields...)
		}
	}
	return out
}

func (p *projection) groups(view projectsnapshot.ContextView) ([]domain.CategoryGroup, map[string]*domain.Collection, map[constraintKey]fieldConstraint, error) {
	groups := []domain.CategoryGroup{}
	collections := map[string]*domain.Collection{}
	enums := map[constraintKey]fieldConstraint{}
	for _, g := range view.Groups {
		cat := domain.CategoryGroup{ID: "__uncategorized__", Name: domain.Translations{"en": "Uncategorized"}, Position: 9999}
		if g.Category != nil {
			cat.ID = g.Category.ID
			cat.Position = int(g.Category.CanonicalOrder)
			if err := decodeProjection(g.Category.UIName, &cat.Name); err != nil {
				return nil, nil, nil, err
			}
		}
		col := domain.CollectionGroup{ID: directCollectionID, Name: domain.Translations{}}
		if g.Collection != nil {
			c, err := p.collection(projectsnapshot.EntityKey{Source: g.CollectionSource, Kind: string(EntityCollection), ID: g.Collection.ID})
			if err != nil {
				return nil, nil, nil, err
			}
			collections[c.ID] = &c
			col.ID = c.ID
			if err := decodeProjection(g.Name, &col.Name); err != nil {
				return nil, nil, nil, err
			}
		}
		if g.Placement != nil {
			r := g.Placement
			col.Placement = &domain.CollectionPlacement{ID: r.ID, ProjectID: r.ProjectID, ModelID: r.ModelID, CategoryID: r.CategoryID, CollectionID: r.CollectionID, IsRequired: r.IsRequired, MinOccurs: int(r.MinOccurs), MaxOccurs: projectionInt(r.MaxOccurs), IsHidden: r.IsHidden}
		}
		for i, field := range g.Fields {
			rf, choices, err := p.resolved(field)
			if err != nil {
				return nil, nil, nil, err
			}
			col.Fields = append(col.Fields, rf)
			enums[constraintKey{rf.OverrideProjectID, rf.OverrideID}] = choices
			if i == 0 || rf.CollectionOrder < col.Position {
				col.Position = rf.CollectionOrder
			}
		}
		col.SharedPathPrefix = computeSharedPathPrefix(col.Fields)
		if len(groups) == 0 || groups[len(groups)-1].ID != cat.ID {
			groups = append(groups, cat)
		}
		groups[len(groups)-1].Collections = append(groups[len(groups)-1].Collections, col)
	}
	return groups, collections, enums, nil
}

func (p *projection) resolved(f projectsnapshot.ContextField) (domain.ResolvedField, fieldConstraint, error) {
	body, err := p.field(f.Source)
	if err != nil {
		return domain.ResolvedField{}, fieldConstraint{}, err
	}
	o := f.Effective
	r := domain.ResolvedField{SourceVersion: f.Source.Source.Version(), OverrideProjectID: o.ProjectID, SetValueEntryID: projectionValue(o.SetValueEntryID), ID: body.ID, SemanticID: body.SemanticID, SystemName: body.SystemName, ProjectID: body.ProjectID, OntologyPath: body.OntologyPath(), PathElements: body.PathElements, SubfieldPaths: body.SubfieldPaths, Position: int(o.Position), ExpectedValueType: body.ExpectedValueType, SetValue: projectionValue(o.SetValue), IsRequired: projectionValue(o.IsRequired), MinOccurs: int(projectionValue(o.MinOccurs)), MaxOccurs: projectionInt(o.MaxOccurs), IsHidden: projectionValue(o.IsHidden), Visibility: projectionValue(o.Visibility), IsExternal: body.ProjectID != p.consumer.ProjectID(), OverrideSource: o.EntityType, OverrideID: o.ID, CategoryID: projectionValue(o.CategoryID), PartOfCollectionID: projectionValue(o.PartOfCollectionID), CollectionOrder: int(o.CollectionOrder)}
	if r.OverrideSource == "" {
		r.OverrideSource = "base"
	}
	for _, v := range []struct {
		raw    []byte
		target any
	}{{o.DisplayName, &r.DisplayName}, {o.Description, &r.Description}, {o.CollectionName, &r.CollectionName}} {
		if err := decodeProjection(v.raw, v.target); err != nil {
			return r, fieldConstraint{}, err
		}
	}
	choices := fieldConstraint{uris: []string{}}
	for _, ref := range f.Refs {
		target := domain.EntityRef{ID: ref.Target.ID, SemanticID: ref.Record.SemanticID, IsClosed: ref.IsClosed, Origin: domain.OriginFromProject(p.consumer.ProjectID(), ref.Target.Source.ProjectID())}
		if err := decodeProjection(ref.Name, &target.Name); err != nil {
			return r, fieldConstraint{}, err
		}
		switch ref.Record.RefType {
		case "resource_model":
			r.ResourceModels = append(r.ResourceModels, target)
		case "collection_model":
			r.CollectionModels = append(r.CollectionModels, target)
		case "concept_list":
			r.ConceptLists = append(r.ConceptLists, target)
			if ref.IsClosed {
				choices.closed = true
				for _, member := range ref.Concepts {
					uri, err := conceptURI(member.Entry.URI, ref.ConceptNamespace)
					if err != nil {
						return r, fieldConstraint{}, err
					}
					choices.uris = append(choices.uris, uri)
				}
			}
		default:
			return r, fieldConstraint{}, fmt.Errorf("unsupported generator reference %q", ref.Record.RefType)
		}
	}
	slices.Sort(choices.uris)
	choices.uris = slices.Compact(choices.uris)
	return r, choices, nil
}

func decodeProjection(data []byte, target any) error {
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("invalid generator projection: %w", err)
	}
	return nil
}
func projectionValue[T any](p *T) (v T) {
	if p != nil {
		return *p
	}
	return v
}
func projectionInt(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}
