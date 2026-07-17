package threem

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/integrations/registry"
)

// Production defaults that match FORTH's stock 3M servlet layout.
// Operators can override per-project via the config form.
const (
	defaultLoginPath         = "/Login"
	defaultUploadPath        = "/ImportXML?type=Mapping"
	defaultEditorURLTemplate = "/3MEditor/Index?type=Mapping&id={id}&lang=en"
)

// id is the canonical registry ID for this integration. It is used as both
// the registry key and the URL segment in
// /projects/{projectID}/integrations/{integrationID}.
const id = "threem"

// New returns a fresh integration.Integration ready to be registered.
// No state at construction time — per-project config arrives through
// ActionInput.Config every time the hub dispatches an action.
func New() registry.Integration { return &integration{} }

type integration struct{}

// ID is the integration's stable identifier. Treat as opaque.
func (i *integration) ID() string { return id }

func (i *integration) Metadata() registry.Metadata {
	return registry.Metadata{
		DisplayName: i18n.L("integrations.threem.display_name", "FORTH 3M"),
		Description: i18n.L("integrations.threem.description", "Upload X3ML mappings to a FORTH 3M instance."),
		Icon:        "upload-cloud",
	}
}

// AppliesTo declares the generator formats this integration consumes.
// The hub uses this to decide which entity sub-tabs surface an action
// button. 3M takes both X3ML A (single document) and X3ML B (split
// collection-spine / field-tail).
func (i *integration) AppliesTo() []registry.Format {
	return []registry.Format{"x3ml", "x3ml-b"}
}

// SecretFields are the config keys the hub encrypts before persisting
// and redacts before echoing back through config-schema.
func (i *integration) SecretFields() []string { return []string{"password"} }

// Actions declares one action: "upload" — POST the X3ML zip to the
// configured 3M instance and surface the new mapping URI.
func (i *integration) Actions() []registry.ActionSpec {
	return []registry.ActionSpec{{
		ID:        "upload",
		Label:     i18n.L("integrations.threem.action.upload.label", "Upload to 3M"),
		Help:      i18n.L("integrations.threem.action.upload.help", "Send this mapping to the configured 3M instance and link to the new mapping."),
		AppliesTo: []registry.Format{"x3ml", "x3ml-b"},
	}}
}

// RunAction handles the single declared action. The hub has already
// loaded + decrypted the per-project config; we only need to build a
// Client and stream the artifact provider through it.
func (i *integration) RunAction(ctx context.Context, actionID string, in registry.ActionInput) (registry.ActionResult, error) {
	if actionID != "upload" {
		return registry.ActionResult{}, fmt.Errorf("unknown action %q", actionID)
	}
	cfg, err := readConfig(in.Config)
	if err != nil {
		return registry.ActionResult{}, err
	}
	format := in.Format
	if format == "" {
		format = "x3ml-b"
	}

	client, err := NewClient(cfg.BaseURL, cfg.Username, cfg.Password, cfg.Timeout)
	if err != nil {
		return registry.ActionResult{}, fmt.Errorf("3m client: %w", err)
	}
	client.LoginPath = cfg.LoginPath
	client.UploadPath = cfg.UploadPath
	if err := client.Login(ctx); err != nil {
		if errors.Is(err, ErrBadCredentials) {
			return registry.ActionResult{
				Status:  "error",
				Message: i18n.L("integrations.threem.action.error.bad_credentials", "3M rejected the configured credentials."),
			}, nil
		}
		return registry.ActionResult{}, fmt.Errorf("3m login: %w", err)
	}

	body, filename, err := in.Artifact(format)
	if err != nil {
		return registry.ActionResult{}, fmt.Errorf("build artifact: %w", err)
	}

	uri, err := client.UploadMapping(ctx, filename, body)
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionExpired):
			// Try once more — session may have been recycled between
			// Login and Upload on a slow snapshot render.
			if loginErr := client.Login(ctx); loginErr != nil {
				return registry.ActionResult{}, fmt.Errorf("3m re-login after session expiry: %w", loginErr)
			}
			retryBody, retryFilename, artifactErr := in.Artifact(format)
			if artifactErr != nil {
				return registry.ActionResult{}, fmt.Errorf("rebuild artifact for retry: %w", artifactErr)
			}
			uri, err = client.UploadMapping(ctx, retryFilename, retryBody)
			if err != nil {
				return registry.ActionResult{}, fmt.Errorf("3m upload (retry): %w", err)
			}
		case errors.Is(err, ErrUploadRejected):
			return registry.ActionResult{
				Status:  "error",
				Message: i18n.L("integrations.threem.action.error.rejected", "3M accepted the request but did not return a mapping URI. The mapping may not have been saved."),
			}, nil
		default:
			return registry.ActionResult{}, fmt.Errorf("3m upload: %w", err)
		}
	}

	return registry.ActionResult{
		Status:    "success",
		Message:   i18n.L("integrations.threem.action.success.message", "Mapping uploaded to 3M."),
		LinkURL:   BuildEditorURL(cfg.BaseURL, uri, cfg.EditorURLTemplate),
		LinkLabel: i18n.L("integrations.threem.action.success.link_label", "Open in 3M"),
	}, nil
}

