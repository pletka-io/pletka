package drafts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// Handler exposes POST /api/v1/drafts.
//
// Dispatches by req.Type to category / model / collection draft creation.
// Always creates the entity in `draft` status with the minimum data
// required to give the user something to select. Promotion to a fully-
// validated entity happens through the entity's normal edit flow, not
// here.
type Handler struct {
	weave domain.WeaveStore
	log   *slog.Logger
}

// NewHandler builds a Handler. nil log → slog.Default.
func NewHandler(weave domain.WeaveStore, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{weave: weave, log: log}
}

// draftRequest is the JSON shape every inline-create call sends.
type draftRequest struct {
	Type          string              `json:"type"`
	ProjectID     string              `json:"project_id"`
	Name          domain.Translations `json:"name"`
	OntologyScope *domain.PathElement `json:"ontology_scope,omitempty"`
}

// draftResponse is what the frontend SelectWidget / PillMultiSelect
// reads to insert the freshly-created option into its local list.
type draftResponse struct {
	Type       string              `json:"type"`
	ID         string              `json:"id"`
	SemanticID string              `json:"semantic_id"`
	SystemName string              `json:"system_name"`
	Name       domain.Translations `json:"name"`
	Status     string              `json:"status"`
}

// validDraftTypes lists the entity kinds the endpoint can mint. Each is
// dispatched to its own create helper below.
var validDraftTypes = map[string]bool{
	"category":     true,
	"model":        true,
	"collection":   true,
	"concept-list": true,
}

// capabilityFor returns the auth.Capability required to mint a draft of
// the given type. Categories are organisational so project-edit is enough;
// model + collection drafts touch the ontology surface and require the
// type-specific create capability.
func capabilityFor(kind string) auth.Capability {
	switch kind {
	case "model":
		return auth.ModelCreate
	case "collection":
		return auth.CollectionCreate
	default:
		return auth.ProjectEdit
	}
}

