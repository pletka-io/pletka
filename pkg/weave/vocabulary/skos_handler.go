package vocabulary

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// SKOSExporter prepares a complete export through the shared generator boundary.
type SKOSExporter interface {
	GenerateConceptListSKOS(context.Context, auth.ReadScope, string, string, io.Writer) error
}

// The export handler has no editing service or store from which to load content.
type skosHandler struct{ exporter SKOSExporter }

func (h skosHandler) serve(w http.ResponseWriter, r *http.Request) {
	project, id := chi.URLParam(r, "projectID"), chi.URLParam(r, "listID")
	current, scope := auth.ProjectFromContext(r.Context()), auth.ReadScopeFromContext(r.Context())
	if current == nil || current.ID != project {
		apierror.Write(w, r, apierror.NotFound("project not found"))
		return
	}
	if h.exporter == nil || !scope.Valid() {
		apierror.Write(w, r, apierror.Internal(fmt.Errorf("SKOS requires a source and explicit scope")))
		return
	}
	var buf bytes.Buffer
	if err := h.exporter.GenerateConceptListSKOS(r.Context(), scope, project, id, &buf); err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	w.Header().Set("Content-Type", "text/turtle; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", id+".ttl"))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
