package example

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

func (p *exampleProjection) modelView(kind, id string) (*domain.ModelView, error) {
	view, err := p.fragment.ComposeContext(p.context, kind, id)
	if err != nil {
		return nil, err
	}
	out := &domain.ModelView{ProjectID: p.context.ProjectID(), ModelID: id, Categories: []domain.CategoryGroup{}}
	for _, group := range view.Groups {
		cat := domain.CategoryGroup{ID: "__uncategorized__", Name: domain.Translations{"en": "Uncategorized"}, Position: 9999}
		if group.Category != nil {
			cat.ID = group.Category.ID
			cat.Name = domain.Translations{}
			cat.Position = int(group.Category.CanonicalOrder)
			if err := exampleJSON(group.Category.UIName, &cat.Name); err != nil {
				return nil, err
			}
		}
		coll := domain.CollectionGroup{ID: directGroupID, Name: domain.Translations{"en": "Direct Fields"}, Fields: []domain.ResolvedField{}}
		if group.Collection != nil {
			coll.ID = group.Collection.ID
			coll.Name = domain.Translations{}
			if err := exampleJSON(group.Name, &coll.Name); err != nil {
				return nil, err
			}
		}
		if row := group.Placement; row != nil {
			coll.Placement = &domain.CollectionPlacement{ID: row.ID, ProjectID: row.ProjectID, ModelID: row.ModelID, CategoryID: row.CategoryID, CollectionID: row.CollectionID, IsRequired: row.IsRequired, MinOccurs: int(row.MinOccurs), MaxOccurs: exampleInt(row.MaxOccurs), IsHidden: row.IsHidden}
		}
		for i, field := range group.Fields {
			v, err := p.field(field)
			if err != nil {
				return nil, err
			}
			coll.Fields = append(coll.Fields, v)
			if i == 0 || v.CollectionOrder < coll.Position {
				coll.Position = v.CollectionOrder
			}
		}
		if group.Collection != nil {
			coll.SharedPathPrefix = exampleSharedPath(coll.Fields)
		}
		if len(out.Categories) == 0 || out.Categories[len(out.Categories)-1].ID != cat.ID {
			out.Categories = append(out.Categories, cat)
		}
		index := len(out.Categories) - 1
		out.Categories[index].Collections = append(out.Categories[index].Collections, coll)
	}
	return out, nil
}

func (p *exampleProjection) ModelView(_ context.Context, id, project string) (*domain.ModelView, error) {
	if project != p.context.ProjectID() {
		return nil, fmt.Errorf("model view cannot retarget its context")
	}
	return p.modelView(modelKind, id)
}

func (p *exampleProjection) CollectionView(_ context.Context, id, project string) ([]domain.ResolvedField, error) {
	if project != p.context.ProjectID() {
		return nil, fmt.Errorf("collection view cannot retarget its context")
	}
	key, ok := p.collections[id]
	if !ok {
		return nil, fmt.Errorf("collection is outside selected references")
	}
	view, err := p.fragment.ComposeContext(key.Source, "collection", id)
	if err != nil {
		return nil, err
	}
	out := []domain.ResolvedField{}
	for _, g := range view.Groups {
		for _, field := range g.Fields {
			row, err := p.field(field)
			if err != nil {
				return nil, err
			}
			out = append(out, row)
		}
	}
	return out, nil
}

func (p *exampleProjection) field(f projectsnapshot.ContextField) (domain.ResolvedField, error) {
	b, o := f.Body, f.Effective
	r := domain.ResolvedField{
		ID:                 b.ID,
		SemanticID:         exampleValue(b.SemanticID),
		SystemName:         exampleValue(b.SystemName),
		ProjectID:          b.ProjectID,
		SourceVersion:      f.Source.Source.Version(),
		OverrideProjectID:  o.ProjectID,
		OverrideID:         o.ID,
		OverrideSource:     o.EntityType,
		Position:           int(o.Position),
		CategoryID:         exampleValue(o.CategoryID),
		PartOfCollectionID: exampleValue(o.PartOfCollectionID),
		CollectionOrder:    int(o.CollectionOrder),
		ExpectedValueType:  exampleValue(b.ExpectedValueType),
		SetValue:           exampleValue(o.SetValue),
		SetValueEntryID:    exampleValue(o.SetValueEntryID),
		IsRequired:         exampleValue(o.IsRequired),
		MinOccurs:          int(exampleValue(o.MinOccurs)),
		MaxOccurs:          exampleInt(o.MaxOccurs),
		IsHidden:           exampleValue(o.IsHidden),
		Visibility:         exampleValue(o.Visibility),
	}
	if o.ExpectedValueType != nil {
		r.ExpectedValueType = *o.ExpectedValueType
	}
	for _, v := range []struct {
		raw   []byte
		value any
	}{{o.DisplayName, &r.DisplayName}, {o.Description, &r.Description}, {o.CollectionName, &r.CollectionName}, {b.PathElements, &r.PathElements}, {b.SubfieldPaths, &r.SubfieldPaths}} {
		if err := exampleJSON(v.raw, v.value); err != nil {
			return r, err
		}
	}
	if len(r.DisplayName) == 0 {
		if err := exampleJSON(b.UIName, &r.DisplayName); err != nil {
			return r, err
		}
	}
	for _, ref := range f.Refs {
		target := domain.EntityRef{ID: ref.Target.ID, SemanticID: ref.Record.SemanticID, IsClosed: ref.IsClosed, URL: exampleURL(ref.Target)}
		if err := exampleJSON(ref.Name, &target.Name); err != nil {
			return r, err
		}
		switch ref.Record.RefType {
		case "resource_model":
			r.ResourceModels = append(r.ResourceModels, target)
		case "collection_model":
			r.CollectionModels = append(r.CollectionModels, target)
			p.collections[target.ID] = ref.Target
		case "concept_list":
			r.ConceptLists = append(r.ConceptLists, target)
			choices := []ConceptChoice{}
			for _, member := range ref.Concepts {
				var label domain.Translations
				if err := exampleJSON(member.Membership.CustomLabel, &label); err != nil {
					return r, err
				}
				if len(label) == 0 {
					if err := exampleJSON(member.Entry.Label, &label); err != nil {
						return r, err
					}
				}
				choices = append(choices, ConceptChoice{URI: member.Entry.URI, Label: label})
			}
			p.concepts[target.ID] = choices
		}
	}
	return r, nil
}

