package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/session"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/organization"
	"github.com/pletka-io/pletka/pkg/weave/orgmembers"
	"github.com/pletka-io/pletka/pkg/weave/project"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

type Handler struct {
	log       *slog.Logger
	renderer  *weavetemplates.Renderer
	i18n      i18n.Manager
	session   *session.Manager
	lang      LangResolver
	languages []formschema.LanguageInfo
	orgs      *organization.Service
	projects  *project.Service
	members   *orgmembers.Service
}

type LangResolver func(*http.Request) string

func NewHandler(
	log *slog.Logger,
	renderer *weavetemplates.Renderer,
	i18n i18n.Manager,
	session *session.Manager,
	lang LangResolver,
	languages []formschema.LanguageInfo,
	orgs *organization.Service,
	projects *project.Service,
	members *orgmembers.Service,
) *Handler {
	if log == nil {
		log = slog.Default()
	}
	if lang == nil {
		lang = func(*http.Request) string { return "en" }
	}
	return &Handler{
		log:       log,
		renderer:  renderer,
		i18n:      i18n,
		session:   session,
		lang:      lang,
		languages: languages,
		orgs:      orgs,
		projects:  projects,
		members:   members,
	}
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/profile", h.ProfilePage)
	r.Get("/profile/settings", h.ProfileSettingsPage)
	r.Get("/profile/settings/schema", h.ProfileSettingsSchema)
	r.Get("/orgs/new", h.OrgNewPage)
	r.Get("/profile/page-schema", h.ProfilePageSchema)
	r.Get("/profile/overview-schema", h.ProfileOverviewSchema)
	r.Get("/profile/entity-list-schema/organization", h.ProfileOrganizationsEntityListSchema)
	r.Get("/profile/entity-list-schema/project-owned", h.ProfileOwnedProjectsEntityListSchema)
	r.Get("/profile/entity-list-schema/project-created", h.ProfileCreatedProjectsEntityListSchema)
	r.Get("/profile/organizations", h.ProfileOrganizationsData)
	r.Get("/profile/projects-owned", h.ProfileOwnedProjectsData)
	r.Get("/profile/projects-created", h.ProfileCreatedProjectsData)
	r.Get("/profile/entity-list-schema/project-collaborating", h.ProfileCollaboratingProjectsEntityListSchema)
	r.Get("/profile/projects-collaborating", h.ProfileCollaboratingProjectsData)
	r.Get("/orgs", h.OrgListPage)
	r.Get("/orgs/{slug}", h.OrgDetailPage)
	r.Get("/orgs/{slug}/settings", h.OrgSettingsPage)

	r.Get("/orgs/{slug}/page-schema", h.OrgPageSchema)
	r.Get("/orgs/{slug}/overview-schema", h.OrgOverviewSchema)
	r.Get("/orgs/{slug}/entity-list-schema/project", h.OrgProjectsEntityListSchema)
	r.Get("/orgs/{slug}/projects/form-schema", h.OrgProjectCreateFormSchema)
	r.Get("/orgs/{slug}/projects", h.OrgProjectsData)
	r.Get("/orgs/{slug}/entity-list-schema/member", h.OrgMembersEntityListSchema)
	r.Get("/orgs/{slug}/members-list", h.OrgMembersData)
}

func (h *Handler) ProfilePage(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		http.Redirect(w, r, "/login?next=/profile", http.StatusFound)
		return
	}
	lang := h.currentLang(r)
	page := h.basePage(r, lang)
	page.Title = principal.DisplayName + " · Profile"
	page.Breadcrumbs = []weavetemplates.Breadcrumb{{Label: "Profile"}}
	page.Island = weavetemplates.IslandMount{
		Name: frontendrefs.Island("project-detail"),
		Props: map[string]string{
			"schema-url":   "/profile/page-schema",
			"entity-label": "profile",
			"lang":         lang,
		},
		Dependencies: []string{frontendrefs.Island("project-detail")},
		Placeholder:  placeholderHTML(),
	}
	if err := h.renderer.RenderIslandPage(w, page); err != nil {
		h.log.Error("render profile page", "err", err)
		h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang))
	}
}

