package actoradmin

import (
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
)

func BuildUserEntityListSchema(lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	return &formschema.EntityListSchema{
		EntityType:        "user",
		Title:             i18n.L("admin.users.title", "Users"),
		DataURL:           "/admin/users/data",
		DataKey:           "items",
		DetailURLTemplate: "",
		ProjectID:         "",
		EmptyState: &formschema.EmptyState{
			Icon:    "users",
			Title:   i18n.L("admin.users.empty_title", "No users"),
			Message: i18n.L("admin.users.empty_message", "No registered users found."),
		},
		Search: &formschema.SearchConfig{
			Placeholder: i18n.L("admin.users.search_placeholder", "Search users..."),
			ParamName:   "search",
		},
		Filters: []formschema.FilterConfig{
			{
				Key:       "role",
				Label:     i18n.L("common.role", "Role"),
				ParamName: "role",
				Type:      "select",
				Options: []formschema.FilterOption{
					{Value: "contributor", Label: i18n.L("user.role.contributor", "Contributor")},
					{Value: "admin", Label: i18n.L("user.role.admin", "Admin")},
					{Value: "super_admin", Label: i18n.L("user.role.super_admin", "Super admin")},
				},
			},
		},
		SortOptions: []formschema.SortOption{
			{Value: "display_name", Label: i18n.L("forms.name", "Name")},
			{Value: "email", Label: i18n.L("common.email", "Email")},
			{Value: "role", Label: i18n.L("common.role", "Role")},
			{Value: "institution", Label: i18n.L("common.institution", "Institution")},
		},
		DefaultSort: "display_name",
		Pagination: &formschema.PaginationConfig{
			PageSize:        25,
			ParamName:       "page",
			PerPageName:     "per_page",
			PageSizeOptions: []int{25, 50, 100},
		},
		RowWidget: frontendrefs.EntityListRowWidget("default"),
		ViewModes: []formschema.ViewMode{
			{ID: "detailed", Label: i18n.L("namespace_binding.list.view_detailed", "Detailed"), Default: true},
			{ID: "compact", Label: i18n.L("common.view_compact", "Compact")},
		},
		RowLayout: formschema.EntityRowLayout{
			TitleField:    "display_name",
			SubtitleField: "email",
			IdentityFields: []formschema.IdentityField{
				{Key: "slug", Style: "mono"},
			},
			SemanticBadges: []formschema.BadgeConfig{
				{Key: "role", Type: "text", Style: "gray"},
				{Key: "institution_name", Type: "text", Style: "blue", HideEmpty: true},
				{Key: "membership_count", Type: "count", Style: "green", Label: i18n.L("common.projects", "Projects"), HideEmpty: true},
				{Key: "owned_project_count", Type: "count", Style: "amber", Label: i18n.L("admin.users.owned_badge", "Owned"), HideEmpty: true},
			},
			ProcessBadges: []formschema.BadgeConfig{
				{Key: "last_login_display", Type: "text", Style: "gray", HideEmpty: true},
			},
		},
		Capabilities: &formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("admin.users.add", "Add user"),
				FormSchemaURL: "/admin/users/form-schema?mode=create",
			},
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: "/admin/users/form-schema?mode=edit&entity_id={id}",
			},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

func BuildInstitutionEntityListSchema(lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	return &formschema.EntityListSchema{
		EntityType:        "institution",
		Title:             i18n.L("admin.institutions.title", "Institutions"),
		DataURL:           "/admin/institutions/data",
		DataKey:           "items",
		DetailURLTemplate: "",
		ProjectID:         "",
		EmptyState: &formschema.EmptyState{
			Icon:    "building-library",
			Title:   i18n.L("admin.institutions.empty_title", "No institutions"),
			Message: i18n.L("admin.institutions.empty_message", "Create the first institution."),
		},
		Search: &formschema.SearchConfig{
			Placeholder: i18n.L("admin.institutions.search_placeholder", "Search institutions..."),
			ParamName:   "search",
		},
		SortOptions: []formschema.SortOption{
			{Value: "display_name", Label: i18n.L("forms.name", "Name")},
			{Value: "slug", Label: i18n.L("forms.fields.slug", "Slug")},
			{Value: "country", Label: i18n.L("common.country", "Country")},
		},
		DefaultSort: "display_name",
		Pagination: &formschema.PaginationConfig{
			PageSize:        25,
			ParamName:       "page",
			PerPageName:     "per_page",
			PageSizeOptions: []int{25, 50, 100},
		},
		RowWidget: frontendrefs.EntityListRowWidget("default"),
		ViewModes: []formschema.ViewMode{
			{ID: "detailed", Label: i18n.L("namespace_binding.list.view_detailed", "Detailed"), Default: true},
			{ID: "compact", Label: i18n.L("common.view_compact", "Compact")},
		},
		RowLayout: formschema.EntityRowLayout{
			TitleField:    "display_name",
			SubtitleField: "website",
			IdentityFields: []formschema.IdentityField{
				{Key: "slug", Style: "mono"},
			},
			SemanticBadges: []formschema.BadgeConfig{
				{Key: "visibility", Type: "status"},
				{Key: "acronym", Type: "text", Style: "gray", HideEmpty: true},
				{Key: "country", Type: "text", Style: "blue", HideEmpty: true},
				{Key: "member_count", Type: "count", Style: "green", Label: i18n.L("admin.institutions.users_badge", "Users"), HideEmpty: true},
				{Key: "owned_project_count", Type: "count", Style: "amber", Label: i18n.L("admin.institutions.owned_projects_badge", "Owned projects"), HideEmpty: true},
			},
		},
		Capabilities: &formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("admin.institutions.add", "Add institution"),
				FormSchemaURL: "/admin/institutions/form-schema?mode=create",
			},
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: "/admin/institutions/form-schema?mode=edit&entity_id={id}",
			},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

