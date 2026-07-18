package apikey

import (
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildListSchema returns the ListManager schema for the caller's API keys
// on /profile/settings. Rows come from GET /me/api-keys (bare array).
// Revoke is a POST row action, hidden on already-revoked rows; there is no
// delete — revoked rows stay visible as an audit trail.
func BuildListSchema(lang string, languages []formschema.LanguageInfo) *formschema.ListSchema {
	return &formschema.ListSchema{
		EntityType: "api_key",
		Title:      i18n.L("profile.apikeys.title", "API keys"),
		EmptyState: &formschema.EmptyState{
			Icon:    "key",
			Title:   i18n.L("profile.apikeys.empty_title", "No API keys yet"),
			Message: i18n.L("profile.apikeys.empty_subtitle", "Create a key to connect agents and tools (MCP) to Pletka."),
		},
		DataURL: "/me/api-keys",
		Caps: formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("profile.apikeys.create", "Create key"),
				FormSchemaURL: "/me/api-keys/form-schema",
			},
		},
		Columns: []formschema.Column{
			{Key: "name", Label: i18n.L("profile.apikeys.col_name", "Name"), Type: "text", Primary: true},
			{Key: "key_prefix", Label: i18n.L("profile.apikeys.col_prefix", "Prefix"), Type: "text_badge"},
			{Key: "status", Label: i18n.L("profile.apikeys.col_status", "Status"), Type: "text_badge",
				BadgeStyleByValue: map[string]string{"active": "green", "revoked": "gray", "expired": "gray"}},
			{Key: "created_at", Label: i18n.L("profile.apikeys.col_created", "Created"), Type: "text", Secondary: true},
			{Key: "last_used_at", Label: i18n.L("profile.apikeys.col_last_used", "Last used"), Type: "text", Secondary: true},
		},
		RowActions: []formschema.RowAction{
			{
				ID:          "revoke",
				Icon:        "no-symbol",
				Label:       i18n.L("profile.apikeys.revoke", "Revoke"),
				Style:       "danger",
				URLTemplate: "/me/api-keys/{id}/revoke",
				Method:      "POST",
				VisibleWhen: "!revoked_at",
			},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

// BuildCreateFormSchema returns the create form: a name and an optional
// expiry in days. UI.RevealField surfaces the plaintext secret exactly once
// from the 201 response body.
func BuildCreateFormSchema(lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	schema := &formschema.FormSchema{
		EntityType: "api_key",
		Mode:       formschema.ModeCreate,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "POST",
			URL:    "/me/api-keys",
		},
		Sections: []formschema.Section{
			{
				ID:    "key",
				Label: i18n.L("profile.apikeys.section", "New API key"),
				Fields: []formschema.FieldDef{
					{
						Name:     "name",
						Widget:   formschema.WidgetText,
						Label:    i18n.L("profile.apikeys.field_name", "Name"),
						Help:     i18n.L("profile.apikeys.field_name_help", "What this key is for, e.g. 'Claude on my laptop'."),
						Required: true,
					},
					{
						Name:   "expires_days",
						Widget: formschema.WidgetNumber,
						Label:  i18n.L("profile.apikeys.field_expires", "Expires after (days)"),
						Help:   i18n.L("profile.apikeys.field_expires_help", "Leave blank for no expiry."),
					},
				},
			},
		},
	}
	schema.UI.RevealField = "secret"
	schema.UI.RevealLabel = i18n.L("profile.apikeys.reveal", "API key (copy now — shown only once)")
	schema.UI.SuccessMessage = i18n.L("profile.apikeys.created", "API key created")
	schema.UI.Languages = languages
	schema.UI.PrimaryLang = lang
	return schema
}