// Create handles POST /api/v1/drafts.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	snap := auth.FromContext(ctx)
	if snap == nil || snap.IsAnonymous {
		apierror.Write(w, apierror.Unauthorized())
		return
	}

	var req draftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid JSON body"))
		return
	}

	errs := map[string][]string{}
	if req.Type == "" {
		errs["type"] = append(errs["type"], "type is required (one of: category, model, collection, concept-list)")
	} else if !validDraftTypes[req.Type] {
		errs["type"] = append(errs["type"], fmt.Sprintf("unsupported type %q", req.Type))
	}
	if req.ProjectID == "" {
		errs["project_id"] = append(errs["project_id"], "project_id is required")
	}
	if len(req.Name) == 0 || strings.TrimSpace(req.Name.Get("en", "")) == "" {
		errs["name"] = append(errs["name"], "name (with at least an 'en' value) is required")
	}
	if len(errs) > 0 {
		apierror.Write(w, apierror.Validation(errs))
		return
	}

	// Project must exist; weave_projects.id is the prefix used for IDs.
	project, err := h.weave.Projects().GetByID(ctx, req.ProjectID)
	if err != nil || project == nil {
		apierror.Write(w, apierror.NotFound("project not found"))
		return
	}

	resource := auth.Resource{ScopeType: "project", ID: project.ID, Visibility: project.Visibility}
	if !snap.Can(capabilityFor(req.Type), resource, nil) {
		apierror.Write(w, apierror.Forbidden("insufficient permissions"))
		return
	}

	// Setup gate (model + collection only). Categories don't need
	// ontology coverage; they're project-internal grouping.
	if req.Type == "model" || req.Type == "collection" {
		resolved, _ := h.weave.Projects().ResolvedOntologyVersions(ctx, req.ProjectID, domain.ResolvedOntologyVersionOpts{})
		if len(resolved) == 0 {
			// Slice-specific envelope — frontend reads error_code +
			// settings_url to render a "configure ontology" link.
			// Mirrors model + collection slice setup-incomplete shape.
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"code":         "no_ontologies",
				"error":        "no ontologies configured",
				"error_code":   "NO_ONTOLOGIES",
				"message":      "Configure at least one ontology before creating drafts of this type.",
				"settings_url": "/projects/" + req.ProjectID + "/settings#ontology",
			})
			return
		}
	}

	// Case-insensitive name dedup within (project, type).
	if conflict, cerr := h.draftNameInUse(ctx, req.Type, req.ProjectID, req.Name.Get("en", "")); cerr != nil {
		h.log.Warn("draft dedup check failed", "type", req.Type, "project_id", req.ProjectID, "err", cerr)
	} else if conflict {
		ae := apierror.Conflict("duplicate name")
		ae.Details = fmt.Sprintf("A %s named %q already exists in this project", req.Type, req.Name.Get("en", ""))
		apierror.Write(w, ae)
		return
	}

	// Allocate the next sequential number race-free via the shared
	// counter. Concept-list's counter kind uses an underscore in
	// existing schema (`concept_list`) so normalise the hyphen form
	// the drafts API exposes here.
	counterKind := req.Type
	if counterKind == "concept-list" {
		counterKind = "concept_list"
	}
	nextN, err := h.weave.AllocateEntityNumber(ctx, project.ID, counterKind)
	if err != nil {
		h.log.Error("allocate draft number", "type", req.Type, "project_id", project.ID, "err", err)
		apierror.Write(w, apierror.Internal())
		return
	}

	systemName := slugifyName(req.Name.Get("en", req.Type))

	var resp *draftResponse
	switch req.Type {
	case "category":
		resp, err = h.createCategoryDraft(ctx, project.ID, nextN, systemName, req.Name)
	case "model":
		resp, err = h.createModelDraft(ctx, project.ID, nextN, systemName, req.Name, req.OntologyScope)
	case "collection":
		resp, err = h.createCollectionDraft(ctx, project.ID, nextN, systemName, req.Name, req.OntologyScope)
	case "concept-list":
		resp, err = h.createConceptListDraft(ctx, project.ID, nextN, systemName, req.Name)
	}
	if err != nil {
		h.log.Error("draft create failed", "type", req.Type, "project_id", project.ID, "err", err)
		apierror.Write(w, apierror.Internal())
		return
	}

	h.log.Info("draft created", "type", req.Type, "id", resp.ID, "project_id", project.ID, "actor_id", snap.ActorID)
	writeJSON(w, http.StatusCreated, resp)
}

// draftNameInUse case-insensitively scans the existing entities of the
// given type for a name match in english. Returns (true, nil) on hit.
// Errors propagate so the caller can choose to log or fail.
func (h *Handler) draftNameInUse(ctx context.Context, kind, projectID, name string) (bool, error) {
	target := strings.TrimSpace(strings.ToLower(name))
	if target == "" {
		return false, nil
	}
	switch kind {
	case "category":
		cats, err := h.weave.WeaveCategories().List(ctx, domain.WithProjectID(projectID))
		if err != nil {
			return false, err
		}
		for _, c := range cats {
			if strings.TrimSpace(strings.ToLower(c.UIName.Get("en", ""))) == target {
				return true, nil
			}
		}
	case "model":
		models, _, err := h.weave.Models().List(ctx, domain.WithProjectID(projectID))
		if err != nil {
			return false, err
		}
		for _, m := range models {
			if strings.TrimSpace(strings.ToLower(m.UIName.Get("en", ""))) == target {
				return true, nil
			}
		}
	case "collection":
		colls, _, err := h.weave.Collections().List(ctx, domain.WithProjectID(projectID))
		if err != nil {
			return false, err
		}
		for _, c := range colls {
			if strings.TrimSpace(strings.ToLower(c.UIName.Get("en", ""))) == target {
				return true, nil
			}
		}
	case "concept-list":
		lists, err := h.weave.ConceptLists().List(ctx, projectID)
		if err != nil {
			return false, err
		}
		for _, l := range lists {
			if strings.TrimSpace(strings.ToLower(l.UIName.Get("en", ""))) == target {
				return true, nil
			}
		}
	}
	return false, nil
}

