package members

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// Handler exposes the project members REST surface. Routes are
// registered by Mount; cross-cutting auth + project-resource
// middleware come from pkg/weave/router via mountSlice.
type Handler struct {
	svc          *Service
	log          *slog.Logger
	languages    []formschema.LanguageInfo
	langResolver LangResolver
}

type LangResolver func(*http.Request) string

func NewHandler(svc *Service, log *slog.Logger, languages []formschema.LanguageInfo, langResolver LangResolver) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{svc: svc, log: log, languages: languages, langResolver: langResolver}
}

func (h *Handler) lang(r *http.Request) string {
	if h.langResolver != nil {
		return h.langResolver(r)
	}
	return "en"
}

// List handles GET /. Returns members joined with actor info.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	rows, err := h.svc.List(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// ListSchema handles GET /list-schema.
func (h *Handler) ListSchema(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	writeJSON(w, http.StatusOK, BuildListSchema(projectID, h.lang(r), h.languages))
}

// FormSchema handles GET /form-schema (the create form).
func (h *Handler) FormSchema(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	writeJSON(w, http.StatusOK, BuildAddForm(projectID, h.lang(r), h.languages))
}

// OptionsActors handles GET /options/actors. Returns the list of
// actors the caller can still add to the project (everyone except
// existing members and the owner) shaped as []formschema.SelectOption.
// Used by the Add Member form's actor_id select widget.
func (h *Handler) OptionsActors(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	rows, err := h.svc.ListAvailableActors(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	opts := make([]formschema.SelectOption, 0, len(rows))
	for _, m := range rows {
		opts = append(opts, formschema.SelectOption{
			Value: m.ActorID,
			Label: actorOptionLabel(m),
		})
	}
	writeJSON(w, http.StatusOK, opts)
}

// actorOptionLabel formats "Display Name (email)" or "Display Name
// (@slug)" so the user can disambiguate in the select dropdown.
func actorOptionLabel(m MemberRow) domain.Translations {
	primary := m.DisplayName
	if primary == "" {
		primary = m.Slug
	}
	suffix := ""
	switch {
	case m.Email != "":
		suffix = m.Email
	case m.Slug != "":
		suffix = "@" + m.Slug
	}
	if suffix != "" {
		primary = primary + " (" + suffix + ")"
	}
	return domain.Translations{"en": primary}
}

// FormSchemaEdit handles GET /{actorID}/form-schema. Returns the
// edit form prefilled with the member's current role.
func (h *Handler) FormSchemaEdit(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	actorID := chi.URLParam(r, "actorID")
	member, err := h.svc.Get(r.Context(), projectID, actorID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, BuildEditForm(projectID, actorID, member, h.lang(r), h.languages))
}

// Create handles POST /. Body: {"email_or_slug": "...", "role": "..."}.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	var body AddInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	row, err := h.svc.Add(r.Context(), projectID, body)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

// Patch handles PATCH /{actorID}. Body: {"role": "..."}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	actorID := chi.URLParam(r, "actorID")
	var body UpdateInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	row, err := h.svc.UpdateRole(r.Context(), projectID, actorID, body)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

// Delete handles DELETE /{actorID}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	actorID := chi.URLParam(r, "actorID")
	if err := h.svc.Remove(r.Context(), projectID, actorID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// HTTP helpers for this slice.
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError + writeValidationErrors forward to apierror.Write so
// the wire shape stays consistent across slices.
func writeError(w http.ResponseWriter, status int, msg string) {
	apierror.Write(w, &apierror.Error{Status: status, Message: msg})
}

func writeValidationErrors(w http.ResponseWriter, fields map[string][]string) {
	apierror.Write(w, apierror.Validation(fields))
}

// writeServiceError maps the slice's typed errors to HTTP statuses.
func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	if IsNotFound(err) {
		apierror.Write(w, apierror.NotFound(err.Error()))
		return
	}
	ae := apierror.FromError(err)
	if ae.Code == apierror.CodeInternal {
		h.log.Error("members service error", "err", err)
	}
	apierror.Write(w, ae)
}
