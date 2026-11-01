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

// CatalogReference is a compact target whose owner is chosen by session membership.
type CatalogReference struct{ Kind, ID string }

// CatalogLabel holds display metadata only, never a vocabulary's contents/configuration.
type CatalogLabel struct {
	Labels domain.Translations `json:"labels"`
	Name   string              `json:"name"`
	URI    string              `json:"uri"`
}

// CatalogReferences lists bounded label targets of this project’s owned inventory.
func (s *Session) CatalogReferences(ctx context.Context, kind string) ([]CatalogReference, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	out := []CatalogReference{}
	if s.Scope().IsRelease() {
		q, err := s.ArchiveQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserCatalogReferences(ctx, archive.BrowserCatalogReferencesParams{ProjectID: s.ProjectID(), Kind: kind, Version: s.Scope().Version()})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if r.ID == nil {
				return nil, fmt.Errorf("catalog reference has no identity")
			}
			out = append(out, CatalogReference{r.Kind, *r.ID})
		}
	} else {
		q, err := s.DraftQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserCatalogReferences(ctx, draft.BrowserCatalogReferencesParams{ProjectID: s.ProjectID(), Kind: kind})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if r.ID == nil {
				return nil, fmt.Errorf("catalog reference has no identity")
			}
			out = append(out, CatalogReference{r.Kind, *r.ID})
		}
	}
	if len(out) > 10000 {
		return nil, fmt.Errorf("catalog reference budget exceeded")
	}
	return out, nil
}

// CatalogLabels loads exactly the requested labels in this session’s project and state.
func (s *Session) CatalogLabels(ctx context.Context, kind string, ids []string) (map[string]CatalogLabel, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	if len(ids) > 10000 {
		return nil, fmt.Errorf("catalog label budget exceeded")
	}
	out := map[string]CatalogLabel{}
	add := func(id string, raw []byte, fallback, uri string) error {
		var labels domain.Translations
		if err := json.Unmarshal(raw, &labels); err != nil {
			return err
		}
		out[id] = CatalogLabel{Labels: labels, Name: labels.Get("en", fallback), URI: uri}
		return nil
	}
	if s.Scope().IsRelease() {
		q, err := s.ArchiveQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserCatalogLabels(ctx, archive.BrowserCatalogLabelsParams{ProjectID: s.ProjectID(), Kind: kind, Ids: ids, Version: s.Scope().Version()})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if err := add(r.ID, r.Label, r.Fallback, r.Uri); err != nil {
				return nil, err
			}
		}
	} else {
		q, err := s.DraftQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserCatalogLabels(ctx, draft.BrowserCatalogLabelsParams{ProjectID: s.ProjectID(), Kind: kind, Ids: ids})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if err := add(r.ID, r.Label, r.Fallback, r.Uri); err != nil {
				return nil, err
			}
		}
	}
	if len(out) != len(ids) {
		return nil, &NotInReleaseError{ProjectID: s.ProjectID(), Version: s.Scope().Version(), EntityID: "catalog label"}
	}
	return out, nil
}

// CatalogPage filters and pages owned metadata with already resolved labels.
func (s *Session) CatalogPage(ctx context.Context, kind string, labels map[string]CatalogLabel, query browserinput.InventoryQuery) (browserinput.Inventory, error) {
	out := browserinput.Inventory{ProjectID: s.ProjectID(), Version: s.Scope().Version(), Kind: kind, Page: query.Page, PerPage: query.PerPage, Vocabularies: map[string]domain.Translations{}}
	if err := s.Check(ctx); err != nil {
		return out, err
	}
	if query.Page < 1 || query.PerPage < 1 || query.PerPage > 1000 || query.Page > 20000/query.PerPage {
		return out, fmt.Errorf("catalog page exceeds bounds")
	}
	raw, err := json.Marshal(labels)
	if err != nil {
		return out, err
	}
	var items, vocabularies []byte
	if s.Scope().IsRelease() {
		q, err := s.ArchiveQueries()
		if err != nil {
			return out, err
		}
		r, err := q.BrowserCatalogPage(ctx, archive.BrowserCatalogPageParams{ProjectID: s.ProjectID(), Kind: kind, Labels: raw, Search: query.Search, StatusFilter: query.Status, VocabularyFilter: query.VocabularyID, EntityType: query.EntityType, EntityID: query.EntityID, SortBy: query.SortBy, Descending: query.Descending, PageLimit: int32(query.PerPage), PageOffset: int64((query.Page - 1) * query.PerPage), Version: s.Scope().Version()})
		if err != nil {
			return out, err
		}
		out.Total, items, vocabularies = r.Total, r.Items, r.Vocabularies
	} else {
		q, err := s.DraftQueries()
		if err != nil {
			return out, err
		}
		r, err := q.BrowserCatalogPage(ctx, draft.BrowserCatalogPageParams{ProjectID: s.ProjectID(), Kind: kind, Labels: raw, Search: query.Search, StatusFilter: query.Status, VocabularyFilter: query.VocabularyID, EntityType: query.EntityType, EntityID: query.EntityID, SortBy: query.SortBy, Descending: query.Descending, PageLimit: int32(query.PerPage), PageOffset: int64((query.Page - 1) * query.PerPage)})
		if err != nil {
			return out, err
		}
		out.Total, items, vocabularies = r.Total, r.Items, r.Vocabularies
	}
	if err := json.Unmarshal(items, &out.Items); err != nil {
		return out, err
	}
	var facets []struct {
		ID    string `json:"vocabulary_id"`
		Label string `json:"vocabulary_label"`
	}
	if err := json.Unmarshal(vocabularies, &facets); err != nil {
		return out, err
	}
	if len(facets) > 1000 {
		return out, fmt.Errorf("catalog facet budget exceeded")
	}
	for _, v := range facets {
		out.Vocabularies[v.ID] = domain.Translations{"en": v.Label}
	}
	return out, nil
}