// ProfileSettingsPage renders the profile settings shell. Reuses the
// project-settings island bound to /profile/settings/schema, so the
// profile gets the same sidebar+pane UX as project settings (General,
// Security, future panes). Per-pane forms still live in
// pkg/weave/actoradmin (/me/form-schema and /me/password-form-schema).
func (h *Handler) ProfileSettingsPage(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		http.Redirect(w, r, "/login?next=/profile/settings", http.StatusFound)
		return
	}
	lang := h.currentLang(r)
	page := h.basePage(r, lang)
	page.Title = principal.DisplayName + " · Settings"
	page.Heading = "Settings"
	page.Breadcrumbs = []weavetemplates.Breadcrumb{
		{Label: "Profile", Href: "/profile"},
		{Label: "Settings"},
	}
	page.Island = weavetemplates.IslandMount{
		Name: frontendrefs.Island("project-settings"),
		Props: map[string]string{
			"project-id":      "profile",
			"schema-url-base": "/profile/settings",
			"lang":            lang,
		},
		Dependencies: []string{frontendrefs.Island("project-settings"), frontendrefs.Island("list-manager"), frontendrefs.Island("entity-form")},
		Placeholder:  placeholderHTML(),
	}
	if err := h.renderer.RenderIslandPage(w, page); err != nil {
		h.log.Error("render profile settings page", "err", err)
		h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang))
	}
}

// ProfileSettingsSchema returns the SettingsSchema that drives the
// /profile/settings sidebar shell. The frontend project-settings island
// reads this and fetches each section's schema_url on demand.
func (h *Handler) ProfileSettingsSchema(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	writeJSON(w, http.StatusOK, buildProfileSettingsSchema(principal, h.lang(r), h.languages))
}

// OrgNewPage renders the create-organization form. Reuses the existing
// /profile/orgs/form-schema endpoint (mounted by organization.routes)
// + POST /profile/orgs (CreateSelf). Page shell only.
func (h *Handler) OrgNewPage(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		http.Redirect(w, r, "/login?next=/orgs/new", http.StatusFound)
		return
	}
	lang := h.currentLang(r)
	page := h.basePage(r, lang)
	page.Title = "Create organization"
	page.Heading = "Create organization"
	page.Breadcrumbs = []weavetemplates.Breadcrumb{
		{Label: "Organizations", Href: "/orgs"},
		{Label: "New"},
	}
	page.Island = weavetemplates.IslandMount{
		Name: frontendrefs.Island("entity-form"),
		Props: map[string]string{
			"schema-url": "/profile/orgs/form-schema",
			"lang":       lang,
		},
		Dependencies: []string{frontendrefs.Island("entity-form")},
		Placeholder:  placeholderHTML(),
	}
	if err := h.renderer.RenderIslandPage(w, page); err != nil {
		h.log.Error("render org new page", "err", err)
		h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang))
	}
}

func (h *Handler) ProfilePageSchema(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	orgIDs := h.profileOrganizationIDs(r.Context())
	owned, collaborating := h.profileProjectRelationCounts(r.Context(), principal.ActorID)
	writeJSON(w, http.StatusOK, buildProfilePageSchema(principal, len(orgIDs), owned, h.profileCreatedProjectCount(r.Context(), principal.ActorID), collaborating, h.lang(r), h.languages))
}

func (h *Handler) ProfileOverviewSchema(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	orgIDs := h.profileOrganizationIDs(r.Context())
	orgItems := h.profileOrganizationSummaryItems(r.Context(), orgIDs)
	owned, collaborating := h.profileProjectRelationCounts(r.Context(), principal.ActorID)
	writeJSON(w, http.StatusOK, buildProfileOverviewSchema(
		principal,
		len(orgIDs),
		owned,
		h.profileCreatedProjectCount(r.Context(), principal.ActorID),
		collaborating,
		h.profileOwnershipSummaryItems(r.Context(), principal.ActorID),
		orgItems,
		h.lang(r),
		h.languages,
	))
}

func (h *Handler) ProfileOrganizationsEntityListSchema(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	writeJSON(w, http.StatusOK, buildProfileOrganizationListSchema(true, h.lang(r), h.languages))
}

func (h *Handler) ProfileOwnedProjectsEntityListSchema(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	writeJSON(w, http.StatusOK, buildProfileOwnedProjectListSchema(true, h.lang(r), h.languages))
}

func (h *Handler) ProfileCreatedProjectsEntityListSchema(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	writeJSON(w, http.StatusOK, buildProfileCreatedProjectListSchema(true, h.lang(r), h.languages))
}