func BuildUserFormSchema(mode string, existing *sqlcgen.WeaveActor, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	schema := &formschema.FormSchema{
		EntityType: "user",
		Mode:       mode,
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("forms.save_changes", "Save Changes"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("user.form.saved", "User saved"),
		},
	}

	if mode == formschema.ModeCreate {
		schema.Endpoint = &formschema.SchemaEndpoint{Method: "POST", URL: "/admin/users"}
		schema.UI.SubmitLabel = i18n.L("user.form.submit_create", "Create user")
		schema.UI.SuccessMessage = i18n.L("user.form.created", "User created")
	} else if existing != nil {
		schema.Endpoint = &formschema.SchemaEndpoint{Method: "PUT", URL: "/admin/users/" + existing.ID}
		schema.UI.SuccessMessage = i18n.L("user.form.updated", "User updated")
	}

	var displayName, slug, role, email, firstName, lastName, orcid, website, country string
	if existing != nil {
		displayName = existing.DisplayName
		slug = existing.Slug
		role = existing.Role
		email = deref(existing.Email)
		firstName = deref(existing.FirstName)
		lastName = deref(existing.LastName)
		orcid = deref(existing.Orcid)
		website = deref(existing.Website)
		country = deref(existing.Country)
	}
	if mode == formschema.ModeCreate && role == "" {
		role = "contributor"
	}

	one := 1
	schema.Sections = []formschema.Section{
		{
			ID:    "identity",
			Label: i18n.L("forms.sections.identity", "Identity"),
			Fields: []formschema.FieldDef{
				{Name: "display_name", Widget: formschema.WidgetText, Required: true, Label: i18n.L("forms.fields.display_name", "Display name"), Value: displayName, Validation: &formschema.ValidationRules{MinLength: &one}},
				{Name: "slug", Widget: map[bool]string{true: formschema.WidgetSlugInput, false: formschema.WidgetText}[mode == formschema.ModeCreate], Required: true, Label: i18n.L("forms.fields.username_slug", "Username / slug"), Value: slug, DerivedFrom: "display_name", Validation: &formschema.ValidationRules{MinLength: &one, Pattern: "^[a-z0-9_-]+$"}},
				{Name: "email", Widget: formschema.WidgetText, Label: i18n.L("forms.fields.email", "Email"), Value: email},
			},
		},
		{
			// Single-institution `parent_id` field intentionally omitted —
			// users can belong to multiple institutions. A separate
			// memberships join table will surface institution affiliation;
			// until that lands the column stays on the actor row but isn't
			// editable through this form.
			ID:    "access",
			Label: i18n.L("forms.sections.access", "Access"),
			Fields: []formschema.FieldDef{
				{
					Name:     "role",
					Widget:   formschema.WidgetSelect,
					Required: true,
					Label:    i18n.L("user.form.role_label", "Default access role"),
					Help:     i18n.L("user.form.role_help", "System-wide default. Per-project membership roles override this."),
					Value:    role,
					Options: []formschema.SelectOption{
						{Value: "contributor", Label: i18n.L("user.role.contributor", "Contributor")},
						{Value: "admin", Label: i18n.L("user.role.admin", "Admin")},
						{Value: "super_admin", Label: i18n.L("user.role.super_admin", "Super admin")},
					},
				},
			},
		},
		{
			ID:    "profile",
			Label: i18n.L("forms.sections.profile", "Profile"),
			Fields: []formschema.FieldDef{
				{Name: "first_name", Widget: formschema.WidgetText, Label: i18n.L("forms.fields.first_name", "First name"), Value: firstName},
				{Name: "last_name", Widget: formschema.WidgetText, Label: i18n.L("forms.fields.last_name", "Last name"), Value: lastName},
				{Name: "orcid", Widget: formschema.WidgetText, Label: i18n.L("forms.fields.orcid", "ORCID"), Value: orcid},
				{Name: "website", Widget: formschema.WidgetText, Label: i18n.L("forms.fields.website", "Website"), Value: website},
				{Name: "country", Widget: formschema.WidgetSearchSelect, Label: i18n.L("forms.fields.country", "Country"), Value: country, Options: formschema.CountryOptions(lang)},
			},
		},
	}
	// On create, offer an optional initial password. Left blank, the
	// server generates one and returns it once for the admin to share.
	if mode == "create" {
		schema.UI.RevealField = "generated_password"
		schema.UI.RevealLabel = i18n.L("forms.fields.generated_password", "Temporary password (copy now — shown only once)")
		schema.Sections = append(schema.Sections, formschema.Section{
			ID:    "credentials",
			Label: i18n.L("forms.sections.credentials", "Credentials"),
			Fields: []formschema.FieldDef{
				{
					Name:   "password",
					Widget: formschema.WidgetPassword,
					Label:  i18n.L("forms.fields.initial_password", "Initial password"),
					Help:   i18n.L("forms.fields.initial_password_help", "Leave blank to auto-generate a password (shown once after creation). Minimum 12 characters."),
				},
			},
		})
	}
	return schema
}