func (p *exampleProjection) name(key projectsnapshot.EntityKey) (domain.Translations, error) {
	var raw []byte
	c := p.content[key.Source]
	if key.Kind == modelKind {
		for _, r := range c.Models {
			if r.ID == key.ID {
				raw = r.UIName
				break
			}
		}
	} else {
		for _, r := range c.Collections {
			if r.ID == key.ID {
				raw = r.UIName
				break
			}
		}
	}
	var out domain.Translations
	err := exampleJSON(raw, &out)
	return out, err
}

func (p *exampleProjection) conceptAllowed(_ context.Context, uri string, refs []domain.EntityRef) (bool, error) {
	for _, ref := range refs {
		for _, entry := range p.concepts[ref.ID] {
			if entry.URI == uri {
				return true, nil
			}
		}
	}
	return false, nil
}

func (p *exampleProjection) linkedExample(_ context.Context, id string) (*domain.Example, error) {
	// Only references selected from the stored link columns may satisfy a payload.
	for _, link := range p.linked {
		if link.ID == id {
			return &domain.Example{ID: link.ID, EntityType: domain.ExampleEntityType(link.EntityType), EntityID: link.EntityID, Title: link.Title, Status: domain.ExampleStatus(link.Status)}, nil
		}
	}
	return nil, nil
}

func (p *exampleProjection) bindConcepts(sections []ExampleFormSection, lang string) {
	var fields func([]ExampleFormField)
	fields = func(rows []ExampleFormField) {
		for i := range rows {
			f := &rows[i]
			for j := range f.ConceptSources {
				source := &f.ConceptSources[j]
				source.SearchURL = ""
				source.Entries = p.concepts[source.ID]
				if source.Entries == nil {
					source.Entries = []ConceptChoice{}
				}
			}
			for j := range f.Occurrences {
				value := &f.Occurrences[j].Value
				if value.ConceptURI != nil && value.ConceptLabel == nil {
					for _, ref := range f.ConceptSources {
						for _, entry := range ref.Entries {
							if entry.URI == *value.ConceptURI {
								label := entry.Label.Get(lang, entry.URI)
								value.ConceptLabel = &label
							}
						}
					}
				}
			}
			if f.Nested != nil {
				fields(f.Nested.Fields)
			}
			for j := range f.NestedInstances {
				fields(f.NestedInstances[j].Fields)
			}
		}
	}
	for i := range sections {
		fields(sections[i].DirectFields)
		for j := range sections[i].Groups {
			fields(sections[i].Groups[j].Fields)
		}
	}
}

func unplacedValues(resolver *slotResolver, values, normalized []domain.ExampleValue) ([]UnplacedValue, error) {
	out := []UnplacedValue{}
	for i, value := range values {
		slot, err := resolver.resolve(valueSlotPath(normalized[i]))
		if err != nil {
			return nil, err
		}
		reason := slot.reason
		if slot.status == slotMoved {
			reason = "This value was saved in a different group."
		}
		if slot.status == slotOK && value.ValuePayload.Kind != valueKindForExpectedType(slot.field.ExpectedValueType) {
			reason = "This recorded value has a different type from its field."
		}
		if slot.status == slotOK && slot.field.ID != value.FieldID {
			reason = "This recorded value belongs to a different field."
		}
		if reason != "" {
			out = append(out, UnplacedValue{Value: value, Reason: reason})
		}
	}
	return out, nil
}

// Match the existing form's visible-field path prefix without a live resolver.
func exampleSharedPath(fields []domain.ResolvedField) []domain.PathElement {
	var prefix []domain.PathElement
	started := false
	for _, field := range fields {
		if field.IsHidden {
			continue
		}
		if !started {
			prefix = append([]domain.PathElement(nil), field.PathElements...)
			started = true
			continue
		}
		n := min(len(prefix), len(field.PathElements))
		for i := 0; i < n; i++ {
			if prefix[i].URI != field.PathElements[i].URI {
				n = i
				break
			}
		}
		prefix = prefix[:n]
	}
	return prefix
}
