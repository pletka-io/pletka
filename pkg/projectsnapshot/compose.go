package projectsnapshot

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ContextView is a detached composition, not a portable complete-project format.
// Raw bodies and overrides stay separate; no renderer can accidentally overwrite
// source provenance while applying a consumer's fixed-value commitment.
type ContextView struct {
	Context Identity
	Entity  EntityKey
	Groups  []ContextGroup
}

// ContextGroup preserves each category/collection placement, including empty ones.
// Nil Category/Collection denotes the direct or uncategorized grouping.
type ContextGroup struct {
	Category                         *CategoryRecord
	Collection                       *CollectionRecord
	CategorySource, CollectionSource Identity
	Name                             []byte
	Placement                        *PlacementRecord
	Fields                           []ContextField
}

// ContextField retains the source body, original override and effective settings.
type ContextField struct {
	Source    EntityKey
	Body      FieldRecord
	Override  OverrideRecord
	Effective OverrideRecord
	FixedBy   *OverrideRecord
	Refs      []ContextRef
}

// ContextRef resolves a required target in its captured source context.
type ContextRef struct {
	Record           OverrideRefRecord
	Target           EntityKey
	Name             []byte
	IsClosed         bool
	ConceptNamespace string
	Concepts         []ConceptChoice
}

// ConceptChoice retains ordered membership, custom labels and exact entry data.
type ConceptChoice struct {
	Membership ConceptEntryRecord
	Source     EntityKey
	Entry      VocabularyEntryRecord
}

// Compose uses the requesting consumer's explicit contextual rows. Source-owned
// definitions are available through ComposeContext, never a missing-row fallback.
// Collection anchor discovery and renderer-specific path grafting belong outside
// this value operation; it has no global catalog or database capability.
func (f EntityFragment) Compose() (ContextView, error) {
	if f.graph == nil {
		return ContextView{}, fmt.Errorf("entity fragment is uninitialized")
	}
	return f.ComposeContext(f.graph.root, f.root.Kind, f.root.ID)
}

// ComposeContext composes a selected model, collection or field in one retained
// context. Every occurrence survives; copy-on-adopt does not choose one winner.
func (f EntityFragment) ComposeContext(context Identity, kind, id string) (ContextView, error) {
	if f.graph == nil || !f.projects[context] {
		return ContextView{}, fmt.Errorf("composition context is outside fragment")
	}
	if kind != kindModel && kind != kindCollection && kind != kindField {
		return ContextView{}, fmt.Errorf("unsupported composition root %s", kind)
	}
	key, err := f.selectedEntity(context, kind, id)
	if err != nil {
		return ContextView{}, err
	}
	c, err := f.Content(context)
	if err != nil {
		return ContextView{}, err
	}
	view := ContextView{Context: context, Entity: EntityKey{Source: key.source, Kind: kind, ID: id}, Groups: []ContextGroup{}}
	groups, err := f.composeGroups(context, kind, id, c)
	if err != nil {
		return ContextView{}, err
	}
	for _, group := range groups {
		slices.SortFunc(group.Fields, func(a, b ContextField) int {
			if c := compareInt(int64(a.Override.Position), int64(b.Override.Position)); c != 0 {
				return c
			}
			return compareInt(a.Override.ID, b.Override.ID)
		})
		if group.Collection != nil && len(group.Fields) > 0 {
			raw := group.Fields[0].Override.CollectionName
			present, err := nonemptyName(raw)
			if err != nil {
				return ContextView{}, err
			}
			if present {
				group.Name = slices.Clone(raw)
			}
		}
		view.Groups = append(view.Groups, *group)
	}
	slices.SortFunc(view.Groups, compareGroups)
	return view, nil
}

type groupKey struct{ category, collection string }

