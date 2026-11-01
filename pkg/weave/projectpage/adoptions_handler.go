package projectpage

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

func (h *Handler) loadAdoptions(ctx context.Context, scope auth.ReadScope, project string) (browserinput.Adoptions, error) {
	resource := auth.ProjectResourceFromContext(ctx)
	if resource.ID != project || !auth.FromContext(ctx).Can(auth.ProjectRead, resource, nil) {
		return browserinput.Adoptions{}, apierror.NotFound("project not found")
	}
	if h.source == nil || !scope.Valid() {
		return browserinput.Adoptions{}, fmt.Errorf("adoptions require source and explicit scope")
	}
	value, err := h.source.Adoptions(ctx, scope, project)
	if err != nil {
		return browserinput.Adoptions{}, err
	}
	if value.ProjectID != project || value.Version != scope.Version() {
		return browserinput.Adoptions{}, fmt.Errorf("adoptions returned a different selection")
	}
	return value, nil
}

// ProjectAdoptionsTabSchema projects captured receipts and their selected source closure.
func (h *Handler) ProjectAdoptionsTabSchema(w http.ResponseWriter, r *http.Request) {
	value, err := h.loadAdoptions(r.Context(), auth.ReadScopeFromContext(r.Context()), chi.URLParam(r, "projectID"))
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	schema, err := adoptionSchema(value)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	h.writeSummary(w, r, schema)
}

// ProjectAdoptionClosure expands the same bill of materials used by the tab counts.
func (h *Handler) ProjectAdoptionClosure(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind != "models" && kind != "collections" && kind != "fields" {
		apierror.Write(w, r, apierror.BadRequest("kind must be models, collections or fields"))
		return
	}
	value, err := h.loadAdoptions(r.Context(), auth.ReadScopeFromContext(r.Context()), chi.URLParam(r, "projectID"))
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	for _, a := range value.Items {
		if a.Receipt.SourceProjectID != chi.URLParam(r, "sourceProjectID") || a.Receipt.SourceEntityID != chi.URLParam(r, "sourceEntityID") {
			continue
		}
		items := make([]ProjectAdoptionClosureItem, 0, len(a.Closure[strings.TrimSuffix(kind, "s")]))
		for _, row := range a.Closure[strings.TrimSuffix(kind, "s")] {
			items = append(items, ProjectAdoptionClosureItem{ID: row.ID, SemanticID: row.SemanticID, Name: row.UIName, SourceProjectID: row.ProjectID, CurrentURL: row.CurrentURL, SourceURL: row.SourceURL})
		}
		h.writeSummary(w, r, ProjectAdoptionClosureResponse{Kind: kind, Count: len(items), Items: items})
		return
	}
	if auth.ReadScopeFromContext(r.Context()).IsRelease() {
		apierror.Write(w, r, &apierror.Error{Status: http.StatusNotFound, Code: apierror.CodeNotInRelease, Message: "receipt is not in the selected release"})
		return
	}
	apierror.Write(w, r, apierror.NotFound("receipt not found"))
}
