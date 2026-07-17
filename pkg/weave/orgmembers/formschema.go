package orgmembers

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
)

func BuildListSchema(slug, lang string, languages []formschema.LanguageInfo) *formschema.ListSchema {
	return &formschema.ListSchema{
		EntityType: "org_member",
		Title:      i18n.L("project_settings.section.members", "Members"),
		EmptyState: &formschema.EmptyState{
			Icon:    "users",
			Title:   i18n.L("workspace.org.members_empty_title", "No members"),
			Message: i18n.L("workspace.org.members_empty_message", "Add the first organization member to grant access."),
		},
		DataURL: fmt.Sprintf("/orgs/%s/members", slug),
		DataKey: "",
		Caps: formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("workspace.org.add_member", "Add member"),
				FormSchemaURL: fmt.Sprintf("/orgs/%s/members/form-schema", slug),
			},
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: fmt.Sprintf("/orgs/%s/members/{id}/form-schema", slug),
			},
			Delete: &formschema.DeleteCap{
				URLTemplate: fmt.Sprintf("/orgs/%s/members/{id}", slug),
			},
		},
		Columns: []formschema.Column{
			{
				Key:     "display_name",
				Label:   i18n.L("forms.name", "Name"),
				Type:    "text",
				Primary: true,
			},
			{
				Key:       "email",
				Label:     i18n.L("common.email", "Email"),
				Type:      "text",
				Secondary: true,
			},
			{
				Key:        "role",
				Label:      i18n.L("common.role", "Role"),
				Type:       "text_badge",
				BadgeStyle: "gray",
				BadgeStyleByValue: map[string]string{
					"member": "gray",
					"admin":  "blue",
					"owner":  "amber",
				},
			},
		},
		RowActions: []formschema.RowAction{
			{ID: "edit", Icon: "pencil", Label: i18n.L("member.list.edit_role", "Edit role")},
			{
				ID:          "delete",
				Icon:        "trash",
				Label:       i18n.L("common.remove", "Remove"),
				Style:       "danger",
				VisibleWhen: "!owner_locked",
			},
		},
		PerRowReadonlyField: "owner_locked",
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

func BuildEditForm(slug, actorID string, member *MemberRow, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	displayLine := ""
	if member != nil {
		displayLine = member.DisplayName
		if member.Email != "" {
			displayLine = fmt.Sprintf("%s (%s)", member.DisplayName, member.Email)
		} else if member.Slug != "" {
			displayLine = fmt.Sprintf("%s (%s)", member.DisplayName, member.Slug)
		}
	}
	currentRole := "member"
	if member != nil && member.Role != "" {
		currentRole = member.Role
	}

	return &formschema.FormSchema{
		EntityType: "org_member",
		Mode:       formschema.ModeEdit,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "PATCH",
			URL:    fmt.Sprintf("/orgs/%s/members/%s", slug, actorID),
		},
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("orgmember.form.submit_save_role", "Save role"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("orgmember.form.role_updated", "Member role updated"),
		},
		Sections: []formschema.Section{
			{
				ID: "identity",
				Fields: []formschema.FieldDef{
					{
						Name:     "member",
						Widget:   formschema.WidgetText,
						Readonly: true,
						Label:    i18n.L("orgmember.form.member", "Member"),
						Value:    displayLine,
					},
					{
						Name:     "role",
						Widget:   formschema.WidgetSelect,
						Required: true,
						Label:    i18n.L("orgmember.form.role", "Role"),
						Help:     i18n.L("orgmember.form.role_help", "member (read) · admin (settings and projects) · owner (full control)"),
						Value:    currentRole,
						Options: []formschema.SelectOption{
							{Value: "member", Label: i18n.L("orgmember.role.member", "Member")},
							{Value: "admin", Label: i18n.L("orgmember.role.admin", "Admin")},
							{Value: "owner", Label: i18n.L("orgmember.role.owner", "Owner")},
						},
					},
				},
			},
		},
	}
}

func BuildAddForm(slug, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	return &formschema.FormSchema{
		EntityType: "org_member",
		Mode:       formschema.ModeCreate,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "POST",
			URL:    fmt.Sprintf("/orgs/%s/members", slug),
		},
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("orgmember.form.submit_add", "Add member"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("orgmember.form.added", "Member added"),
		},
		Sections: []formschema.Section{
			{
				ID: "identity",
				Fields: []formschema.FieldDef{
					{
						Name:       "actor_id",
						Widget:     formschema.WidgetSearchSelect,
						Required:   true,
						Label:      i18n.L("orgmember.form.user", "User"),
						OptionsURL: fmt.Sprintf("/orgs/%s/members/options/actors", slug),
						Help:       i18n.L("orgmember.form.user_help", "Pick a registered user. Users not yet in the system must sign up first."),
					},
					{
						Name:     "role",
						Widget:   formschema.WidgetSelect,
						Required: true,
						Label:    i18n.L("orgmember.form.role", "Role"),
						Help:     i18n.L("orgmember.form.role_help", "member (read) · admin (settings and projects) · owner (full control)"),
						Value:    "member",
						Options: []formschema.SelectOption{
							{Value: "member", Label: i18n.L("orgmember.role.member", "Member")},
							{Value: "admin", Label: i18n.L("orgmember.role.admin", "Admin")},
							{Value: "owner", Label: i18n.L("orgmember.role.owner", "Owner")},
						},
					},
				},
			},
		},
	}
}