func (f EntityFragment) contextGroup(context Identity, groups map[groupKey]*ContextGroup, key groupKey) (*ContextGroup, error) {
	if g := groups[key]; g != nil {
		return g, nil
	}
	g := &ContextGroup{Fields: []ContextField{}}
	if key.category != "" {
		node, err := f.selectedEntity(context, kindCategory, key.category)
		if err != nil {
			return nil, err
		}
		g.Category = &CategoryRecord{}
		if err := json.Unmarshal([]byte(f.graph.nodes[node].data), g.Category); err != nil {
			return nil, err
		}
		g.CategorySource = node.source
	}
	if key.collection != "" {
		node, err := f.selectedEntity(context, kindCollection, key.collection)
		if err != nil {
			return nil, err
		}
		g.Collection = &CollectionRecord{}
		if err := json.Unmarshal([]byte(f.graph.nodes[node].data), g.Collection); err != nil {
			return nil, err
		}
		g.CollectionSource = node.source
		g.Name = slices.Clone(g.Collection.UIName)
	}
	groups[key] = g
	return g, nil
}

func (f EntityFragment) selectedEntity(context Identity, kind, id string) (nodeKey, error) {
	node, err := f.graph.resolve(context, kind, id)
	if err != nil {
		return nodeKey{}, err
	}
	if !f.selected[node] {
		return nodeKey{}, fmt.Errorf("entity %s %s is outside selected fragment", kind, id)
	}
	return node, nil
}

func (f EntityFragment) composeField(context Identity, o OverrideRecord, base map[string]OverrideRecord, refs []OverrideRefRecord) (ContextField, error) {
	node, err := f.selectedEntity(context, kindField, o.FieldID)
	if err != nil {
		return ContextField{}, err
	}
	field := ContextField{Source: EntityKey{Source: node.source, Kind: kindField, ID: o.FieldID}, Override: o, Effective: o, Refs: []ContextRef{}}
	if err := json.Unmarshal([]byte(f.graph.nodes[node].data), &field.Body); err != nil {
		return ContextField{}, err
	}
	// Existing resolution takes type from the body and all display/cardinality
	// settings from the explicit copied override, including false/zero/null.
	field.Effective.ExpectedValueType = field.Body.ExpectedValueType
	if b, ok := base[o.FieldID]; ok && (str(b.SetValue) != "" || str(b.SetValueEntryID) != "") {
		field.FixedBy = &b
		field.Effective.SetValue = b.SetValue
		field.Effective.SetValueEntryID = b.SetValueEntryID
	}
	for _, r := range refs {
		if r.OverrideID != o.ID {
			continue
		}
		ref, err := f.composeRef(context, r)
		if err != nil {
			return ContextField{}, err
		}
		field.Refs = append(field.Refs, ref)
	}
	slices.SortFunc(field.Refs, func(a, b ContextRef) int {
		if c := strings.Compare(a.Record.RefType, b.Record.RefType); c != 0 {
			return c
		}
		return compareInt(int64(a.Record.Position), int64(b.Record.Position))
	})
	return field, nil
}

