package detailview

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// projectedDetail contains display data only. Capability decoration is a separate
// current-policy operation; this projection cannot call any service or store.
type projectedDetail struct {
	Response      Response
	Stats         domain.ModelViewStats
	Consumer      projectsnapshot.Identity
	ProjectName   domain.Translations
	ConceptSource string
	RemoteSource  bool
	Closed        bool
}

type detailProjection struct {
	fragment projectsnapshot.EntityFragment
	consumer projectsnapshot.Identity
	root     projectsnapshot.EntityKey
	content  map[projectsnapshot.Identity]projectsnapshot.Content
	err      error
}

func projectDetail(input browserinput.Detail) (projectedDetail, error) {
	var out projectedDetail
	consumer, err := input.Fragment.Consumer()
	if err != nil {
		return out, err
	}
	root, err := input.Fragment.Root()
	if err != nil {
		return out, err
	}
	ids, err := input.Fragment.Projects()
	if err != nil {
		return out, err
	}
	p := &detailProjection{fragment: input.Fragment, consumer: consumer, root: root, content: map[projectsnapshot.Identity]projectsnapshot.Content{}}
	for _, id := range ids {
		c, err := input.Fragment.Content(id)
		if err != nil {
			return out, err
		}
		p.content[id] = c
	}
	out.Consumer = consumer
	out.Response.Sections = []ViewSection{}
	out.Response.Refs = ViewRefs{Categories: map[string]ViewRefEntry{}, Collections: map[string]ViewRefEntry{}, Models: map[string]ViewRefEntry{}}
	out.Response.ViewMode = ViewModeDetailed
	out.ProjectName = retainedJSON[domain.Translations](p, p.content[consumer].Projects[0].UIName)
	out.Response.Entity = p.header(root)
	if root.Source != consumer {
		out.Response.Entity.SourceURL = capturedEntityURL(root.Source, root.Kind, root.ID)
		out.Response.Entity.SourceLabel = i18n.L("common.capabilities.source_definition", "View source definition")
	}
	if root.Kind == detailConceptListRecord {
		p.conceptList(&out)
	} else {
		view, err := input.Fragment.Compose()
		if err != nil {
			return projectedDetail{}, err
		}
		p.composition(&out, view)
		if root.Kind == detailField {
			if input.FieldUsage == nil {
				return projectedDetail{}, fmt.Errorf("field detail requires captured usage counts")
			}
			out.Stats = domain.ModelViewStats{ModelsUsing: input.FieldUsage.ModelsUsing, CollectionsUsing: input.FieldUsage.CollectionsUsing, FieldOverrideCount: input.FieldUsage.OverrideRowCount}
		}
	}
	out.Response.Adoptions = p.adoptions()
	out.Response.Entity.Origin = p.origin(root)
	out.Response.Entity.OriginLabel = originDetailLabel(out.Response.Entity.Origin)
	out.Response.Entity.OriginHelp = originDetailHelp(out.Response.Entity.Origin)
	if p.err != nil {
		return projectedDetail{}, p.err
	}
	return out, nil
}

func retainedValue[T any](v *T) (out T) {
	if v != nil {
		return *v
	}
	return out
}

func retainedJSON[T any](p *detailProjection, raw []byte) (out T) {
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			p.err = fmt.Errorf("invalid retained browser JSON: %w", err)
		}
	}
	return out
}

func (p *detailProjection) timestamp(raw projectsnapshot.Timestamp) time.Time {
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, string(raw))
	if err != nil {
		p.err = fmt.Errorf("invalid browser timestamp: %w", err)
	}
	return t
}

func (p *detailProjection) metadata(id, kind, status string, system *string, name, description, scope []byte, deprecated bool, created, updated projectsnapshot.Timestamp) EntityViewMeta {
	out := EntityViewMeta{ID: id, Type: strings.ReplaceAll(kind, "_", "-"), Name: retainedJSON[domain.Translations](p, name), Description: retainedJSON[domain.Translations](p, description), SystemName: retainedValue(system), Status: status, Deprecated: deprecated, CreatedAt: p.timestamp(created), UpdatedAt: p.timestamp(updated)}
	if kind != detailConceptListRecord {
		s := retainedJSON[domain.PathElement](p, scope)
		out.Scope = &ViewScopeInfo{Prefix: s.Prefix, LocalName: s.LocalName}
	}
	return out
}

