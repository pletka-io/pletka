package project

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
)

func (h *Handler) OverrideFieldSidebarSchema(w http.ResponseWriter, r *http.Request) {
	if denyReleaseEditorSurface(w, r) {
		return
	}
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "projectID is required")
		return
	}

	project, err := h.svc.Get(ctx, projectID)
	if err != nil || project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	entityType := r.URL.Query().Get("entity_type")
	if entityType != "model" && entityType != "collection" {
		writeError(w, http.StatusBadRequest, "entity_type must be model or collection")
		return
	}
	groupWidget := r.URL.Query().Get("group_widget")
	if groupWidget == "" {
		groupWidget = "field-group"
	}
	if groupWidget != "field-group" && groupWidget != "collection-group" {
		writeError(w, http.StatusBadRequest, "group_widget must be field-group or collection-group")
		return
	}

	expectedValueType := r.URL.Query().Get("expected_value_type")
	canEdit := h.svc.CanEdit(ctx, projectID, project.Visibility)

	schema := formschema.BuildCompositionFieldSidebarSchema(
		&formschema.CompositionFieldSidebarInput{ExpectedValueType: expectedValueType},
		formschema.CompositionFieldSidebarOptions{
			EntityType:         entityType,
			GroupWidget:        groupWidget,
			CanEdit:            canEdit,
			CanHideFields:      entityType == "model",
			CategoryOptions:    h.sidebarCategoryOptions(ctx, projectID, true),
			ModelOptions:       h.sidebarModelOptions(ctx, projectID, expectedValueType),
			CollectionOptions:  h.sidebarCollectionOptions(ctx, projectID, expectedValueType),
			ConceptListOptions: h.sidebarConceptListOptions(ctx, projectID, expectedValueType),
		},
		h.lang(r),
		h.languages,
	)

	writeJSON(w, http.StatusOK, schema)
}

func (h *Handler) OverrideCollectionGroupSidebarSchema(w http.ResponseWriter, r *http.Request) {
	if denyReleaseEditorSurface(w, r) {
		return
	}
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "projectID is required")
		return
	}

	project, err := h.svc.Get(ctx, projectID)
	if err != nil || project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	schema := formschema.BuildCompositionCollectionSidebarSchema(
		nil,
		formschema.CompositionCollectionSidebarOptions{
			CanEdit:         h.svc.CanEdit(ctx, projectID, project.Visibility),
			CategoryOptions: h.sidebarCategoryOptions(ctx, projectID, true),
		},
		h.lang(r),
		h.languages,
	)

	writeJSON(w, http.StatusOK, schema)
}

func (h *Handler) sidebarCategoryOptions(ctx context.Context, projectID string, includeUncategorized bool) []formschema.SelectOption {
	options := make([]formschema.SelectOption, 0, 8)
	if includeUncategorized {
		options = append(options, formschema.SelectOption{
			Value: "",
			Label: i18n.L("category.uncategorized", "Uncategorized"),
		})
	}
	for _, ref := range h.availableCategories(ctx, projectID) {
		options = append(options, formschema.SelectOption{
			Value:      ref.ID,
			Label:      ref.Name,
			Status:     ref.Status,
			SemanticID: ref.SemanticID,
		})
	}
	return options
}

func (h *Handler) sidebarModelOptions(ctx context.Context, projectID, expectedValueType string) []formschema.SelectOption {
	if expectedValueType != "Model" && expectedValueType != "Reference Model" {
		return nil
	}
	chain, err := h.weave.Projects().ProjectChain(ctx, projectID)
	if err != nil || len(chain) == 0 {
		chain = []string{projectID}
	}
	seen := map[string]bool{}
	out := make([]formschema.SelectOption, 0)
	for _, pid := range chain {
		models, mErr := h.weave.Models().ListOptions(ctx, pid)
		if mErr != nil {
			continue
		}
		var sourceLabel string
		if pid != projectID {
			sourceLabel = h.projectLabel(ctx, pid)
		}
		for _, model := range models {
			if seen[model.ID] {
				continue
			}
			seen[model.ID] = true
			opt := formschema.SelectOption{
				Value:      model.ID,
				Label:      model.UIName,
				Status:     model.Status,
				SemanticID: model.SemanticID,
			}
			if pid != projectID {
				opt.SourceProjectID = pid
				opt.SourceProjectLabel = sourceLabel
			}
			out = append(out, opt)
		}
	}
	return out
}

func (h *Handler) sidebarCollectionOptions(ctx context.Context, projectID, expectedValueType string) []formschema.SelectOption {
	if expectedValueType != "Collection" && expectedValueType != "Reference Collection" {
		return nil
	}
	chain, err := h.weave.Projects().ProjectChain(ctx, projectID)
	if err != nil || len(chain) == 0 {
		chain = []string{projectID}
	}
	seen := map[string]bool{}
	out := make([]formschema.SelectOption, 0)
	for _, pid := range chain {
		collections, cErr := h.weave.Collections().ListOptions(ctx, pid)
		if cErr != nil {
			continue
		}
		var sourceLabel string
		if pid != projectID {
			sourceLabel = h.projectLabel(ctx, pid)
		}
		for _, collection := range collections {
			if seen[collection.ID] {
				continue
			}
			seen[collection.ID] = true
			opt := formschema.SelectOption{
				Value:      collection.ID,
				Label:      collection.UIName,
				Status:     collection.Status,
				SemanticID: collection.SemanticID,
			}
			if pid != projectID {
				opt.SourceProjectID = pid
				opt.SourceProjectLabel = sourceLabel
			}
			out = append(out, opt)
		}
	}
	return out
}

func (h *Handler) sidebarConceptListOptions(ctx context.Context, projectID, expectedValueType string) []formschema.SelectOption {
	if expectedValueType != "Concept" {
		return nil
	}
	chain, err := h.weave.Projects().ProjectChain(ctx, projectID)
	if err != nil || len(chain) == 0 {
		chain = []string{projectID}
	}
	seen := map[string]bool{}
	out := make([]formschema.SelectOption, 0)
	for _, pid := range chain {
		lists, lErr := h.weave.ConceptLists().List(ctx, pid)
		if lErr != nil {
			continue
		}
		var sourceLabel string
		if pid != projectID {
			sourceLabel = h.projectLabel(ctx, pid)
		}
		for _, list := range lists {
			if seen[list.ID] {
				continue
			}
			seen[list.ID] = true
			opt := formschema.SelectOption{
				Value:       list.ID,
				Label:       list.UIName,
				Description: list.Description,
				Status:      string(list.Status),
				SemanticID:  list.SemanticID,
			}
			if pid != projectID {
				opt.SourceProjectID = pid
				opt.SourceProjectLabel = sourceLabel
			}
			out = append(out, opt)
		}
	}
	return out
}

// projectLabel resolves the friendly UI name for an ancestor project id,
// used to give chain-walked sidebar options a readable provenance label
// instead of a raw project id. Returns "" if the project cannot be loaded;
// callers fall back to the id.
func (h *Handler) projectLabel(ctx context.Context, projectID string) string {
	project, err := h.weave.Projects().GetByID(ctx, projectID)
	if err != nil || project == nil {
		return ""
	}
	return project.UIName.Get("en", project.ID)
}