func (h *Handler) ProfileCollaboratingProjectsEntityListSchema(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	writeJSON(w, http.StatusOK, buildProfileCollaboratingProjectListSchema(true, h.lang(r), h.languages))
}

func (h *Handler) ProfileOrganizationsData(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	page, perPage := parsePageParams(r)
	items, err := h.orgs.ListByIDs(r.Context(), h.profileOrganizationIDs(r.Context()))
	if err != nil {
		h.log.Error("profile organizations failed", "err", err)
		apierror.Write(w, apierror.InternalWith("failed to load organizations"))
		return
	}
	search := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("search")))
	filtered := make([]organization.BrowseItem, 0, len(items))
	for _, item := range items {
		if search != "" {
			matches := strings.Contains(strings.ToLower(item.DisplayName), search) ||
				strings.Contains(strings.ToLower(item.Slug), search)
			if item.Acronym != nil {
				matches = matches || strings.Contains(strings.ToLower(*item.Acronym), search)
			}
			if !matches {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	slices.SortFunc(filtered, func(a, b organization.BrowseItem) int {
		if cmp := strings.Compare(strings.ToLower(a.DisplayName), strings.ToLower(b.DisplayName)); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Slug, b.Slug)
	})
	start, end, total := paginate(len(filtered), page, perPage)
	writeJSON(w, http.StatusOK, map[string]any{
		"organizations": filtered[start:end],
		"total":         total,
		"page":          page,
		"per_page":      perPage,
	})
}

func (h *Handler) ProfileOwnedProjectsData(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	page, perPage := parsePageParams(r)
	projects := h.profileProjectsByRelation(r.Context(), principal.ActorID, "owned", r.URL.Query().Get("search"))
	items := h.projectRows(r.Context(), projects)
	start, end, total := paginate(len(items), page, perPage)
	writeJSON(w, http.StatusOK, map[string]any{
		"projects": items[start:end],
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *Handler) ProfileCollaboratingProjectsData(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	page, perPage := parsePageParams(r)
	projects := h.profileProjectsByRelation(r.Context(), principal.ActorID, "collaborating", r.URL.Query().Get("search"))
	items := h.projectRows(r.Context(), projects)
	start, end, total := paginate(len(items), page, perPage)
	writeJSON(w, http.StatusOK, map[string]any{
		"projects": items[start:end],
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *Handler) ProfileCreatedProjectsData(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	page, perPage := parsePageParams(r)
	projects, total, err := h.projects.ListVisible(r.Context(),
		domain.WithFilter("created_by_id", principal.ActorID),
		domain.WithSearch(r.URL.Query().Get("search")),
		domain.WithOrderBy(defaultProjectSort(r.URL.Query().Get("sort_by")), r.URL.Query().Get("sort_dir") == "desc"),
		domain.WithLimit(perPage),
		domain.WithOffset((page-1)*perPage),
	)
	if err != nil {
		h.log.Error("profile created projects failed", "actor_id", principal.ActorID, "err", err)
		apierror.Write(w, apierror.InternalWith("failed to load created projects"))
		return
	}
	items := h.projectRows(r.Context(), projects)
	writeJSON(w, http.StatusOK, map[string]any{
		"projects": items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *Handler) OrgListPage(w http.ResponseWriter, r *http.Request) {
	lang := h.currentLang(r)
	page := h.basePage(r, lang)
	page.Title = "Organizations"
	page.Heading = "Organizations"
	page.Island = weavetemplates.IslandMount{
		Name: frontendrefs.Island("entity-list"),
		Props: map[string]string{
			"schema-url": "/orgs/entity-list-schema",
			"lang":       lang,
		},
		Dependencies: []string{frontendrefs.Island("entity-list")},
		Placeholder:  placeholderHTML(),
	}
	if err := h.renderer.RenderIslandPage(w, page); err != nil {
		h.log.Error("render org list page", "err", err)
		h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang))
	}
}

func (h *Handler) OrgDetailPage(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		apierror.Write(w, apierror.NotFound(""))
		return
	}
	lang := h.currentLang(r)
	page := h.basePage(r, lang)
	page.Title = org.DisplayName
	page.Breadcrumbs = []weavetemplates.Breadcrumb{
		{Label: "Organizations", Href: "/orgs"},
		{Label: org.DisplayName},
	}
	page.Island = weavetemplates.IslandMount{
		Name: frontendrefs.Island("project-detail"),
		Props: map[string]string{
			"schema-url":   fmt.Sprintf("/orgs/%s/page-schema", org.Slug),
			"entity-label": "organization",
			"lang":         lang,
		},
		Dependencies: []string{frontendrefs.Island("project-detail")},
		Placeholder:  placeholderHTML(),
	}
	if err := h.renderer.RenderIslandPage(w, page); err != nil {
		h.log.Error("render org detail page", "slug", org.Slug, "err", err)
		h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang))
	}
}

func (h *Handler) OrgSettingsPage(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		apierror.Write(w, apierror.NotFound(""))
		return
	}
	lang := h.currentLang(r)
	page := h.basePage(r, lang)
	page.Title = org.DisplayName + " · Settings"
	page.Breadcrumbs = []weavetemplates.Breadcrumb{
		{Label: "Organizations", Href: "/orgs"},
		{Label: org.DisplayName, Href: "/orgs/" + org.Slug},
		{Label: "Settings"},
	}
	page.Island = weavetemplates.IslandMount{
		Name: frontendrefs.Island("project-settings"),
		Props: map[string]string{
			"schema-url-base": fmt.Sprintf("/orgs/%s/settings", org.Slug),
			"lang":            lang,
		},
		Dependencies: []string{frontendrefs.Island("project-settings"), frontendrefs.Island("list-manager"), frontendrefs.Island("entity-form")},
		Placeholder:  placeholderHTML(),
	}
	if err := h.renderer.RenderIslandPage(w, page); err != nil {
		h.log.Error("render org settings page", "slug", org.Slug, "err", err)
		h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang))
	}
}

