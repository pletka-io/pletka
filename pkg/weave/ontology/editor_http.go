package ontology

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// DraftLifecycle is the transaction-owning editor boundary. HTTP adapters never
// acquire a Store or a writable release capability.
type DraftLifecycle interface {
	Start(context.Context, ontologybody.OntologyID, DraftDocument) (DraftSnapshot, error)
	StartFromRelease(context.Context, ontologybody.OntologyID, ontologybody.VersionID) (DraftSnapshot, error)
	Current(context.Context, ontologybody.OntologyID) (DraftSnapshot, error)
	Read(context.Context, DraftReference, func(DraftReader) error) error
	Edit(context.Context, DraftReference, int64, func(DraftEditor) error) error
	Publish(context.Context, DraftReference, int64, DraftPublishRequest) (DraftPublication, error)
}

// EditorCatalog supplies catalog defaults only when creating a blank draft.
type EditorCatalog interface {
	GetOntology(context.Context, string) (*domain.Ontology, error)
}

// EditorReleases validates explicit immutable source selections.
type EditorReleases interface {
	Read(context.Context, ontologyrelease.Reference, func(ontologyrelease.Reader) error) error
}

// EditorHost is supplied by app wiring. Catalog access is discovery for a new
// blank draft only; existing editor content comes exclusively from DraftReader.
type EditorHost struct {
	Drafts       DraftLifecycle
	Catalog      EditorCatalog
	Releases     EditorReleases
	Languages    []formschema.LanguageInfo
	LangResolver func(*http.Request) string
}

func (h *EditorHost) lang(r *http.Request) string {
	if h.LangResolver != nil {
		if lang := h.LangResolver(r); lang != "" {
			return lang
		}
	}
	return "en"
}
func editorBase(owner string) string { return "/api/v2/ontologies/" + url.PathEscape(owner) + "/draft" }
func editorPage(owner string) string {
	return "/admin/ontologies/" + url.PathEscape(owner) + "/draft/page"
}
func editorBound(ref DraftReference) string {
	return editorBase(string(ref.owner)) + "/" + url.PathEscape(string(ref.id))
}

// MountEditorAPI is independently authorized, including every schema/read route.
func MountEditorAPI(parent chi.Router, host *EditorHost) {
	if host == nil {
		return
	}
	if host.Drafts == nil || host.Catalog == nil || host.Releases == nil {
		panic("ontology editor host missing lifecycle/catalog/release dependencies")
	}
	parent.Route("/api/v2/ontologies/{ontologyID}/draft", func(r chi.Router) {
		r.Use(requireOntologyEditor)
		r.Get("/", host.current)
		r.Post("/", host.start)
		r.Get("/page-schema", host.pageSchema)
		r.Get("/start-form-schema", host.startFormSchema)
		r.Post("/form", host.startForm)
		r.Route("/{draftID}", func(r chi.Router) {
			r.Get("/", host.get)
			r.Put("/", host.save)
			r.Get("/form-schema", host.editFormSchema)
			r.Put("/form", host.saveForm)
			r.Get("/publish-form-schema", host.publishFormSchema)
			r.Post("/publish", host.publish)
		})
	})
}
func requireOntologyEditor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snap := auth.FromContext(r.Context())
		if snap == nil || snap.IsAnonymous || !snap.IsSuperAdmin {
			apierror.Write(w, r, apierror.NotFound("not found"))
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		next.ServeHTTP(w, r)
	})
}
func editorError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotFound) {
		err = apierror.NotFound("ontology not found")
	}
	apierror.Write(w, r, apierror.FromError(err))
}
func editorDecode(w http.ResponseWriter, r *http.Request, dst any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		apierror.Write(w, r, apierror.BadRequest("Content-Type must be application/json"))
		return false
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		apierror.Write(w, r, apierror.BadRequest("invalid request body: "+err.Error()))
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		apierror.Write(w, r, apierror.BadRequest("request must contain one JSON object"))
		return false
	}
	return true
}
func editorReference(r *http.Request) (DraftReference, error) {
	return NewDraftReference(ontologybody.OntologyID(chi.URLParam(r, "ontologyID")), DraftID(chi.URLParam(r, "draftID")))
}
func (h *EditorHost) snapshot(r *http.Request) (DraftSnapshot, error) {
	ref, err := editorReference(r)
	if err != nil {
		return DraftSnapshot{}, err
	}
	var out DraftSnapshot
	err = h.Drafts.Read(r.Context(), ref, func(reader DraftReader) error { var err error; out, err = reader.Snapshot(r.Context()); return err })
	return out, err
}
func (h *EditorHost) current(w http.ResponseWriter, r *http.Request) {
	out, err := h.Drafts.Current(r.Context(), ontologybody.OntologyID(chi.URLParam(r, "ontologyID")))
	if err != nil {
		editorError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (h *EditorHost) get(w http.ResponseWriter, r *http.Request) {
	out, err := h.snapshot(r)
	if err != nil {
		editorError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (h *EditorHost) start(w http.ResponseWriter, r *http.Request) {
	var input struct {
		FromRelease string         `json:"from_release"`
		Document    *DraftDocument `json:"document"`
	}
	if !editorDecode(w, r, &input) {
		return
	}
	if (input.FromRelease == "") == (input.Document == nil) {
		editorError(w, r, draftInvalid("document", "choose a source release or a draft document"))
		return
	}
	owner := ontologybody.OntologyID(chi.URLParam(r, "ontologyID"))
	var out DraftSnapshot
	var err error
	if input.Document != nil {
		out, err = h.Drafts.Start(r.Context(), owner, *input.Document)
	} else {
		out, err = h.Drafts.StartFromRelease(r.Context(), owner, ontologybody.VersionID(input.FromRelease))
	}
	if err != nil {
		editorError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
func (h *EditorHost) save(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Revision int64         `json:"revision"`
		Document DraftDocument `json:"document"`
	}
	if !editorDecode(w, r, &input) {
		return
	}
	ref, err := editorReference(r)
	if err != nil {
		editorError(w, r, err)
		return
	}
	var out DraftSnapshot
	err = h.Drafts.Edit(r.Context(), ref, input.Revision, func(editor DraftEditor) error {
		var err error
		out, err = editor.Save(r.Context(), input.Document)
		return err
	})
	if err != nil {
		editorError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (h *EditorHost) publish(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Revision  int64  `json:"revision"`
		Version   string `json:"version"`
		SetActive bool   `json:"set_active"`
	}
	if !editorDecode(w, r, &input) {
		return
	}
	ref, err := editorReference(r)
	if err != nil {
		editorError(w, r, err)
		return
	}
	out, err := h.Drafts.Publish(r.Context(), ref, input.Revision, DraftPublishRequest{Version: input.Version, SetActive: input.SetActive})
	if err != nil {
		editorError(w, r, err)
		return
	}
	status := http.StatusOK
	if out.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, out)
}
