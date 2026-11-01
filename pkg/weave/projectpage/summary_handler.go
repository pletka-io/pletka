package projectpage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

func (h *Handler) loadSummary(ctx context.Context, scope auth.ReadScope, project string) (browserinput.ProjectSummary, error) {
	resource := auth.ProjectResourceFromContext(ctx)
	if resource.ID != project || !auth.FromContext(ctx).Can(auth.ProjectRead, resource, nil) {
		return browserinput.ProjectSummary{}, apierror.NotFound("project not found")
	}
	if h.source == nil || !scope.Valid() {
		return browserinput.ProjectSummary{}, fmt.Errorf("project summary requires a source and explicit scope")
	}
	value, err := h.source.Summary(ctx, scope, project)
	if err != nil {
		return browserinput.ProjectSummary{}, err
	}
	if value.Header.Record.ID != project || value.Header.Record.VersionNumber != scope.Version() {
		return browserinput.ProjectSummary{}, fmt.Errorf("project summary returned a different selection")
	}
	return value, nil
}

// ProjectPageSchema combines selected display content with current navigation rights.
func (h *Handler) ProjectPageSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "projectID")
	scope := auth.ReadScopeFromContext(ctx)
	value, err := h.loadSummary(ctx, scope, id)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	project, err := value.Header.Project()
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	resource := auth.ProjectResourceFromContext(ctx)
	snap := auth.FromContext(ctx)
	// Release catalog and working changes are explicitly current operational data.
	releases, err := h.releases.ListByProject(ctx, id)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	unreleased := 0
	if h.publication != nil && !scope.IsRelease() {
		rollup, err := h.publication.ProjectRollup(ctx, id)
		if err != nil {
			apierror.Write(w, r, apierror.FromError(err))
			return
		}
		unreleased = rollup.Unreleased()
	}
	var warnings []formschema.SettingsWarning
	if !scope.IsRelease() {
		warnings = formschema.ComputeProjectWarnings(id, formschema.ProjectSetupState{HasOntology: len(value.Ontologies) > 0}, snap, resource)
	}
	c := value.Counts
	schema := formschema.BuildProjectPageSchema(project, c.Models, c.Collections, c.Fields, c.Examples, c.ConceptLists, c.Adoptions, len(releases), unreleased, snap.Can(auth.ProjectEdit, resource, nil), snap.IsProjectMember(resource), warnings, h.currentLang(r), h.languages, scope.Version())
	h.writeSummary(w, r, schema)
}

// ProjectOverviewSchema projects one captured summary into the existing schema.
func (h *Handler) ProjectOverviewSchema(w http.ResponseWriter, r *http.Request) {
	value, err := h.loadSummary(r.Context(), auth.ReadScopeFromContext(r.Context()), chi.URLParam(r, "projectID"))
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	schema, err := projectOverview(value, h.currentLang(r), h.languages)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	h.writeSummary(w, r, schema)
}

func (h *Handler) writeSummary(w http.ResponseWriter, r *http.Request, value any) {
	if h.i18n != nil {
		h.i18n.Resolve(value, h.currentLang(r))
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		h.logger.Error("encode project summary", "err", err)
	}
}
