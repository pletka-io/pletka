package attribution

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/formschema"
)

// Handler exposes the HTTP surface of the attribution slice.
//
// Routes (mounted under /projects/{projectID}/settings/attributions):
//
//	GET    /                                          List rows
//	POST   /                                          Create row
//	GET    /list-schema                               Curator list schema
//	GET    /form-schema                               Curator create form schema
//	PATCH  /reorder                                   Reorder within a kind
//	PATCH  /{actorID}/{kind}/{position}               Edit note
//	DELETE /{actorID}/{kind}/{position}               Remove row
type Handler struct {
	svc       *Service
	log       *slog.Logger
	languages []formschema.LanguageInfo
}

// NewHandler wires a Handler. nil log → slog.Default.
func NewHandler(svc *Service, log *slog.Logger, languages []formschema.LanguageInfo) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{svc: svc, log: log, languages: languages}
}

// List handles GET /projects/{projectID}/settings/attributions.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	rows, err := h.svc.ListForProject(ctx, projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"attributions": rows})
}

// Create handles POST /projects/{projectID}/settings/attributions.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	var body struct {
		ActorID string `json:"actor_id"`
		Kind    string `json:"kind"`
		Note    string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeValidationError(w, map[string][]string{"body": {"invalid JSON body"}})
		return
	}
	row, err := h.svc.Create(ctx, projectID, Input{
		ActorID: body.ActorID,
		Kind:    body.Kind,
		Note:    body.Note,
	})
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

// UpdateNote handles PATCH /projects/{projectID}/settings/attributions/{actorID}/{kind}/{position}.
func (h *Handler) UpdateNote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	actorID := chi.URLParam(r, "actorID")
	kind := chi.URLParam(r, "kind")
	position, perr := strconv.Atoi(chi.URLParam(r, "position"))
	if actorID == "" || kind == "" || perr != nil {
		writeValidationError(w, map[string][]string{"path": {"actorID, kind and position are required"}})
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeValidationError(w, map[string][]string{"body": {"invalid JSON body"}})
		return
	}
	if err := h.svc.UpdateNote(ctx, projectID, actorID, kind, position, body.Note); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete handles DELETE /projects/{projectID}/settings/attributions/{actorID}/{kind}/{position}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	actorID := chi.URLParam(r, "actorID")
	kind := chi.URLParam(r, "kind")
	position, perr := strconv.Atoi(chi.URLParam(r, "position"))
	if actorID == "" || kind == "" || perr != nil {
		writeValidationError(w, map[string][]string{"path": {"actorID, kind and position are required"}})
		return
	}
	if err := h.svc.Delete(ctx, projectID, actorID, kind, position); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Reorder handles PATCH /projects/{projectID}/settings/attributions/reorder.
func (h *Handler) Reorder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	var body struct {
		Kind            string   `json:"kind"`
		ActorIDsInOrder []string `json:"actor_ids_in_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeValidationError(w, map[string][]string{"body": {"invalid JSON body"}})
		return
	}
	body.Kind = strings.TrimSpace(body.Kind)
	if err := h.svc.Reorder(ctx, projectID, body.Kind, body.ActorIDsInOrder); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListSchema handles GET /projects/{projectID}/settings/attributions/list-schema.
func (h *Handler) ListSchema(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	lang := r.URL.Query().Get("lang")
	writeJSON(w, http.StatusOK, BuildSettingsListSchema(projectID, lang, h.languages))
}

// FormSchema handles GET /projects/{projectID}/settings/attributions/form-schema.
func (h *Handler) FormSchema(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	lang := r.URL.Query().Get("lang")
	writeJSON(w, http.StatusOK, BuildSettingsCreateSchema(projectID, lang, h.languages))
}

// OptionsActors handles
//
//	GET /projects/{projectID}/settings/attributions/options/actors
//
// Returns every actor in the system as {value, label} entries for the
// create form's actor search-select. Gated on ProjectEdit.
func (h *Handler) OptionsActors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	rows, err := h.svc.ListAllActors(ctx, projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	// Sort organisations to the top so curators can spot the
	// institutional options immediately. Within each type, sort
	// alphabetically by display name.
	sort.SliceStable(rows, func(i, j int) bool {
		isOrgI := rows[i].Type == "organization"
		isOrgJ := rows[j].Type == "organization"
		if isOrgI != isOrgJ {
			return isOrgI
		}
		return strings.ToLower(rows[i].DisplayName) < strings.ToLower(rows[j].DisplayName)
	})
	opts := make([]map[string]any, 0, len(rows))
	for _, a := range rows {
		// Clearly mark organisations so curators don't confuse them
		// with same-named persons. Persons get an "@slug" suffix;
		// orgs get a leading 🏛 badge plus an explicit "(organisation)"
		// suffix.
		var label string
		switch a.Type {
		case "organization":
			label = "🏛 " + a.DisplayName + " (organisation)"
		default:
			label = a.DisplayName
			if a.Slug != "" {
				label = label + " (@" + a.Slug + ")"
			}
		}
		opts = append(opts, map[string]any{
			"value": a.ID,
			"label": map[string]string{"en": label},
		})
	}
	writeJSON(w, http.StatusOK, opts)
}

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	var verr *ValidationError
	switch {
	case errors.As(err, &verr):
		writeValidationError(w, verr.Errors)
	case errors.Is(err, ErrNotFound):
		http.Error(w, "project not found", http.StatusNotFound)
	case errors.Is(err, ErrForbidden):
		http.Error(w, "forbidden", http.StatusForbidden)
	default:
		h.log.Error("attribution handler error", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeValidationError(w http.ResponseWriter, errs map[string][]string) {
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
		"error":   "validation error",
		"errors":  errs,
		"message": "Please correct the highlighted fields and try again.",
	})
}