func (p *detailProjection) header(key projectsnapshot.EntityKey) EntityViewMeta {
	c := p.content[key.Source]
	switch key.Kind {
	case detailModel:
		for _, r := range c.Models {
			if r.ID == key.ID {
				m := p.metadata(r.ID, key.Kind, r.Status, r.SystemName, r.UIName, r.Description, r.OntologyScope, r.Deprecated, r.CreatedAt, r.UpdatedAt)
				m.ModelType = r.ModelType
				return m
			}
		}
	case detailCollection:
		for _, r := range c.Collections {
			if r.ID == key.ID {
				return p.metadata(r.ID, key.Kind, r.Status, r.SystemName, r.UIName, r.Description, r.OntologyScope, r.Deprecated, r.CreatedAt, r.UpdatedAt)
			}
		}
	case detailField:
		for _, r := range c.Fields {
			if r.ID == key.ID {
				m := p.metadata(r.ID, key.Kind, r.Status, r.SystemName, r.UIName, r.Description, r.OntologyScope, r.Deprecated, r.CreatedAt, r.UpdatedAt)
				m.ExpectedValueType = retainedValue(r.ExpectedValueType)
				m.PathElements = retainedJSON[[]domain.PathElement](p, r.PathElements)
				f := domain.Field{PathElements: m.PathElements}
				m.OntologyPath = f.OntologyPath()
				return m
			}
		}
	case detailConceptListRecord:
		for _, r := range c.ConceptLists {
			if r.ID == key.ID {
				return p.metadata(r.ID, key.Kind, r.Status, r.SystemName, r.UIName, r.Description, nil, false, r.CreatedAt, r.UpdatedAt)
			}
		}
	}
	p.err = fmt.Errorf("detail root is outside retained input")
	return EntityViewMeta{}
}

func capturedEntityURL(source projectsnapshot.Identity, kind, id string) string {
	segment := entitySegment(kind)
	if segment == "" {
		return ""
	}
	return withVersion("/projects/"+url.PathEscape(source.ProjectID())+"/"+segment+"/"+url.PathEscape(id), source.Version())
}

func (p *detailProjection) origin(key projectsnapshot.EntityKey) domain.Origin {
	c := p.content[p.consumer]
	for _, f := range c.Forks {
		if f.EntityType == key.Kind && f.ForkEntityID == key.ID {
			return domain.Origin{Kind: domain.OriginForked, SourceProjectID: f.SourceProjectID, SourceEntityID: f.SourceEntityID}
		}
	}
	if key.Source == p.consumer {
		return domain.OwnOrigin()
	}
	for _, a := range c.Adoptions {
		if a.EntityType == key.Kind && a.SourceEntityID == key.ID && a.SourceProjectID == key.Source.ProjectID() {
			return domain.AdoptedOrigin(a.SourceProjectID, a.SourceEntityID)
		}
	}
	o := domain.Origin{Kind: domain.OriginInherited, SourceProjectID: key.Source.ProjectID(), SourceEntityID: key.ID}
	if p.referenced(key) {
		o.Kind = domain.OriginAdoptedReference
	}

	return o
}