// BuildSelfProfileFormSchema is the slim self-edit form mounted at
// PUT /me. Restricts editable fields to display name, country, website,
// and ORCID — slug/email/role/parent_id only move through the admin
// UpdateUser path. Mirrors BuildUserFormSchema's shape but with one
// section, four fields, and a non-admin endpoint.
func BuildSelfProfileFormSchema(existing *sqlcgen.WeaveActor, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	schema := &formschema.FormSchema{
		EntityType: "profile",
		Mode:       formschema.ModeEdit,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "PUT",
			URL:    "/me",
		},
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("profile.form.submit_save", "Save profile"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("profile.form.saved", "Profile updated"),
		},
	}

	one := 1
	schema.Sections = []formschema.Section{
		{
			ID:    "profile",
			Label: i18n.L("forms.sections.profile", "Profile"),
			Fields: []formschema.FieldDef{
				{
					Name:       "display_name",
					Widget:     formschema.WidgetText,
					Required:   true,
					Label:      i18n.L("forms.name", "Name"),
					Help:       i18n.L("profile.form.name_help", "How your name appears across the platform."),
					Value:      existing.DisplayName,
					Validation: &formschema.ValidationRules{MinLength: &one},
				},
				{
					// Email is read-only here — it's a login identifier;
					// changing it has auth-flow implications, so it stays an
					// admin-path edit. Shown so the user can see and confirm
					// the address used to contact them.
					Name:     "email",
					Widget:   formschema.WidgetText,
					Readonly: true,
					Label:    i18n.L("forms.fields.email", "Email"),
					Help:     i18n.L("profile.form.email_help", "The address used to sign in and contact you. Ask an administrator to change it."),
					Value:    deref(existing.Email),
				},
				{
					Name:   "orcid",
					Widget: formschema.WidgetText,
					Label:  i18n.L("forms.fields.orcid", "ORCID"),
					Help:   i18n.L("profile.form.orcid_help", "Your ORCID iD (e.g. 0000-0002-1825-0097)."),
					Value:  deref(existing.Orcid),
				},
				{
					Name:   "website",
					Widget: formschema.WidgetText,
					Label:  i18n.L("forms.fields.website", "Website"),
					Value:  deref(existing.Website),
				},
				{
					Name:    "country",
					Widget:  formschema.WidgetSearchSelect,
					Label:   i18n.L("forms.fields.country", "Country"),
					Value:   deref(existing.Country),
					Options: formschema.CountryOptions(lang),
				},
			},
		},
	}
	return schema
}

// BuildAdminResetPasswordFormSchema is the admin "set password for this
// user" form (no current-password field — admin authority). Posts to
// /admin/users/{id}/password.
func BuildAdminResetPasswordFormSchema(userID, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	return &formschema.FormSchema{
		EntityType: "admin-password",
		Mode:       "edit",
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("admin.password.submit", "Set password"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("admin.password.saved", "Password set"),
		},
		Endpoint: &formschema.SchemaEndpoint{
			Method: "POST",
			URL:    "/admin/users/" + userID + "/password",
		},
		Sections: []formschema.Section{{
			ID:    "password",
			Label: i18n.L("admin.password.section", "Set password"),
			Fields: []formschema.FieldDef{{
				Name:     "new_password",
				Widget:   formschema.WidgetPassword,
				Label:    i18n.L("admin.password.new", "New password"),
				Help:     i18n.L("admin.password.help", "Minimum 12 characters."),
				Required: true,
			}},
		}},
	}
}

