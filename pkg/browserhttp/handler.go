// Package browserhttp serves the shared, scope-bound schema UI inventories.
package browserhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/publication"
)

const (
	catalogConceptList = "concept_list"
	catalogExample     = "example"
	catalogVocabulary  = "vocabulary"
	sortUpdatedAt      = "updated_at"
)

// Source must finish its scoped reads before returning display values.
type Source interface {
	Inventory(context.Context, auth.ReadScope, string, string, browserinput.InventoryQuery) (browserinput.Inventory, error)
}

// Handler has no entity store or fallback path. Current publication badges are
// operational draft metadata; released rows never call the publication reader.
type Handler struct {
	Source      Source
	I18n        i18n.Manager
	Lang        func(*http.Request) string
	Publication *publication.Reader
}

func (h Handler) load(r *http.Request, kind string, facets bool) (browserinput.Inventory, error) {
	project := chi.URLParam(r, "projectID")
	scope := auth.ReadScopeFromContext(r.Context())
	resource := auth.ProjectResourceFromContext(r.Context())
	if project == "" || resource.ID != project || !auth.FromContext(r.Context()).Can(auth.ProjectRead, resource, nil) {
		return browserinput.Inventory{}, apierror.NotFound("project not found")
	}
	if h.Source == nil || !scope.Valid() {
		return browserinput.Inventory{}, fmt.Errorf("inventory requires a source and explicit scope")
	}
	q := browserinput.InventoryQuery{Page: 1, PerPage: 50, SortBy: "ui_name"}
	if !facets {
		query := r.URL.Query()
		for _, item := range []struct {
			name  string
			value *int
		}{{"page", &q.Page}, {"per_page", &q.PerPage}} {
			if raw := query.Get(item.name); raw != "" {
				v, err := strconv.Atoi(raw)
				if err != nil || v < 1 {
					return browserinput.Inventory{}, apierror.BadRequest("invalid pagination")
				}
				*item.value = v
			}
		}
		q.PerPage = min(q.PerPage, 1000)
		if q.Page > 20000/q.PerPage {
			return browserinput.Inventory{}, apierror.BadRequest("requested page exceeds inventory merge window")
		}
		q.Search = query.Get("search")
		q.Origin = query.Get("origin_kind")
		q.Scope = query.Get("scope_class")
		if kind == "field" {
			q.Scope = query.Get("ontology_scope")
		}
		q.Category = query.Get("category_id")
		q.ModelType = query.Get("model_type")
		q.Status = normalizeCatalogCSV(query.Get("status"))
		q.VocabularyID = normalizeCatalogCSV(query.Get("vocabulary_id"))
		q.EntityType = query.Get("entity_type")
		q.EntityID = query.Get("entity_id")
		q.Search = strings.TrimSpace(q.Search)
		q.Descending = query.Get("sort_dir") == "desc"
		if sort := query.Get("sort_by"); sort != "" {
			q.SortBy = sort
		}
		sorts := []string{"ui_name", "name", "scope"}
		if kind == catalogConceptList {
			sorts = []string{"ui_name", "semantic_id", "system_name", catalogVocabulary, "entries", "bound_fields", "status"}
		}
		if kind == catalogExample {
			sorts = []string{"ui_name", sortUpdatedAt, "created_at", "status"}
			if query.Get("sort_by") == "" {
				q.SortBy = sortUpdatedAt
			}
			if query.Get("sort_dir") == "" && (q.SortBy == sortUpdatedAt || q.SortBy == "created_at") {
				q.Descending = true
			}
		}
		if !slices.Contains(sorts, q.SortBy) {
			return browserinput.Inventory{}, apierror.BadRequest("unsupported inventory sort")
		}
	} else {
		q.PerPage = 1
	}
	value, err := h.Source.Inventory(r.Context(), scope, project, kind, q)
	if err != nil {
		return browserinput.Inventory{}, err
	}
	if value.ProjectID != project || value.Version != scope.Version() || value.Kind != kind {
		return browserinput.Inventory{}, fmt.Errorf("inventory returned a different selection")
	}
	return value, nil
}

// List preserves the existing entity-list response envelope.
func (h Handler) List(w http.ResponseWriter, r *http.Request, kind string) {
	value, err := h.load(r, kind, false)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	// Live change badges are deliberately separate from captured display data.
	if !auth.ReadScopeFromContext(r.Context()).IsRelease() && h.Publication != nil && kind != catalogConceptList && kind != catalogExample {
		var ids []string
		for _, row := range value.Items {
			if row.Owned {
				ids = append(ids, row.ID)
			}
		}
		if len(ids) > 0 {
			states, err := h.Publication.BatchState(r.Context(), value.ProjectID, kind, ids)
			if err != nil {
				apierror.Write(w, r, apierror.FromError(err))
				return
			}
			for i := range value.Items {
				if state, ok := states[value.Items[i].ID]; ok {
					value.Items[i].Status = string(state)
				}
			}
		}
	}
	lang := "en"
	if h.Lang != nil {
		lang = h.Lang(r)
	}
	for i := range value.Items {
		row := &value.Items[i]
		if kind == catalogExample {
			row.EntityName = row.EntityNames.Get(lang, row.EntityName)
		}
	}
	rows := projectRows(value)
	if h.I18n != nil {
		h.I18n.Resolve(&rows, lang)
	}
	body := map[string]any{kind + "s": rows, "total": value.Total, "page": value.Page, "per_page": value.PerPage}
	if kind == catalogExample || kind == catalogConceptList {
		body["items"] = rows
	}
	write(w, body)
}

// Filters uses the same inventory membership as the list, including adopted rows.
func (h Handler) Filters(w http.ResponseWriter, r *http.Request, kind, facet string) {
	value, err := h.load(r, kind, true)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	options := []map[string]any{}
	if facet == "scope" {
		for _, scope := range value.Scopes {
			options = append(options, map[string]any{"value": scope, "label": map[string]string{"en": scope}})
		}
	} else {
		labels := value.Categories
		if facet == catalogVocabulary {
			labels = value.Vocabularies
		}
		ids := make([]string, 0, len(labels))
		for id := range labels {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		if facet == catalogVocabulary {
			slices.SortFunc(ids, func(a, b string) int {
				if order := strings.Compare(labels[a].Get("en", a), labels[b].Get("en", b)); order != 0 {
					return order
				}
				return strings.Compare(a, b)
			})
		}
		for _, id := range ids {
			options = append(options, map[string]any{"value": id, "label": labels[id]})
		}
	}
	write(w, map[string]any{"options": options})
}
func write(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func normalizeCatalogCSV(raw string) string {
	parts := []string{}
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, ",")
}
