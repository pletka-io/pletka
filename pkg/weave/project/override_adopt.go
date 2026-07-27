package project

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
)

const overrideEditorUncategorizedID = "__uncategorized__"

type adoptCollectionRequest struct {
	CollectionID string `json:"collection_id"`
	CategoryID   string `json:"category_id,omitempty"`
}

type adoptCollectionResponse struct {
	CategoryID string             `json:"category_id"`
	Item       overrideEditorItem `json:"item"`
}

func (h *Handler) AdoptCollectionIntoModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	modelID := chi.URLParam(r, "modelID")
	if projectID == "" || modelID == "" {
		writeError(w, http.StatusBadRequest, "projectID and modelID are required")
		return
	}

	project, err := h.svc.Get(ctx, projectID)
	if err != nil || project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if !h.svc.CanEdit(ctx, projectID, project.Visibility) {
		writeEditDenied(w, r)
		return
	}

	model, err := h.weave.Models().GetByID(ctx, modelID)
	if err != nil || model == nil {
		writeError(w, http.StatusNotFound, "model not found")
		return
	}

	var req adoptCollectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid adopt collection payload")
		return
	}
	if req.CollectionID == "" {
		writeError(w, http.StatusBadRequest, "collection_id is required")
		return
	}

	collection, err := h.weave.Collections().GetByID(ctx, req.CollectionID)
	if err != nil || collection == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}

	sourceProjectID := projectID
	if collection.ProjectID != "" {
		sourceProjectID = collection.ProjectID
	}

	fields, err := h.weave.CollectionView(ctx, collection.ID, sourceProjectID)
	if err != nil {
		h.log.Error(
			"build adopted collection payload",
			"project_id", projectID,
			"model_id", modelID,
			"collection_id", collection.ID,
			"source_project_id", sourceProjectID,
			"err", err,
		)
		writeError(w, http.StatusInternalServerError, "failed to build adopted collection payload")
		return
	}

	categories, err := h.weave.WeaveCategories().List(ctx, domain.WithProjectID(sourceProjectID))
	if err != nil {
		h.log.Error(
			"list categories for adopted collection payload",
			"project_id", projectID,
			"model_id", modelID,
			"collection_id", collection.ID,
			"source_project_id", sourceProjectID,
			"err", err,
		)
		writeError(w, http.StatusInternalServerError, "failed to build adopted collection payload")
		return
	}

	targetCategoryID := normalizeEditorCategoryID(req.CategoryID)
	if targetCategoryID == "" && collection.DefaultCategoryID != nil {
		targetCategoryID = *collection.DefaultCategoryID
	}

	sourceCategories := h.collectionOverrideCategories(ctx, sourceProjectID, fields, categories)
	resp := adoptCollectionResponse{
		CategoryID: denormalizeEditorCategoryID(targetCategoryID),
		Item: buildAdoptedCollectionItem(
			collection,
			targetCategoryID,
			sourceCategories,
			weave.ComputeSharedPathPrefix(fields),
		),
	}

	writeJSON(w, http.StatusOK, resp)
}

func buildAdoptedCollectionItem(
	collection *domain.Collection,
	targetCategoryID string,
	sourceCategories []overrideEditorCategory,
	sharedPrefix []domain.PathElement,
) overrideEditorItem {
	fields := flattenAdoptedCollectionFields(collection.ID, targetCategoryID, sourceCategories)
	return overrideEditorItem{
		Widget:           "collection-group",
		ID:               collection.ID,
		SemanticID:       collection.SemanticID,
		Name:             collectionDisplayName(collection),
		Position:         1,
		FieldCount:       len(fields),
		SharedPathPrefix: sharedPrefix,
		Fields:           fields,
	}
}

func flattenAdoptedCollectionFields(
	collectionID string,
	targetCategoryID string,
	sourceCategories []overrideEditorCategory,
) []overrideEditorField {
	fields := make([]overrideEditorField, 0)
	for _, category := range sourceCategories {
		for _, item := range category.Items {
			for _, field := range item.Fields {
				fields = append(fields, overrideEditorField{
					FieldID:                     field.FieldID,
					OverrideID:                  0,
					Position:                    len(fields) + 1,
					DisplayName:                 field.DisplayName,
					Description:                 field.Description,
					OntologyPath:                field.OntologyPath,
					PathElements:                field.PathElements,
					CategoryID:                  targetCategoryID,
					PartOfCollectionID:          collectionID,
					ExpectedValueType:           field.ExpectedValueType,
					ExpectedResourceModels:      field.ExpectedResourceModels,
					ExpectedCollectionModels:    field.ExpectedCollectionModels,
					ExpectedConceptLists:        field.ExpectedConceptLists,
					ExpectedResourceModelRefs:   field.ExpectedResourceModelRefs,
					ExpectedCollectionModelRefs: field.ExpectedCollectionModelRefs,
					ExpectedConceptListRefs:     field.ExpectedConceptListRefs,
					SetValue:                    field.SetValue,
					IsRequired:                  field.IsRequired,
					MinOccurs:                   field.MinOccurs,
					MaxOccurs:                   field.MaxOccurs,
					IsHidden:                    field.IsHidden,
					Visibility:                  field.Visibility,
				})
			}
		}
	}
	return fields
}

func normalizeEditorCategoryID(categoryID string) string {
	if categoryID == overrideEditorUncategorizedID {
		return ""
	}
	return categoryID
}

func denormalizeEditorCategoryID(categoryID string) string {
	if categoryID == "" {
		return overrideEditorUncategorizedID
	}
	return categoryID
}

func collectionDisplayName(collection *domain.Collection) domain.Translations {
	if collection == nil {
		return nil
	}
	if collection.UIName != nil && collection.UIName.Get("en", "") != "" {
		return collection.UIName
	}
	if collection.SystemName != "" {
		return domain.Translations{"en": collection.SystemName}
	}
	if collection.ID != "" {
		return domain.Translations{"en": collection.ID}
	}
	return nil
}
