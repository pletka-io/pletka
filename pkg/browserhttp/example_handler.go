package browserhttp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// DetailSource returns a bounded detached entity, with no fallback or lazy reads.
type DetailSource interface {
	Detail(context.Context, auth.ReadScope, string, string, string) (browserinput.Detail, error)
}

// ExampleHandler has no editing/validation service: all display reads use its source.
type ExampleHandler struct{ Source DetailSource }

// Show serves recorded example values at the selected scope.
func (h ExampleHandler) Show(w http.ResponseWriter, r *http.Request) {
	project, id := chi.URLParam(r, "projectID"), chi.URLParam(r, "exampleID")
	scope := auth.ReadScopeFromContext(r.Context())
	resource := auth.ProjectResourceFromContext(r.Context())
	if project == "" || resource.ID != project || !auth.FromContext(r.Context()).Can(auth.ProjectRead, resource, nil) {
		apierror.Write(w, r, apierror.NotFound("project not found"))
		return
	}
	if h.Source == nil || !scope.Valid() {
		apierror.Write(w, r, apierror.Internal(fmt.Errorf("example view requires a source and explicit scope")))
		return
	}
	input, err := h.Source.Detail(r.Context(), scope, project, catalogExample, id)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	consumer, err := input.Fragment.Consumer()
	root, rootErr := input.Fragment.Root()
	if err != nil || rootErr != nil || consumer.ProjectID() != project || consumer.Version() != scope.Version() || root.ID != id || root.Kind != catalogExample {
		apierror.Write(w, r, apierror.Internal(fmt.Errorf("example view returned a different selection")))
		return
	}
	value, err := ProjectExample(input)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	write(w, value)
}
