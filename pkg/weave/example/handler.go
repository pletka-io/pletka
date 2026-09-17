package example

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// wantsHTML reports whether the request comes from a browser navigation
// (an Accept header that names text/html). API clients send */* or
// application/json and keep getting JSON.
func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// projectExamplesPageURL is the browser-facing home of an example: the
// project page's Examples tab with the item open. The frontend reads the
// hash (see project-detail.svelte.ts readHashItem); the same shape is
// published to the editor as page_url_template in the entity-list schema.
func projectExamplesPageURL(projectID, exampleID string) string {
	return fmt.Sprintf("/projects/%s#tab=examples&item=%s", projectID, exampleID)
}

type LangResolver func(r *http.Request) string

type Handler struct {
	svc       *Service
	log       *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
}

func NewHandler(svc *Service, log *slog.Logger, languages []formschema.LanguageInfo, lang LangResolver) *Handler {
	if log == nil {
		log = slog.Default()
	}
	if lang == nil {
		lang = func(*http.Request) string { return "en" }
	}
	return &Handler{svc: svc, log: log, languages: languages, lang: lang}
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/form-schema", h.FormSchema)
	r.Route("/{exampleID}", func(r chi.Router) {
		r.Get("/", h.Detail)
		r.Put("/", h.Update)
		r.Delete("/", h.Delete)
	})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	page := 1
	perPage := 25
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && v > 0 {
		perPage = v
	}
	opts := []domain.QueryOption{
		domain.WithLimit(perPage),
		domain.WithOffset((page - 1) * perPage),
	}
	if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
		opts = append(opts, domain.WithOrderBy(sortBy, r.URL.Query().Get("sort_dir") == "desc"))
	}
	if search := r.URL.Query().Get("search"); search != "" {
		opts = append(opts, domain.WithSearch(search))
	}
	if entityType := r.URL.Query().Get("entity_type"); entityType != "" {
		opts = append(opts, domain.WithFilter("entity_type", entityType))
	}
	if entityID := r.URL.Query().Get("entity_id"); entityID != "" {
		opts = append(opts, domain.WithFilter("entity_id", entityID))
	}
	if status := r.URL.Query().Get("status"); status != "" {
		opts = append(opts, domain.WithFilter("status", status))
	}
	items, total, err := h.svc.List(r.Context(), projectID, opts...)
	if err != nil {
		h.log.Error("example list failed", "project_id", projectID, "err", err)
		apierror.Write(w, apierror.Internal())
		return
	}
	type item struct {
		ID              string              `json:"id"`
		ProjectID       string              `json:"project_id"`
		EntityType      string              `json:"entity_type"`
		EntityTypeLabel string              `json:"entity_type_label"`
		EntityID        string              `json:"entity_id"`
		Title           domain.Translations `json:"title,omitempty"`
		Description     domain.Translations `json:"description,omitempty"`
		Status          string              `json:"status"`
		UpdatedAt       any                 `json:"updated_at,omitempty"`
		CreatedAt       any                 `json:"created_at,omitempty"`
	}
	rows := make([]item, 0, len(items))
	for _, ex := range items {
		label := "Model"
		if ex.EntityType == domain.ExampleEntityTypeCollection {
			label = "Collection"
		}
		rows = append(rows, item{
			ID:              ex.ID,
			ProjectID:       ex.ProjectID,
			EntityType:      string(ex.EntityType),
			EntityTypeLabel: label,
			EntityID:        ex.EntityID,
			Title:           ex.Title,
			Description:     ex.Description,
			Status:          string(ex.Status),
			UpdatedAt:       ex.UpdatedAt,
			CreatedAt:       ex.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    rows,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	var in CreateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid json"))
		return
	}
	record, err := h.svc.Create(r.Context(), projectID, in)
	if err != nil {
		apierror.Write(w, apierror.BadRequest(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	exampleID := chi.URLParam(r, "exampleID")
	record, err := h.svc.Get(r.Context(), projectID, exampleID)
	if err != nil {
		apierror.Write(w, apierror.NotFound(err.Error()))
		return
	}
	if wantsHTML(r) {
		http.Redirect(w, r, projectExamplesPageURL(projectID, record.Example.ID), http.StatusFound)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	exampleID := chi.URLParam(r, "exampleID")
	var in UpdateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid json"))
		return
	}
	record, err := h.svc.Update(r.Context(), projectID, exampleID, in)
	if err != nil {
		apierror.Write(w, apierror.BadRequest(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	exampleID := chi.URLParam(r, "exampleID")
	if err := h.svc.Delete(r.Context(), projectID, exampleID); err != nil {
		apierror.Write(w, apierror.NotFound(err.Error()))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) FormSchema(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	mode := r.URL.Query().Get("mode")
	targetType := r.URL.Query().Get("target_type")
	targetID := r.URL.Query().Get("target_id")
	exampleID := r.URL.Query().Get("example_id")
	schema, err := h.svc.BuildFormSchema(r.Context(), projectID, mode, targetType, targetID, exampleID, h.lang(r), h.languages)
	if err != nil {
		apierror.Write(w, apierror.BadRequest(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, schema)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