func (p *detailProjection) composition(out *projectedDetail, view projectsnapshot.ContextView) {
	sections := map[string]int{}
	allFields := []domain.ResolvedField{}
	collections := map[string]bool{}
	collectionScopes := map[string]int{}
	for _, group := range view.Groups {
		if group.Placement != nil && group.Placement.IsHidden {
			continue
		}
		catID := ""
		catName := domain.Translations{"en": "Uncategorized"}
		order := 0
		if group.Category != nil {
			catID, catName, order = group.Category.ID, retainedJSON[domain.Translations](p, group.Category.UIName), int(group.Category.CanonicalOrder)
			key := projectsnapshot.EntityKey{Source: group.CategorySource, Kind: "category", ID: catID}
			out.Response.Refs.Categories[catID] = ViewRefEntry{Name: catName, Order: order, URL: capturedEntityURL(key.Source, key.Kind, key.ID), Origin: p.origin(key)}
		}
		if p.root.Kind == detailField {
			for _, f := range group.Fields {
				out.Response.Entity.CategoryID = retainedValue(f.Effective.CategoryID)
				out.Response.Entity.SetValue = retainedValue(f.Effective.SetValue)
			}
			continue
		}
		fields, items := p.groupFields(group, out.Response.Refs)
		if p.root.Kind == detailCollection && len(fields) == 0 {
			continue
		}
		index, exists := sections[catID]
		if !exists {
			index = len(out.Response.Sections)
			sections[catID] = index
			out.Response.Sections = append(out.Response.Sections, ViewSection{Widget: "category-group", ID: catID, Name: catName, CanonicalOrder: order, Items: []ViewItem{}})
		}
		item := ViewItem{Widget: "field-group", ID: "__direct__", Name: domain.Translations{"en": "Direct Fields"}, Fields: items, FieldCount: len(items)}
		if p.root.Kind == detailCollection {
			item.ID = p.root.ID
		}
		if p.root.Kind == detailModel && group.Collection != nil {
			item.Widget, item.ID, item.Name = "collection-group", group.Collection.ID, retainedJSON[domain.Translations](p, group.Name)
			item.SourceURL = capturedEntityURL(group.CollectionSource, detailCollection, group.Collection.ID)
			item.SharedPrefix = sharedBrowserPrefix(fields)
			key := projectsnapshot.EntityKey{Source: group.CollectionSource, Kind: detailCollection, ID: group.Collection.ID}
			out.Response.Refs.Collections[item.ID] = ViewRefEntry{Name: item.Name, URL: item.SourceURL, Origin: p.origin(key)}
			collections[item.ID] = true
			if len(item.SharedPrefix) > 0 {
				e := item.SharedPrefix[0]
				collectionScopes[e.Prefix+":"+e.LocalName]++
			}
		}
		if g := group.Placement; g != nil {
			item.Placement = &domain.CollectionPlacement{ID: g.ID, ProjectID: g.ProjectID, ModelID: g.ModelID, CategoryID: g.CategoryID, CollectionID: g.CollectionID, IsRequired: g.IsRequired, MinOccurs: int(g.MinOccurs), MaxOccurs: browserMax(g.MaxOccurs), IsHidden: g.IsHidden}
		}
		if p.root.Kind == detailCollection && len(out.Response.Sections[index].Items) > 0 {
			previous := &out.Response.Sections[index].Items[0]
			previous.Fields = append(previous.Fields, item.Fields...)
			previous.FieldCount = len(previous.Fields)
		} else {
			out.Response.Sections[index].Items = append(out.Response.Sections[index].Items, item)
		}
		allFields = append(allFields, fields...)
	}
	sortSectionsByOrder(out.Response.Sections)
	out.Stats = computeCollectionStats(allFields, out.Response.Sections)
	if p.root.Kind == detailModel {
		out.Stats.TotalCollections = len(collections)
		out.Stats.OverriddenFields = len(allFields)
		out.Stats.CollScopesBreakdown = buildStatItemsFromCounts(collectionScopes)
	}
}

func browserMax(v *int32) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

func sharedBrowserPrefix(fields []domain.ResolvedField) []domain.PathElement {
	if len(fields) == 0 {
		return nil
	}
	first := fields[0].PathElements
	n := len(first)
	for _, f := range fields[1:] {
		n = min(n, len(f.PathElements))
		for i := 0; i < n; i++ {
			if first[i].URI != f.PathElements[i].URI {
				n = i
				break
			}
		}
	}
	return slices.Clone(first[:n])
}

func (p *detailProjection) field(f projectsnapshot.ContextField) domain.ResolvedField {
	o, b := f.Effective, f.Body
	r := domain.ResolvedField{ID: b.ID, SemanticID: retainedValue(b.SemanticID), SystemName: retainedValue(b.SystemName), ProjectID: b.ProjectID, SourceVersion: f.Source.Source.Version(), OverrideID: o.ID, OverrideProjectID: o.ProjectID, OverrideSource: o.EntityType, Position: int(o.Position), DisplayName: retainedJSON[domain.Translations](p, o.DisplayName), Description: retainedJSON[domain.Translations](p, o.Description), ExpectedValueType: retainedValue(b.ExpectedValueType), SetValue: retainedValue(o.SetValue), SetValueEntryID: retainedValue(o.SetValueEntryID), IsRequired: retainedValue(o.IsRequired), IsHidden: retainedValue(o.IsHidden), MinOccurs: int(retainedValue(o.MinOccurs)), MaxOccurs: browserMax(o.MaxOccurs), CategoryID: retainedValue(o.CategoryID), PartOfCollectionID: retainedValue(o.PartOfCollectionID), PathElements: retainedJSON[[]domain.PathElement](p, b.PathElements), SubfieldPaths: retainedJSON[[]domain.SubfieldPath](p, b.SubfieldPaths)}
	body := domain.Field{PathElements: r.PathElements}
	r.OntologyPath = body.OntologyPath()
	for _, ref := range f.Refs {
		e := domain.EntityRef{ID: ref.Target.ID, SemanticID: ref.Record.SemanticID, Name: retainedJSON[domain.Translations](p, ref.Name), URL: capturedEntityURL(ref.Target.Source, ref.Target.Kind, ref.Target.ID)}
		switch ref.Target.Kind {
		case detailModel:
			r.ResourceModels = append(r.ResourceModels, e)
		case detailCollection:
			r.CollectionModels = append(r.CollectionModels, e)
		case detailConceptListRecord:
			r.ConceptLists = append(r.ConceptLists, e)
		}
	}
	return r
}

