package field

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/browserhttp"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
)

// LangResolver returns the caller's preferred UI language for a request.
type LangResolver func(r *http.Request) string

// Handler exposes the field slice's read endpoints.
type Handler struct {
	inventory browserhttp.Handler
	svc       *Service
	log       *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
	// i18n walks LocalizedText nodes inside the row payload (origin_label)
	// — schema-driven UI rule (.claude/rules/ui-patterns.md).
	i18n i18n.Manager
}

// NewHandler constructs a Handler. nil log → slog.Default; nil lang → "en".
func NewHandler(svc *Service, log *slog.Logger, languages []formschema.LanguageInfo, lang LangResolver, i18nMgr i18n.Manager) *Handler {
	if log == nil {
		log = slog.Default()
	}
	if lang == nil {
		lang = func(*http.Request) string { return "en" }
	}
	return &Handler{svc: svc, log: log, languages: languages, lang: lang, i18n: i18nMgr}
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

// List handles GET /projects/{projectID}/fields. Paginated + searchable
// list rendered as fieldListItem JSON.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	h.inventory.List(w, r, "field")
}

// OntologyScopesFilter handles GET /projects/{projectID}/fields/filters/ontology-scopes.
// Returns the distinct ontology scopes present on this project's
// fields, in the FilterOption shape the entity-list schema expects.
func (h *Handler) OntologyScopesFilter(w http.ResponseWriter, r *http.Request) {
	h.inventory.Filters(w, r, "field", "scope")
}

// CategoriesFilter handles GET /projects/{projectID}/fields/filters/categories.
// Returns the distinct categories assigned to base field overrides in
// the project, in the {"options":[...]} envelope.
func (h *Handler) CategoriesFilter(w http.ResponseWriter, r *http.Request) {
	h.inventory.Filters(w, r, "field", "category")
}

// Detail handles GET /projects/{projectID}/fields/api/{fieldID}.
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	if projectID == "" || fieldID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "Project ID and Field ID are required")
		return
	}

	field, err := h.svc.Get(r.Context(), projectID, fieldID)
	if err != nil {
		if IsNotFound(err) {
			errresp.Error(w, r, http.StatusNotFound, "not_found", "Field not found")
			return
		}
		h.writeServiceError(w, r, err)
		return
	}
	if field == nil {
		errresp.Error(w, r, http.StatusNotFound, "not_found", "Field not found")
		return
	}

	writeJSON(w, http.StatusOK, field)
}

// ModelRefs handles GET /projects/{projectID}/fields/{fieldID}/models.
// Returns models that reference this field via weave_field_overrides.
func (h *Handler) ModelRefs(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	if fieldID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "Field ID is required")
		return
	}

	refs, err := h.svc.GetModels(r.Context(), projectID, fieldID)
	if err != nil {
		h.log.Error("get field models", "field_id", fieldID, "err", err)
		// Match legacy behaviour: degrade to empty slice on error.
		refs = []domain.FieldModelRef{}
	}

	writeJSON(w, http.StatusOK, refs)
}

// CollectionRefs handles GET /projects/{projectID}/fields/{fieldID}/collections.
func (h *Handler) CollectionRefs(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	if fieldID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "Field ID is required")
		return
	}

	refs, err := h.svc.GetCollections(r.Context(), projectID, fieldID)
	if err != nil {
		h.log.Error("get field collections", "field_id", fieldID, "err", err)
		refs = []domain.FieldCollectionRef{}
	}

	writeJSON(w, http.StatusOK, refs)
}

