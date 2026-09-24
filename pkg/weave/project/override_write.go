package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	overridepkg "github.com/pletka-io/pletka/pkg/weave/override"
)

type overrideSaveRequest struct {
	CommitMessage string                   `json:"commit_message"`
	Categories    []overrideEditorCategory `json:"categories"`
	// Fingerprint is the content hash the editor loaded its draft from
	// (see overrideEditorResponse.Fingerprint). Empty skips the staleness
	// check entirely — ops tooling, git restore, and forks all save
	// without ever having loaded an editor payload.
	Fingerprint string `json:"fingerprint,omitempty"`
}

type overrideSaveResponse struct {
	Success    bool                     `json:"success"`
	EntityType string                   `json:"entity_type"`
	EntityID   string                   `json:"entity_id"`
	ProjectID  string                   `json:"project_id"`
	Categories []overrideEditorCategory `json:"categories"`
	// Fingerprint is the entity's new content hash after this save —
	// the caller's next save round-trips it back to detect the next
	// conflict.
	Fingerprint string `json:"fingerprint"`
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

	// Everything from the fingerprint check through the adoption sync runs
	// under the entity's advisory lock, so two saves of the same pattern —
	// including the adoption receipts they leave behind — queue instead
	// of racing (see design doc, Task 3 fix round 1, finding 3:
	// ReplaceForContext is delete-all-then-reinsert with no lock of its
	// own, so it must serialize with the override write it derives from,
	// not run after the lock is released). Ownership/edit checks already
	// happened above the lock — WithEntityLock and EntityFingerprint
	// perform no permission check of their own by design.
	//
	// callbackErr, not the WithEntityLock return value, is what decides
	// success vs. failure below: WithAdvisoryLock joins its own teardown
	// errors (RESET lock_timeout, the unlock) onto whatever the callback
	// returned, so a callback that succeeded but whose connection then
	// failed to tear down cleanly must not be reported as a save failure
	// — the overrides, placements, refs and adoptions are already
	// committed at that point.
	//
	// callbackRan additionally distinguishes "the callback ran and
	// returned nil" from "the callback never ran at all": WithAdvisoryLock
	// has three failure paths before it ever calls the callback (the pool
	// acquire for the lock's own connection, setting lock_timeout on it,
	// and a lock-acquisition error that isn't SQLSTATE 55P03) — none of
	// them are ErrLockBusy, and none of them touch callbackErr, which
	// would otherwise stay at its nil zero value and be misread as "ran
	// and succeeded". Without this flag that misread reports 200 with an
	// empty fingerprint for a save that wrote nothing — and the empty
	// fingerprint then makes the client's NEXT save skip the staleness
	// check entirely (round 1 fix round 2, finding 1).
	var newFingerprint string
	var callbackErr error
	var callbackRan bool
	lockErr := h.overrides.WithEntityLock(ctx, projectID, entityType, entityID, func(ctx context.Context) error {
		callbackRan = true
		callbackErr = func() error {
			if req.Fingerprint != "" {
				current, err := h.overrides.EntityFingerprint(ctx, entityType, entityID)
				if err != nil {
					return err
				}
				if current != req.Fingerprint {
					return &apierror.Error{
						Status:  http.StatusConflict,
						Code:    apierror.CodeConflict,
						Message: "Someone saved this pattern while you were editing.",
						Details: "pattern_changed",
					}
				}
			}

			// Collection placements: model-scoped only. Validate
			// and diff BEFORE the override save so an invalid payload rejects the
			// whole request.
			var placementUpserts []domain.CollectionPlacement
			var placementDeletes []placementKey
			if entityType == "model" {
				existing, err := h.overrides.ListPlacements(ctx, entityID)
				if err != nil {
					return err
				}
				placementUpserts, placementDeletes, err = reconcilePlacements(existing, req.Categories, projectID, entityID)
				if err != nil {
					return &apierror.Error{Status: http.StatusUnprocessableEntity, Message: err.Error()}
				}
			}

			saved, _, err := h.overrides.SaveForEntity(ctx, projectID, entityType, entityID, desired, req.CommitMessage)
			if err != nil {
				return err
			}

			for i := range placementUpserts {
				if err := h.overrides.UpsertPlacement(ctx, &placementUpserts[i]); err != nil {
					return err
				}
			}
			for _, key := range placementDeletes {
				if err := h.overrides.DeletePlacement(ctx, entityID, key.CategoryID, key.CollectionID); err != nil {
					return err
				}
			}

			// Always call SetRefs — including with an empty slice — so that clearing
			// every expected-ref in the drawer actually deletes the existing refs.
			// The previous "skip when len == 0" path silently left stale refs alive
			// when a curator removed the last expected resource/collection model.
			for i := range saved {
				if err := h.overrides.SetRefs(ctx, projectID, saved[i].ID, draftRows[i].refs); err != nil {
					return err
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
				return err
			}

			fp, err := h.overrides.EntityFingerprint(ctx, entityType, entityID)
			if err != nil {
				return err
			}
			newFingerprint = fp
			return nil
		}()
		return callbackErr
	})

	if !callbackRan {
		// The callback never ran at all — nothing was written. Distinct
		// from "lock busy" (someone else holds it, a normal, expected
		// 409) vs. any other acquisition failure (pool exhaustion, a
		// lock_timeout setup failure, a connection error): both must be
		// reported as a failed save, never as success.
		if errors.Is(lockErr, overridepkg.ErrLockBusy) {
			h.log.Warn("override save: lock busy", "entity_type", entityType, "entity_id", entityID, "err", lockErr)
			apierror.Write(w, &apierror.Error{
				Status:  http.StatusConflict,
				Code:    apierror.CodeConflict,
				Message: "Someone else is saving this pattern right now. Try again in a moment.",
				Details: "save_in_progress",
			})
			return
		}
		h.writeOverrideError(w, entityType, entityID, lockErr)
		return
	}
	if callbackErr != nil {
		// The callback ran and failed: classify and respond exactly as
		// before. lockErr still carries callbackErr (possibly joined with
		// a teardown failure on top) — writeOverrideError logs that extra
		// half instead of silently dropping it.
		h.writeOverrideError(w, entityType, entityID, lockErr)
		return
	}
	if lockErr != nil {
		// The callback ran and committed everything successfully; only
		// the lock's own teardown afterwards (RESET lock_timeout or the
		// unlock) failed. That is an operational problem with the
		// connection, not a save failure — proceed to the 200 the client
		// earned, and let an operator correlate any connection churn from
		// this log.
		h.log.Warn("override save: lock teardown failed after a successful save",
			"entity_type", entityType, "entity_id", entityID, "err", lockErr)
	}

	writeJSON(w, http.StatusOK, overrideSaveResponse{
		Success:     true,
		EntityType:  entityType,
		EntityID:    entityID,
		ProjectID:   projectID,
		Categories:  req.Categories,
		Fingerprint: newFingerprint,
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

func (h *Handler) writeOverrideError(w http.ResponseWriter, entityType, entityID string, err error) {
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		// err is more than just apiErr when something else (a lock
		// teardown failure) got errors.Join'd onto the callback's own
		// error — log the whole thing so that half isn't silently
		// dropped, since only apiErr itself reaches the client below.
		var joined interface{ Unwrap() []error }
		if errors.As(err, &joined) {
			h.log.Warn("override save: additional error joined onto the typed API response",
				"entity_type", entityType, "entity_id", entityID, "err", err)
		}
		apierror.Write(w, apiErr)
		return
	}
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