// threemConfig is the typed per-project config the integration consumes.
type threemConfig struct {
	BaseURL           string
	Username          string
	Password          string
	Timeout           time.Duration
	LoginPath         string
	UploadPath        string
	EditorURLTemplate string
}

// readConfig pulls + validates fields out of the raw map. Defensive:
// the hub has already run ValidateConfig but RunAction is called from
// the action endpoint which trusts the prior validation only insofar
// as the integration confirms.
func readConfig(raw map[string]any) (threemConfig, error) {
	cfg := threemConfig{Timeout: 60 * time.Second}
	if v, _ := raw["base_url"].(string); v != "" {
		cfg.BaseURL = strings.TrimRight(v, "/")
	}
	if cfg.BaseURL == "" {
		return cfg, fmt.Errorf("base_url is empty")
	}
	if v, _ := raw["username"].(string); v != "" {
		cfg.Username = v
	}
	if v, _ := raw["password"].(string); v != "" {
		cfg.Password = v
	}
	if v, ok := raw["timeout_seconds"]; ok {
		switch t := v.(type) {
		case float64:
			cfg.Timeout = time.Duration(t) * time.Second
		case int:
			cfg.Timeout = time.Duration(t) * time.Second
		}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if v, _ := raw["login_path"].(string); v != "" {
		cfg.LoginPath = normaliseSubPath(v)
	} else {
		cfg.LoginPath = defaultLoginPath
	}
	if v, _ := raw["upload_path"].(string); v != "" {
		cfg.UploadPath = normaliseSubPath(v)
	} else {
		cfg.UploadPath = defaultUploadPath
	}
	if v, _ := raw["editor_url_template"].(string); strings.TrimSpace(v) != "" {
		cfg.EditorURLTemplate = strings.TrimSpace(v)
	} else {
		cfg.EditorURLTemplate = defaultEditorURLTemplate
	}
	return cfg, nil
}

// BuildEditorURL takes the mapping URI 3M returned (e.g.
// http://cidoc_mappings.com/Mapping/Mapping646) and constructs the
// browseable editor URL the user follows from the success banner.
// Combines the configured base URL's scheme + host with the
// editor_url_template, substituting {id} with the parsed mapping ID.
// Falls back to the raw URI when the inputs don't compose cleanly so
// the user still gets *some* link.
func BuildEditorURL(baseURL, mappingURI, template string) string {
	id := ExtractMappingID(mappingURI)
	if id == "" {
		return mappingURI
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return mappingURI
	}
	path := strings.ReplaceAll(template, "{id}", id)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return u.Scheme + "://" + u.Host + path
}

// normaliseSubPath ensures the path begins with a single leading slash
// and has no trailing slash, so NewClient + the base URL concatenation
// produce a clean URL regardless of how the operator typed it.
func normaliseSubPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

// ValidateConfig is the formschema-side check the hub runs before
// persisting. Field errors map to 422 with the canonical
// {"errors":{"field":["msg"]}} payload.
func (i *integration) ValidateConfig(raw map[string]any) (map[string]any, map[string][]string) {
	errs := map[string][]string{}
	baseURL, _ := raw["base_url"].(string)
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		errs["base_url"] = []string{"3M base URL is required"}
	} else if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		errs["base_url"] = []string{"3M base URL must start with http:// or https://"}
	}
	username, _ := raw["username"].(string)
	if strings.TrimSpace(username) == "" {
		errs["username"] = []string{"username is required"}
	}
	// password may be empty here when the caller is editing an
	// existing row — the hub will preserve the prior ciphertext.
	if len(errs) > 0 {
		return nil, errs
	}
	out := map[string]any{
		"base_url": strings.TrimRight(baseURL, "/"),
		"username": strings.TrimSpace(username),
	}
	if pw, ok := raw["password"].(string); ok {
		out["password"] = pw
	}
	if t, ok := raw["timeout_seconds"]; ok {
		out["timeout_seconds"] = t
	}
	if v, _ := raw["login_path"].(string); strings.TrimSpace(v) != "" {
		out["login_path"] = normaliseSubPath(v)
	}
	if v, _ := raw["upload_path"].(string); strings.TrimSpace(v) != "" {
		out["upload_path"] = normaliseSubPath(v)
	}
	if v, _ := raw["editor_url_template"].(string); strings.TrimSpace(v) != "" {
		out["editor_url_template"] = strings.TrimSpace(v)
	}
	return out, nil
}

