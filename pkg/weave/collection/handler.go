package collection

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserhttp"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
)

type LangResolver func(r *http.Request) string

type Handler struct {
	inventory browserhttp.Handler
	svc       *Service
	log       *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
	// i18n walks LocalizedText nodes inside list-row payloads (origin_label
	// is the first such node) — schema-driven UI rule, see
	// .claude/rules/ui-patterns.md.
	i18n i18n.Manager
}

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
// Reads
// ---------------------------------------------------------------------------

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	h.inventory.List(w, r, "collection")
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	c, err := h.svc.Get(r.Context(), auth.ReadScopeFromContext(r.Context()), projectID, collectionID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	if c == nil {
		errresp.Error(w, r, http.StatusNotFound, "not_found", "Collection not found")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	report, err := h.svc.Stats(r.Context(), auth.ReadScopeFromContext(r.Context()), projectID, collectionID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) Fork(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	if projectID == "" || collectionID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "Project ID and Collection ID are required")
		return
	}

	collection, err := h.svc.ForkFromSource(r.Context(), projectID, collectionID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":  collection.ID,
		"url": "/projects/" + projectID + "/collections/" + collection.ID,
	})
}

// ScopeClassesOptions returns the distinct ontology scope classes used
// by collections in the project, in the {"options":[...]} envelope
// expected by the entity-list lazy-options loader.
func (h *Handler) ScopeClassesOptions(w http.ResponseWriter, r *http.Request) {
	h.inventory.Filters(w, r, "collection", "scope")
}

// CategoriesOptions returns the distinct categories assigned to
// collections in the project, in the {"options":[...]} envelope.
func (h *Handler) CategoriesOptions(w http.ResponseWriter, r *http.Request) {
	h.inventory.Filters(w, r, "collection", "category")
}

// ---------------------------------------------------------------------------
// Override editing
// ---------------------------------------------------------------------------

func (h *Handler) ListOverrides(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	scope := auth.ReadScopeFromContext(r.Context())
	rows, err := h.svc.ListOverrides(r.Context(), scope, projectID, collectionID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entity_type": "collection",
		"entity_id":   collectionID,
		"project_id":  projectID,
		"overrides":   rows,
	})
}

func (h *Handler) SaveOverrides(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")

	var body struct {
		CommitMessage string                 `json:"commit_message"`
		Overrides     []domain.FieldOverride `json:"overrides"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	saved, diff, err := h.svc.SaveOverrides(r.Context(), projectID, collectionID, body.Overrides, body.CommitMessage)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entity_type": "collection",
		"entity_id":   collectionID,
		"project_id":  projectID,
		"overrides":   saved,
		"diff": map[string]any{
			"added":   len(diff.Added),
			"removed": len(diff.Removed),
			"changed": len(diff.Changed),
		},
	})
}

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

type collectionWriteBody struct {
	UIName                   domain.Translations `json:"ui_name"`
	Description              domain.Translations `json:"description,omitempty"`
	SystemName               string              `json:"system_name,omitempty"`
	Status                   string              `json:"status,omitempty"`
	OntologyScope            any                 `json:"ontology_scope,omitempty"`
	DefaultCategoryID        *string             `json:"default_category_id,omitempty"`
	CollectionNumber         *int                `json:"collection_number,omitempty"`
	CanonicalCollectionOrder *int                `json:"canonical_collection_order,omitempty"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	var body collectionWriteBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	scope, scopeErr := parseOntologyScopeInput(body.OntologyScope)
	if scopeErr != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "validation error",
			"errors":  map[string][]string{"ontology_scope": {scopeErr.Error()}},
			"message": "Please correct the highlighted fields and try again.",
		})
		return
	}
	in := CreateInput{
		UIName:        body.UIName,
		Description:   body.Description,
		SystemName:    body.SystemName,
		OntologyScope: scope,
	}
	if body.DefaultCategoryID != nil {
		in.DefaultCategoryID = *body.DefaultCategoryID
	}
	c, err := h.svc.Create(r.Context(), projectID, in)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	var body collectionWriteBody
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
	if body.OntologyScope != nil {
		scope, scopeErr := parseOntologyScopeInput(body.OntologyScope)
		if scopeErr != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error":   "validation error",
				"errors":  map[string][]string{"ontology_scope": {scopeErr.Error()}},
				"message": "Please correct the highlighted fields and try again.",
			})
			return
		}
		in.OntologyScope = &scope
	}
	if body.DefaultCategoryID != nil {
		in.DefaultCategoryID = body.DefaultCategoryID
	}
	if body.CollectionNumber != nil {
		in.CollectionNumber = body.CollectionNumber
	}
	if body.CanonicalCollectionOrder != nil {
		in.CanonicalCollectionOrder = body.CanonicalCollectionOrder
	}
	c, err := h.svc.Update(r.Context(), projectID, collectionID, in)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	if err := h.svc.Delete(r.Context(), projectID, collectionID); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Deprecate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	if err := h.svc.Deprecate(r.Context(), projectID, collectionID); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	if err := h.svc.Activate(r.Context(), projectID, collectionID); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Error mapping
// ---------------------------------------------------------------------------

func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var inUseErr *ErrEntityInUse
	if errors.As(err, &inUseErr) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":          string(apierror.CodeInUse),
			"error":         "collection_in_use",
			"field_count":   inUseErr.Usage.FieldCount,
			"field_samples": inUseErr.Usage.FieldSamples,
			"message":       "Fields reference this collection as a value target. Deprecate or repoint those fields first.",
		})
		return
	}
	var setupErr *ErrSetupIncomplete
	if errors.As(err, &setupErr) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":         "no_ontologies",
			"error":        "no_ontologies",
			"error_code":   "NO_ONTOLOGIES",
			"message":      "No ontologies configured for this project. Configure at least one before creating collections.",
			"settings_url": "/projects/" + setupErr.ProjectID + "/settings-v2#ontology",
		})
		return
	}
	if IsNotFound(err) {
		apierror.Write(w, r, apierror.NotFound(err.Error()))
		return
	}
	ae := apierror.FromError(err)
	if ae.Code == apierror.CodeInternal {
		h.log.Error("collection handler error", "err", err)
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

func parsePrefixedClass(s string) (domain.PathElement, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return domain.PathElement{}, nil
	}
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return domain.PathElement{}, fmt.Errorf("ontology_scope must be in prefix:LocalName form (got %q)", s)
	}
	return domain.PathElement{
		Type:      "class",
		Prefix:    parts[0],
		LocalName: parts[1],
	}, nil
}

func parseOntologyScopeInput(raw any) (domain.PathElement, error) {
	switch v := raw.(type) {
	case nil:
		return domain.PathElement{}, nil
	case map[string]any:
		prefix, _ := v["prefix"].(string)
		localName, _ := v["local_name"].(string)
		uri, _ := v["uri"].(string)
		typ, _ := v["type"].(string)
		classCode, _ := v["class_code"].(string)
		if typ == "" {
			typ = "class"
		}
		if prefix == "" || localName == "" {
			return domain.PathElement{}, fmt.Errorf("ontology scope requires prefix and local_name")
		}
		if uri == "" {
			uri = prefix + ":" + localName
		}
		if classCode == "" {
			classCode = domain.DeriveClassCode(localName)
		}
		return domain.PathElement{
			Type:      typ,
			Prefix:    prefix,
			LocalName: localName,
			URI:       uri,
			ClassCode: classCode,
		}, nil
	default:
		return domain.PathElement{}, fmt.Errorf("ontology_scope must be a PathElement object")
	}
}