func (h *Handler) OrgPageSchema(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		apierror.Write(w, apierror.NotFound(""))
		return
	}
	projects, projectTotal, err := h.projects.ListVisible(r.Context(),
		domain.WithFilter("owner_id", org.ID),
		domain.WithLimit(1),
	)
	_ = projects
	if err != nil {
		h.log.Error("org page schema project count failed", "slug", org.Slug, "err", err)
		apierror.Write(w, apierror.InternalWith("failed to load organization"))
		return
	}
	memberCount := h.orgMemberCount(r.Context(), org)
	canEdit := weaveauth.FromContext(r.Context()).Can(weaveauth.OrgEdit, weaveauth.OrgResourceFromContext(r.Context()), nil)
	writeJSON(w, http.StatusOK, buildOrgPageSchema(org, int(projectTotal), memberCount, canEdit, h.lang(r), h.languages))
}

func (h *Handler) OrgOverviewSchema(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		apierror.Write(w, apierror.NotFound(""))
		return
	}
	_, projectTotal, err := h.projects.ListVisible(r.Context(),
		domain.WithFilter("owner_id", org.ID),
		domain.WithLimit(1),
	)
	if err != nil {
		h.log.Error("org overview project count failed", "slug", org.Slug, "err", err)
		apierror.Write(w, apierror.InternalWith("failed to load organization overview"))
		return
	}
	memberCount := h.orgMemberCount(r.Context(), org)
	writeJSON(w, http.StatusOK, buildOrgOverviewSchema(org, int(projectTotal), memberCount, h.lang(r), h.languages))
}

// orgMemberCount counts the org's members via orgmembers.Service. The
// service requires OrgRead which the route's WithOrgResource +
// RequireRead middleware already enforced. Returns 0 on error so the
// overview card degrades gracefully rather than 500'ing.
func (h *Handler) orgMemberCount(ctx context.Context, org *domain.Organization) int {
	if h.members == nil || org == nil {
		return 0
	}
	rows, err := h.members.List(ctx, org.ID)
	if err != nil {
		h.log.Warn("org members count failed", "slug", org.Slug, "err", err)
		return 0
	}
	return len(rows)
}

// OrgMembersEntityListSchema returns the EntityListSchema that drives
// the Members tab on the org-detail page.
func (h *Handler) OrgMembersEntityListSchema(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		apierror.Write(w, apierror.NotFound(""))
		return
	}
	canEdit := weaveauth.FromContext(r.Context()).Can(weaveauth.OrgEdit, weaveauth.OrgResourceFromContext(r.Context()), nil)
	writeJSON(w, http.StatusOK, buildOrgMembersEntityListSchema(org.Slug, canEdit, h.lang(r), h.languages))
}