func (f EntityFragment) composeRef(context Identity, r OverrideRefRecord) (ContextRef, error) {
	kind := r.RefType
	if kind == refResourceModel {
		kind = kindModel
	}
	if kind == refCollectionModel {
		kind = kindCollection
	}
	id := r.TargetID
	if id == "" {
		id = r.SemanticID
	}
	node, err := f.selectedEntity(context, kind, id)
	if err != nil {
		return ContextRef{}, err
	}
	ref := ContextRef{Record: r, Target: EntityKey{Source: node.source, Kind: kind, ID: id}, Concepts: []ConceptChoice{}}
	var name struct {
		UIName []byte `json:"ui_name"`
	}
	if err := json.Unmarshal([]byte(f.graph.nodes[node].data), &name); err != nil {
		return ContextRef{}, err
	}
	ref.Name = name.UIName
	if kind != kindConceptList {
		return ref, nil
	}
	var list ConceptListRecord
	if err := json.Unmarshal([]byte(f.graph.nodes[node].data), &list); err != nil {
		return ContextRef{}, err
	}
	ref.IsClosed = list.IsClosed
	c, err := f.Content(node.source)
	if err != nil {
		return ContextRef{}, err
	}
	ref.ConceptNamespace = str(c.Projects[0].EffectiveConceptNamespace)
	for _, member := range c.ConceptEntries {
		if member.ConceptListID != id {
			continue
		}
		entry, err := f.selectedEntity(node.source, kindVocabularyEntry, member.VocabularyEntryID)
		if err != nil {
			return ContextRef{}, err
		}
		choice := ConceptChoice{Membership: member, Source: EntityKey{Source: entry.source, Kind: kindVocabularyEntry, ID: member.VocabularyEntryID}}
		if err := json.Unmarshal([]byte(f.graph.nodes[entry].data), &choice.Entry); err != nil {
			return ContextRef{}, err
		}
		ref.Concepts = append(ref.Concepts, choice)
	}
	slices.SortFunc(ref.Concepts, func(a, b ConceptChoice) int {
		if c := compareInt(int64(a.Membership.Position), int64(b.Membership.Position)); c != 0 {
			return c
		}
		return strings.Compare(a.Membership.ID, b.Membership.ID)
	})
	return ref, nil
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func compareInt(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
func nonemptyName(raw []byte) (bool, error) {
	if len(raw) == 0 {
		return false, nil
	}
	var names map[string]string
	if err := json.Unmarshal(raw, &names); err != nil {
		return false, fmt.Errorf("invalid collection label: %w", err)
	}
	return len(names) > 0, nil
}

func compareGroups(a, b ContextGroup) int {
	category := func(g ContextGroup) (int64, string) {
		if g.Category == nil {
			return 9999, ""
		}
		return int64(g.Category.CanonicalOrder), g.Category.ID
	}
	ap, ai := category(a)
	bp, bi := category(b)
	if c := compareInt(ap, bp); c != 0 {
		return c
	}
	if c := strings.Compare(ai, bi); c != 0 {
		return c
	}
	order := func(g ContextGroup) int64 {
		n := int64(0)
		for i, f := range g.Fields {
			if i == 0 || int64(f.Override.CollectionOrder) < n {
				n = int64(f.Override.CollectionOrder)
			}
		}
		return n
	}
	if c := compareInt(order(a), order(b)); c != 0 {
		return c
	}
	minPosition := func(g ContextGroup) int64 {
		if len(g.Fields) == 0 {
			return 0
		}
		return int64(g.Fields[0].Override.Position)
	}
	if c := compareInt(minPosition(a), minPosition(b)); c != 0 {
		return c
	}

	id := func(g ContextGroup) string {
		if g.Collection == nil {
			return ""
		}
		return g.Collection.ID
	}
	return strings.Compare(id(a), id(b))
}

func (f EntityFragment) composeGroups(context Identity, kind, id string, c Content) (map[groupKey]*ContextGroup, error) {
	base := map[string]OverrideRecord{}
	for _, o := range c.Overrides {
		if o.EntityType == "" {
			if _, ok := base[o.FieldID]; ok {
				return nil, fmt.Errorf("duplicate base override")
			}
			base[o.FieldID] = o
		}
	}
	groups := map[groupKey]*ContextGroup{}
	for _, o := range c.Overrides {
		if !matchesContext(o, kind, id) {
			continue
		}
		field, err := f.composeField(context, o, base, c.OverrideRefs)
		if err != nil {
			return nil, err
		}
		group, err := f.contextGroup(context, groups, groupKey{str(o.CategoryID), str(o.PartOfCollectionID)})
		if err != nil {
			return nil, err
		}
		group.Fields = append(group.Fields, field)
	}
	if kind == kindModel {
		for _, placement := range c.Placements {
			if placement.ModelID != id {
				continue
			}
			group, err := f.contextGroup(context, groups, groupKey{placement.CategoryID, placement.CollectionID})
			if err != nil {
				return nil, err
			}
			if group.Placement != nil {
				return nil, fmt.Errorf("duplicate collection placement")
			}
			group.Placement = &placement
		}
	}

	return groups, nil
}

func matchesContext(o OverrideRecord, kind, id string) bool {
	return o.EntityType == kind && o.EntityID == id || kind == kindField && o.EntityType == "" && o.FieldID == id
}
