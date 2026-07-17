package orgmembers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

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

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		http.NotFound(w, r)
		return
	}
	rows, err := h.svc.List(r.Context(), org.ID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) ListSchema(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	writeJSON(w, http.StatusOK, BuildListSchema(slug, h.lang(r), h.languages))
}

func (h *Handler) FormSchema(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	writeJSON(w, http.StatusOK, BuildAddForm(slug, h.lang(r), h.languages))
}

func (h *Handler) OptionsActors(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		http.NotFound(w, r)
		return
	}
	rows, err := h.svc.ListAvailableActors(r.Context(), org.ID)
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

func (h *Handler) FormSchemaEdit(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		http.NotFound(w, r)
		return
	}
	actorID := chi.URLParam(r, "actorID")
	member, err := h.svc.Get(r.Context(), org.ID, actorID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	if member == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, BuildEditForm(org.Slug, actorID, member, h.lang(r), h.languages))
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		http.NotFound(w, r)
		return
	}
	var body AddInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	row, err := h.svc.Add(r.Context(), org.ID, body)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		http.NotFound(w, r)
		return
	}
	actorID := chi.URLParam(r, "actorID")
	var body UpdateInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	row, err := h.svc.UpdateRole(r.Context(), org.ID, actorID, body)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		http.NotFound(w, r)
		return
	}
	actorID := chi.URLParam(r, "actorID")
	if err := h.svc.Remove(r.Context(), org.ID, actorID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError + writeValidationErrors forward to apierror so the wire
// shape stays consistent with every other slice. New code should use
// apierror constructors directly (apierror.NotFound, apierror.Validation,
// etc.) — they set the canonical "code" discriminator the frontend
// can branch on.
func writeError(w http.ResponseWriter, status int, msg string) {
	apierror.Write(w, &apierror.Error{Status: status, Message: msg})
}

func writeValidationErrors(w http.ResponseWriter, fields map[string][]string) {
	apierror.Write(w, apierror.Validation(fields))
}

// writeServiceError maps a service-layer error to its apierror.Error
// shape via the adapter interfaces in apierror_adapters.go. The
// sentinel errNotFound stays explicit because it's a free var (not a
// typed error), so it can't satisfy the NotFounder interface
// directly.
func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	if IsNotFound(err) {
		apierror.Write(w, apierror.NotFound(err.Error()))
		return
	}
	ae := apierror.FromError(err)
	if ae.Code == apierror.CodeInternal {
		// Only log when we're dropping the underlying cause —
		// matched-type errors carry their own message into the
		// response and don't need the noise.
		h.log.Error("orgmembers service error", "err", err)
	}
	apierror.Write(w, ae)
}