// OrgMembersData serves the row payload for the Members tab. Delegates
// to orgmembers.Service.List which gates on OrgRead via the snapshot
// (route attaches the org resource via WithOrgResource).
func (h *Handler) OrgMembersData(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		apierror.Write(w, apierror.NotFound(""))
		return
	}
	if h.members == nil {
		writeJSON(w, http.StatusOK, map[string]any{"members": []any{}, "total": 0})
		return
	}
	rows, err := h.members.List(r.Context(), org.ID)
	if err != nil {
		h.log.Error("list org members failed", "slug", org.Slug, "err", err)
		apierror.Write(w, apierror.InternalWith("failed to list members"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": rows, "total": len(rows)})
}

func (h *Handler) OrgProjectsEntityListSchema(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		apierror.Write(w, apierror.NotFound(""))
		return
	}
	canCreate := weaveauth.FromContext(r.Context()).Can(weaveauth.OrgProjectCreate, weaveauth.OrgResourceFromContext(r.Context()), nil)
	writeJSON(w, http.StatusOK, buildOrgProjectListSchema(org, canCreate, h.lang(r), h.languages))
}

func (h *Handler) OrgProjectCreateFormSchema(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		apierror.Write(w, apierror.NotFound(""))
		return
	}
	if !weaveauth.FromContext(r.Context()).Can(weaveauth.OrgProjectCreate, weaveauth.OrgResourceFromContext(r.Context()), nil) {
		apierror.Write(w, apierror.NotFound(""))
		return
	}

	schema := project.BuildFormSchema(formschema.ModeCreate, nil, h.lang(r), h.languages)
	if len(schema.Sections) > 0 {
		schema.Sections[0].Fields = append(schema.Sections[0].Fields, formschema.FieldDef{
			Name:     "owner_id",
			Widget:   formschema.WidgetSelect,
			Required: true,
			Readonly: true,
			Label:    i18n.L("workspace.org_project_form.owner_label", "Owner"),
			Help:     i18n.L("workspace.org_project_form.owner_help", "This project will be created under the current organization."),
			Value:    org.ID,
			Options: []formschema.SelectOption{
				{
					Value: org.ID,
					Label: domain.Translations{"en": org.DisplayName},
				},
			},
		})
	}
	writeJSON(w, http.StatusOK, schema)
}

func (h *Handler) OrgProjectsData(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		apierror.Write(w, apierror.NotFound(""))
		return
	}
	page := 1
	perPage := 30
	if v := r.URL.Query().Get("page"); v != "" {
		fmt.Sscanf(v, "%d", &page)
		if page <= 0 {
			page = 1
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		fmt.Sscanf(v, "%d", &perPage)
		if perPage <= 0 {
			perPage = 30
		}
	}
	projects, total, err := h.projects.ListVisible(r.Context(),
		domain.WithFilter("owner_id", org.ID),
		domain.WithSearch(r.URL.Query().Get("search")),
		domain.WithOrderBy(defaultProjectSort(r.URL.Query().Get("sort_by")), r.URL.Query().Get("sort_dir") == "desc"),
		domain.WithLimit(perPage),
		domain.WithOffset((page-1)*perPage),
	)
	if err != nil {
		h.log.Error("org projects data failed", "slug", org.Slug, "err", err)
		apierror.Write(w, apierror.InternalWith("failed to list organization projects"))
		return
	}
	ids := make([]string, 0, len(projects))
	for _, p := range projects {
		ids = append(ids, p.ID)
	}
	stats, _ := h.projects.StatsForProjects(r.Context(), ids)
	items := make([]map[string]any, 0, len(projects))
	for _, p := range projects {
		row := map[string]any{
			"id":          p.ID,
			"ui_name":     p.UIName,
			"description": p.Description,
			"system_name": p.SystemName,
			"visibility":  p.Visibility,
			"status":      string(p.Status),
		}
		if p.Visibility == "" {
			row["visibility"] = "public"
		}
		if st := stats[p.ID]; st != nil {
			row["model_count"] = st.ModelCount
			row["collection_count"] = st.CollectionCount
			row["field_count"] = st.FieldCount
			row["category_count"] = st.CategoryCount
		}
		items = append(items, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"projects": items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *Handler) currentLang(r *http.Request) string {
	if h.lang != nil {
		if lang := h.lang(r); lang != "" {
			return lang
		}
	}
	if h.session != nil {
		if lang := h.session.Language(r.Context()); lang != "" {
			return lang
		}
	}
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return lang
	}
	return "en"
}

func (h *Handler) basePage(r *http.Request, lang string) weavetemplates.IslandPage {
	return weavetemplates.IslandPage{
		Lang:        lang,
		Path:        r.URL.Path,
		Languages:   h.i18n.Languages(),
		Principal:   weaveauth.PrincipalFromContext(r.Context()),
		IsAnonymous: weaveauth.FromContext(r.Context()).IsAnonymous,
		Labels:      h.renderer.ShellLabels(lang),
	}
}

func placeholderHTML() template.HTML {
	return template.HTML(`
<div class="bg-white shadow-sm rounded-lg p-6 animate-pulse">
    <div class="h-8 bg-gray-200 rounded w-1/3 mb-4"></div>
    <div class="h-4 bg-gray-200 rounded w-2/3 mb-6"></div>
    <div class="space-y-3">
        <div class="h-10 bg-gray-100 rounded"></div>
        <div class="h-10 bg-gray-100 rounded"></div>
        <div class="h-10 bg-gray-100 rounded"></div>
    </div>
</div>`)
}

func parsePageParams(r *http.Request) (int, int) {
	page := 1
	perPage := 30
	if v := r.URL.Query().Get("page"); v != "" {
		fmt.Sscanf(v, "%d", &page)
		if page <= 0 {
			page = 1
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		fmt.Sscanf(v, "%d", &perPage)
		if perPage <= 0 {
			perPage = 30
		}
	}
	return page, perPage
}

func paginate(length, page, perPage int) (int, int, int) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 30
	}
	total := length
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return start, end, total
}

