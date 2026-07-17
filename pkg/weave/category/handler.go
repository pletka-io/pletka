package category

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// LangResolver returns the caller's preferred UI language for a request.
// Typically reads from session or Accept-Language. Pass any function that
// matches; a reasonable default is implemented in the router wiring.
type LangResolver func(r *http.Request) string

// OriginResolver resolves provenance labels for categories without requiring
// the handler to depend on the broad domain.WeaveStore aggregate.
type OriginResolver interface {
	CategoryOriginsBySystemName(ctx context.Context, projectID string, categories []*domain.Category) (map[string]domain.Origin, error)
}

// OriginResolverFunc adapts a function into an OriginResolver.
type OriginResolverFunc func(context.Context, string, []*domain.Category) (map[string]domain.Origin, error)

// CategoryOriginsBySystemName implements OriginResolver.
func (f OriginResolverFunc) CategoryOriginsBySystemName(ctx context.Context, projectID string, categories []*domain.Category) (map[string]domain.Origin, error) {
	return f(ctx, projectID, categories)
}

// Handler wraps the Service and exposes JSON-API endpoints. HTML page
// rendering for the category section is the responsibility of a separate
// page handler (svelte islands consume the JSON endpoints below). Methods
// accept and emit JSON only; content-negotiation for dual-purpose GETs
// would slot in here when templates are wired.
type Handler struct {
	svc       *Service
	origins   OriginResolver
	log       *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
}

// NewHandler constructs a Handler over the given Service.
//   - svc:       business logic
//   - log:       error reporting; pass nil for slog.Default
//   - languages: available UI languages (used by schema endpoints)
//   - lang:      resolves a request's preferred language; pass nil to default to "en"
func NewHandler(svc *Service, origins OriginResolver, log *slog.Logger, languages []formschema.LanguageInfo, lang LangResolver) *Handler {
	if log == nil {
		log = slog.Default()
	}
	if lang == nil {
		lang = func(*http.Request) string { return "en" }
	}
	return &Handler{svc: svc, origins: origins, log: log, languages: languages, lang: lang}
}

// ---------------------------------------------------------------------------
// Request / response DTOs
// ---------------------------------------------------------------------------

// categoryBody is the JSON shape accepted by Create and Update. Optional
// fields are pointers so handlers can distinguish "absent" from "set to
// zero value" — required for partial updates.
type categoryBody struct {
	SystemName     *string              `json:"system_name,omitempty"`
	UIName         *domain.Translations `json:"ui_name,omitempty"`
	Description    *domain.Translations `json:"description,omitempty"`
	CanonicalOrder *int                 `json:"canonical_order,omitempty"`
	Status         *domain.Status       `json:"status,omitempty"`
}

// reorderBody is the JSON shape for PATCH /reorder.
//
// Field name is `category_ids` to match the schema's
// `OrderField: "category_ids"` (see weave/category/formschema.go).
// ListManager builds the body as `{ [cap.order_field]: ids }`, so the
// schema field name and this struct tag must agree — a mismatch silently
// breaks reordering.
type reorderBody struct {
	CategoryIDs []string `json:"category_ids"`
}

