package example

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserhttp"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// readHandler has a single source; it cannot reach the example editing service.
type readHandler struct {
	source    browserhttp.DetailSource
	maxDepth  int
	lang      LangResolver
	languages []formschema.LanguageInfo
}

func (h readHandler) load(r *http.Request, kind, id string) (browserinput.Detail, error) {
	project := chi.URLParam(r, "projectID")
	scope := auth.ReadScopeFromContext(r.Context())
	resource := auth.ProjectResourceFromContext(r.Context())
	if project == "" || resource.ID != project || !auth.FromContext(r.Context()).Can(auth.ProjectRead, resource, nil) {
		return browserinput.Detail{}, apierror.NotFound("project not found")
	}
	if h.source == nil || !scope.Valid() {
		return browserinput.Detail{}, fmt.Errorf("example reads require a source and explicit scope")
	}
	if id == "" {
		return browserinput.Detail{}, apierror.BadRequest("example or target identity required")
	}
	value, err := h.source.Detail(r.Context(), scope, project, kind, id)
	if err != nil {
		return value, err
	}
	consumer, err := value.Fragment.Consumer()
	root, rootErr := value.Fragment.Root()
	if err != nil || rootErr != nil || consumer.ProjectID() != project || consumer.Version() != scope.Version() || root.Kind != kind || root.ID != id {
		return browserinput.Detail{}, fmt.Errorf("example source returned a different selection")
	}
	return value, nil
}

func (h readHandler) detail(w http.ResponseWriter, r *http.Request) {
	input, err := h.load(r, exampleKind, chi.URLParam(r, "exampleID"))
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	if wantsHTML(r) {
		path := projectExamplesPageURL(chi.URLParam(r, "projectID"), chi.URLParam(r, "exampleID"))
		u, _ := url.Parse(path)
		q := u.Query()
		if version := auth.ReadScopeFromContext(r.Context()).Version(); version != "" {
			q.Set("version", version)
		}
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
		return
	}
	record, _, err := ProjectRead(r.Context(), input, h.maxDepth, h.lang(r), h.languages)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h readHandler) formSchema(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = formschema.ModeCreate
	}
	kind, id := exampleKind, r.URL.Query().Get("example_id")
	switch mode {
	case formschema.ModeCreate:
		kind, id = r.URL.Query().Get("target_type"), r.URL.Query().Get("target_id")
		if kind != modelKind {
			apierror.Write(w, r, apierror.BadRequest("target_type=model and target_id are required"))
			return
		}
	case formschema.ModeEdit, formschema.ModeView:
	default:
		apierror.Write(w, r, apierror.BadRequest("unsupported example form mode"))
		return
	}
	input, err := h.load(r, kind, id)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	record, schema, err := ProjectRead(r.Context(), input, h.maxDepth, h.lang(r), h.languages)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	project := chi.URLParam(r, "projectID")
	scope := auth.ReadScopeFromContext(r.Context())
	editable := !scope.IsRelease() && auth.FromContext(r.Context()).Can(auth.ProjectEdit, auth.ProjectResourceFromContext(r.Context()), nil)
	if record != nil && record.Example.ProjectID != project {
		editable = false
	}
	if mode == formschema.ModeCreate && !editable {
		apierror.Write(w, r, apierror.Forbidden("example creation is not permitted"))
		return
	}
	if editable && mode != formschema.ModeView {
		setExampleEndpoint(schema, project, schema.ExampleID)
	}
	writeJSON(w, http.StatusOK, schema)
}
