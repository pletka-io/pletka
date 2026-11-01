package browserread

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

const (
	inventoryModel      = "model"
	inventoryField      = "field"
	inventoryCollection = "collection"
)

type inventoryEntity struct{ project, kind, id string }
type inventoryLocal struct{ kind, id string }
type inventoryState struct {
	root       *readsession.Session
	sessions   map[string]*readsession.Session
	members    map[string]map[inventoryLocal]string
	receipts   []projectsnapshot.AdoptionRecord
	forks      map[inventoryLocal]domain.Origin
	relations  []readsession.InventoryRelation
	rows       map[inventoryEntity]browserinput.InventoryRow
	edges      map[inventoryEntity][]readsession.InventoryRelation
	edgeCount  int
	categories map[string]string
}

func newInventoryState(ctx context.Context, root *readsession.Session) (*inventoryState, error) {
	if _, err := root.ProjectHeader(ctx); err != nil {
		return nil, err
	}
	deps, err := root.Dependencies(ctx)
	if err != nil {
		return nil, err
	}
	if len(deps) > 31 {
		return nil, fmt.Errorf("inventory project budget exceeded")
	}
	state := &inventoryState{root: root, sessions: map[string]*readsession.Session{root.ProjectID(): root}, members: map[string]map[inventoryLocal]string{}, forks: map[inventoryLocal]domain.Origin{}, rows: map[inventoryEntity]browserinput.InventoryRow{}, edges: map[inventoryEntity][]readsession.InventoryRelation{}, categories: map[string]string{}}
	if err := state.loadReceipts(ctx); err != nil {
		return nil, err
	}
	state.relations, err = root.InventoryRelations(ctx, "", nil)
	if err != nil {
		return nil, err
	}
	for _, r := range state.relations {
		if r.EntityKind == inventoryField && r.CategoryID != "" {
			if old := state.categories[r.FieldID]; old != "" && old != r.CategoryID {
				return nil, fmt.Errorf("conflicting base field categories")
			}
			state.categories[r.FieldID] = r.CategoryID
		}
	}
	return state, nil
}

func (s *inventoryState) loadReceipts(ctx context.Context) error {
	reader, err := s.root.ContentRows()
	if err != nil {
		return err
	}
	receipts, err := inventoryRecords(ctx, reader.ContentAdoptions)
	if err != nil {
		return err
	}
	for _, r := range receipts {
		if r.ContextEntityType == "project" && r.ContextEntityID == s.root.ProjectID() && inventoryKind(r.EntityType) {
			s.receipts = append(s.receipts, r)
		}
	}
	forks, err := inventoryRecords(ctx, reader.ContentForks)
	if err != nil {
		return err
	}
	for _, r := range forks {
		s.forks[inventoryLocal{r.EntityType, r.ForkEntityID}] = domain.Origin{Kind: domain.OriginForked, SourceProjectID: r.SourceProjectID, SourceEntityID: r.SourceEntityID}
	}
	return nil
}
func inventoryRecords[T any](ctx context.Context, load func(context.Context, readsession.Page) ([]T, error)) ([]T, error) {
	var out []T
	for {
		rows, err := load(ctx, readsession.Page{Limit: 1000, Offset: int64(len(out))})
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if len(out) > 10000 {
			return nil, fmt.Errorf("inventory receipt/fork budget exceeded")
		}
		if len(rows) < 1000 {
			return out, nil
		}
	}
}

func inventoryKind(kind string) bool {
	return kind == inventoryModel || kind == inventoryCollection || kind == inventoryField
}
func (s *inventoryState) session(ctx context.Context, project string) (*readsession.Session, error) {
	if bound := s.sessions[project]; bound != nil {
		return bound, nil
	}
	bound, err := s.root.ProjectDependency(ctx, project)
	if err != nil {
		return nil, err
	}
	if _, err := bound.ProjectHeader(ctx); err != nil {
		return nil, err
	}
	s.sessions[project] = bound
	return bound, nil
}
func (s *inventoryState) resolve(ctx context.Context, project, kind, id string) (inventoryEntity, error) {
	if id == "" {
		return inventoryEntity{}, fmt.Errorf("inventory reference has no identity")
	}
	if s.members[project] == nil {
		bound, err := s.session(ctx, project)
		if err != nil {
			return inventoryEntity{}, err
		}
		members, err := bound.Membership(ctx)
		if err != nil {
			return inventoryEntity{}, err
		}
		s.members[project] = map[inventoryLocal]string{}
		for _, m := range members {
			s.members[project][inventoryLocal{m.Kind, m.ID}] = m.Source.ProjectID
		}
	}
	owner := project
	if selected := s.members[project][inventoryLocal{kind, id}]; selected != "" {
		owner = selected
	}
	return inventoryEntity{owner, kind, id}, nil
}
func refKind(kind string) string {
	switch kind {
	case "resource_model":
		return inventoryModel
	case "collection_model":
		return inventoryCollection
	}
	return ""
}