func (p *detailProjection) adoptions() []AdoptionItem {
	items := []AdoptionItem{}
	evidence, err := p.fragment.Evidence(p.consumer)
	if err != nil {
		p.err = err
		return items
	}
	for _, a := range p.content[p.consumer].Adoptions {
		if a.ContextEntityType != p.root.Kind || a.ContextEntityID != p.root.ID {
			continue
		}
		key, err := p.fragment.Resolve(p.consumer, a.EntityType, a.SourceEntityID)
		if err != nil {
			p.err = err
			continue
		}
		actor, err := evidence.Descriptors().Actor(a.CreatedByID)
		if err != nil {
			p.err = err
			continue
		}
		createdBy := domain.ActorRef{}
		if actor != nil {
			createdBy = domain.ActorRef{ID: retainedValue(a.CreatedByID), Label: actor.DisplayName}
		}
		items = append(items, AdoptionItem{EntityType: a.EntityType, SourceProjectID: a.SourceProjectID, SourceEntityID: a.SourceEntityID, SourceURL: capturedEntityURL(key.Source, key.Kind, key.ID), AdoptedAt: p.timestamp(a.AdoptedAt), CreatedBy: createdBy, Origin: domain.AdoptedOrigin(a.SourceProjectID, a.SourceEntityID), Label: adoptionLabel(domain.Adoption{EntityType: a.EntityType, SourceEntityID: a.SourceEntityID})})
	}
	slices.SortFunc(items, func(a, b AdoptionItem) int {
		if n := strings.Compare(a.EntityType, b.EntityType); n != 0 {
			return n
		}
		return strings.Compare(a.SourceEntityID, b.SourceEntityID)
	})
	return items
}

func (p *detailProjection) referenced(key projectsnapshot.EntityKey) bool {
	c := p.content[p.consumer]
	for _, row := range c.Overrides {
		if key.Kind == detailField && row.FieldID == key.ID || key.Kind == detailCollection && retainedValue(row.PartOfCollectionID) == key.ID {
			return true
		}
	}
	for _, row := range c.Placements {
		if key.Kind == detailCollection && row.CollectionID == key.ID {
			return true
		}
	}
	for _, r := range c.OverrideRefs {
		kind := strings.TrimSuffix(r.RefType, "_model")
		if r.RefType == "resource_model" {
			kind = detailModel
		}
		if kind == key.Kind && (r.TargetID == key.ID || r.SemanticID == key.ID) {
			return true
		}
	}
	return false
}

func (p *detailProjection) groupFields(group projectsnapshot.ContextGroup, refs ViewRefs) ([]domain.ResolvedField, []ViewField) {
	fields := []domain.ResolvedField{}
	items := []ViewField{}
	for _, f := range group.Fields {
		if retainedValue(f.Effective.IsHidden) {
			continue
		}
		r := p.field(f)
		fields = append(fields, r)
		item := ViewField{Widget: "field-override", OverrideID: r.OverrideID, FieldID: r.ID, FieldSemanticID: r.SemanticID, FieldSystemName: r.SystemName, Position: r.Position, DisplayName: r.DisplayName, Description: r.Description, ExpectedValueType: r.ExpectedValueType, SetValue: r.SetValue, IsRequired: r.IsRequired, IsHidden: r.IsHidden, OntologyPath: r.OntologyPath, PathElements: r.PathElements, ResourceModelRefs: r.ResourceModels, CollectionModelRefs: r.CollectionModels, ConceptListRefs: r.ConceptLists, CategoryID: r.CategoryID, CollectionID: r.PartOfCollectionID}
		item.Origin = p.origin(f.Source)
		item.SourceFieldURL = capturedEntityURL(f.Source.Source, detailField, f.Source.ID)
		item.SourceCollectionURL = ""
		if group.Collection != nil {
			item.SourceCollectionURL = capturedEntityURL(group.CollectionSource, detailCollection, group.Collection.ID)
		}
		for _, ref := range f.Refs {
			entry := ViewRefEntry{Name: retainedJSON[domain.Translations](p, ref.Name), URL: capturedEntityURL(ref.Target.Source, ref.Target.Kind, ref.Target.ID), Origin: p.origin(ref.Target)}
			if ref.Target.Kind == detailModel {
				refs.Models[ref.Target.ID] = entry
			}
			if ref.Target.Kind == detailCollection {
				refs.Collections[ref.Target.ID] = entry
			}
		}
		items = append(items, item)
	}
	return fields, items
}