func (h *Handler) createCategoryDraft(ctx context.Context, projectID string, n int64, systemName string, name domain.Translations) (*draftResponse, error) {
	semanticID := fmt.Sprintf("%s.CAT.%d", projectID, n)
	cat := &domain.Category{
		Entity: domain.Entity{
			ID:         semanticID,
			SemanticID: semanticID,
			SystemName: systemName,
			UIName:     name,
			Status:     "draft",
			ProjectID:  projectID,
		},
		// CanonicalOrder set to n is good-enough default — the UI lets
		// users reorder later. New drafts go to the end.
		CanonicalOrder: int(n),
	}
	if err := h.weave.WeaveCategories().Create(ctx, cat); err != nil {
		return nil, err
	}
	return &draftResponse{
		Type: "category", ID: cat.ID, SemanticID: cat.SemanticID,
		SystemName: cat.SystemName, Name: cat.UIName, Status: string(cat.Status),
	}, nil
}

func (h *Handler) createModelDraft(ctx context.Context, projectID string, n int64, systemName string, name domain.Translations, scope *domain.PathElement) (*draftResponse, error) {
	semanticID := fmt.Sprintf("%sM.%d", projectID, n)
	m := &domain.Model{
		Entity: domain.Entity{
			ID:         semanticID,
			SemanticID: semanticID,
			SystemName: systemName,
			UIName:     name,
			Status:     "draft",
			ProjectID:  projectID,
		},
	}
	if scope != nil && scope.LocalName != "" {
		s := *scope
		if s.Type == "" {
			s.Type = "class"
		}
		m.OntologyScope = s
	}
	if err := h.weave.Models().Create(ctx, m); err != nil {
		return nil, err
	}
	return &draftResponse{
		Type: "model", ID: m.ID, SemanticID: m.SemanticID,
		SystemName: m.SystemName, Name: m.UIName, Status: string(m.Status),
	}, nil
}

func (h *Handler) createConceptListDraft(ctx context.Context, projectID string, n int64, systemName string, name domain.Translations) (*draftResponse, error) {
	semanticID := fmt.Sprintf("%s.CL.%d", projectID, n)
	list := &domain.ConceptList{
		Entity: domain.Entity{
			ID:         semanticID,
			SemanticID: semanticID,
			SystemName: systemName,
			UIName:     name,
			Status:     "draft",
			ProjectID:  projectID,
		},
	}
	if err := h.weave.ConceptLists().Create(ctx, list); err != nil {
		return nil, err
	}
	return &draftResponse{
		Type: "concept-list", ID: list.ID, SemanticID: list.SemanticID,
		SystemName: list.SystemName, Name: list.UIName, Status: string(list.Status),
	}, nil
}

func (h *Handler) createCollectionDraft(ctx context.Context, projectID string, n int64, systemName string, name domain.Translations, scope *domain.PathElement) (*draftResponse, error) {
	semanticID := fmt.Sprintf("%sC.%d", projectID, n)
	c := &domain.Collection{
		Entity: domain.Entity{
			ID:         semanticID,
			SemanticID: semanticID,
			SystemName: systemName,
			UIName:     name,
			Status:     "draft",
			ProjectID:  projectID,
		},
		CollectionNumber: int(n),
	}
	if scope != nil && scope.LocalName != "" {
		s := *scope
		if s.Type == "" {
			s.Type = "class"
		}
		c.OntologyScope = s
	}
	if err := h.weave.Collections().Create(ctx, c); err != nil {
		return nil, err
	}
	return &draftResponse{
		Type: "collection", ID: c.ID, SemanticID: c.SemanticID,
		SystemName: c.SystemName, Name: c.UIName, Status: string(c.Status),
	}, nil
}

// writeJSON serializes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// slugifyName produces a snake_case slug from a free-text name.
func slugifyName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	return value
}
