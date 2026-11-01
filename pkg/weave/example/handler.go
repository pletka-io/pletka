package example

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserhttp"
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
	return fmt.Sprintf("/projects/%s#tab=examples&item=%s", url.PathEscape(projectID), url.QueryEscape(exampleID))
}

type LangResolver func(r *http.Request) string

type Handler struct {
	inventory browserhttp.Handler
	reads     readHandler
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
	h.inventory.List(w, r, "example")
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	var in CreateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apierror.Write(w, r, apierror.BadRequest("invalid json"))
		return
	}
	record, err := h.svc.Create(r.Context(), auth.ReadScopeFromContext(r.Context()), projectID, in)
	if err != nil {
		writeMutationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	h.reads.detail(w, r)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	exampleID := chi.URLParam(r, "exampleID")
	var in UpdateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apierror.Write(w, r, apierror.BadRequest("invalid json"))
		return
	}
	record, err := h.svc.Update(r.Context(), auth.ReadScopeFromContext(r.Context()), projectID, exampleID, in)
	if err != nil {
		writeMutationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	exampleID := chi.URLParam(r, "exampleID")
	if err := h.svc.Delete(r.Context(), auth.ReadScopeFromContext(r.Context()), projectID, exampleID); err != nil {
		writeMutationError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) FormSchema(w http.ResponseWriter, r *http.Request) {
	h.reads.formSchema(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Preserve existing input errors while passing typed source/access/conflict
// failures through the common envelope.
func writeMutationError(w http.ResponseWriter, r *http.Request, err error) {
	out := apierror.FromError(err)
	if out.Code == apierror.CodeInternal {
		out = apierror.BadRequest(err.Error())
	}
	apierror.Write(w, r, out)
}