func (h *Handler) profileOrganizationIDs(ctx context.Context) []string {
	return profileOrganizationIDs(weaveauth.FromContext(ctx))
}

func profileOrganizationIDs(snap *weaveauth.AuthSnapshot) []string {
	if snap == nil {
		return nil
	}
	out := make([]string, 0, len(snap.Roles))
	for key := range snap.Roles {
		if strings.HasPrefix(key, "org:") {
			out = append(out, strings.TrimPrefix(key, "org:"))
		}
	}
	slices.Sort(out)
	return out
}

func (h *Handler) profileProjectIDs(ctx context.Context) []string {
	return profileProjectIDs(weaveauth.FromContext(ctx))
}

func (h *Handler) profileCreatedProjectCount(ctx context.Context, actorID string) int {
	if actorID == "" {
		return 0
	}
	_, total, err := h.projects.ListVisible(ctx,
		domain.WithFilter("created_by_id", actorID),
		domain.WithLimit(1),
	)
	if err != nil {
		h.log.Debug("profile created project count failed", "actor_id", actorID, "err", err)
		return 0
	}
	return int(total)
}

func (h *Handler) profileProjectRelationCounts(ctx context.Context, actorID string) (owned, collaborating int) {
	owned = len(h.profileProjectsByRelation(ctx, actorID, "owned", ""))
	collaborating = len(h.profileProjectsByRelation(ctx, actorID, "collaborating", ""))
	return owned, collaborating
}

func (h *Handler) profileOwnershipSummaryItems(ctx context.Context, actorID string) []formschema.ProjectOverviewItem {
	ownedProjects := h.profileProjectsByRelation(ctx, actorID, "owned", "")
	createdCount := h.profileCreatedProjectCount(ctx, actorID)
	collaboratingCount := len(h.profileProjectsByRelation(ctx, actorID, "collaborating", ""))
	personalOwned := 0
	organizationOwned := 0
	orgIDs := h.profileOrganizationIDs(ctx)
	orgSet := make(map[string]struct{}, len(orgIDs))
	for _, id := range orgIDs {
		orgSet[id] = struct{}{}
	}
	for _, p := range ownedProjects {
		if p.OwnerID == actorID {
			personalOwned++
			continue
		}
		if _, ok := orgSet[p.OwnerID]; ok {
			organizationOwned++
		}
	}
	return []formschema.ProjectOverviewItem{
		{Label: i18n.L("workspace.profile.ownership_personal", "Personal ownership"), Value: fmt.Sprintf("%d", personalOwned)},
		{Label: i18n.L("workspace.profile.ownership_organization", "Organization ownership"), Value: fmt.Sprintf("%d", organizationOwned)},
		{Label: i18n.L("workspace.profile.ownership_created", "Created by you"), Value: fmt.Sprintf("%d", createdCount)},
		{Label: i18n.L("workspace.profile.ownership_collaboration", "Collaboration only"), Value: fmt.Sprintf("%d", collaboratingCount)},
	}
}

