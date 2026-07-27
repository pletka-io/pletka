package project

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	overridepkg "github.com/pletka-io/pletka/pkg/weave/override"
)

// authPrincipalFromContext is a thin wrapper so tests can stub the
// auth lookup if needed. Today it just calls auth.PrincipalFromContext.
var authPrincipalFromContext = func(r *http.Request) *auth.Principal {
	return auth.PrincipalFromContext(r.Context())
}

// LangResolver returns the caller's preferred UI language for a request.
type LangResolver func(r *http.Request) string

// Handler exposes the Project slice's read endpoints.
type Handler struct {
	svc       *Service
	overrides *overridepkg.Service
	weave     domain.WeaveStore
	log       *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
}

// NewHandler constructs a Handler. nil log → slog.Default; nil lang → "en".
func NewHandler(svc *Service, overrides *overridepkg.Service, weave domain.WeaveStore, log *slog.Logger, languages []formschema.LanguageInfo, lang LangResolver) *Handler {
	if log == nil {
		log = slog.Default()
	}
	if lang == nil {
		lang = func(*http.Request) string { return "en" }
	}
	return &Handler{svc: svc, overrides: overrides, weave: weave, log: log, languages: languages, lang: lang}
}

// ---------------------------------------------------------------------------
// Read endpoints
// ---------------------------------------------------------------------------

