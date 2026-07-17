package release

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

type LangResolver func(*http.Request) string

type Handler struct {
	svc       *Service
	log       *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
}

func NewHandler(svc *Service, log *slog.Logger, languages []formschema.LanguageInfo, langResolver LangResolver) *Handler {
	if log == nil {
		log = slog.Default()
	}
	if langResolver == nil {
		langResolver = func(*http.Request) string { return "en" }
	}
	return &Handler{svc: svc, log: log, languages: languages, lang: langResolver}
}

func (h *Handler) FormSchema(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	lang := h.lang(r)
	schema := formschema.FormSchema{
		EntityType: "release",
		Mode:       formschema.ModeCreate,
		Endpoint: &formschema.SchemaEndpoint{
			Method: http.MethodPost,
			URL:    "/projects/" + projectID + "/releases",
		},
		Sections: []formschema.Section{
			{
				ID:    "release",
				Label: i18n.L("release.form.section_label", "Release"),
				Fields: []formschema.FieldDef{
					{
						Name:     "version",
						Widget:   formschema.WidgetText,
						Required: true,
						Label:    i18n.L("release.form.version", "Version"),
						// George flagged that the form
						// accepted only full MAJOR.MINOR.PATCH versions
						// without saying so. Spell the constraint out.
						Help: i18n.L("release.form.version_help",
							"Must be a full semantic version — three numbers separated by dots, e.g. 1.0.0. Partial versions like 1.0 or 1 are rejected. An optional pre-release or build suffix (1.0.0-rc.1) is allowed."),
						Value: "",
						Validation: &formschema.ValidationRules{
							Pattern: `^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`,
						},
					},
					{
						Name:   "title",
						Widget: formschema.WidgetText,
						Label:  i18n.L("release.form.title", "Title"),
						Value:  "",
					},
					{
						Name:   "description",
						Widget: formschema.WidgetTextarea,
						Label:  i18n.L("forms.description", "Description"),
						Value:  "",
					},
				},
			},
		},
		UI: formschema.SchemaUI{
			SubmitLabel:    i18n.L("release.form.submit_create", "Create release"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("release.form.created", "Release created."),
			Languages:      h.languages,
			PrimaryLang:    lang,
		},
	}
	writeJSON(w, http.StatusOK, schema)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	items, err := h.svc.ListByProject(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"releases": items,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	version := chi.URLParam(r, "version")
	item, err := h.svc.Get(r.Context(), projectID, version)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	var in CreateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	item, err := h.svc.Create(r.Context(), projectID, in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// writeServiceError maps service errors to apierror responses.
// permissionDenied 404s (rather than 403s) on purpose — release
// surfaces shouldn't leak the existence of a project the caller
// can't see. ValidationFielder + Conflicter adapters in
// apierror_adapters.go cover the typed cases.
func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	var denied *permissionDenied
	if errors.As(err, &denied) {
		apierror.Write(w, apierror.NotFound(""))
		return
	}
	ae := apierror.FromError(err)
	if ae.Code == apierror.CodeInternal {
		h.log.Error("release handler error", "err", err)
	}
	apierror.Write(w, ae)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