// Stats handles GET /projects/{projectID}/fields/{fieldID}/stats.
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	report, err := h.svc.Stats(r.Context(), projectID, fieldID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// ---------------------------------------------------------------------------
// Write endpoints
// ---------------------------------------------------------------------------

// fieldWriteBody is the shared JSON body for Create + Update. PUT
// treats every populated field as a partial change; POST treats them
// as full create state.
type fieldWriteBody struct {
	UIName            domain.Translations  `json:"ui_name"`
	Description       domain.Translations  `json:"description,omitempty"`
	SystemName        string               `json:"system_name,omitempty"`
	SemanticID        string               `json:"semantic_id,omitempty"`
	Status            string               `json:"status,omitempty"`
	OntologyScope     domain.PathElement   `json:"ontology_scope"`
	OntologyPath      []domain.PathElement `json:"ontology_path"`
	ExpectedValueType string               `json:"expected_value_type,omitempty"`

	// Override-shaped — written into the base override row.
	CategoryID               *string   `json:"category_id,omitempty"`
	SetValue                 *string   `json:"set_value,omitempty"`
	ExpectedResourceModels   *[]string `json:"expected_resource_models,omitempty"`
	ExpectedCollectionModels *[]string `json:"expected_collection_models,omitempty"`
}

// Create handles POST /projects/{projectID}/fields.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var body fieldWriteBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	in := CreateInput{
		UIName:                body.UIName,
		Description:           body.Description,
		SystemName:            body.SystemName,
		SemanticID:            body.SemanticID,
		Status:                domain.Status(body.Status),
		OntologyScope:         body.OntologyScope,
		OntologyPath:          body.OntologyPath,
		ExpectedValueType:     body.ExpectedValueType,
		CategoryID:            derefString(body.CategoryID),
		SetValue:              derefString(body.SetValue),
		ExpectedModelIDs:      derefStringSlice(body.ExpectedResourceModels),
		ExpectedCollectionIDs: derefStringSlice(body.ExpectedCollectionModels),
	}

	field, err := h.svc.Create(r.Context(), projectID, in)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, field)
}

// Update handles PUT /projects/{projectID}/fields/{fieldID}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")

	var body fieldWriteBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	in := UpdateInput{}
	if len(body.UIName) > 0 {
		ui := body.UIName
		in.UIName = &ui
	}
	if len(body.Description) > 0 {
		desc := body.Description
		in.Description = &desc
	}
	if body.SystemName != "" {
		s := body.SystemName
		in.SystemName = &s
	}
	if body.Status != "" {
		st := domain.Status(body.Status)
		in.Status = &st
	}
	if body.OntologyScope.LocalName != "" {
		sc := body.OntologyScope
		in.OntologyScope = &sc
	}
	if len(body.OntologyPath) > 0 {
		in.OntologyPath = body.OntologyPath
	}
	if body.ExpectedValueType != "" {
		v := body.ExpectedValueType
		in.ExpectedValueType = &v
	}
	if body.CategoryID != nil {
		c := *body.CategoryID
		in.CategoryID = &c
	}
	if body.SetValue != nil {
		v := *body.SetValue
		in.SetValue = &v
	}
	if body.ExpectedResourceModels != nil {
		v := append([]string(nil), (*body.ExpectedResourceModels)...)
		in.ExpectedModelIDs = &v
	}
	if body.ExpectedCollectionModels != nil {
		v := append([]string(nil), (*body.ExpectedCollectionModels)...)
		in.ExpectedCollectionIDs = &v
	}

	field, err := h.svc.Update(r.Context(), projectID, fieldID, in)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, field)
}

// Delete handles DELETE /projects/{projectID}/fields/{fieldID}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	if err := h.svc.Delete(r.Context(), projectID, fieldID); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Deprecate handles POST /projects/{projectID}/fields/{fieldID}/deprecate.
func (h *Handler) Deprecate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	if err := h.svc.Deprecate(r.Context(), projectID, fieldID); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Activate handles POST /projects/{projectID}/fields/{fieldID}/activate.
func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	if err := h.svc.Activate(r.Context(), projectID, fieldID); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func derefStringSlice(v *[]string) []string {
	if v == nil {
		return nil
	}
	return append([]string(nil), (*v)...)
}

// ---------------------------------------------------------------------------
// Error mapping
// ---------------------------------------------------------------------------

func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	if IsNotFound(err) {
		apierror.Write(w, r, apierror.NotFound(err.Error()))
		return
	}
	// In-use conflict carries slice-specific counts the frontend reads to steer
	// the user to Deprecate — mirrors the model + collection handlers. Without
	// this branch it fell through to a generic 500 and the UI showed nothing
	// useful ("delete did nothing").
	var inUseErr *ErrEntityInUse
	if errors.As(err, &inUseErr) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":             string(apierror.CodeInUse),
			"error":            "field_in_use",
			"model_count":      inUseErr.Usage.ModelCount,
			"collection_count": inUseErr.Usage.CollectionCount,
			"message":          "This field is used by other models or collections. Deprecate it instead, or repoint those references first.",
		})
		return
	}
	ae := apierror.FromError(err)
	if ae.Code == apierror.CodeInternal {
		h.log.Error("field handler error", "err", err)
	}
	apierror.Write(w, r, ae)
}

// ---------------------------------------------------------------------------
// Local HTTP helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError forwards to apierror.Write — see pkg/weave/apierror.
func writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	apierror.Write(w, r, &apierror.Error{Status: status, Message: msg})
}
