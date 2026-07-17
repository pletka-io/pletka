package namespacebinding

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// LangResolver returns the caller's preferred UI language for a request.
type LangResolver func(r *http.Request) string

// Handler exposes the namespace-binding JSON API.
type Handler struct {
	svc       *Service
	log       *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
}

// NewHandler constructs a Handler.
//   - log:       error logger; nil → slog.Default
//   - languages: available UI languages (used by schema endpoints)
//   - lang:      per-request language resolver; nil → constant "en"
func NewHandler(svc *Service, log *slog.Logger, languages []formschema.LanguageInfo, lang LangResolver) *Handler {
	if log == nil {
		log = slog.Default()
	}
	if lang == nil {
		lang = func(*http.Request) string { return "en" }
	}
	return &Handler{svc: svc, log: log, languages: languages, lang: lang}
}

// ---------------------------------------------------------------------------
// Request DTO
// ---------------------------------------------------------------------------

// bindingBody is the JSON shape accepted by Create and Update. All fields
// pointers so a missing field stays as the existing value on update.
type bindingBody struct {
	Prefix    *string `json:"prefix,omitempty"`
	Namespace *string `json:"namespace,omitempty"`
	Weight    any     `json:"weight,omitempty"`
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

// List handles GET /. Returns []bindingItem so system rows carry the
// _readonly flag the ListManager uses to hide row actions.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}

	bindings, err := h.svc.List(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(bindings))
	for _, b := range bindings {
		items = append(items, map[string]any{
			"id":        b.ID,
			"prefix":    b.Prefix,
			"namespace": b.Namespace,
			"weight":    b.Weight,
			"source":    b.Source,
			// ListManager reads _readonly per row to hide row actions on
			// system / imported rows. Only "user" rows are mutable.
			"_readonly": b.Source != "user",
		})
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) ListGlobal(w http.ResponseWriter, r *http.Request) {
	bindings, err := h.svc.ListGlobal(r.Context())
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(bindings))
	for _, b := range bindings {
		items = append(items, map[string]any{
			"id":        b.ID,
			"prefix":    b.Prefix,
			"namespace": b.Namespace,
			"weight":    b.Weight,
			"source":    b.Source,
			"_mutable":  b.Source == "user",
		})
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) ListGlobalData(w http.ResponseWriter, r *http.Request) {
	page := 1
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, "page must be a positive integer")
			return
		}
		page = parsed
	}
	perPage := 25
	if raw := strings.TrimSpace(r.URL.Query().Get("per_page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, "per_page must be a positive integer")
			return
		}
		perPage = parsed
	}

	result, err := h.svc.ListGlobalBrowse(r.Context(), GlobalListInput{
		Search:  r.URL.Query().Get("search"),
		Source:  r.URL.Query().Get("source"),
		SortBy:  r.URL.Query().Get("sort_by"),
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(result.Items))
	for _, b := range result.Items {
		items = append(items, map[string]any{
			"id":             b.ID,
			"prefix":         b.Prefix,
			"namespace":      b.Namespace,
			"weight":         b.Weight,
			"weight_display": fmt.Sprintf("w:%d", b.Weight),
			"source":         b.Source,
			"_mutable":       b.Source == "user",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": result.Total,
	})
}

// Get handles GET /{id}. Returns 404 / 403.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	id := chi.URLParam(r, "id")
	if projectID == "" || id == "" {
		writeError(w, http.StatusBadRequest, "project_id and id required")
		return
	}

	b, err := h.svc.Get(r.Context(), projectID, id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *Handler) GetGlobal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	b, err := h.svc.GetGlobal(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// Create handles POST /. Body fields: prefix, namespace, weight (default 10).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}

	body, err := decodeJSON[bindingBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := requireValidOptionalInt64(body.Weight, "weight"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	in := CreateInput{
		Prefix:    dbutil.NilToEmpty(body.Prefix),
		Namespace: dbutil.NilToEmpty(body.Namespace),
		Weight:    parseOptionalInt64(body.Weight),
	}

	b, err := h.svc.Create(r.Context(), projectID, in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (h *Handler) CreateGlobal(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[bindingBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := requireValidOptionalInt64(body.Weight, "weight"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	in := CreateInput{
		Prefix:    dbutil.NilToEmpty(body.Prefix),
		Namespace: dbutil.NilToEmpty(body.Namespace),
		Weight:    parseOptionalInt64(body.Weight),
	}
	b, err := h.svc.CreateGlobal(r.Context(), in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// Update handles PATCH /{id}. Partial update — fields absent stay as-is.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	id := chi.URLParam(r, "id")
	if projectID == "" || id == "" {
		writeError(w, http.StatusBadRequest, "project_id and id required")
		return
	}

	body, err := decodeJSON[bindingBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := requireValidOptionalInt64(body.Weight, "weight"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	in := UpdateInput{
		Prefix:    body.Prefix,
		Namespace: body.Namespace,
		Weight:    parseOptionalInt64Ptr(body.Weight),
	}

	b, err := h.svc.Update(r.Context(), projectID, id, in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *Handler) UpdateGlobal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	body, err := decodeJSON[bindingBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := requireValidOptionalInt64(body.Weight, "weight"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	in := UpdateInput{
		Prefix:    body.Prefix,
		Namespace: body.Namespace,
		Weight:    parseOptionalInt64Ptr(body.Weight),
	}
	b, err := h.svc.UpdateGlobal(r.Context(), id, in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// Delete handles DELETE /{id}. Returns 204 / 404 / 403 / 422 (read-only).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	id := chi.URLParam(r, "id")
	if projectID == "" || id == "" {
		writeError(w, http.StatusBadRequest, "project_id and id required")
		return
	}

	if err := h.svc.Delete(r.Context(), projectID, id); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteGlobal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	if err := h.svc.DeleteGlobal(r.Context(), id); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) StatsGlobal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	report, err := h.svc.StatsGlobal(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// ---------------------------------------------------------------------------
// Schema endpoints
// ---------------------------------------------------------------------------

// ListSchema handles GET /list-schema.
func (h *Handler) ListSchema(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}
	writeJSON(w, http.StatusOK, BuildListSchema(projectID, h.lang(r), h.languages))
}

func (h *Handler) ListSchemaGlobal(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, BuildGlobalListSchema(h.lang(r), h.languages))
}

func (h *Handler) EntityListSchemaGlobal(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, BuildGlobalEntityListSchema(h.lang(r), h.languages))
}

// FormSchemaCreate handles GET /form-schema.
func (h *Handler) FormSchemaCreate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}
	writeJSON(w, http.StatusOK, BuildCreateForm(projectID, h.lang(r), h.languages))
}

func (h *Handler) FormSchemaCreateGlobal(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, BuildGlobalCreateForm(h.lang(r), h.languages))
}

// FormSchemaEdit handles GET /{id}/form-schema. Loads the row and renders
// the edit form. Returns 404 / 403 / 422 (read-only).
func (h *Handler) FormSchemaEdit(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	id := chi.URLParam(r, "id")
	if projectID == "" || id == "" {
		writeError(w, http.StatusBadRequest, "project_id and id required")
		return
	}

	b, err := h.svc.Get(r.Context(), projectID, id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, BuildEditForm(projectID, b, h.lang(r), h.languages))
}

func (h *Handler) FormSchemaEditGlobal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	b, err := h.svc.GetGlobal(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, BuildGlobalEditForm(b, h.lang(r), h.languages))
}

// ---------------------------------------------------------------------------
// Error mapping
// ---------------------------------------------------------------------------

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrReadOnly) {
		apierror.Write(w, apierror.Validation(map[string][]string{
			"_": {"this binding is read-only and cannot be modified"},
		}))
		return
	}
	if IsNotFound(err) {
		apierror.Write(w, apierror.NotFound(err.Error()))
		return
	}
	ae := apierror.FromError(err)
	if ae.Code == apierror.CodeInternal {
		h.log.Error("namespace binding handler error", "err", err)
	}
	apierror.Write(w, ae)
}

// ---------------------------------------------------------------------------
// Local HTTP helpers (slice-private, mirrors the Category slice)
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError forwards to apierror.Write — see pkg/weave/apierror.
func writeError(w http.ResponseWriter, status int, msg string) {
	apierror.Write(w, &apierror.Error{Status: status, Message: msg})
}

func decodeJSON[T any](r *http.Request) (T, error) {
	var v T
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, err
	}
	return v, nil
}

func parseOptionalInt64(v any) int64 {
	p := parseOptionalInt64Ptr(v)
	if p == nil {
		return 0
	}
	return *p
}

func parseOptionalInt64Ptr(v any) *int64 {
	if v == nil {
		return nil
	}
	switch n := v.(type) {
	case float64:
		out := int64(n)
		return &out
	case string:
		if n == "" {
			return nil
		}
		out, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return nil
		}
		return &out
	case json.Number:
		out, err := n.Int64()
		if err != nil {
			return nil
		}
		return &out
	default:
		return nil
	}
}

func requireValidOptionalInt64(v any, field string) error {
	if v == nil {
		return nil
	}
	if parseOptionalInt64Ptr(v) == nil {
		return fmt.Errorf("%s must be a number", field)
	}
	return nil
}
