package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	overridepkg "github.com/pletka-io/pletka/pkg/weave/override"
)

type overrideSaveRequest struct {
	CommitMessage string                   `json:"commit_message"`
	Categories    []overrideEditorCategory `json:"categories"`
}

type overrideSaveResponse struct {
	Success    bool                     `json:"success"`
	EntityType string                   `json:"entity_type"`
	EntityID   string                   `json:"entity_id"`
	ProjectID  string                   `json:"project_id"`
	Categories []overrideEditorCategory `json:"categories"`
}

type overrideDraftRow struct {
	override domain.FieldOverride
	refs     []domain.OverrideRef
}

func (h *Handler) SaveModelOverrides(w http.ResponseWriter, r *http.Request) {
	h.saveOverrides(w, r, "model", chi.URLParam(r, "modelID"))
}

func (h *Handler) SaveCollectionOverrides(w http.ResponseWriter, r *http.Request) {
	h.saveOverrides(w, r, "collection", chi.URLParam(r, "collectionID"))
}

func (h *Handler) saveOverrides(w http.ResponseWriter, r *http.Request, entityType, entityID string) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" || entityID == "" {
		writeError(w, http.StatusBadRequest, "projectID and entityID are required")
		return
	}

	project, err := h.svc.Get(ctx, projectID)
	if err != nil || project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if !h.svc.CanEdit(ctx, project) {
		writeEditDenied(w, r)
		return
	}

	switch entityType {
	case "model":
		model, err := h.weave.Models().GetByID(ctx, entityID)
		// A project may only save the patterns it owns; another project's
		// model is "not found" here, as in model.Service.requireOwn.
		if err != nil || model == nil || model.ProjectID != projectID {
			writeError(w, http.StatusNotFound, "model not found")
			return
		}
	case "collection":
		collection, err := h.weave.Collections().GetByID(ctx, entityID)
		if err != nil || collection == nil || collection.ProjectID != projectID {
			writeError(w, http.StatusNotFound, "collection not found")
			return
		}
	}

	var req overrideSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid override payload")
		return
	}

	draftRows := flattenOverrideDraft(projectID, entityType, entityID, req.Categories)
	desired := make([]domain.FieldOverride, len(draftRows))
	for i := range draftRows {
		desired[i] = draftRows[i].override
	}

	// Collection placements: model-scoped only. Validate
	// and diff BEFORE the override save so an invalid payload rejects the
	// whole request.
	var placementUpserts []domain.CollectionPlacement
	var placementDeletes []placementKey
	if entityType == "model" {
		existing, err := h.overrides.ListPlacements(ctx, entityID)
		if err != nil {
			h.writeOverrideError(w, err)
			return
		}
		placementUpserts, placementDeletes, err = reconcilePlacements(existing, req.Categories, projectID, entityID)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}

	saved, _, err := h.overrides.SaveForEntity(ctx, projectID, entityType, entityID, desired, req.CommitMessage)
	if err != nil {
		h.writeOverrideError(w, err)
		return
	}

	for i := range placementUpserts {
		if err := h.overrides.UpsertPlacement(ctx, &placementUpserts[i]); err != nil {
			h.writeOverrideError(w, err)
			return
		}
	}
	for _, key := range placementDeletes {
		if err := h.overrides.DeletePlacement(ctx, entityID, key.CategoryID, key.CollectionID); err != nil {
			h.writeOverrideError(w, err)
			return
		}
	}

	// Always call SetRefs — including with an empty slice — so that clearing
	// every expected-ref in the drawer actually deletes the existing refs.
	// The previous "skip when len == 0" path silently left stale refs alive
	// when a curator removed the last expected resource/collection model.
	for i := range saved {
		if err := h.overrides.SetRefs(ctx, projectID, saved[i].ID, draftRows[i].refs); err != nil {
			h.writeOverrideError(w, err)
			return
		}
	}

	var createdByID *string
	if principal := authPrincipalFromContext(r); principal != nil && strings.TrimSpace(principal.ActorID) != "" {
		createdByID = &principal.ActorID
	}
	adoptions := buildAdoptionsFromOverrideCategories(projectID, entityType, entityID, req.Categories, createdByID)
	if err := h.weave.Adoptions().ReplaceForContext(ctx, projectID, entityType, entityID, adoptions); err != nil {
		h.log.Error(
			"sync adoptions failed",
			"project_id", projectID,
			"entity_type", entityType,
			"entity_id", entityID,
			"err", err,
		)
		writeError(w, http.StatusInternalServerError, "failed to sync adoption receipts")
		return
	}

	writeJSON(w, http.StatusOK, overrideSaveResponse{
		Success:    true,
		EntityType: entityType,
		EntityID:   entityID,
		ProjectID:  projectID,
		Categories: req.Categories,
	})
}