// Inventory returns a single ordered page after filtering the full membership union.
// Only compact merge keys are read before pagination; bodies are hydrated for that page.
func (s *Source) Inventory(ctx context.Context, scope auth.ReadScope, project, kind string, q browserinput.InventoryQuery) (browserinput.Inventory, error) {
	var out browserinput.Inventory
	if !inventoryKind(kind) && kind != catalogConceptList && kind != catalogExample {
		return out, fmt.Errorf("unsupported inventory kind %q", kind)
	}
	if q.Page < 1 || q.PerPage < 1 || q.PerPage > 1000 || q.Page > 20000/q.PerPage {
		return out, fmt.Errorf("inventory page exceeds bounds (maximum merge window 20000)")
	}
	if s == nil || s.sessions == nil {
		return out, fmt.Errorf("inventory source is uninitialized")
	}
	err := s.sessions.Read(ctx, scope, project, func(root *readsession.Session) error {
		if kind == catalogConceptList || kind == catalogExample {
			var err error
			out, err = catalogInventory(ctx, root, kind, q)
			return err
		}
		state, err := newInventoryState(ctx, root)
		if err != nil {
			return err
		}
		out, err = state.inventory(ctx, kind, q)
		return err
	})
	if err != nil {
		return browserinput.Inventory{}, err
	}
	return out, nil
}

type inventoryMembership struct {
	origins    map[string]domain.Origin
	referenced map[string]bool
	sources    map[string]map[string]bool
	rootRefs   map[string]bool
}
type inventoryCandidate struct {
	readsession.InventoryKey
	project string
}
type inventoryCandidates struct {
	keys       []inventoryCandidate
	total      int64
	scopes     map[string]bool
	categories map[inventoryEntity]bool
	projects   []string
}

func (s *inventoryState) membership(ctx context.Context, kind string) (inventoryMembership, error) {
	origin := map[string]domain.Origin{}
	referenced := map[string]bool{}
	sources := map[string]map[string]bool{}
	rootRefs := map[string]bool{}
	add := func(k, id string, explicit bool) error {
		if k != kind || id == "" {
			return nil
		}
		key, err := s.resolve(ctx, s.root.ProjectID(), k, id)
		if err != nil {
			return err
		}
		if !explicit {
			referenced[id] = true
		}
		if key.project == s.root.ProjectID() {
			rootRefs[id] = true
			return nil
		}
		if sources[key.project] == nil {
			sources[key.project] = map[string]bool{}
		}
		sources[key.project][id] = true
		if old, ok := origin[id]; !ok || explicit {
			v := domain.AdoptedOrigin(key.project, id)
			if !explicit {
				v.Kind = domain.OriginAdoptedReference
			}
			origin[id] = v
		} else if old.SourceProjectID != key.project {
			return fmt.Errorf("conflicting inventory owner")
		}
		return nil
	}
	for _, r := range s.relations {
		for _, v := range []inventoryLocal{{r.EntityKind, r.EntityID}, {inventoryField, r.FieldID}, {inventoryCollection, r.CollectionID}, {refKind(r.RefKind), r.RefID}} {
			if err := add(v.kind, v.id, false); err != nil {
				return inventoryMembership{}, err
			}
		}
	}
	for _, r := range s.receipts {
		if err := add(r.EntityType, r.SourceEntityID, true); err != nil {
			return inventoryMembership{}, err
		}
		key, err := s.resolve(ctx, s.root.ProjectID(), r.EntityType, r.SourceEntityID)
		if err != nil {
			return inventoryMembership{}, err
		}
		if key.project != r.SourceProjectID {
			return inventoryMembership{}, fmt.Errorf("receipt source differs from selected membership")
		}
	}
	for k, v := range s.forks {
		if k.kind == kind {
			origin[k.id] = v
		}
	}
	return inventoryMembership{origin, referenced, sources, rootRefs}, nil
}