// deleteBody is the JSON shape for DELETE /{id}. Empty body is fine; the
// reassign_fields_to field switches to the cascade delete path. The key
// matches what the shared delete components (ListManager, EntityListView)
// POST — they reassign the deleted category's fields to another category.
type deleteBody struct {
	ReassignTo string `json:"reassign_fields_to,omitempty"`
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

// List handles GET /. Returns []WithCounts (categories with usage counts +
// in_use boolean for UI delete-button gating).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}

	rows, err := h.svc.ListWithCounts(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	if err := h.applyOriginsToCategoryRows(r.Context(), projectID, rows); err != nil {
		h.log.Warn("enrich category origins", "project_id", projectID, "err", err)
	}

	writeJSON(w, http.StatusOK, rows)
}

// Get handles GET /{id}. Returns the single category or 404.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	id := chi.URLParam(r, "id")
	if projectID == "" || id == "" {
		writeError(w, http.StatusBadRequest, "project_id and id required")
		return
	}

	cat, err := h.svc.Get(r.Context(), projectID, id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	if cat == nil {
		writeError(w, http.StatusNotFound, "category not found")
		return
	}
	if err := h.applyOriginToCategory(r.Context(), projectID, cat); err != nil {
		h.log.Warn("enrich category origin", "project_id", projectID, "category_id", id, "err", err)
	}

	writeJSON(w, http.StatusOK, cat)
}

// Create handles POST /. Body is JSON-encoded categoryBody. Returns 201
// with the created row.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}

	body, err := decodeJSON[categoryBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	in := CreateInput{
		SystemName:     dbutil.NilToEmpty(body.SystemName),
		UIName:         derefTranslations(body.UIName),
		Description:    derefTranslations(body.Description),
		CanonicalOrder: dbutil.Deref(body.CanonicalOrder),
	}
	if body.Status != nil {
		in.Status = *body.Status
	}

	cat, err := h.svc.Create(r.Context(), projectID, in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, cat)
}

// Update handles PUT /{id}. Partial update: fields absent from the JSON
// body are not modified. Returns 200 with the post-update row.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	id := chi.URLParam(r, "id")
	if projectID == "" || id == "" {
		writeError(w, http.StatusBadRequest, "project_id and id required")
		return
	}

	body, err := decodeJSON[categoryBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	in := UpdateInput{
		UIName:         body.UIName,
		Description:    body.Description,
		SystemName:     body.SystemName,
		CanonicalOrder: body.CanonicalOrder,
		Status:         body.Status,
	}

	cat, err := h.svc.Update(r.Context(), projectID, id, in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, cat)
}

// Delete handles DELETE /{id}. Body may include reassign_to to switch to
// the cascade path. Returns 204 on success, 409 with usage payload when
// the category is in use and no reassignment was provided.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	id := chi.URLParam(r, "id")
	if projectID == "" || id == "" {
		writeError(w, http.StatusBadRequest, "project_id and id required")
		return
	}

	// Body is optional — empty body means "plain delete" with the in-use
	// preflight enabled.
	var body deleteBody
	if r.ContentLength > 0 {
		decoded, err := decodeJSON[deleteBody](r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		body = decoded
	}

	err := h.svc.Delete(r.Context(), projectID, id, DeleteOpts{ReassignTo: body.ReassignTo})
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Reorder handles POST /reorder. Body is {"ids": ["id1", "id2", ...]}.
// Each ID's canonical_order is rewritten to its 1-based position.
func (h *Handler) Reorder(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}

	body, err := decodeJSON[reorderBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.svc.Reorder(r.Context(), projectID, body.CategoryIDs); err != nil {
		h.writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Deprecate handles POST /{id}/deprecate. Returns 204.
func (h *Handler) Deprecate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	id := chi.URLParam(r, "id")
	if projectID == "" || id == "" {
		writeError(w, http.StatusBadRequest, "project_id and id required")
		return
	}

	if err := h.svc.Deprecate(r.Context(), projectID, id); err != nil {
		h.writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Activate handles POST /{id}/activate. Returns 204.
func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	id := chi.URLParam(r, "id")
	if projectID == "" || id == "" {
		writeError(w, http.StatusBadRequest, "project_id and id required")
		return
	}

	if err := h.svc.Activate(r.Context(), projectID, id); err != nil {
		h.writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Auxiliary read endpoints
// ---------------------------------------------------------------------------

// Options handles GET /options. Returns []formschema.SelectOption — the
// shape consumed by select widgets (autocomplete-style category pickers).
// Each option uses the semantic_id when present, falling back to ULID.
func (h *Handler) Options(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}

	cats, err := h.svc.List(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	// Stable foreign key for the option value. The override editor stores
	// `field.category_id` and groups by it; that has to match what's
	// already persisted on existing override rows. We always use c.ID
	// (the row PK) to keep round-trips consistent — semantic_id-as-value
	// would diverge from rows whose category_id was written before the
	// semantic_id was allocated.
	opts := make([]formschema.SelectOption, 0, len(cats))
	for _, c := range cats {
		// Fall back to system_name when UIName is empty — the dropdown
		// must always have something readable. SemanticID rides along
		// as a separate field so the frontend can render a badge.
		label := c.UIName
		if label == nil || label.Get("en", "") == "" {
			if c.SystemName != "" {
				label = domain.Translations{"en": c.SystemName}
			}
		}
		opts = append(opts, formschema.SelectOption{
			Value:      c.ID,
			Label:      label,
			SemanticID: c.SemanticID,
			Status:     string(c.Status),
		})
	}
	writeJSON(w, http.StatusOK, opts)
}

// Stats handles GET /{id}/stats. Returns a StatsReport with the category
// + override usage for the "category usage" modal.
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	id := chi.URLParam(r, "id")
	if projectID == "" || id == "" {
		writeError(w, http.StatusBadRequest, "project_id and id required")
		return
	}

	report, err := h.svc.Stats(r.Context(), projectID, id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	if report != nil && report.Category != nil {
		if err := h.applyOriginToCategory(r.Context(), projectID, report.Category); err != nil {
			h.log.Warn("enrich category stats origin", "project_id", projectID, "category_id", id, "err", err)
		}
	}
	writeJSON(w, http.StatusOK, report)
}

// ---------------------------------------------------------------------------
// Schema endpoints (consumed by Svelte ListManager + FormRenderer)
// ---------------------------------------------------------------------------

// ListSchema handles GET /list-schema. Returns the ListSchema describing
// columns, capabilities, row actions, and the data URL.
func (h *Handler) ListSchema(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}
	schema := BuildListSchema(projectID, h.lang(r), h.languages)
	writeJSON(w, http.StatusOK, schema)
}

// FormSchemaCreate handles GET /form-schema. Returns the create-mode
// FormSchema for a new category.
func (h *Handler) FormSchemaCreate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}
	schema := BuildCreateForm(projectID, h.lang(r), h.languages)
	writeJSON(w, http.StatusOK, schema)
}

// FormSchemaEdit handles GET /{id}/form-schema. Loads the existing
// category and returns the edit-mode FormSchema with values populated.
func (h *Handler) FormSchemaEdit(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	id := chi.URLParam(r, "id")
	if projectID == "" || id == "" {
		writeError(w, http.StatusBadRequest, "project_id and id required")
		return
	}

	cat, err := h.svc.Get(r.Context(), projectID, id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	if cat == nil {
		writeError(w, http.StatusNotFound, "category not found")
		return
	}
	if err := h.applyOriginToCategory(r.Context(), projectID, cat); err != nil {
		h.log.Warn("enrich category form origin", "project_id", projectID, "category_id", id, "err", err)
	}

	schema := BuildEditForm(projectID, cat, h.lang(r), h.languages)
	writeJSON(w, http.StatusOK, schema)
}

// ---------------------------------------------------------------------------
// Error mapping
// ---------------------------------------------------------------------------

// writeServiceError translates a service-layer error to the matching
// HTTP response. ErrEntityInUse carries slice-specific payload
// (entity_type, entity_id, semantic_id) the frontend reads to render
// the "still referenced by …" link, so it stays inline. Everything
// else routes through apierror.
func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	var inUseErr *ErrEntityInUse
	if errors.As(err, &inUseErr) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":        string(apierror.CodeInUse),
			"error":       "category is in use",
			"entity_type": inUseErr.EntityType,
			"entity_id":   inUseErr.EntityID,
			"semantic_id": inUseErr.SemanticID,
		})
		return
	}
	if IsNotFound(err) {
		apierror.Write(w, apierror.NotFound(err.Error()))
		return
	}
	ae := apierror.FromError(err)
	if ae.Code == apierror.CodeInternal {
		h.log.Error("category handler error", "err", err)
	}
	apierror.Write(w, ae)
}