// BuildPasswordFormSchema is the change-password form mounted at
// POST /me/password. Three fields, no entity data — the form is the
// whole contract. The 12-character minimum mirrors the Register handler.
func BuildPasswordFormSchema(lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	minLen := 12
	return &formschema.FormSchema{
		EntityType: "password",
		Mode:       formschema.ModeEdit,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "POST",
			URL:    "/me/password",
		},
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("profile.password.submit", "Change password"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("profile.password.saved", "Password updated"),
		},
		Sections: []formschema.Section{
			{
				ID:    "password",
				Label: i18n.L("profile.password.section", "Change password"),
				Fields: []formschema.FieldDef{
					{
						Name:     "current_password",
						Widget:   formschema.WidgetPassword,
						Required: true,
						Label:    i18n.L("profile.password.current", "Current password"),
					},
					{
						Name:       "new_password",
						Widget:     formschema.WidgetPassword,
						Required:   true,
						Label:      i18n.L("profile.password.new", "New password"),
						Help:       i18n.L("profile.password.new_help", "At least 12 characters."),
						Validation: &formschema.ValidationRules{MinLength: &minLen},
					},
					{
						Name:     "confirm_password",
						Widget:   formschema.WidgetPassword,
						Required: true,
						Label:    i18n.L("profile.password.confirm", "Confirm new password"),
					},
				},
			},
		},
	}
}

func BuildInstitutionFormSchema(mode string, existing *sqlcgen.WeaveActor, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	schema := &formschema.FormSchema{
		EntityType: "institution",
		Mode:       mode,
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("forms.save_changes", "Save Changes"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("institution.form.saved", "Institution saved"),
		},
	}

	if mode == formschema.ModeCreate {
		schema.Endpoint = &formschema.SchemaEndpoint{Method: "POST", URL: "/admin/institutions"}
		schema.UI.SubmitLabel = i18n.L("institution.form.submit_create", "Create institution")
	} else if existing != nil {
		schema.Endpoint = &formschema.SchemaEndpoint{Method: "PUT", URL: "/admin/institutions/" + existing.ID}
	}

	one := 1
	var displayName, slug, acronym, website, country, visibility string
	if existing != nil {
		displayName = existing.DisplayName
		slug = existing.Slug
		acronym = deref(existing.Acronym)
		website = deref(existing.Website)
		country = deref(existing.Country)
		visibility = existing.Visibility
	}
	if visibility == "" {
		visibility = "public"
	}

	schema.Sections = []formschema.Section{
		{
			ID:    "identity",
			Label: i18n.L("forms.sections.identity", "Identity"),
			Fields: []formschema.FieldDef{
				{Name: "display_name", Widget: formschema.WidgetText, Required: true, Label: i18n.L("forms.fields.display_name", "Display name"), Value: displayName, Validation: &formschema.ValidationRules{MinLength: &one}},
				{Name: "slug", Widget: map[bool]string{true: formschema.WidgetSlugInput, false: formschema.WidgetText}[mode == formschema.ModeCreate], Required: true, Label: i18n.L("forms.fields.slug", "Slug"), Value: slug, DerivedFrom: "display_name", Help: i18n.L("forms.fields.slug_help", "Suggested from the display name. Immutable after creation."), Validation: &formschema.ValidationRules{MinLength: &one, Pattern: "^[a-z0-9_-]+$"}},
				{Name: "acronym", Widget: formschema.WidgetText, Label: i18n.L("forms.fields.acronym", "Acronym"), Value: acronym},
			},
		},
		{
			ID:    "profile",
			Label: i18n.L("forms.sections.profile", "Profile"),
			Fields: []formschema.FieldDef{
				{Name: "website", Widget: formschema.WidgetText, Label: i18n.L("forms.fields.website", "Website"), Value: website},
				{Name: "country", Widget: formschema.WidgetSearchSelect, Label: i18n.L("forms.fields.country", "Country"), Value: country, Options: formschema.CountryOptions(lang)},
			},
		},
		{
			ID:    "visibility",
			Label: i18n.L("organization.form.visibility", "Visibility"),
			Fields: []formschema.FieldDef{
				{
					Name:     "visibility",
					Widget:   formschema.WidgetRadioGroup,
					Required: true,
					Label:    i18n.L("organization.form.visibility", "Visibility"),
					Value:    visibility,
					Options: []formschema.SelectOption{
						{
							Value:       "private",
							Label:       i18n.L("organization.visibility.private", "Private"),
							Description: i18n.L("organization.visibility.private_help", "Only members and privileged users can view this organization."),
						},
						{
							Value:       "public",
							Label:       i18n.L("organization.visibility.public", "Public"),
							Description: i18n.L("organization.visibility.public_help", "Visible in /orgs and on the public organization page."),
						},
					},
				},
			},
		},
	}
	return schema
}