// Data handles GET /projects/data — paginated, filtered list with stats
// and per-row "incomplete" badge for editable rows.
func (h *Handler) Data(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	search := r.URL.Query().Get("search")
	sortBy := r.URL.Query().Get("sort_by")
	if sortBy == "" {
		sortBy = "ui_name"
	}
	sortDir := r.URL.Query().Get("sort_dir")
	institutionID := r.URL.Query().Get("institution_id")

	page := 1
	perPage := 30
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && v > 0 {
		perPage = v
	}

	opts := []domain.QueryOption{
		domain.WithLimit(perPage),
		domain.WithOffset((page - 1) * perPage),
		domain.WithOrderBy(sortBy, sortDir == "desc"),
	}
	if search != "" {
		opts = append(opts, domain.WithSearch(search))
	}
	if institutionID != "" {
		opts = append(opts, domain.WithFilter("institution_id", institutionID))
	}

	projects, total, err := h.svc.ListVisible(ctx, opts...)
	if err != nil {
		h.log.Error("list projects failed", "err", err)
		http.Error(w, "failed to list projects", http.StatusInternalServerError)
		return
	}

	ids := make([]string, len(projects))
	for i, p := range projects {
		ids[i] = p.ID
	}

	allStats, err := h.svc.StatsForProjects(ctx, ids)
	if err != nil {
		h.log.Debug("project stats failed", "err", err)
		allStats = map[string]*domain.WeaveProjectStats{}
	}
	allOwners, err := h.svc.OwnersForProjects(ctx, ids)
	if err != nil {
		h.log.Debug("project owners failed", "err", err)
		allOwners = map[string]*domain.ProjectActor{}
	}

	type item struct {
		ID              string              `json:"id"`
		UIName          domain.Translations `json:"ui_name"`
		Description     domain.Translations `json:"description,omitempty"`
		SystemName      string              `json:"system_name"`
		Status          string              `json:"status"`
		Visibility      string              `json:"visibility"`
		SemanticID      string              `json:"semantic_id"`
		Institution     string              `json:"institution,omitempty"`
		InstitutionID   string              `json:"institution_id,omitempty"`
		Owner           string              `json:"owner,omitempty"`
		OwnerKind       string              `json:"owner_kind,omitempty"`
		ModelCount      int64               `json:"model_count"`
		CollectionCount int64               `json:"collection_count"`
		FieldCount      int64               `json:"field_count"`
		CategoryCount   int64               `json:"category_count"`
		Incomplete      bool                `json:"incomplete,omitempty"`
		CanEdit         bool                `json:"can_edit,omitempty"`
	}

	items := make([]item, len(projects))
	for i, p := range projects {
		row := item{
			ID:          p.ID,
			UIName:      p.UIName,
			Description: p.Description,
			SystemName:  p.SystemName,
			Status:      string(p.Status),
			SemanticID:  p.SemanticID,
			Visibility:  p.Visibility,
		}
		if row.Visibility == "" {
			row.Visibility = "public"
		}
		if stats := allStats[p.ID]; stats != nil {
			row.ModelCount = stats.ModelCount
			row.CollectionCount = stats.CollectionCount
			row.FieldCount = stats.FieldCount
			row.CategoryCount = stats.CategoryCount
		}
		if owner := allOwners[p.ID]; owner != nil {
			row.Institution = owner.DisplayName
			row.InstitutionID = owner.ID
			row.Owner = owner.DisplayName
			switch owner.Type {
			case "person":
				row.OwnerKind = "Personal"
			case "organization":
				row.OwnerKind = "Organization"
			default:
				row.OwnerKind = owner.Type
			}
		}
		if h.svc.CanEdit(ctx, p.ID, row.Visibility) {
			row.CanEdit = true
			resolved, lerr := h.svc.ResolvedOntologyVersions(ctx, p.ID, domain.ResolvedOntologyVersionOpts{})
			if lerr != nil {
				h.log.Debug("resolve ontology versions for badge", "project", p.ID, "err", lerr)
			} else if len(resolved) == 0 {
				row.Incomplete = true
			}
		}
		items[i] = row
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"projects": items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// EntityListSchema handles GET /projects/entity-list-schema.
//
// Gates the top-level "New Project" CTA on the caller being authenticated.
// Anonymous browsers see the project list (read is public) but no Create
// button — submitting a create request without a session would 401 anyway,
// and exposing the CTA misled users.
func (h *Handler) EntityListSchema(w http.ResponseWriter, r *http.Request) {
	snap := auth.FromContext(r.Context())
	canCreate := snap != nil && !snap.IsAnonymous
	schema := BuildEntityListSchema(canCreate, h.lang(r), h.languages)
	writeJSON(w, http.StatusOK, schema)
}

// FilterInstitutions handles GET /projects/filters/institutions.
func (h *Handler) FilterInstitutions(w http.ResponseWriter, r *http.Request) {
	actors, err := h.svc.ListVisibleOwnerInstitutions(r.Context())
	if err != nil {
		h.log.Error("load institutions failed", "err", err)
		http.Error(w, "failed to load institutions", http.StatusInternalServerError)
		return
	}
	options := make([]formschema.FilterOption, 0, len(actors)+1)
	options = append(options, formschema.FilterOption{
		Value: "",
		Label: i18n.L("project.list.all_institutions", "All Institutions"),
	})
	for _, a := range actors {
		options = append(options, formschema.FilterOption{
			Value: a.ID,
			Label: domain.Translations{"en": a.DisplayName},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"options": options})
}

// CheckIDPrefix handles GET /projects/check-prefix?prefix=X.
func (h *Handler) CheckIDPrefix(w http.ResponseWriter, r *http.Request) {
	prefix := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("prefix")))
	report, err := h.svc.CheckIDPrefix(r.Context(), prefix)
	if err != nil {
		h.log.Error("check id prefix failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"available": false,
			"valid":     true,
			"message":   "Failed to check availability",
		})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// FormSchema handles GET /projects/form-schema/project?mode=...
func (h *Handler) FormSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = formschema.ModeCreate
	}
	entityID := r.URL.Query().Get("entity_id")

	var existing *formschema.ProjectInput
	if (mode == formschema.ModeEdit || mode == formschema.ModeView) && entityID != "" {
		p, err := h.svc.Get(ctx, entityID)
		if err != nil {
			h.log.Error("load project for form schema", "id", entityID, "err", err)
		} else if p != nil {
			existing = &formschema.ProjectInput{
				ID:          p.ID, // weave_projects.id == IDPrefix
				SystemName:  p.SystemName,
				UIName:      p.UIName,
				Description: p.Description,
			}
		}
	}

	schema := BuildFormSchema(mode, existing, h.lang(r), h.languages)
	writeJSON(w, http.StatusOK, schema)
}