func (s *inventoryState) inventory(ctx context.Context, kind string, q browserinput.InventoryQuery) (browserinput.Inventory, error) {
	out := browserinput.Inventory{ProjectID: s.root.ProjectID(), Version: s.root.Scope().Version(), Kind: kind, Page: q.Page, PerPage: q.PerPage, Scopes: []string{}}
	membership, err := s.membership(ctx, kind)
	if err != nil {
		return out, err
	}
	candidates, err := s.candidates(ctx, kind, q, membership)
	if err != nil {
		return out, err
	}
	out.Total = candidates.total
	slices.SortFunc(candidates.keys, func(a, b inventoryCandidate) int {
		cmp := strings.Compare(a.SortKey, b.SortKey)
		if q.Descending {
			cmp = -cmp
		}
		if cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ID, b.ID)
	})
	start := min((q.Page-1)*q.PerPage, len(candidates.keys))
	end := min(start+q.PerPage, len(candidates.keys))
	out.Items, err = s.page(ctx, kind, candidates.keys[start:end], membership)
	if err != nil {
		return out, err
	}
	for scope := range candidates.scopes {
		out.Scopes = append(out.Scopes, scope)
	}
	slices.Sort(out.Scopes)
	out.Categories, err = s.categoryLabels(ctx, candidates.categories)
	return out, err
}

func (s *inventoryState) candidates(ctx context.Context, kind string, q browserinput.InventoryQuery, m inventoryMembership) (inventoryCandidates, error) {
	out := inventoryCandidates{projects: []string{s.root.ProjectID()}, scopes: map[string]bool{}, categories: map[inventoryEntity]bool{}}
	origins := map[string]string{}
	for id, o := range m.origins {
		origins[id] = string(o.Kind)
	}
	for p := range m.sources {
		out.projects = append(out.projects, p)
	}
	slices.Sort(out.projects)
	if len(m.rootRefs) > 0 {
		ids := inventoryIDs(m.rootRefs)
		idx, err := s.root.InventoryIndex(ctx, kind, ids, nil, s.categories, browserinput.InventoryQuery{}, 0)
		if err != nil {
			return out, err
		}
		if idx.Available != int64(len(ids)) {
			return out, &readsession.NotInReleaseError{ProjectID: s.root.ProjectID(), Version: s.root.Scope().Version(), EntityID: "referenced inventory member"}
		}
	}
	for _, project := range out.projects {
		bound, err := s.session(ctx, project)
		if err != nil {
			return out, err
		}
		var ids []string
		if project != s.root.ProjectID() {
			ids = inventoryIDs(m.sources[project])
		}
		idx, err := bound.InventoryIndex(ctx, kind, ids, origins, s.categories, q, q.Page*q.PerPage)
		if err != nil {
			return out, err
		}
		if ids != nil && idx.Available != int64(len(ids)) {
			return out, &readsession.NotInReleaseError{ProjectID: project, Version: bound.Scope().Version(), EntityID: "inventory member"}
		}
		out.total += idx.Total
		for _, key := range idx.Items {
			out.keys = append(out.keys, inventoryCandidate{key, project})
		}
		if len(out.keys) > 100000 {
			return out, fmt.Errorf("inventory merge budget exceeded")
		}
		for _, scope := range idx.Scopes {
			out.scopes[scope] = true
		}
		contextProject := project
		if kind == inventoryField {
			contextProject = s.root.ProjectID()
		}
		if err := s.categoryOwners(ctx, contextProject, idx.Categories, out.categories); err != nil {
			return out, err
		}
	}
	return out, nil
}
func inventoryIDs(set map[string]bool) []string {
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
func (s *inventoryState) categoryOwners(ctx context.Context, project string, ids []string, out map[inventoryEntity]bool) error {
	for _, id := range ids {
		key, err := s.resolve(ctx, project, "category", id)
		if err != nil {
			return err
		}
		out[key] = true
	}
	return nil
}

func (s *inventoryState) page(ctx context.Context, kind string, selected []inventoryCandidate, m inventoryMembership) ([]browserinput.InventoryRow, error) {
	byProject := map[string][]string{}
	for _, key := range selected {
		byProject[key.project] = append(byProject[key.project], key.ID)
	}
	values := map[string]browserinput.InventoryRow{}
	for project, ids := range byProject {
		bound, err := s.session(ctx, project)
		if err != nil {
			return nil, err
		}
		rows, err := bound.InventoryRows(ctx, kind, ids)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			row = s.decorate(row, bound, kind)
			row.Origin = domain.OwnOrigin()
			if o, ok := m.origins[row.ID]; ok {
				row.Origin = o
			}
			row.OriginKind = string(row.Origin.Kind)
			row.NotReferenced = row.Origin.Kind == domain.OriginAdopted && !m.referenced[row.ID]
			if row.Origin.Kind == domain.OriginForked {
				row.OriginalProjectID = row.Origin.SourceProjectID
			}
			if kind == inventoryField {
				row.CategoryID = s.categories[row.ID]
			}
			values[row.ID] = row
		}
	}
	s.counts(values, kind)
	out := make([]browserinput.InventoryRow, 0, len(selected))
	for _, key := range selected {
		out = append(out, values[key.ID])
	}
	return out, nil
}
func (s *inventoryState) categoryLabels(ctx context.Context, owners map[inventoryEntity]bool) (map[string]domain.Translations, error) {
	out := map[string]domain.Translations{}
	groups := map[string][]string{}
	for k := range owners {
		groups[k.project] = append(groups[k.project], k.id)
	}
	for project, ids := range groups {
		bound, err := s.session(ctx, project)
		if err != nil {
			return nil, err
		}
		labels, err := bound.InventoryCategories(ctx, ids)
		if err != nil {
			return nil, err
		}
		if len(labels) != len(ids) {
			return nil, &readsession.NotInReleaseError{ProjectID: project, Version: bound.Scope().Version(), EntityID: "inventory category"}
		}
		for id, label := range labels {
			out[id] = label
		}
	}
	return out, nil
}

