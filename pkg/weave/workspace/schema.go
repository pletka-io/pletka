package workspace

import (
	"fmt"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
	weaveproject "github.com/pletka-io/pletka/pkg/weave/project"
)

func buildProfilePageSchema(principal *weaveauth.Principal, orgCount, ownedCount, createdCount, collaboratingCount int, lang string, languages []formschema.LanguageInfo) *formschema.ProjectPageSchema {
	name := "Profile"
	if principal != nil && principal.DisplayName != "" {
		name = principal.DisplayName
	}
	navLinks := []formschema.ProjectPageNavLink{
		{
			Label: i18n.L("nav.settings", "Settings"),
			Href:  "/profile/settings",
			Icon:  "cog",
		},
	}
	return &formschema.ProjectPageSchema{
		Entity: formschema.ProjectPageEntity{
			ID:   "profile",
			Name: domain.Translations{"en": name},
		},
		NavLinks: navLinks,
		// Tab order: Overview first (landing on the
		// summary makes more sense than a project list with no context).
		// Project tabs prefixed with "Projects" so the relationship to
		// the user is explicit ("Owned" / "Created" /
		// "Collaborating" alone reads like organisation roles or task
		// statuses without the "Projects" cue).
		Tabs: []formschema.ProjectPageTab{
			{
				ID:         "overview",
				Label:      i18n.L("workspace.tabs.overview", "Overview"),
				Icon:       "home",
				ContentURL: "/profile/overview-schema",
				Default:    true,
			},
			{
				ID:         "organizations",
				Label:      i18n.L("workspace.tabs.organizations", "Organizations"),
				Icon:       "building-office-2",
				ContentURL: "/profile/entity-list-schema/organization",
				Count:      orgCount,
			},
			{
				ID:         "owned",
				Label:      i18n.L("workspace.profile.tab_owned", "Projects: Owned"),
				Icon:       "folder",
				ContentURL: "/profile/entity-list-schema/project-owned",
				Count:      ownedCount,
			},
			{
				ID:         "created",
				Label:      i18n.L("workspace.profile.tab_created", "Projects: Created"),
				Icon:       "sparkles",
				ContentURL: "/profile/entity-list-schema/project-created",
				Count:      createdCount,
			},
			{
				ID:         "collaborating",
				Label:      i18n.L("workspace.profile.tab_collaborating", "Projects: Collaborating"),
				Icon:       "users",
				ContentURL: "/profile/entity-list-schema/project-collaborating",
				Count:      collaboratingCount,
			},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

// buildProfileSettingsSchema returns the sidebar-shell schema for the
// profile settings page. Mirrors BuildSettingsSchema for projects so
// /profile/settings reuses the project-settings island. Sections are
// always emitted (authenticated user editing their own profile); per-
// section gating would land here if we add admin-only panes later.
func buildProfileSettingsSchema(principal *weaveauth.Principal, lang string, languages []formschema.LanguageInfo) *formschema.SettingsSchema {
	_ = lang
	_ = languages
	name := "Profile"
	if principal != nil && principal.DisplayName != "" {
		name = principal.DisplayName
	}
	return &formschema.SettingsSchema{
		ProjectID:   "profile",
		ProjectName: domain.Translations{"en": name},
		Sections: []formschema.SettingsSection{
			{
				ID:        "general",
				Label:     i18n.L("project_settings.section.general", "General"),
				Icon:      "cog-6-tooth",
				Kind:      "form",
				SchemaURL: "/me/form-schema",
			},
			{
				ID:        "security",
				Label:     i18n.L("profile.settings.section.security", "Security"),
				Icon:      "lock-closed",
				Kind:      "form",
				SchemaURL: "/me/password-form-schema",
			},
		},
	}
}

func buildProfileOverviewSchema(
	principal *weaveauth.Principal,
	orgCount, ownedCount, createdCount, collaboratingCount int,
	ownershipItems []formschema.ProjectOverviewItem,
	organizationItems []formschema.ProjectOverviewItem,
	lang string,
	languages []formschema.LanguageInfo,
) *formschema.ProjectOverviewSchema {
	name := "your workspace"
	if principal != nil && principal.DisplayName != "" {
		name = principal.DisplayName
	}
	sections := []formschema.ProjectOverviewSection{
		{
			Widget: "stats-cards",
			Title:  i18n.L("workspace.stats.workspace", "Workspace"),
			Items: []formschema.ProjectOverviewItem{
				{Label: i18n.L("workspace.tabs.organizations", "Organizations"), Count: orgCount, Icon: "building-office-2", Color: "purple"},
				{Label: i18n.L("workspace.profile.stats_owned", "Owned"), Count: ownedCount, Icon: "folder", Color: "blue"},
				{Label: i18n.L("workspace.profile.stats_created", "Created"), Count: createdCount, Icon: "sparkles", Color: "amber"},
				{Label: i18n.L("workspace.profile.stats_collaborating", "Collaborating"), Count: collaboratingCount, Icon: "users", Color: "teal"},
			},
		},
		{
			Widget: "description",
			// description_template is interpolated with the principal's
			// display name; raw Translations stays so we don't lose the
			// dynamic %s. The English copy here is the source of truth.
			ContentI18n: domain.Translations{"en": fmt.Sprintf("Use the Organizations tab to manage shared spaces, Owned for projects that live in your personal or organization workspaces, Created for projects you started yourself, and Collaborating for projects where you participate without owning them. This profile gives %s a quick workspace summary across all three relationships.", name)},
		},
		{
			Widget: "info-sidebar",
			Title:  i18n.L("workspace.profile.info_profile", "Profile"),
			Items: []formschema.ProjectOverviewItem{
				{Label: i18n.L("forms.name", "Name"), Value: name},
				{Label: i18n.L("common.role", "Role"), Value: principal.Role},
				{Label: i18n.L("common.email", "Email"), Value: principal.Email},
			},
		},
		{
			// Quick links into the tab system. Replaces the per-tab item
			// listings that used to live in the sidebar — those duplicated
			// the tab views below. Items here are anchors with counts; the
			// tabs themselves still render the rich data.
			Widget: "info-sidebar",
			Title:  i18n.L("workspace.profile.quick_links", "Quick Links"),
			Items: []formschema.ProjectOverviewItem{
				{Label: i18n.L("workspace.tabs.organizations", "Organizations"), Count: orgCount, Icon: "building-office-2", URL: "/profile#tab=organizations"},
				{Label: i18n.L("workspace.profile.tab_owned", "Projects: Owned"), Count: ownedCount, Icon: "folder", URL: "/profile#tab=owned"},
				{Label: i18n.L("workspace.profile.tab_created", "Projects: Created"), Count: createdCount, Icon: "sparkles", URL: "/profile#tab=created"},
				{Label: i18n.L("workspace.profile.tab_collaborating", "Projects: Collaborating"), Count: collaboratingCount, Icon: "users", URL: "/profile#tab=collaborating"},
			},
		},
	}
	// ownershipItems + organizationItems were previously rendered as
	// extra sidebars that duplicated the tab views — dropped.
	// Parameters retained to avoid churning every caller while the
	// quick-links pattern settles.
	_ = ownershipItems
	_ = organizationItems
	return &formschema.ProjectOverviewSchema{
		Sections: sections,
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

func buildProfileOrganizationListSchema(canCreate bool, lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	schema := organizationEntityListSchemaClone(canCreate, lang, languages)
	schema.Title = i18n.L("workspace.profile.org_list_title", "My Organizations")
	schema.DataURL = "/profile/organizations"
	schema.EmptyState = &formschema.EmptyState{
		Icon:    "building-office-2",
		Title:   i18n.L("organization.list.empty_title", "No organizations yet"),
		Message: i18n.L("workspace.profile.org_empty_message", "Create your first organization to collaborate around projects."),
	}
	return schema
}

func buildProfileOwnedProjectListSchema(canCreate bool, lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	s := weaveproject.BuildEntityListSchema(canCreate, lang, languages)
	s.Title = i18n.L("workspace.profile.owned_title", "Owned Projects")
	s.DataURL = "/profile/projects-owned"
	s.Filters = nil
	s.EmptyState = &formschema.EmptyState{
		Icon:    "folder",
		Title:   i18n.L("workspace.profile.owned_empty_title", "No owned projects yet"),
		Message: i18n.L("workspace.profile.owned_empty_message", "Projects owned by you or one of your organizations will appear here."),
	}
	return s
}

func buildProfileCreatedProjectListSchema(canCreate bool, lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	s := weaveproject.BuildEntityListSchema(canCreate, lang, languages)
	s.Title = i18n.L("workspace.profile.created_title", "Projects I Created")
	s.DataURL = "/profile/projects-created"
	s.Filters = nil
	s.EmptyState = &formschema.EmptyState{
		Icon:    "sparkles",
		Title:   i18n.L("workspace.profile.created_empty_title", "No created projects yet"),
		Message: i18n.L("workspace.profile.created_empty_message", "Projects you create yourself will appear here."),
	}
	return s
}

func buildProfileCollaboratingProjectListSchema(canCreate bool, lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	s := weaveproject.BuildEntityListSchema(canCreate, lang, languages)
	s.Title = i18n.L("workspace.profile.collaborating_title", "Collaborating Projects")
	s.DataURL = "/profile/projects-collaborating"
	s.Filters = nil
	s.EmptyState = &formschema.EmptyState{
		Icon:    "users",
		Title:   i18n.L("workspace.profile.collaborating_empty_title", "No collaborating projects yet"),
		Message: i18n.L("workspace.profile.collaborating_empty_message", "Projects where you collaborate without owning them will appear here."),
	}
	return s
}

func organizationEntityListSchemaClone(canCreate bool, lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	return &formschema.EntityListSchema{
		EntityType:        "organization",
		Title:             i18n.L("organization.list.title", "Organizations"),
		DataURL:           "/orgs/data",
		DataKey:           "organizations",
		DetailURLTemplate: "/orgs/{id}",
		EmptyState: &formschema.EmptyState{
			Icon:    "building-office-2",
			Title:   i18n.L("organization.list.empty_title", "No organizations yet"),
			Message: i18n.L("organization.list.empty_message", "No public organizations are available."),
		},
		Search: &formschema.SearchConfig{
			Placeholder: i18n.L("organization.list.search_placeholder", "Search organizations..."),
			ParamName:   "search",
		},
		SortOptions: []formschema.SortOption{
			{Value: "display_name", Label: i18n.L("forms.name", "Name")},
			{Value: "slug", Label: i18n.L("forms.fields.slug", "Slug")},
		},
		DefaultSort: "display_name",
		Pagination: &formschema.PaginationConfig{
			PageSize:        30,
			ParamName:       "page",
			PerPageName:     "per_page",
			PageSizeOptions: []int{25, 50, 100},
		},
		RowWidget: frontendrefs.EntityListRowWidget("editorial"),
		ViewModes: []formschema.ViewMode{
			{ID: "editorial", Label: i18n.L("common.view_editorial", "Editorial"), Default: true, Widget: frontendrefs.EntityListRowWidget("editorial")},
			{ID: "compact", Label: i18n.L("common.view_compact", "Compact"), Widget: frontendrefs.EntityListRowWidget("default")},
		},
		ColumnHeader: &formschema.ColumnHeaderConfig{
			Show:       true,
			IDLabel:    i18n.L("forms.fields.slug", "Slug"),
			TitleLabel: i18n.L("organization.list.title_label", "Organization"),
		},
		RowLayout: formschema.EntityRowLayout{
			TitleField:    "display_name",
			SubtitleField: "website",
			IdentityFields: []formschema.IdentityField{
				{Key: "slug", Style: "mono"},
				{Key: "acronym", Style: "code"},
			},
			ProcessBadges: []formschema.BadgeConfig{
				{Key: "visibility", Type: "status"},
			},
			CountColumns: []formschema.CountColumn{
				{Key: "project_count", Label: i18n.L("common.projects", "Projects")},
			},
		},
		Capabilities: organizationCapabilities(canCreate),
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

func organizationCapabilities(canCreate bool) *formschema.Capabilities {
	caps := &formschema.Capabilities{}
	if canCreate {
		caps.Create = &formschema.CreateCap{
			Label:         i18n.L("organization.list.add", "New Organization"),
			FormSchemaURL: "/profile/orgs/form-schema",
		}
	}
	return caps
}

func buildOrgPageSchema(org *domain.Organization, projectCount, memberCount int, canEdit bool, lang string, languages []formschema.LanguageInfo) *formschema.ProjectPageSchema {
	tabs := []formschema.ProjectPageTab{
		{
			ID:         "overview",
			Label:      i18n.L("workspace.tabs.overview", "Overview"),
			Icon:       "home",
			ContentURL: fmt.Sprintf("/orgs/%s/overview-schema", org.Slug),
			Default:    true,
		},
		{
			ID:         "projects",
			Label:      i18n.L("common.projects", "Projects"),
			Icon:       "folder",
			ContentURL: fmt.Sprintf("/orgs/%s/entity-list-schema/project", org.Slug),
			Count:      projectCount,
		},
	}
	// Members tab — admins/owners only (mirrors the Settings nav link
	// gating). Non-admin org members see the count on the overview but
	// not the management list.
	if canEdit {
		tabs = append(tabs, formschema.ProjectPageTab{
			ID:         "members",
			Label:      i18n.L("project_settings.section.members", "Members"),
			Icon:       "users",
			ContentURL: fmt.Sprintf("/orgs/%s/entity-list-schema/member", org.Slug),
			Count:      memberCount,
		})
	}
	navLinks := []formschema.ProjectPageNavLink{}
	if canEdit {
		navLinks = append(navLinks, formschema.ProjectPageNavLink{
			Label: i18n.L("workspace.org.settings", "Settings"),
			Href:  fmt.Sprintf("/orgs/%s/settings", org.Slug),
			Icon:  "cog",
		})
	}
	return &formschema.ProjectPageSchema{
		Entity: formschema.ProjectPageEntity{
			ID:          org.Slug,
			Name:        domain.Translations{"en": org.DisplayName},
			Description: nil,
			Status:      "",
		},
		Tabs:     tabs,
		NavLinks: navLinks,
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

func buildOrgOverviewSchema(org *domain.Organization, projectCount, memberCount int, lang string, languages []formschema.LanguageInfo) *formschema.ProjectOverviewSchema {
	return &formschema.ProjectOverviewSchema{
		Sections: []formschema.ProjectOverviewSection{
			{
				Widget: "stats-cards",
				Title:  i18n.L("workspace.stats.workspace", "Workspace"),
				Items: []formschema.ProjectOverviewItem{
					{
						Label: i18n.L("common.projects", "Projects"),
						Count: projectCount,
						Icon:  "folder",
						Color: "blue",
					},
					{
						Label: i18n.L("project_settings.section.members", "Members"),
						Count: memberCount,
						Icon:  "users",
						Color: "teal",
					},
				},
			},
			{
				Widget: "info-sidebar",
				Title:  i18n.L("workspace.org.info_title", "Organization"),
				Items: []formschema.ProjectOverviewItem{
					{Label: i18n.L("forms.fields.slug", "Slug"), Value: org.Slug},
					{Label: i18n.L("project_settings.section.visibility", "Visibility"), Value: org.Visibility},
					{Label: i18n.L("common.country", "Country"), Value: deref(org.Country)},
					{Label: i18n.L("common.website", "Website"), Value: deref(org.Website)},
				},
			},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

func buildOrgProjectListSchema(org *domain.Organization, canCreate bool, lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	s := weaveproject.BuildEntityListSchema(false, lang, languages)
	s.DataURL = fmt.Sprintf("/orgs/%s/projects", org.Slug)
	s.Filters = nil
	s.DetailURLTemplate = "/projects/{id}"
	s.ColumnHeader = &formschema.ColumnHeaderConfig{
		Show:       true,
		IDLabel:    i18n.L("common.id", "ID"),
		TitleLabel: i18n.L("common.project", "Project"),
	}
	if canCreate {
		if s.Capabilities == nil {
			s.Capabilities = &formschema.Capabilities{}
		}
		s.Capabilities.Create = &formschema.CreateCap{
			Label:         i18n.L("project.list.add", "New Project"),
			FormSchemaURL: fmt.Sprintf("/orgs/%s/projects/form-schema", org.Slug),
		}
	}
	return s
}

func defaultProjectSort(v string) string {
	if v == "" {
		return "ui_name"
	}
	return v
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// buildOrgMembersEntityListSchema produces the EntityListSchema that
// drives the org-detail Members tab. Reads from the
// canonical /orgs/{slug}/members-list endpoint (workspace-owned, gated
// at route layer on org membership). Capabilities (add / edit / remove)
// link back to the orgmembers slice's existing form + DELETE
// endpoints — those keep their RequireEdit gating, so non-admin
// callers see read-only rows.
func buildOrgMembersEntityListSchema(slug string, canEdit bool, lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	caps := &formschema.Capabilities{}
	if canEdit {
		caps.Create = &formschema.CreateCap{
			Label:         i18n.L("workspace.org.add_member", "Add member"),
			FormSchemaURL: fmt.Sprintf("/orgs/%s/members/form-schema", slug),
		}
		caps.Edit = &formschema.EditCap{
			FormSchemaURLTemplate: fmt.Sprintf("/orgs/%s/members/{id}/form-schema", slug),
		}
		caps.Delete = &formschema.DeleteCap{
			URLTemplate: fmt.Sprintf("/orgs/%s/members/{id}", slug),
		}
	}
	return &formschema.EntityListSchema{
		EntityType: "org_member",
		Title:      i18n.L("project_settings.section.members", "Members"),
		DataURL:    fmt.Sprintf("/orgs/%s/members-list", slug),
		DataKey:    "members",
		EmptyState: &formschema.EmptyState{
			Icon:    "users",
			Title:   i18n.L("workspace.org.members_empty_title", "No members"),
			Message: i18n.L("workspace.org.members_empty_message", "Add the first organization member to grant access."),
		},
		Search: &formschema.SearchConfig{
			Placeholder: i18n.L("workspace.org.members_search", "Search members..."),
			ParamName:   "search",
		},
		SortOptions: []formschema.SortOption{
			{Value: "display_name", Label: i18n.L("forms.name", "Name")},
			{Value: "role", Label: i18n.L("common.role", "Role")},
		},
		DefaultSort: "display_name",
		Pagination: &formschema.PaginationConfig{
			PageSize:        50,
			ParamName:       "page",
			PerPageName:     "per_page",
			PageSizeOptions: []int{25, 50, 100},
		},
		RowWidget: frontendrefs.EntityListRowWidget("default"),
		ColumnHeader: &formschema.ColumnHeaderConfig{
			Show:       true,
			TitleLabel: i18n.L("workspace.org.member_singular", "Member"),
		},
		RowLayout: formschema.EntityRowLayout{
			TitleField:    "display_name",
			SubtitleField: "email",
			IdentityFields: []formschema.IdentityField{
				{Key: "slug", Style: "text"},
			},
			SemanticBadges: []formschema.BadgeConfig{
				{Key: "role", Type: "text", Style: "blue", Label: i18n.L("common.role", "Role"), HideEmpty: true},
			},
		},
		Capabilities: caps,
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}
