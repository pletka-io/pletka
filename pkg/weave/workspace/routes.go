package workspace

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/session"
	"github.com/pletka-io/pletka/pkg/weave/organization"
	"github.com/pletka-io/pletka/pkg/weave/orgmembers"
	"github.com/pletka-io/pletka/pkg/weave/project"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

type Host struct {
	Logger        *slog.Logger
	Templates     *weavetemplates.Renderer
	I18n          i18n.Manager
	Session       *session.Manager
	LangResolver  LangResolver
	Languages     []formschema.LanguageInfo
	Organizations *organization.Service
	Projects      *project.Service
	Members       *orgmembers.Service
}

func (h Host) Validate() error {
	var missing []string
	if h.Templates == nil {
		missing = append(missing, "Templates")
	}
	if h.I18n == nil {
		missing = append(missing, "I18n")
	}
	if h.Organizations == nil {
		missing = append(missing, "Organizations")
	}
	if h.Projects == nil {
		missing = append(missing, "Projects")
	}
	if h.Members == nil {
		missing = append(missing, "Members")
	}
	if len(missing) > 0 {
		return fmt.Errorf("workspace host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

func Mount(parent chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := NewHandler(host.Logger, host.Templates, host.I18n, host.Session, host.LangResolver, host.Languages, host.Organizations, host.Projects, host.Members)

	parent.Get("/profile", h.ProfilePage)
	parent.Get("/profile/settings", h.ProfileSettingsPage)
	parent.Get("/profile/settings/schema", h.ProfileSettingsSchema)
	parent.Get("/orgs/new", h.OrgNewPage)
	parent.Get("/profile/page-schema", h.ProfilePageSchema)
	parent.Get("/profile/overview-schema", h.ProfileOverviewSchema)
	parent.Get("/profile/entity-list-schema/organization", h.ProfileOrganizationsEntityListSchema)
	parent.Get("/profile/entity-list-schema/project-owned", h.ProfileOwnedProjectsEntityListSchema)
	parent.Get("/profile/entity-list-schema/project-created", h.ProfileCreatedProjectsEntityListSchema)
	parent.Get("/profile/entity-list-schema/project-collaborating", h.ProfileCollaboratingProjectsEntityListSchema)
	parent.Get("/profile/organizations", h.ProfileOrganizationsData)
	parent.Get("/profile/projects-owned", h.ProfileOwnedProjectsData)
	parent.Get("/profile/projects-created", h.ProfileCreatedProjectsData)
	parent.Get("/profile/projects-collaborating", h.ProfileCollaboratingProjectsData)
	parent.Get("/orgs", h.OrgListPage)
	parent.With(auth.WithOrgResource(host.Organizations), auth.RequireOrgRead).Get("/orgs/{slug}", h.OrgDetailPage)
	parent.With(auth.WithOrgResource(host.Organizations), auth.RequireOrgEdit).Get("/orgs/{slug}/settings", h.OrgSettingsPage)
	parent.With(auth.WithOrgResource(host.Organizations), auth.RequireOrgRead).Get("/orgs/{slug}/page-schema", h.OrgPageSchema)
	parent.With(auth.WithOrgResource(host.Organizations), auth.RequireOrgRead).Get("/orgs/{slug}/overview-schema", h.OrgOverviewSchema)
	parent.With(auth.WithOrgResource(host.Organizations), auth.RequireOrgRead).Get("/orgs/{slug}/entity-list-schema/project", h.OrgProjectsEntityListSchema)
	parent.With(auth.WithOrgResource(host.Organizations), auth.RequireOrgEdit).Get("/orgs/{slug}/projects/form-schema", h.OrgProjectCreateFormSchema)
	parent.With(auth.WithOrgResource(host.Organizations), auth.RequireOrgRead).Get("/orgs/{slug}/projects", h.OrgProjectsData)
	// Members tab routes — RequireOrgEdit so the underlying member roster
	// is admin/owner-only (matches the Settings nav link gating). Non-
	// admin readers see the member count on the overview but not the
	// list.
	parent.With(auth.WithOrgResource(host.Organizations), auth.RequireOrgEdit).Get("/orgs/{slug}/entity-list-schema/member", h.OrgMembersEntityListSchema)
	parent.With(auth.WithOrgResource(host.Organizations), auth.RequireOrgEdit).Get("/orgs/{slug}/members-list", h.OrgMembersData)
}