func (s *inventoryState) decorate(row browserinput.InventoryRow, bound *readsession.Session, kind string) browserinput.InventoryRow {
	row.SourceVersion = bound.Scope().Version()
	row.Owned = bound.ProjectID() == s.root.ProjectID()
	row.CanDeprecate = row.Owned && !row.Deprecated && !s.root.Scope().IsRelease()
	row.CanActivate = row.Owned && row.Deprecated && !s.root.Scope().IsRelease()
	row.CurrentURL = inventoryURL(s.root, kind, row.ID)
	row.SourceURL = inventoryURL(bound, kind, row.ID)
	return row
}
func inventoryURL(bound *readsession.Session, kind, id string) string {
	path := "/projects/" + url.PathEscape(bound.ProjectID()) + "/" + kind + "s/" + url.PathEscape(id)
	if v := bound.Scope().Version(); v != "" {
		path += "?version=" + url.QueryEscape(v)
	}
	return path
}

// Counts scan the consumer's relation index once for the selected page.
func (s *inventoryState) counts(rows map[string]browserinput.InventoryRow, kind string) {
	counts := map[string]*inventoryCounts{}
	for id := range rows {
		counts[id] = &inventoryCounts{fields: map[string]bool{}, categories: map[string]bool{}, collections: map[string]bool{}, models: map[string]bool{}}
	}
	for _, r := range s.relations {
		affected := map[string]bool{}
		if kind == inventoryField {
			affected[r.FieldID] = true
		}
		if r.EntityKind == kind {
			affected[r.EntityID] = true
		}
		if kind == inventoryCollection {
			affected[r.CollectionID] = true
		}
		if refKind(r.RefKind) == kind {
			affected[r.RefID] = true
		}
		for id := range affected {
			if c := counts[id]; c != nil {
				c.add(r, kind, id)
			}
		}
	}
	for id, c := range counts {
		row := rows[id]
		row.FieldCount = len(c.fields)
		row.CategoryCount = len(c.categories)
		row.CollectionCount = len(c.collections)
		row.ModelCount = len(c.models)
		row.InUse = c.inUse
		rows[id] = row
	}
}

type inventoryCounts struct {
	fields, categories, collections, models map[string]bool
	inUse                                   bool
}

func (c *inventoryCounts) add(r readsession.InventoryRelation, kind, id string) {
	if kind == inventoryField && r.FieldID == id && r.EntityKind != inventoryField {
		if r.EntityKind == inventoryModel {
			c.models[r.EntityID] = true
		}
		if r.EntityKind == inventoryCollection {
			c.collections[r.EntityID] = true
		}
		if r.CollectionID != "" {
			c.collections[r.CollectionID] = true
		}
		c.inUse = true
	}
	if (r.EntityKind == kind && r.EntityID == id) || (kind == inventoryCollection && r.CollectionID == id) {
		if r.FieldID != "" {
			c.fields[r.FieldID] = true
		}
		if r.CategoryID != "" {
			c.categories[r.CategoryID] = true
		}
		if r.CollectionID != "" {
			c.collections[r.CollectionID] = true
		}
	}
	if refKind(r.RefKind) == kind && r.RefID == id {
		c.inUse = true
	}
}
