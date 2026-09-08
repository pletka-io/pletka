package projectontologyversion

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// itemKeyLabel is the list-item / list-column key carrying an ontology's
// display label.
const itemKeyLabel = "label"

// LangResolver returns the caller's preferred UI language for a request.
type LangResolver func(r *http.Request) string

// Handler exposes the project-ontology-version JSON API.
type Handler struct {
	svc       *Service
	log       *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
}

// NewHandler constructs a Handler. nil log → slog.Default; nil lang → "en".
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
// Read endpoints
// ---------------------------------------------------------------------------

// List handles GET /. Returns either {items: []} (empty) or
// {groups: [...]} (own + inherited).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	view, err := h.svc.ListView(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	if view.IsEmpty {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}

	groups := make([]formschema.Group, 0, len(view.InheritedGroups)+len(view.OwnGroups))
	for _, g := range view.InheritedGroups {
		groups = append(groups, renderInheritedGroup(g))
	}
	for _, g := range view.OwnGroups {
		groups = append(groups, renderOwnGroup(g))
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

// Pane handles GET /pane. Returns the rich PaneView the Svelte
// settings ontology pane consumes — own + inherited groups with full
// ontology metadata per item plus the catalog of available
// extensions per base.
func (h *Handler) Pane(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	view, err := h.svc.PaneView(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// Stats handles GET /{versionID}/stats.
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	report, err := h.svc.Stats(r.Context(), projectID, versionID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// ListSchema handles GET /list-schema.
func (h *Handler) ListSchema(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	schema := BuildListSchema(projectID, h.lang(r), h.languages)
	writeJSON(w, http.StatusOK, schema)
}

// FormSchema handles GET /form-schema?mode=create|edit&entity_id=X.
func (h *Handler) FormSchema(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = formschema.ModeCreate
	}

	switch mode {
	case formschema.ModeCreate:
		bases, err := h.svc.ListBaseOntologies(r.Context())
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, BuildCreateForm(projectID, bases, h.lang(r), h.languages))
	case formschema.ModeEdit:
		entityID := r.URL.Query().Get("entity_id")
		if entityID == "" {
			writeError(w, http.StatusBadRequest, "entity_id is required in edit mode")
			return
		}
		link, err := h.svc.Get(r.Context(), projectID, entityID)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		if link == nil {
			writeError(w, http.StatusNotFound, "ontology link not found")
			return
		}
		writeJSON(w, http.StatusOK, BuildEditForm(projectID, link, h.lang(r), h.languages))
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported mode: %s", mode))
	}
}

// OptionsVersions handles GET /options/versions?base={ontology_id}.
func (h *Handler) OptionsVersions(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	base := r.URL.Query().Get("base")
	opts, err := h.svc.ListAvailableVersions(r.Context(), projectID, base)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, opts)
}

// OptionsExtensions handles GET /options/extensions?base_version={version_id}.
// The response is an extends-hierarchy forest ([]ExtensionTreeNode), not a
// flat option list — see Service.ListAvailableExtensions.
func (h *Handler) OptionsExtensions(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	baseVersion := r.URL.Query().Get("base_version")
	opts, err := h.svc.ListAvailableExtensions(r.Context(), projectID, baseVersion)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, opts)
}

// ---------------------------------------------------------------------------
// Write endpoints
// ---------------------------------------------------------------------------

// createBody is the POST body shape.
type createBody struct {
	OntologyID string   `json:"ontology_id"`
	VersionID  string   `json:"version_id"`
	Extensions []string `json:"extensions"`
	IsPrimary  bool     `json:"is_primary"`
	UsageNotes string   `json:"usage_notes"`
}

// patchBody is the PATCH body shape.
type patchBody struct {
	IsPrimary  *bool   `json:"is_primary,omitempty"`
	UsageNotes *string `json:"usage_notes,omitempty"`
}

