package members

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildListSchema returns the ListSchema consumed by ListManager when
// the settings panel mounts. Columns: display name (primary), email,
// role badge. Row actions: edit role + remove.
func BuildListSchema(projectID, lang string, languages []formschema.LanguageInfo) *formschema.ListSchema {
	return &formschema.ListSchema{
		EntityType: "member",
		Title:      i18n.L("project_settings.section.members", "Members"),
		EmptyState: &formschema.EmptyState{
			Icon:    "users",
			Title:   i18n.L("member.list.empty_title", "No members"),
			Message: i18n.L("member.list.empty_message", "Add the first project member to grant access."),
		},
		DataURL: fmt.Sprintf("/projects/%s/members", projectID),
		DataKey: "",
		Caps: formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("workspace.org.add_member", "Add member"),
				FormSchemaURL: fmt.Sprintf("/projects/%s/members/form-schema", projectID),
			},
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: fmt.Sprintf("/projects/%s/members/{id}/form-schema", projectID),
			},
			Delete: &formschema.DeleteCap{
				URLTemplate: fmt.Sprintf("/projects/%s/members/{id}", projectID),
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
					"viewer":      "gray",
					"contributor": "blue",
					"maintainer":  "purple",
					"owner":       "amber",
				},
			},
		},
		// Edit always available (changing role is harmless on the owner
		// row — the service-layer guard rejects role=owner downgrade
		// attempts elsewhere). Remove is hidden on the owner row via
		// VisibleWhen="!is_owner" — the project owner can't be removed
		// from their own project here.
		RowActions: []formschema.RowAction{
			{ID: "edit", Icon: "pencil", Label: i18n.L("member.list.edit_role", "Edit role")},
			{
				ID:          "delete",
				Icon:        "trash",
				Label:       i18n.L("common.remove", "Remove"),
				Style:       "danger",
				VisibleWhen: "!is_owner",
			},
		},
		PerRowReadonlyField: "is_owner",
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

// BuildEditForm returns the FormSchema for editing an existing
// member's role. Email/slug rendered read-only; only role is editable.
// Submits PATCH /{actorID} with {"role": "..."}.
func BuildEditForm(projectID, actorID string, member *MemberRow, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	displayLine := ""
	if member != nil {
		displayLine = member.DisplayName
		if member.Email != "" {
			displayLine = fmt.Sprintf("%s (%s)", member.DisplayName, member.Email)
		} else if member.Slug != "" {
			displayLine = fmt.Sprintf("%s (%s)", member.DisplayName, member.Slug)
		}
	}
	currentRole := "contributor"
	if member != nil && member.Role != "" {
		currentRole = member.Role
	}

	return &formschema.FormSchema{
		EntityType: "member",
		Mode:       formschema.ModeEdit,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "PATCH",
			URL:    fmt.Sprintf("/projects/%s/members/%s", projectID, actorID),
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
						Help:     i18n.L("member.form.role_help", "viewer (read-only) · contributor (edit) · maintainer (settings) · owner (full control)"),
						Value:    currentRole,
						Options: []formschema.SelectOption{
							{Value: "viewer", Label: i18n.L("member.role.viewer", "Viewer")},
							{Value: "contributor", Label: i18n.L("member.role.contributor", "Contributor")},
							{Value: "maintainer", Label: i18n.L("member.role.maintainer", "Maintainer")},
							{Value: "owner", Label: i18n.L("orgmember.role.owner", "Owner")},
						},
					},
				},
			},
		},
	}
}

// BuildAddForm returns the FormSchema for the Add Member modal /
// inline form. Two fields: actor_id (autocomplete select fed by
// /options/actors) + role.
func BuildAddForm(projectID, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	return &formschema.FormSchema{
		EntityType: "member",
		Mode:       formschema.ModeCreate,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "POST",
			URL:    fmt.Sprintf("/projects/%s/members", projectID),
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
						Widget:     formschema.WidgetSelect,
						Required:   true,
						Label:      i18n.L("orgmember.form.user", "User"),
						OptionsURL: fmt.Sprintf("/projects/%s/members/options/actors", projectID),
						Help:       i18n.L("orgmember.form.user_help", "Pick a registered user. Users not yet in the system must sign up first."),
					},
					{
						Name:     "role",
						Widget:   formschema.WidgetSelect,
						Required: true,
						Label:    i18n.L("orgmember.form.role", "Role"),
						Help:     i18n.L("member.form.role_help", "viewer (read-only) · contributor (edit) · maintainer (settings) · owner (full control)"),
						Value:    "contributor",
						Options: []formschema.SelectOption{
							{Value: "viewer", Label: i18n.L("member.role.viewer", "Viewer")},
							{Value: "contributor", Label: i18n.L("member.role.contributor", "Contributor")},
							{Value: "maintainer", Label: i18n.L("member.role.maintainer", "Maintainer")},
							{Value: "owner", Label: i18n.L("orgmember.role.owner", "Owner")},
						},
					},
				},
			},
		},
	}
}