func flattenOverrideDraft(projectID, entityType, entityID string, categories []overrideEditorCategory) []overrideDraftRow {
	rows := make([]overrideDraftRow, 0)
	for _, category := range categories {
		for _, item := range category.Items {
			groupPosition := item.Position
			for _, field := range item.Fields {
				row := domain.FieldOverride{
					FieldID:            field.FieldID,
					ProjectID:          projectID,
					EntityType:         entityType,
					EntityID:           entityID,
					Position:           field.Position,
					CollectionOrder:    groupPosition,
					DisplayName:        field.DisplayName,
					Description:        field.Description,
					CategoryID:         normalizeEditorCategoryID(field.CategoryID),
					SetValue:           field.SetValue,
					IsRequired:         field.IsRequired,
					MinOccurs:          field.MinOccurs,
					MaxOccurs:          field.MaxOccurs,
					IsHidden:           field.IsHidden,
					Visibility:         field.Visibility,
					PartOfCollectionID: "",
				}
				if item.Widget == "collection-group" {
					row.PartOfCollectionID = item.ID
					row.CollectionName = item.Name
				}
				if field.OverrideID > 0 {
					row.ID = field.OverrideID
				}
				rows = append(rows, overrideDraftRow{
					override: row,
					refs:     buildOverrideRefs(field),
				})
			}
		}
	}
	return rows
}

func buildOverrideRefs(field overrideEditorField) []domain.OverrideRef {
	refs := make([]domain.OverrideRef, 0, len(field.ExpectedResourceModels)+len(field.ExpectedCollectionModels)+len(field.ExpectedConceptLists))
	for i, id := range field.ExpectedResourceModels {
		refs = append(refs, domain.OverrideRef{
			RefType:    "resource_model",
			TargetID:   id,
			SemanticID: id,
			Position:   i + 1,
		})
	}
	for i, id := range field.ExpectedCollectionModels {
		refs = append(refs, domain.OverrideRef{
			RefType:    "collection_model",
			TargetID:   id,
			SemanticID: id,
			Position:   i + 1,
		})
	}
	for i, id := range field.ExpectedConceptLists {
		refs = append(refs, domain.OverrideRef{
			RefType:    "concept_list",
			TargetID:   id,
			SemanticID: id,
			Position:   i + 1,
		})
	}
	return refs
}

func (h *Handler) writeOverrideError(w http.ResponseWriter, err error) {
	var forbiddenErr *overridepkg.ErrForbidden
	if errors.As(err, &forbiddenErr) {
		writeError(w, http.StatusForbidden, forbiddenErr.Error())
		return
	}
	var validationErr *overridepkg.ErrValidation
	if errors.As(err, &validationErr) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":  "validation error",
			"fields": validationErr.Fields,
		})
		return
	}
	if overridepkg.IsNotFound(err) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	h.log.Error("override save failed", "err", err)
	writeError(w, http.StatusInternalServerError, fmt.Sprintf("internal error: %v", err))
}
