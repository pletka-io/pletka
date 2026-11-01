package browserread

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
)

type inventoryGroup struct{ project, kind string }

// Adoptions returns explicit receipts and the bill of materials at each selected source.
// Both the tab and drilldowns consume this value, so counts cannot use another graph.
func (s *Source) Adoptions(ctx context.Context, scope auth.ReadScope, project string) (browserinput.Adoptions, error) {
	var out browserinput.Adoptions
	if s == nil || s.sessions == nil {
		return out, fmt.Errorf("adoption source is uninitialized")
	}
	err := s.sessions.Read(ctx, scope, project, func(root *readsession.Session) error {
		state, err := newInventoryState(ctx, root)
		if err != nil {
			return err
		}
		if len(state.receipts) > 1000 {
			return fmt.Errorf("adoption receipt budget exceeded")
		}
		actors, err := state.adoptionActors(ctx)
		if err != nil {
			return err
		}
		out = browserinput.Adoptions{ProjectID: project, Version: scope.Version(), Items: []browserinput.Adoption{}}
		expandedRows := 0
		for _, r := range state.receipts {
			key, err := state.resolve(ctx, project, r.EntityType, r.SourceEntityID)
			if err != nil {
				return err
			}
			if key.project != r.SourceProjectID {
				return fmt.Errorf("receipt source differs from selected membership")
			}
			bound, err := state.session(ctx, key.project)
			if err != nil {
				return err
			}
			if r.SourceVersion != "" && r.SourceVersion != bound.Scope().Version() {
				return fmt.Errorf("receipt version differs from selected source")
			}
			r.SourceVersion = bound.Scope().Version()
			direct, closure, err := state.closure(ctx, key)
			if err != nil {
				return err
			}
			item := browserinput.Adoption{Receipt: r, Root: state.rows[key], Direct: direct, Closure: closure}
			if r.CreatedByID != nil {
				var ok bool
				item.Actor, ok = actors[*r.CreatedByID]
				if *r.CreatedByID != "" && !ok {
					return fmt.Errorf("adoption actor unavailable")
				}
			}
			expandedRows += expandedAdoptionRows(item)
			if expandedRows > 100000 {
				return fmt.Errorf("adoption expanded response budget exceeded")
			}
			out.Items = append(out.Items, item)
		}
		slices.SortFunc(out.Items, func(a, b browserinput.Adoption) int {
			if cmp := strings.Compare(string(b.Receipt.AdoptedAt), string(a.Receipt.AdoptedAt)); cmp != 0 {
				return cmp
			}
			if cmp := strings.Compare(a.Receipt.SourceEntityID, b.Receipt.SourceEntityID); cmp != 0 {
				return cmp
			}
			return strings.Compare(a.Receipt.EntityType, b.Receipt.EntityType)
		})
		return nil
	})
	if err != nil {
		return browserinput.Adoptions{}, err
	}
	return out, nil
}

