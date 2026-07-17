package organization

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
)

func BuildEntityListSchema(canCreate bool, lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	caps := &formschema.Capabilities{}
	if canCreate {
		caps.Create = &formschema.CreateCap{
			Label:         i18n.L("organization.list.add", "New Organization"),
			FormSchemaURL: "/profile/orgs/form-schema",
		}
	}

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
		Filters: []formschema.FilterConfig{
			{
				Key:         "country",
				Label:       i18n.L("forms.fields.country", "Country"),
				ParamName:   "country",
				Type:        "select",
				OptionsURL:  "/orgs/filters/countries",
				OptionsType: "checklist",
				Multi:       true,
			},
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
		Capabilities: caps,
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

func BuildCreateFormSchema(lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	return &formschema.FormSchema{
		EntityType: "organization",
		Mode:       formschema.ModeCreate,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "POST",
			URL:    "/profile/orgs",
		},
		Sections: []formschema.Section{
			{
				ID: "identity",
				Fields: []formschema.FieldDef{
					{
						Name:     "display_name",
						Widget:   formschema.WidgetText,
						Required: true,
						Label:    i18n.L("organization.form.display_name", "Organization name"),
					},
					{
						Name:        "slug",
						Widget:      formschema.WidgetSlugInput,
						Required:    true,
						DerivedFrom: "display_name",
						Label:       i18n.L("forms.fields.slug", "Slug"),
						Help:        i18n.L("organization.form.slug_help", "Suggested from the organization name. You can adjust it before creating the organization. Immutable after creation."),
						Validation:  &formschema.ValidationRules{Pattern: "^[a-z0-9](?:[a-z0-9]|[-_][a-z0-9]){1,49}$"},
					},
					{
						Name:   "acronym",
						Widget: formschema.WidgetText,
						Label:  i18n.L("forms.fields.acronym", "Acronym"),
					},
					{
						Name:    "country",
						Widget:  formschema.WidgetSearchSelect,
						Label:   i18n.L("forms.fields.country", "Country"),
						Options: formschema.CountryOptions(lang),
					},
					{
						Name:   "website",
						Widget: formschema.WidgetText,
						Label:  i18n.L("forms.fields.website", "Website"),
					},
				},
			},
		},
		UI: formschema.SchemaUI{
			Languages:                  languages,
			PrimaryLang:                lang,
			SubmitLabel:                i18n.L("organization.form.submit_create", "Create Organization"),
			CancelLabel:                i18n.L("forms.cancel", "Cancel"),
			SuccessMessage:             i18n.L("organization.form.created", "Organization created"),
			SuccessRedirectURLTemplate: "/orgs/{id}/settings",
		},
	}
}

func BuildSettingsSchema(org *domain.Organization, lang string, languages []formschema.LanguageInfo) *formschema.SettingsSchema {
	return &formschema.SettingsSchema{
		ProjectID:   org.Slug,
		ProjectName: domain.Translations{"en": org.DisplayName},
		Sections: []formschema.SettingsSection{
			{
				ID:        "general",
				Label:     i18n.L("project_settings.section.general", "General"),
				Icon:      "cog-6-tooth",
				Kind:      "form",
				SchemaURL: fmt.Sprintf("/orgs/%s/settings/form-schema/general", org.Slug),
			},
			{
				ID:        "members",
				Label:     i18n.L("project_settings.section.members", "Members"),
				Icon:      "users",
				Kind:      "list",
				SchemaURL: fmt.Sprintf("/orgs/%s/members/list-schema", org.Slug),
			},
		},
	}
}

func BuildGeneralFormSchema(org *domain.Organization, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	return &formschema.FormSchema{
		EntityType: "organization_settings",
		Mode:       formschema.ModeEdit,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "PUT",
			URL:    fmt.Sprintf("/orgs/%s/settings/general", org.Slug),
		},
		Sections: []formschema.Section{
			{
				ID: "identity",
				Fields: []formschema.FieldDef{
					{
						Name:     "display_name",
						Widget:   formschema.WidgetText,
						Required: true,
						Label:    i18n.L("organization.form.display_name", "Organization name"),
						Value:    org.DisplayName,
					},
					{
						Name:     "slug",
						Widget:   formschema.WidgetText,
						Readonly: true,
						Label:    i18n.L("forms.fields.slug", "Slug"),
						Help:     i18n.L("organization.form.slug_immutable_help", "Immutable after creation."),
						Value:    org.Slug,
					},
					{
						Name:   "acronym",
						Widget: formschema.WidgetText,
						Label:  i18n.L("forms.fields.acronym", "Acronym"),
						Value:  deref(org.Acronym),
					},
					{
						Name:    "country",
						Widget:  formschema.WidgetSearchSelect,
						Label:   i18n.L("forms.fields.country", "Country"),
						Value:   deref(org.Country),
						Options: formschema.CountryOptions(lang),
					},
					{
						Name:   "website",
						Widget: formschema.WidgetText,
						Label:  i18n.L("forms.fields.website", "Website"),
						Value:  deref(org.Website),
					},
				},
			},
			{
				ID: "visibility",
				Fields: []formschema.FieldDef{
					{
						Name:     "visibility",
						Widget:   formschema.WidgetRadioGroup,
						Required: true,
						Label:    i18n.L("organization.form.visibility", "Visibility"),
						Value:    org.Visibility,
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
		},
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("forms.save_changes", "Save Changes"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("organization.form.updated", "Organization updated successfully"),
		},
	}
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
