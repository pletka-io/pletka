package project

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

func (h *Handler) Adoptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if strings.TrimSpace(projectID) == "" {
		writeError(w, http.StatusBadRequest, "projectID is required")
		return
	}

	opts := []domain.QueryOption{domain.WithProjectID(projectID)}
	if version := weaveauth.ProjectVersionFromContext(ctx); version != "" {
		opts = append(opts, domain.WithVersion(version))
	}
	for _, key := range []string{"context_entity_type", "context_entity_id", "entity_type", "source_project_id", "source_entity_id"} {
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
			opts = append(opts, domain.WithFilter(key, value))
		}
	}

	adoptions, err := h.weave.Adoptions().List(ctx, opts...)
	if err != nil {
		h.log.Error("list adoptions failed", "project_id", projectID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list adoptions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"adoptions": adoptions,
	})
}

func buildAdoptionsFromOverrideCategories(
	projectID string,
	contextEntityType string,
	contextEntityID string,
	categories []overrideEditorCategory,
	createdByID *string,
) []domain.Adoption {
	adoptions := make([]domain.Adoption, 0)
	seen := map[string]struct{}{}

	for _, category := range categories {
		for _, item := range category.Items {
			switch item.Widget {
			case "collection-group":
				source := domain.ParseSemanticID(item.ID)
				if source.Valid() && source.ProjectID != "" && source.ProjectID != projectID {
					adoption := domain.Adoption{
						ProjectID:         projectID,
						ContextEntityType: contextEntityType,
						ContextEntityID:   contextEntityID,
						EntityType:        "collection",
						SourceProjectID:   source.ProjectID,
						SourceEntityID:    source.ID(),
						CreatedByID:       createdByID,
					}
					key := adoptionReceiptKey(adoption)
					if _, ok := seen[key]; !ok {
						seen[key] = struct{}{}
						adoptions = append(adoptions, adoption)
					}
				}
			default:
				for _, field := range item.Fields {
					source := domain.ParseSemanticID(field.FieldID)
					if !source.Valid() || source.ProjectID == "" || source.ProjectID == projectID {
						continue
					}
					adoption := domain.Adoption{
						ProjectID:         projectID,
						ContextEntityType: contextEntityType,
						ContextEntityID:   contextEntityID,
						EntityType:        "field",
						SourceProjectID:   source.ProjectID,
						SourceEntityID:    source.ID(),
						CreatedByID:       createdByID,
					}
					key := adoptionReceiptKey(adoption)
					if _, ok := seen[key]; ok {
						continue
					}
					seen[key] = struct{}{}
					adoptions = append(adoptions, adoption)
				}
			}
		}
	}

	return adoptions
}

func adoptionReceiptKey(a domain.Adoption) string {
	return strings.Join([]string{
		a.ContextEntityType,
		a.ContextEntityID,
		a.EntityType,
		a.SourceProjectID,
		a.SourceEntityID,
		a.SourceVersion,
	}, "|")
}

type adoptableOption struct {
	Value             string              `json:"value"`
	Label             domain.Translations `json:"label"`
	SemanticID        string              `json:"semantic_id,omitempty"`
	SourceProjectID   string              `json:"source_project_id"`
	SourceProjectName string              `json:"source_project_name,omitempty"`
	OntologyScope     string              `json:"ontology_scope,omitempty"`
}