func (s *inventoryState) adoptionActors(ctx context.Context) (map[string]domain.ActorRef, error) {
	actors := map[string]domain.ActorRef{}
	if s.root.Scope().IsRelease() {
		descriptors, err := s.root.Descriptors(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range s.receipts {
			a, err := descriptors.Actor(r.CreatedByID)
			if err != nil {
				return nil, err
			}
			if a != nil {
				actors[a.ActorID] = domain.ActorRef{ID: a.ActorID, Label: a.DisplayName}
			}
		}
	} else {
		values, err := s.root.DraftActors(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range values {
			actors[a.ActorID] = domain.ActorRef{ID: a.ActorID, Label: a.DisplayName}
		}
	}
	return actors, nil
}

func (s *inventoryState) loadRows(ctx context.Context, keys map[inventoryEntity]bool) error {
	groups := map[inventoryGroup][]string{}
	for k := range keys {
		if _, ok := s.rows[k]; !ok {
			group := inventoryGroup{k.project, k.kind}
			groups[group] = append(groups[group], k.id)
		}
	}
	for group, ids := range groups {
		if len(s.rows)+len(ids) > 10000 {
			return fmt.Errorf("adoption entity budget exceeded")
		}
		bound, err := s.session(ctx, group.project)
		if err != nil {
			return err
		}
		rows, err := bound.InventoryRows(ctx, group.kind, ids)
		if err != nil {
			return err
		}
		for _, row := range rows {
			row = s.decorate(row, bound, group.kind)
			// Only direct consumer membership may link into the consumer view. A purely
			// transitive source dependency opens its explicitly pinned source context.
			consumer, err := s.resolve(ctx, s.root.ProjectID(), group.kind, row.ID)
			if err != nil {
				return err
			}
			if consumer.project != bound.ProjectID() {
				row.CurrentURL = row.SourceURL
			}
			s.rows[inventoryEntity{bound.ProjectID(), group.kind, row.ID}] = row
		}
	}
	return nil
}
func (s *inventoryState) loadEdges(ctx context.Context, keys map[inventoryEntity]bool) error {
	if err := s.loadRows(ctx, keys); err != nil {
		return err
	}
	groups := map[inventoryGroup][]string{}
	for k := range keys {
		if _, ok := s.edges[k]; !ok {
			group := inventoryGroup{k.project, k.kind}
			groups[group] = append(groups[group], k.id)
			s.edges[k] = nil
		}
	}
	for group, ids := range groups {
		bound, err := s.session(ctx, group.project)
		if err != nil {
			return err
		}
		rows, err := bound.InventoryRelations(ctx, group.kind, ids)
		if err != nil {
			return err
		}
		s.edgeCount += len(rows)
		if s.edgeCount > 100000 {
			return fmt.Errorf("adoption edge budget exceeded")
		}
		for _, r := range rows {
			key := inventoryEntity{bound.ProjectID(), r.EntityKind, r.EntityID}
			s.edges[key] = append(s.edges[key], r)
		}
	}
	return nil
}
func (s *inventoryState) closure(ctx context.Context, seed inventoryEntity) (map[string][]browserinput.InventoryRow, map[string][]browserinput.InventoryRow, error) {
	direct, all := map[inventoryEntity]bool{}, map[inventoryEntity]bool{seed: true}
	seen := map[inventoryEntity]bool{}
	pending := map[inventoryEntity]bool{seed: true}
	if seed.kind == inventoryField {
		direct[seed] = true
	}
	for len(pending) > 0 {
		if err := s.loadEdges(ctx, pending); err != nil {
			return nil, nil, err
		}
		next := map[inventoryEntity]bool{}
		for key := range pending {
			seen[key] = true
			for _, edge := range s.edges[key] {
				if edge.FieldID != "" {
					field, err := s.resolve(ctx, key.project, inventoryField, edge.FieldID)
					if err != nil {
						return nil, nil, err
					}
					all[field] = true
					if key == seed {
						direct[field] = true
					}
				}
				targetKind := refKind(edge.RefKind)
				if targetKind == "" || edge.RefID == "" {
					continue
				}
				target, err := s.resolve(ctx, key.project, targetKind, edge.RefID)
				if err != nil {
					return nil, nil, err
				}
				all[target] = true
				if key == seed {
					direct[target] = true
				}
				if !seen[target] {
					next[target] = true
				}
			}
		}
		for k := range seen {
			delete(next, k)
		}
		pending = next
		if len(all) > 10000 {
			return nil, nil, fmt.Errorf("adoption closure budget exceeded")
		}
	}
	if err := s.loadRows(ctx, all); err != nil {
		return nil, nil, err
	}
	project := func(keys map[inventoryEntity]bool) map[string][]browserinput.InventoryRow {
		out := map[string][]browserinput.InventoryRow{inventoryModel: {}, inventoryCollection: {}, inventoryField: {}}
		for key := range keys {
			out[key.kind] = append(out[key.kind], s.rows[key])
		}
		for kind := range out {
			slices.SortFunc(out[kind], func(a, b browserinput.InventoryRow) int {
				if cmp := strings.Compare(a.ProjectID, b.ProjectID); cmp != 0 {
					return cmp
				}
				return strings.Compare(a.ID, b.ID)
			})
		}
		return out
	}
	return project(direct), project(all), nil
}

func expandedAdoptionRows(item browserinput.Adoption) int {
	total := 0
	for _, rows := range item.Closure {
		total += len(rows)
	}
	for _, rows := range item.Direct {
		total += len(rows)
	}
	return total
}