// ---------------------------------------------------------------------------
// Local HTTP helpers
// ---------------------------------------------------------------------------

// writeJSON writes status + JSON-encoded body. Sets Content-Type.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError forwards to apierror.Write so the wire shape stays
// consistent across slices. New code should use the apierror
// constructors directly (apierror.NotFound, apierror.BadRequest,
// etc.) rather than this generic helper — they set the canonical
// "code" field that the frontend can branch on.
func writeError(w http.ResponseWriter, status int, msg string) {
	apierror.Write(w, &apierror.Error{Status: status, Message: msg})
}

type categoryOriginInfo struct {
	origin domain.Origin
	label  string
}

func (h *Handler) applyOriginsToCategoryRows(ctx context.Context, projectID string, rows []WithCounts) error {
	if len(rows) == 0 {
		return nil
	}
	categories := make([]*domain.Category, 0, len(rows))
	for i := range rows {
		categories = append(categories, &rows[i].Category)
	}
	origins, err := h.categoryOriginsBySystemName(ctx, projectID, categories)
	if err != nil {
		return err
	}
	for i := range rows {
		info, ok := origins[rows[i].SystemName]
		if !ok {
			rows[i].Origin = domain.OwnOrigin()
			rows[i].OriginLabel = ""
			continue
		}
		rows[i].Origin = info.origin
		rows[i].OriginLabel = info.label
	}
	return nil
}

func (h *Handler) applyOriginToCategory(ctx context.Context, projectID string, category *domain.Category) error {
	if category == nil {
		return nil
	}
	origins, err := h.categoryOriginsBySystemName(ctx, projectID, []*domain.Category{category})
	if err != nil {
		return err
	}
	if info, ok := origins[category.SystemName]; ok {
		category.Origin = info.origin
		return nil
	}
	category.Origin = domain.OwnOrigin()
	return nil
}

func (h *Handler) categoryOriginsBySystemName(ctx context.Context, projectID string, categories []*domain.Category) (map[string]categoryOriginInfo, error) {
	if h.origins == nil {
		return nil, nil
	}
	origins, err := h.origins.CategoryOriginsBySystemName(ctx, projectID, categories)
	if err != nil {
		return nil, err
	}
	if len(origins) == 0 {
		return map[string]categoryOriginInfo{}, nil
	}
	out := make(map[string]categoryOriginInfo, len(origins))
	for systemName, origin := range origins {
		out[systemName] = categoryOriginInfo{
			origin: origin,
			label:  categoryOriginLabel(origin),
		}
	}
	return out, nil
}

func categoryOriginLabel(origin domain.Origin) string {
	if origin.Kind != domain.OriginAdopted {
		return ""
	}
	if origin.SourceProjectID == "" {
		return "Adopted"
	}
	return "Adopted from " + origin.SourceProjectID
}

// decodeJSON decodes the request body into a fresh T. Disallows unknown
// fields so typos in the JSON surface as 400 rather than silent ignores.
func decodeJSON[T any](r *http.Request) (T, error) {
	var v T
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, err
	}
	return v, nil
}

// derefTranslations returns *p, or nil if p is nil. Translations is a
// reference type (map) so a nil here means "not provided"; the inner map
// being non-nil empty is also possible from JSON {"ui_name": {}}.
func derefTranslations(p *domain.Translations) domain.Translations {
	if p == nil {
		return nil
	}
	return *p
}
