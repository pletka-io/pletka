package project

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	overridepkg "github.com/pletka-io/pletka/pkg/weave/override"
)

// entityTypeModel and entityTypeCollection name the two entity types a
// pattern's presence endpoint covers — mirrors the (unexported, ad hoc)
// literals saveOverrides already switches on in override_write.go.
const (
	entityTypeModel      = "model"
	entityTypeCollection = "collection"
)

// presenceRequest is the heartbeat body the editor posts every 20s while
// open, and once more (best effort) on unmount/pagehide with state "left".
type presenceRequest struct {
	State string `json:"state"`
}

// presenceResponse always carries a non-nil Editors slice so the JSON body
// is "editors":[] rather than "editors":null when there is no one else.
type presenceResponse struct {
	Editors []overridepkg.Editor `json:"editors"`
}

// ModelOverridesPresence handles POST
// /projects/{projectID}/models/{modelID}/overrides/presence.
func (h *Handler) ModelOverridesPresence(w http.ResponseWriter, r *http.Request) {
	h.overridesPresence(w, r, entityTypeModel, chi.URLParam(r, "modelID"))
}

// CollectionOverridesPresence handles POST
// /projects/{projectID}/collections/{collectionID}/overrides/presence.
func (h *Handler) CollectionOverridesPresence(w http.ResponseWriter, r *http.Request) {
	h.overridesPresence(w, r, entityTypeCollection, chi.URLParam(r, "collectionID"))
}

// overridesPresence is a notice, never a lock: it records that the caller
// is editing (entityType, entityID) and reports who else is, or — for
// {"state":"left"} — drops the caller's own entry. It gates on the same
// project + ownership checks as saveOverrides (see override_write.go), but
// diverges on the edit check itself: a caller who fails it is not an
// error here, unlike the save. They get {"editors":[]} back and nothing is
// recorded for them, exactly like a viewer just watching.
func (h *Handler) overridesPresence(w http.ResponseWriter, r *http.Request, entityType, entityID string) {
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

	switch entityType {
	case entityTypeModel:
		model, err := h.weave.Models().GetByID(ctx, entityID)
		// Same ownership rule as saveOverrides: another project's model
		// or collection is "not found" here, never a different error.
		if err != nil || model == nil || model.ProjectID != projectID {
			writeError(w, http.StatusNotFound, "model not found")
			return
		}
	case entityTypeCollection:
		collection, err := h.weave.Collections().GetByID(ctx, entityID)
		if err != nil || collection == nil || collection.ProjectID != projectID {
			writeError(w, http.StatusNotFound, "collection not found")
			return
		}
	}

	actorID := auth.FromContext(ctx).ActorID
	if actorID == "" || !h.svc.CanEdit(ctx, project) {
		// Anonymous, session-lapsed, or a viewer without edit rights:
		// presence is only for actors who could actually save this
		// pattern. No entry recorded, nothing reported back.
		writeJSON(w, http.StatusOK, presenceResponse{Editors: []overridepkg.Editor{}})
		return
	}

	var req presenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid presence payload")
		return
	}

	if req.State == "left" {
		h.presence.Leave(entityType, entityID, actorID)
		writeJSON(w, http.StatusOK, presenceResponse{Editors: []overridepkg.Editor{}})
		return
	}

	now := time.Now()
	h.presence.Beat(entityType, entityID, actorID, now)
	others := h.presence.Others(entityType, entityID, actorID, now)
	editors := make([]overridepkg.Editor, len(others))
	for i, o := range others {
		editors[i] = overridepkg.Editor{
			ActorID: o.ActorID,
			Name:    h.actorDisplayName(ctx, o.ActorID),
			Since:   o.Since,
		}
	}
	writeJSON(w, http.StatusOK, presenceResponse{Editors: editors})
}

// actorDisplayName resolves actorID's display name for the presence
// response. domain.WeaveStore has no dedicated actor-lookup method
// (no Actors() in the interface) — the nearest equivalent is the auth
// store's actor-profile join, which works for any actor id, not just the
// caller's own. Falls back to the actor id itself when the lookup errors,
// finds nothing, or the display name is blank.
func (h *Handler) actorDisplayName(ctx context.Context, actorID string) string {
	profile, err := h.weave.Auth().GetProfileByActorID(ctx, actorID)
	if err != nil || profile == nil || profile.DisplayName == "" {
		return actorID
	}
	return profile.DisplayName
}