func (h *Handler) profileOrganizationSummaryItems(ctx context.Context, orgIDs []string) []formschema.ProjectOverviewItem {
	if len(orgIDs) == 0 {
		return nil
	}
	items, err := h.orgs.ListByIDs(ctx, orgIDs)
	if err != nil {
		h.log.Debug("profile organization summary failed", "err", err)
		return nil
	}
	out := make([]formschema.ProjectOverviewItem, 0, len(items))
	for _, item := range items {
		value := item.Slug
		if item.ProjectCount > 0 {
			value = fmt.Sprintf("%s · %d projects", item.Slug, item.ProjectCount)
		}
		out = append(out, formschema.ProjectOverviewItem{
			Label: domain.Translations{"en": item.DisplayName},
			Value: value,
			Style: "mono",
		})
	}
	return out
}

func profileProjectIDs(snap *weaveauth.AuthSnapshot) []string {
	if snap == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(snap.OwnedProjectIDs)+len(snap.Roles))
	out := make([]string, 0, len(snap.OwnedProjectIDs)+len(snap.Roles))
	for id := range snap.OwnedProjectIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for key := range snap.Roles {
		if !strings.HasPrefix(key, "project:") {
			continue
		}
		id := strings.TrimPrefix(key, "project:")
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

func (h *Handler) profileProjectsByRelation(ctx context.Context, actorID, relation, search string) []*domain.Project {
	projectIDs := h.profileProjectIDs(ctx)
	orgIDs := h.profileOrganizationIDs(ctx)
	orgSet := make(map[string]struct{}, len(orgIDs))
	for _, id := range orgIDs {
		orgSet[id] = struct{}{}
	}
	search = strings.TrimSpace(strings.ToLower(search))
	projects := make([]*domain.Project, 0, len(projectIDs))
	for _, id := range projectIDs {
		p, err := h.projects.Get(ctx, id)
		if err != nil {
			h.log.Debug("profile project lookup failed", "project_id", id, "err", err)
			continue
		}
		if p == nil || !h.projects.CanRead(ctx, p) {
			continue
		}
		if search != "" {
			name := strings.ToLower(p.UIName.Get("en", ""))
			desc := strings.ToLower(p.Description.Get("en", ""))
			if !strings.Contains(name, search) && !strings.Contains(desc, search) && !strings.Contains(strings.ToLower(p.ID), search) {
				continue
			}
		}
		owned := p.OwnerID == actorID
		if !owned {
			_, owned = orgSet[p.OwnerID]
		}
		switch relation {
		case "owned":
			if !owned {
				continue
			}
		case "collaborating":
			if owned {
				continue
			}
		}
		projects = append(projects, p)
	}
	slices.SortFunc(projects, func(a, b *domain.Project) int {
		if cmp := strings.Compare(strings.ToLower(a.UIName.Get("en", "")), strings.ToLower(b.UIName.Get("en", ""))); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ID, b.ID)
	})
	return projects
}

func (h *Handler) projectRows(ctx context.Context, projects []*domain.Project) []map[string]any {
	projectIDs := make([]string, 0, len(projects))
	for _, p := range projects {
		projectIDs = append(projectIDs, p.ID)
	}
	stats, _ := h.projects.StatsForProjects(ctx, projectIDs)
	owners, _ := h.projects.OwnersForProjects(ctx, projectIDs)
	items := make([]map[string]any, 0, len(projects))
	for _, p := range projects {
		row := map[string]any{
			"id":          p.ID,
			"ui_name":     p.UIName,
			"description": p.Description,
			"system_name": p.SystemName,
			"visibility":  p.Visibility,
			"status":      string(p.Status),
		}
		if row["visibility"] == "" {
			row["visibility"] = "public"
		}
		if owner := owners[p.ID]; owner != nil {
			row["owner"] = owner.DisplayName
			row["owner_kind"] = owner.Type
			row["institution"] = owner.DisplayName
			row["institution_id"] = owner.ID
		}
		if st := stats[p.ID]; st != nil {
			row["model_count"] = st.ModelCount
			row["collection_count"] = st.CollectionCount
			row["field_count"] = st.FieldCount
			row["category_count"] = st.CategoryCount
		}
		items = append(items, row)
	}
	return items
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