// AdoptableEntities handles
//
//	GET /projects/{projectID}/adoptable/{entityType}
//
// Returns candidate models or collections from the project's ancestor
// chain that aren't already local to the project and haven't been
// adopted yet. Drives the Adopt-existing side panel.
func (h *Handler) AdoptableEntities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	entityType := chi.URLParam(r, "entityType")
	if projectID == "" || entityType == "" {
		writeError(w, http.StatusBadRequest, "projectID and entityType are required")
		return
	}
	switch entityType {
	case "model", "collection":
	default:
		writeError(w, http.StatusBadRequest, "entityType must be model or collection")
		return
	}

	chain, err := h.weave.Projects().ProjectChain(ctx, projectID)
	if err != nil || len(chain) == 0 {
		chain = []string{projectID}
	}

	existingAdoptions, err := h.weave.Adoptions().List(ctx,
		domain.WithProjectID(projectID),
		domain.WithFilter("context_entity_type", "project"),
		domain.WithFilter("context_entity_id", projectID),
		domain.WithFilter("entity_type", entityType),
	)
	if err != nil {
		h.log.Error("list adoptions for adoptable options", "project_id", projectID, "entity_type", entityType, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load existing adoptions")
		return
	}
	adoptedKey := func(sourceProjectID, sourceEntityID string) string {
		return sourceProjectID + "|" + sourceEntityID
	}
	adopted := make(map[string]bool, len(existingAdoptions))
	for _, a := range existingAdoptions {
		adopted[adoptedKey(a.SourceProjectID, a.SourceEntityID)] = true
	}

	projectNames := make(map[string]string, len(chain))
	for _, pid := range chain {
		if p, perr := h.weave.Projects().GetByID(ctx, pid); perr == nil && p != nil {
			projectNames[pid] = p.UIName.Get("en", p.ID)
		}
	}

	opts := make([]adoptableOption, 0)
	seen := map[string]bool{}
	for _, pid := range chain {
		if pid == projectID {
			// Skip own project — adopting your own row is a no-op.
			continue
		}
		switch entityType {
		case "model":
			models, _, lerr := h.weave.Models().List(ctx, domain.WithProjectID(pid))
			if lerr != nil {
				h.log.Warn("list models for adoptable options", "project_id", projectID, "ancestor", pid, "err", lerr)
				continue
			}
			for _, m := range models {
				if seen[m.ID] || adopted[adoptedKey(pid, m.ID)] {
					continue
				}
				seen[m.ID] = true
				opts = append(opts, adoptableOption{
					Value:             m.ID,
					Label:             m.UIName,
					SemanticID:        m.SemanticID,
					SourceProjectID:   pid,
					SourceProjectName: projectNames[pid],
					OntologyScope:     m.OntologyScope.PrefixedName(),
				})
			}
		case "collection":
			collections, _, lerr := h.weave.Collections().List(ctx, domain.WithProjectID(pid))
			if lerr != nil {
				h.log.Warn("list collections for adoptable options", "project_id", projectID, "ancestor", pid, "err", lerr)
				continue
			}
			for _, c := range collections {
				if seen[c.ID] || adopted[adoptedKey(pid, c.ID)] {
					continue
				}
				seen[c.ID] = true
				opts = append(opts, adoptableOption{
					Value:             c.ID,
					Label:             c.UIName,
					SemanticID:        c.SemanticID,
					SourceProjectID:   pid,
					SourceProjectName: projectNames[pid],
					OntologyScope:     c.OntologyScope.PrefixedName(),
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"options": opts})
}

// CreateProjectAdoption handles
//
//	POST /projects/{projectID}/adoptions
//
// Appends an adoption receipt for a single source entity. Reads
// existing adoptions, appends the new row, and writes back via
// ReplaceForContext (the AdoptionStore's append semantics). Drives the
// Adopt-existing side-panel submit.
func (h *Handler) CreateProjectAdoption(w http.ResponseWriter, r *http.Request) {
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
	if !h.svc.CanEdit(ctx, projectID, project.Visibility) {
		writeEditDenied(w, r)
		return
	}

	var body struct {
		EntityType      string `json:"entity_type"`
		SourceProjectID string `json:"source_project_id"`
		SourceEntityID  string `json:"source_entity_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.EntityType = strings.TrimSpace(body.EntityType)
	body.SourceProjectID = strings.TrimSpace(body.SourceProjectID)
	body.SourceEntityID = strings.TrimSpace(body.SourceEntityID)
	if body.EntityType == "" || body.SourceProjectID == "" || body.SourceEntityID == "" {
		writeError(w, http.StatusBadRequest, "entity_type, source_project_id, and source_entity_id are required")
		return
	}
	switch body.EntityType {
	case "model", "collection":
	default:
		writeError(w, http.StatusBadRequest, "entity_type must be model or collection")
		return
	}
	if body.SourceProjectID == projectID {
		writeError(w, http.StatusBadRequest, "cannot adopt own entity")
		return
	}

	var actorID *string
	if snap := weaveauth.FromContext(ctx); snap != nil && snap.ActorID != "" {
		id := snap.ActorID
		actorID = &id
	}

	existing, err := h.weave.Adoptions().List(ctx,
		domain.WithProjectID(projectID),
		domain.WithFilter("context_entity_type", "project"),
		domain.WithFilter("context_entity_id", projectID),
	)
	if err != nil {
		h.log.Error("list existing adoptions", "project_id", projectID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load existing adoptions")
		return
	}
	newRow := domain.Adoption{
		ProjectID:         projectID,
		ContextEntityType: "project",
		ContextEntityID:   projectID,
		EntityType:        body.EntityType,
		SourceProjectID:   body.SourceProjectID,
		SourceEntityID:    body.SourceEntityID,
		CreatedByID:       actorID,
	}
	key := adoptionReceiptKey(newRow)
	for _, a := range existing {
		if adoptionReceiptKey(a) == key {
			writeError(w, http.StatusConflict, "already adopted")
			return
		}
	}
	merged := append(existing[:0:0], existing...)
	merged = append(merged, newRow)
	if err := h.weave.Adoptions().ReplaceForContext(ctx, projectID, "project", projectID, merged); err != nil {
		h.log.Error("replace adoptions", "project_id", projectID, "err", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to record adoption: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"adoption": newRow,
	})
}