// Create handles POST /.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	body, err := decodeJSON[createBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	link, err := h.svc.Create(r.Context(), projectID, CreateInput{
		OntologyID: body.OntologyID,
		VersionID:  body.VersionID,
		Extensions: body.Extensions,
		IsPrimary:  body.IsPrimary,
		UsageNotes: body.UsageNotes,
	})
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

// Patch handles PATCH /{versionID}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	body, err := decodeJSON[patchBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	link, err := h.svc.Update(r.Context(), projectID, versionID, UpdateInput{
		IsPrimary:  body.IsPrimary,
		UsageNotes: body.UsageNotes,
	})
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

// Delete handles DELETE /{versionID}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	if err := h.svc.Delete(r.Context(), projectID, versionID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Render helpers
// ---------------------------------------------------------------------------

func renderOwnGroup(g domain.LinkedOntologyGroup) formschema.Group {
	items := make([]map[string]any, 0, len(g.Items))
	for _, it := range g.Items {
		label := it.OntologyName
		if label == "" {
			label = g.BaseLabel
		}
		items = append(items, map[string]any{
			"id":          it.Link.OntologyVersionID,
			itemKeyLabel:  label,
			"version":     "",
			"primary":     it.Link.IsPrimary,
			"usage_count": it.UsageCount,
		})
	}
	var badges []formschema.Badge
	if g.Primary {
		badges = append(badges, formschema.Badge{Label: "Primary", Tone: "primary"})
	}
	return formschema.Group{
		ID:          fmt.Sprintf("own-%s", g.BaseVersionID),
		Label:       domain.Translations{"en": g.BaseLabel},
		Collapsible: true,
		Collapsed:   false,
		ReadOnly:    false,
		Badges:      badges,
		Items:       items,
	}
}

func renderInheritedGroup(g InheritedGroup) formschema.Group {
	items := make([]map[string]any, 0, len(g.Rows))
	for _, r := range g.Rows {
		itemLabel := r.OntologyName
		if itemLabel == "" {
			itemLabel = r.Link.OntologyVersionID
		}
		items = append(items, map[string]any{
			"id":          r.Link.OntologyVersionID,
			itemKeyLabel:  itemLabel,
			"version":     r.VersionString,
			"primary":     r.Link.IsPrimary,
			"usage_count": int64(0),
		})
	}
	return formschema.Group{
		ID: fmt.Sprintf("inherited-%s", g.SourceProjectID),
		Label: domain.Translations{
			"en": fmt.Sprintf("Inherited from %s", g.SourceProjectLabel),
			"nl": fmt.Sprintf("Overgeërfd van %s", g.SourceProjectLabel),
		},
		Collapsible: true,
		Collapsed:   false,
		ReadOnly:    true,
		Badges:      []formschema.Badge{{Label: "Read only", Tone: "neutral"}},
		Items:       items,
	}
}

// ---------------------------------------------------------------------------
// Error mapping
// ---------------------------------------------------------------------------

// writeServiceError translates service errors to the matching HTTP
// response. Common shapes go through apierror; the in-use + duplicate
// envelopes carry slice-specific payload (field_count, field_samples)
// the frontend relies on, so they stay inline.
func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	var inUseErr *ErrInUse
	if errors.As(err, &inUseErr) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":          string(apierror.CodeInUse),
			"error":         "version_in_use",
			"field_count":   inUseErr.TotalUsage,
			"field_samples": inUseErr.Samples,
			"message":       "Fields reference this ontology version; re-path or delete them first.",
		})
		return
	}
	if IsDuplicate(err) {
		apierror.Write(w, apierror.Conflict("version already linked"))
		return
	}
	if IsNotFound(err) {
		apierror.Write(w, apierror.NotFound(err.Error()))
		return
	}
	ae := apierror.FromError(err)
	if ae.Code == apierror.CodeInternal {
		h.log.Error("project ontology version handler error", "err", err)
	}
	apierror.Write(w, ae)
}

// ---------------------------------------------------------------------------
// Local HTTP helpers (slice-private)
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