// InheritanceTree handles GET /projects/{projectID}/inheritance-tree.
func (h *Handler) InheritanceTree(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")

	project, err := h.svc.Get(ctx, projectID)
	if err != nil || project == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	type ontologyInfo struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Version   string `json:"version"`
		IsPrimary bool   `json:"is_primary"`
	}
	type parentInfo struct {
		ID              string         `json:"id"`
		Name            string         `json:"name"`
		SystemName      string         `json:"system_name"`
		OntologiesCount int            `json:"ontologies_count"`
		Ontologies      []ontologyInfo `json:"ontologies,omitempty"`
	}
	type response struct {
		Success          bool           `json:"success"`
		ProjectID        string         `json:"project_id"`
		ProjectName      string         `json:"project_name"`
		DirectOntologies []ontologyInfo `json:"direct_ontologies"`
		ParentProjects   []parentInfo   `json:"parent_projects"`
	}

	resolved, err := h.svc.ResolvedOntologyVersions(ctx, projectID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		h.log.Error("resolve ontology versions for inheritance tree", "project_id", projectID, "err", err)
		http.Error(w, "failed to resolve ontology tree", http.StatusInternalServerError)
		return
	}

	resp := response{
		Success:          true,
		ProjectID:        projectID,
		ProjectName:      project.SystemName,
		DirectOntologies: []ontologyInfo{},
		ParentProjects:   []parentInfo{},
	}

	type bucket struct {
		idx    int
		parent parentInfo
	}
	parentIdx := make(map[string]*bucket)

	for _, row := range resolved {
		if row.Link == nil {
			continue
		}
		info := ontologyInfo{
			ID:        row.Link.OntologyVersionID,
			Name:      row.OntologyName,
			Version:   row.VersionString,
			IsPrimary: row.Link.IsPrimary,
		}
		if row.SourceProjectID == "" {
			resp.DirectOntologies = append(resp.DirectOntologies, info)
			continue
		}
		b, ok := parentIdx[row.SourceProjectID]
		if !ok {
			label := row.SourceProjectID
			systemName := row.SourceProjectID
			if parent, perr := h.svc.Get(ctx, row.SourceProjectID); perr == nil && parent != nil {
				if name := parent.UIName.Get("en"); name != "" {
					label = name
				}
				systemName = parent.SystemName
			}
			resp.ParentProjects = append(resp.ParentProjects, parentInfo{
				ID:         row.SourceProjectID,
				Name:       label,
				SystemName: systemName,
				Ontologies: []ontologyInfo{},
			})
			b = &bucket{idx: len(resp.ParentProjects) - 1}
			parentIdx[row.SourceProjectID] = b
		}
		resp.ParentProjects[b.idx].Ontologies = append(resp.ParentProjects[b.idx].Ontologies, info)
		resp.ParentProjects[b.idx].OntologiesCount++
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Write endpoints
// ---------------------------------------------------------------------------

// Create handles POST /projects. The formschema-driven create flow
// posts JSON {ui_name, description, id_prefix, system_name} here;
// owner is the authenticated principal.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	principal := authPrincipalFromContext(r)
	if principal == nil || principal.ActorID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var body struct {
		UIName      domain.Translations `json:"ui_name"`
		Description domain.Translations `json:"description"`
		IDPrefix    string              `json:"id_prefix"`
		SystemName  string              `json:"system_name"`
		OwnerID     string              `json:"owner_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	ownerID := principal.ActorID
	if requested := strings.TrimSpace(body.OwnerID); requested != "" && requested != principal.ActorID {
		if !auth.FromContext(r.Context()).Can(auth.OrgProjectCreate, auth.Resource{ScopeType: "org", ID: requested}, nil) {
			writeError(w, http.StatusForbidden, "forbidden: requires org.project_create on org:"+requested)
			return
		}
		ownerID = requested
	}

	created, err := h.svc.Create(r.Context(), CreateInput{
		UIName:      body.UIName,
		Description: body.Description,
		IDPrefix:    strings.ToUpper(strings.TrimSpace(body.IDPrefix)),
		SystemName:  strings.TrimSpace(body.SystemName),
		OwnerID:     ownerID,
		CreatedByID: principal.ActorID,
	})
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// ---------------------------------------------------------------------------
// Error mapping
// ---------------------------------------------------------------------------

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	if IsNotFound(err) {
		apierror.Write(w, apierror.NotFound(err.Error()))
		return
	}
	ae := apierror.FromError(err)
	if ae.Code == apierror.CodeInternal {
		h.log.Error("project handler error", "err", err)
	}
	apierror.Write(w, ae)
}

// ---------------------------------------------------------------------------
// Local HTTP helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError forwards to apierror.Write — see pkg/weave/apierror.
func writeError(w http.ResponseWriter, status int, msg string) {
	apierror.Write(w, &apierror.Error{Status: status, Message: msg})
}

// writeEditDenied writes the correct failure for a blocked edit. An anonymous
// caller means the session lapsed — common when an editor page sits open past
// the session lifetime — so return 401 (code "unauthorized") to signal the
// client should prompt re-login and preserve unsaved edits, rather than a 403
// that reads as a permissions problem. An authenticated caller that reaches
// here genuinely lacks ProjectEdit, so keep the 403.
func writeEditDenied(w http.ResponseWriter, r *http.Request) {
	if snap := auth.FromContext(r.Context()); snap == nil || snap.IsAnonymous {
		apierror.Write(w, &apierror.Error{
			Status:  http.StatusUnauthorized,
			Code:    apierror.CodeUnauthorized,
			Message: "session expired: sign in again to save your changes",
		})
		return
	}
	apierror.Write(w, &apierror.Error{
		Status:  http.StatusForbidden,
		Code:    apierror.CodeForbidden,
		Message: "forbidden: requires ProjectEdit on project",
	})
}