// ConfigSchema renders the per-project config form. Plain-text widgets
// for everything except the password, which uses WidgetPassword so the
// frontend never echoes the prior value.
func (i *integration) ConfigSchema(req registry.ConfigRequest, current map[string]any) *formschema.FormSchema {
	getStr := func(k string) string {
		if v, ok := current[k].(string); ok {
			return v
		}
		return ""
	}
	getStrOr := func(k, fallback string) string {
		if v, ok := current[k].(string); ok && v != "" {
			return v
		}
		return fallback
	}
	getNum := func(k string) any {
		if v, ok := current[k]; ok {
			return v
		}
		return 60
	}
	return &formschema.FormSchema{
		EntityType: "integration.threem",
		Mode:       "edit",
		Endpoint: &formschema.SchemaEndpoint{
			Method: "PUT",
			URL:    fmt.Sprintf("/projects/%s/integrations/%s/config", req.ProjectID, id),
		},
		Sections: []formschema.Section{{
			ID:    "connection",
			Label: i18n.L("integrations.threem.section.connection", "3M connection"),
			Fields: []formschema.FieldDef{
				{
					Name:     "base_url",
					Widget:   formschema.WidgetText,
					Required: true,
					Label:    i18n.L("integrations.threem.field.base_url.label", "Base URL"),
					Help:     i18n.L("integrations.threem.field.base_url.help", "Root URL of the 3M instance (no trailing slash)."),
					Value:    getStr("base_url"),
				},
				{
					Name:     "username",
					Widget:   formschema.WidgetText,
					Required: true,
					Label:    i18n.L("integrations.threem.field.username.label", "Username"),
					Value:    getStr("username"),
				},
				{
					Name:   "password",
					Widget: formschema.WidgetPassword,
					Label:  i18n.L("integrations.threem.field.password.label", "Password"),
					Help:   i18n.L("integrations.threem.field.password.help", "Leave blank to keep the previously stored password."),
					// Always render blank — secrets never leave the
					// server. The hub preserves the prior ciphertext
					// when this field arrives empty.
					Value: "",
				},
				{
					Name:   "timeout_seconds",
					Widget: formschema.WidgetNumber,
					Label:  i18n.L("integrations.threem.field.timeout_seconds.label", "Request timeout (seconds)"),
					Value:  getNum("timeout_seconds"),
				},
				{
					Name:   "login_path",
					Widget: formschema.WidgetText,
					Label:  i18n.L("integrations.threem.field.login_path.label", "Login path"),
					Help:   i18n.L("integrations.threem.field.login_path.help", "Form-login URL. Defaults to /Login on stock 3M; override when your deployment uses a different servlet."),
					Value:  getStrOr("login_path", defaultLoginPath),
				},
				{
					Name:   "upload_path",
					Widget: formschema.WidgetText,
					Label:  i18n.L("integrations.threem.field.upload_path.label", "Upload path"),
					Help:   i18n.L("integrations.threem.field.upload_path.help", "Mapping upload URL. Defaults to /ImportXML?type=Mapping on stock 3M."),
					Value:  getStrOr("upload_path", defaultUploadPath),
				},
				{
					Name:   "editor_url_template",
					Widget: formschema.WidgetText,
					Label:  i18n.L("integrations.threem.field.editor_url_template.label", "Editor URL template"),
					Help:   i18n.L("integrations.threem.field.editor_url_template.help", "Sibling path on the same host used to build the success banner's 'Open in 3M' link. Use {id} as the mapping ID placeholder."),
					Value:  getStrOr("editor_url_template", defaultEditorURLTemplate),
				},
			},
		}},
		UI: formschema.SchemaUI{
			SubmitLabel:    i18n.L("common.save", "Save"),
			CancelLabel:    i18n.L("common.cancel", "Cancel"),
			SuccessMessage: i18n.L("integrations.threem.success.message", "3M configuration saved."),
			Languages:      req.Languages,
			PrimaryLang:    "en",
		},
	}
}
