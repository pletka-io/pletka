package readsession

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/database/readsql/archive"
	"github.com/pletka-io/pletka/pkg/database/readsql/draft"
	"github.com/pletka-io/pletka/pkg/domain"
)

// InventoryKey carries only a stable merge key, never entity bodies.
type InventoryKey struct {
	ID      string `json:"id"`
	SortKey string `json:"sort_key"`
}

// InventoryIndex is a bounded sorted prefix plus the exact total and facets.
type InventoryIndex struct {
	Items              []InventoryKey
	Total, Available   int64
	Scopes, Categories []string
}

// InventoryRelation retains structural edges without loading field bodies.
type InventoryRelation struct{ EntityKind, EntityID, FieldID, CategoryID, CollectionID, RefKind, RefID string }

// InventoryIndex reads candidates in this session only. Nil ids selects owned rows;
// a nonnil list selects exactly the foreign membership chosen by the caller.
func (s *Session) InventoryIndex(ctx context.Context, kind string, ids []string, origins, categories map[string]string, query browserinput.InventoryQuery, limit int) (InventoryIndex, error) {
	var out InventoryIndex
	if err := s.Check(ctx); err != nil {
		return out, err
	}
	if limit < 0 || limit > 20000 {
		return out, fmt.Errorf("inventory page exceeds merge budget")
	}
	if origins == nil {
		origins = map[string]string{}
	}
	if categories == nil {
		categories = map[string]string{}
	}
	oj, err := json.Marshal(origins)
	if err != nil {
		return out, err
	}
	cj, err := json.Marshal(categories)
	if err != nil {
		return out, err
	}
	var items, scopes, cats []byte
	if s.Scope().IsRelease() {
		q, err := s.ArchiveQueries()
		if err != nil {
			return out, err
		}
		r, err := q.BrowserInventoryIndex(ctx, archive.BrowserInventoryIndexParams{ProjectID: s.ProjectID(), Version: s.Scope().Version(), Kind: kind, Ids: ids, Owned: ids == nil, Origins: oj, Categories: cj, SortBy: query.SortBy, Search: query.Search, OriginFilter: query.Origin, ScopeFilter: query.Scope, CategoryFilter: query.Category, ModelFilter: query.ModelType, StatusFilter: query.Status, Descending: query.Descending, RowLimit: int32(limit)})
		if err != nil {
			return out, err
		}
		out.Total = r.Total
		out.Available = r.Available
		items = r.Items
		scopes = r.Scopes
		cats = r.Categories
	} else {
		q, err := s.DraftQueries()
		if err != nil {
			return out, err
		}
		r, err := q.BrowserInventoryIndex(ctx, draft.BrowserInventoryIndexParams{ProjectID: s.ProjectID(), Kind: kind, Ids: ids, Owned: ids == nil, Origins: oj, Categories: cj, SortBy: query.SortBy, Search: query.Search, OriginFilter: query.Origin, ScopeFilter: query.Scope, CategoryFilter: query.Category, ModelFilter: query.ModelType, StatusFilter: query.Status, Descending: query.Descending, RowLimit: int32(limit)})
		if err != nil {
			return out, err
		}
		out.Total = r.Total
		out.Available = r.Available
		items = r.Items
		scopes = r.Scopes
		cats = r.Categories
	}
	for _, part := range []struct {
		data  []byte
		value any
	}{{items, &out.Items}, {scopes, &out.Scopes}, {cats, &out.Categories}} {
		if err := json.Unmarshal(part.data, part.value); err != nil {
			return InventoryIndex{}, err
		}
	}
	if len(out.Scopes) > 1000 || len(out.Categories) > 1000 {
		return InventoryIndex{}, fmt.Errorf("inventory facet budget exceeded")
	}
	return out, nil
}

// InventoryRows loads detached values inside this selected project.
func (s *Session) InventoryRows(ctx context.Context, kind string, ids []string) ([]browserinput.InventoryRow, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	out := make([]browserinput.InventoryRow, 0)
	if s.Scope().IsRelease() {
		q, err := s.ArchiveQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserInventoryRows(ctx, archive.BrowserInventoryRowsParams{ProjectID: s.ProjectID(), Version: s.Scope().Version(), Kind: kind, Ids: ids})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			var payload struct {
				browserinput.InventoryRow
				Scope domain.PathElement `json:"scope_definition"`
			}
			if err := json.Unmarshal(r.Body, &payload); err != nil {
				return nil, err
			}
			v := payload.InventoryRow
			v.ScopeClassCode = payload.Scope.ShortCode()
			v.OntologyScope = payload.Scope.PrefixedName()
			out = append(out, v)
		}
	} else {
		q, err := s.DraftQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserInventoryRows(ctx, draft.BrowserInventoryRowsParams{ProjectID: s.ProjectID(), Kind: kind, Ids: ids})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			var payload struct {
				browserinput.InventoryRow
				Scope domain.PathElement `json:"scope_definition"`
			}
			if err := json.Unmarshal(r.Body, &payload); err != nil {
				return nil, err
			}
			v := payload.InventoryRow
			v.ScopeClassCode = payload.Scope.ShortCode()
			v.OntologyScope = payload.Scope.PrefixedName()
			out = append(out, v)
		}
	}
	if len(out) != len(ids) {
		return nil, &NotInReleaseError{ProjectID: s.ProjectID(), Version: s.Scope().Version(), EntityID: "inventory member"}
	}
	return out, nil
}

// InventoryRelations loads detached values inside this selected project.
func (s *Session) InventoryRelations(ctx context.Context, kind string, ids []string) ([]InventoryRelation, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	out := make([]InventoryRelation, 0)
	if s.Scope().IsRelease() {
		q, err := s.ArchiveQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserInventoryRelations(ctx, archive.BrowserInventoryRelationsParams{ProjectID: s.ProjectID(), Version: s.Scope().Version(), Kind: kind, Ids: ids, AllEntities: ids == nil})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, InventoryRelation{r.EntityKind, r.EntityID, r.FieldID, r.CategoryID, r.CollectionID, r.RefKind, r.RefID})
		}
	} else {
		q, err := s.DraftQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserInventoryRelations(ctx, draft.BrowserInventoryRelationsParams{ProjectID: s.ProjectID(), Kind: kind, Ids: ids, AllEntities: ids == nil})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, InventoryRelation{r.EntityKind, r.EntityID, r.FieldID, r.CategoryID, r.CollectionID, r.RefKind, r.RefID})
		}
	}
	if len(out) > 100000 {
		return nil, fmt.Errorf("inventory relation budget exceeded")
	}
	return out, nil
}

// InventoryCategories loads detached values inside this selected project.
func (s *Session) InventoryCategories(ctx context.Context, ids []string) (map[string]domain.Translations, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	out := make(map[string]domain.Translations)
	if s.Scope().IsRelease() {
		q, err := s.ArchiveQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserInventoryCategories(ctx, archive.BrowserInventoryCategoriesParams{ProjectID: s.ProjectID(), Version: s.Scope().Version(), Ids: ids})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			var v domain.Translations
			if len(r.UiName) > 0 {
				if err := json.Unmarshal(r.UiName, &v); err != nil {
					return nil, err
				}
			}
			out[r.ID] = v
		}
	} else {
		q, err := s.DraftQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserInventoryCategories(ctx, draft.BrowserInventoryCategoriesParams{ProjectID: s.ProjectID(), Ids: ids})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			var v domain.Translations
			if len(r.UiName) > 0 {
				if err := json.Unmarshal(r.UiName, &v); err != nil {
					return nil, err
				}
			}
			out[r.ID] = v
		}
	}
	return out, nil
}
