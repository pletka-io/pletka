package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/integrations"
	"github.com/pletka-io/pletka/pkg/integrations/registry"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	weavex3ml "github.com/pletka-io/pletka/pkg/weave/generators/x3ml"
)

// ontologyBundleReader supplies a project's linked ontologies for the
// X3ML zip artifact. Matches visualization's ontologyBundleReader so
// the projectontologyversion slice's Store satisfies both.
type ontologyBundleReader interface {
	BundleForProject(ctx context.Context, projectID string) ([]domain.OntologyBundleEntry, error)
	BundleForVersions(ctx context.Context, versionIDs []string) ([]domain.OntologyBundleEntry, error)
}

// Handler serves the integrations hub HTTP endpoints. Read-only routes
// gate on auth.ProjectRead; write + action routes gate on
// auth.ProjectEdit.
type Handler struct {
	weave            domain.WeaveStore
	svc              *Service
	gens             *generators.Service
	bundles          ontologyBundleReader
	projectArtifacts func(context.Context, string) registry.ProjectArtifactProvider
	logger           *slog.Logger
}

// NewHandler builds a Handler. The generator service + bundle reader
// are required for the action endpoint's artifact provider — the hub
// renders X3ML zips in-memory without going through HTTP. The optional
// projectArtifacts factory supplies host-level project bundles.
func NewHandler(weave domain.WeaveStore, svc *Service, gens *generators.Service, bundles ontologyBundleReader, projectArtifacts func(context.Context, string) registry.ProjectArtifactProvider, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{weave: weave, svc: svc, gens: gens, bundles: bundles, projectArtifacts: projectArtifacts, logger: logger.With("handler", "integrations.hub")}
}

func (h *Handler) loadProjectAndGate(ctx context.Context, w http.ResponseWriter, projectID string, cap auth.Capability) (*domain.Project, bool) {
	project, err := h.weave.Projects().GetByID(ctx, projectID)
	if err != nil || project == nil {
		apierror.Write(w, apierror.NotFound("Project not found"))
		return nil, false
	}
	res := projectResource(project)
	snap := auth.FromContext(ctx)
	if !snap.Can(cap, res, nil) {
		if snap.IsAnonymous {
			apierror.Write(w, apierror.Unauthorized())
		} else {
			apierror.Write(w, apierror.Forbidden("Access denied"))
		}
		return nil, false
	}
	return project, true
}

func projectResource(p *domain.Project) auth.Resource {
	visibility := "private"
	if p.Visibility != "" {
		visibility = p.Visibility
	}
	return auth.Resource{
		ScopeType:  "project",
		ID:         p.ID,
		Visibility: visibility,
	}
}

// listSchemaItem is one integration card. Each carries zero or more
// configs the project has set up, plus the URLs the frontend uses to
// add a new one.
type listSchemaItem struct {
	ID           string                  `json:"id"`
	DisplayName  domain.Localizable      `json:"display_name,omitempty"`
	Description  domain.Localizable      `json:"description,omitempty"`
	Icon         string                  `json:"icon,omitempty"`
	AddConfigURL string                  `json:"add_config_url"`
	Configs      []listSchemaConfigEntry `json:"configs"`
}

// listSchemaConfigEntry describes one stored (project, integration,
// config) row plus the URLs that mutate it. Settings-only actions
// (integrations with empty AppliesTo) surface here so operators can
// run them without an entity detail view; entity-bound actions live
// on DerivativesCap.IntegrationActions instead.
type listSchemaConfigEntry struct {
	ID                   string                    `json:"id"`
	Label                string                    `json:"label"`
	Enabled              bool                      `json:"enabled"`
	ConfigSchemaURL      string                    `json:"config_schema_url"`
	ConfigURL            string                    `json:"config_url"`
	EnableURL            string                    `json:"enable_url"`
	RemoveURL            string                    `json:"remove_url"`
	Actions              []formschema.ActionSchema `json:"actions,omitempty"`
	ManagedInstanceID    string                    `json:"managed_instance_id,omitempty"`
	ManagedInstanceLabel string                    `json:"managed_instance_label,omitempty"`
}

// listSchemaResponse is the full payload for GET /list-schema/data.
type listSchemaResponse struct {
	Integrations []listSchemaItem `json:"integrations"`
}

// ListSchema is the settings-page entry point. It returns a
// composite-pane envelope with a single panel of kind "integrations".
// The settings ProjectSettings shell sees kind=composite-pane and
// dispatches to CompositePane.svelte, which renders the panel via the
// IntegrationsPanel component. That component then fetches the actual
// per-integration data from /list-schema/data.
//
// Two endpoints (envelope + data) match the established settings
// pattern (linked-ontologies does the same) and let us reuse the
// existing settings probe + slide-form lifecycle.
//
// GET /list-schema.
func (h *Handler) ListSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	project, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}
	pane := formschema.CompositePaneSchema{
		Kind:  "composite-pane",
		Title: domain.Translations{"en": "Integrations"},
		Panels: []formschema.CompositePanel{{
			ID:        "integrations",
			Kind:      "integrations",
			SchemaURL: fmt.Sprintf("/projects/%s/integrations/list-schema/data", project.ID),
		}},
	}
	writeJSON(w, http.StatusOK, pane)
}

// ListSchemaData backs the IntegrationsPanel — the actual list of
// integrations + per-project state. GET /list-schema/data.
func (h *Handler) ListSchemaData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	project, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}
	entries, err := h.svc.List(ctx, project.ID)
	if err != nil {
		h.logger.Error("list integrations", "project", project.ID, "err", err)
		apierror.Write(w, apierror.Internal())
		return
	}
	out := listSchemaResponse{Integrations: make([]listSchemaItem, 0, len(entries))}
	for _, e := range entries {
		md := e.Integration.Metadata()
		item := listSchemaItem{
			ID:           e.Integration.ID(),
			DisplayName:  md.DisplayName,
			Description:  md.Description,
			Icon:         md.Icon,
			AddConfigURL: fmt.Sprintf("/projects/%s/integrations/%s/configs", project.ID, e.Integration.ID()),
			Configs:      make([]listSchemaConfigEntry, 0, len(e.Configs)),
		}
		settingsActions := settingsModeActions(e.Integration)
		for _, cfg := range e.Configs {
			entry := listSchemaConfigEntry{
				ID:                cfg.ConfigID,
				Label:             cfg.Label,
				Enabled:           cfg.Enabled,
				ConfigSchemaURL:   fmt.Sprintf("/projects/%s/integrations/%s/configs/%s/config-schema", project.ID, e.Integration.ID(), cfg.ConfigID),
				ConfigURL:         fmt.Sprintf("/projects/%s/integrations/%s/configs/%s/config", project.ID, e.Integration.ID(), cfg.ConfigID),
				EnableURL:         fmt.Sprintf("/projects/%s/integrations/%s/configs/%s/enable", project.ID, e.Integration.ID(), cfg.ConfigID),
				RemoveURL:         fmt.Sprintf("/projects/%s/integrations/%s/configs/%s", project.ID, e.Integration.ID(), cfg.ConfigID),
				ManagedInstanceID: cfg.ManagedInstanceID,
			}
			if cfg.ManagedInstanceID != "" {
				entry.ManagedInstanceLabel = h.svc.ResolveManagedLabel(ctx, cfg.ManagedInstanceID)
			}
			if cfg.Enabled && len(settingsActions) > 0 {
				entry.Actions = make([]formschema.ActionSchema, 0, len(settingsActions))
				for _, act := range settingsActions {
					method := "POST"
					// "status" and "audit_tail" are read-only panel
					// reads. Frontend fires them automatically when
					// the admin slide opens — GET keeps them
					// cacheable + side-effect-free.
					if act.Result == "panel" {
						method = "GET"
					}
					entry.Actions = append(entry.Actions, formschema.ActionSchema{
						Kind:     "action",
						ID:       e.Integration.ID() + "." + cfg.ConfigID + "." + act.ID,
						Label:    act.Label,
						Help:     act.Help,
						Theme:    act.Theme,
						Category: act.Category,
						Result:   act.Result,
						Endpoint: formschema.SchemaEndpoint{
							Method:  method,
							URL:     fmt.Sprintf("/projects/%s/integrations/%s/configs/%s/actions/%s", project.ID, e.Integration.ID(), cfg.ConfigID, act.ID),
							Confirm: act.Confirm,
						},
					})
				}
			}
			item.Configs = append(item.Configs, entry)
		}
		out.Integrations = append(out.Integrations, item)
	}
	writeJSON(w, http.StatusOK, out)
}

// settingsModeActions returns the actions that should surface on the
// settings panel (per-config) rather than on entity detail views. An
// integration is considered settings-only when its AppliesTo is empty
// (no generator-format binding). Entity-bound integrations like 3M
// surface their actions through DerivativesCap.IntegrationActions
// instead, so we don't double-render them.
func settingsModeActions(integ registry.Integration) []registry.ActionSpec {
	if len(integ.AppliesTo()) > 0 {
		return nil
	}
	return integ.Actions()
}

// addConfigRequestBody is the JSON body for POST /{integrationID}/configs.
// Just a label — the server assigns the ULID.
type addConfigRequestBody struct {
	Label string `json:"label"`
}

// AddConfig creates a new config row for (project, integration), then
// returns the seed entry the IntegrationsPanel uses to expand its
// inline config form. POST /{integrationID}/configs.
func (h *Handler) AddConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	integID := chi.URLParam(r, "integrationID")
	project, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}
	var body addConfigRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.Validation(map[string][]string{"body": {"invalid JSON body"}}))
		return
	}
	saved, err := h.svc.AddConfig(ctx, AddConfigInput{
		ProjectID:     project.ID,
		IntegrationID: integID,
		Label:         body.Label,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, listSchemaConfigEntry{
		ID:              saved.ConfigID,
		Label:           saved.Label,
		Enabled:         saved.Enabled,
		ConfigSchemaURL: fmt.Sprintf("/projects/%s/integrations/%s/configs/%s/config-schema", project.ID, integID, saved.ConfigID),
		ConfigURL:       fmt.Sprintf("/projects/%s/integrations/%s/configs/%s/config", project.ID, integID, saved.ConfigID),
		EnableURL:       fmt.Sprintf("/projects/%s/integrations/%s/configs/%s/enable", project.ID, integID, saved.ConfigID),
		RemoveURL:       fmt.Sprintf("/projects/%s/integrations/%s/configs/%s", project.ID, integID, saved.ConfigID),
	})
}

// ConfigSchema returns the integration's per-project config form with
// secrets redacted. GET /{integrationID}/configs/{configID}/config-schema.
func (h *Handler) ConfigSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	integID := chi.URLParam(r, "integrationID")
	configID := chi.URLParam(r, "configID")
	project, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}

	integ, _, cfg, err := h.svc.GetForConfigForm(ctx, project.ID, integID, configID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	lang := resolveLang(r)
	schema := integ.ConfigSchema(registry.ConfigRequest{
		ProjectID: project.ID,
		Lang:      lang,
		Languages: nil,
	}, cfg)
	if schema == nil {
		apierror.Write(w, apierror.BadRequest("integration has no config schema"))
		return
	}
	// Rewrite the schema's endpoint URL so the FormRenderer PUTs to the
	// config-scoped path rather than the integration-only path the
	// ConfigSchema author may have hard-coded.
	schema.Endpoint = &formschema.SchemaEndpoint{
		Method: "PUT",
		URL:    fmt.Sprintf("/projects/%s/integrations/%s/configs/%s/config", project.ID, integID, configID),
	}
	writeJSON(w, http.StatusOK, schema)
}

// UpdateConfig validates + persists a config row. Body is the flat
// config map (matches FormRenderer's submit shape). Label is preserved
// from the prior row unless the caller supplies one in the body.
//
// PUT /{integrationID}/configs/{configID}/config.
func (h *Handler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	integID := chi.URLParam(r, "integrationID")
	configID := chi.URLParam(r, "configID")
	project, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}

	var cfg map[string]any
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		apierror.Write(w, apierror.Validation(map[string][]string{"body": {"invalid JSON body"}}))
		return
	}
	// Pop a label override if present in the body so it doesn't leak
	// into the integration's ValidateConfig field map.
	label, _ := cfg["label"].(string)
	delete(cfg, "label")

	saved, fieldErrs, err := h.svc.SaveConfig(ctx, SaveConfigInput{
		ProjectID:     project.ID,
		IntegrationID: integID,
		ConfigID:      configID,
		Label:         label,
		RawConfig:     cfg,
	})
	if err != nil {
		if errors.Is(err, integrations.ErrCipherUnavailable) {
			apierror.Write(w, apierror.BadRequest("integrations.secret_key is not configured on this server — cannot store secret fields"))
			return
		}
		writeServiceError(w, err)
		return
	}
	if fieldErrs != nil {
		apierror.Write(w, apierror.Validation(fieldErrs))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"integration_id": saved.IntegrationID,
		"config_id":      saved.ConfigID,
		"label":          saved.Label,
		"enabled":        saved.Enabled,
		"updated_at":     saved.UpdatedAt,
	})
}

// enableRequestBody is the JSON body for POST /{integrationID}/configs/{configID}/enable.
type enableRequestBody struct {
	Enabled bool `json:"enabled"`
}

// SetEnabled toggles a config's enabled flag without touching config.
// POST /{integrationID}/configs/{configID}/enable.
func (h *Handler) SetEnabled(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	integID := chi.URLParam(r, "integrationID")
	configID := chi.URLParam(r, "configID")
	project, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}
	var body enableRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.Validation(map[string][]string{"body": {"invalid JSON body"}}))
		return
	}
	saved, err := h.svc.SetEnabled(ctx, project.ID, integID, configID, body.Enabled)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"integration_id": saved.IntegrationID,
		"config_id":      saved.ConfigID,
		"enabled":        saved.Enabled,
	})
}

// Remove deletes a single config row.
// DELETE /{integrationID}/configs/{configID}.
func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	integID := chi.URLParam(r, "integrationID")
	configID := chi.URLParam(r, "configID")
	project, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}
	if err := h.svc.Remove(ctx, project.ID, integID, configID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RunAction dispatches an integration action: load + decrypt config,
// build the artifact provider, invoke RunAction, return ActionResultUI.
// POST /{integrationID}/configs/{configID}/actions/{actionID}.
//
// kind/id/format query params are optional — settings-only integrations
// (Arches scaffold) skip them.
func (h *Handler) RunAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	integID := chi.URLParam(r, "integrationID")
	configID := chi.URLParam(r, "configID")
	actionID := chi.URLParam(r, "actionID")
	project, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}

	q := r.URL.Query()
	kind := registry.EntityKind(q.Get("kind"))
	entityID := q.Get("id")
	format := registry.Format(q.Get("format"))

	integ, _, cfg, err := h.svc.LoadForAction(ctx, project.ID, integID, configID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	var provider registry.ArtifactProvider
	if kind != "" && entityID != "" {
		provider = h.artifactProvider(ctx, project.ID, kind, entityID)
	}
	var projectProvider registry.ProjectArtifactProvider
	if h.projectArtifacts != nil {
		projectProvider = h.projectArtifacts(ctx, project.ID)
	}
	result, err := integ.RunAction(ctx, actionID, registry.ActionInput{
		ProjectID:       project.ID,
		EntityKind:      kind,
		EntityID:        entityID,
		Format:          format,
		Config:          cfg,
		Artifact:        provider,
		ProjectArtifact: projectProvider,
	})
	if err != nil {
		h.logger.Error("integration action", "integration", integID, "action", actionID, "err", err)
		apierror.Write(w, apierror.InternalWith("integration action failed: "+err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, formschema.ActionResultUI{
		Status:    result.Status,
		Message:   result.Message,
		LinkURL:   result.LinkURL,
		LinkLabel: result.LinkLabel,
		Panel:     result.Panel,
	})
}

// artifactProvider returns a closure that lazily renders the requested
// generator output for (kind, entityID). For X3ML formats the
// closure wraps the rendered mapping in a zip alongside the project's
// ontology bundle. The integration calls this when (and only when) it
// needs the artifact bytes — RunAction can fail fast on bad credentials
// without paying snapshot construction cost.
func (h *Handler) artifactProvider(ctx context.Context, projectID string, kind registry.EntityKind, entityID string) registry.ArtifactProvider {
	return func(format registry.Format) (io.Reader, string, error) {
		resolved, err := h.weave.Projects().ResolvedOntologyVersions(ctx, projectID, domain.ResolvedOntologyVersionOpts{})
		if err != nil {
			return nil, "", fmt.Errorf("resolve ontology versions: %w", err)
		}
		versionIDs := make([]string, 0, len(resolved))
		for _, r := range resolved {
			versionIDs = append(versionIDs, r.Link.OntologyVersionID)
		}
		bundle, err := h.bundles.BundleForVersions(ctx, versionIDs)
		if err != nil {
			return nil, "", fmt.Errorf("load ontology bundle: %w", err)
		}
		snap, err := h.snapshotFor(ctx, projectID, kind, entityID, bundle)
		if err != nil {
			return nil, "", err
		}
		var buf bytes.Buffer
		if err := h.gens.RenderSnapshot(ctx, generators.Format(format), snap, &buf); err != nil {
			return nil, "", fmt.Errorf("render %q: %w", format, err)
		}
		base := artifactBaseName(snap) + artifactExtension(format)
		switch generators.Format(format) {
		case generators.FormatX3ML, generators.FormatX3MLB:
			data, err := weavex3ml.BuildZip(base, buf.Bytes(), bundle)
			if err != nil {
				return nil, "", err
			}
			return bytes.NewReader(data), base + ".zip", nil
		default:
			return bytes.NewReader(buf.Bytes()), base, nil
		}
	}
}

func (h *Handler) snapshotFor(ctx context.Context, projectID string, kind registry.EntityKind, entityID string, bundle []domain.OntologyBundleEntry) (*generators.Snapshot, error) {
	opts := generators.Options{X3MLTargets: x3mlTargets(bundle)}
	switch generators.EntityKind(kind) {
	case generators.EntityModel:
		return h.gens.SnapshotForModel(ctx, projectID, entityID, opts)
	case generators.EntityCollection:
		return h.gens.SnapshotForCollection(ctx, projectID, entityID, opts)
	case generators.EntityField:
		return h.gens.SnapshotForField(ctx, projectID, entityID, opts)
	default:
		return nil, fmt.Errorf("unsupported entity kind %q", kind)
	}
}

func x3mlTargets(bundle []domain.OntologyBundleEntry) []generators.X3MLTarget {
	out := make([]generators.X3MLTarget, 0, len(bundle))
	for _, o := range bundle {
		out = append(out, generators.X3MLTarget{
			Prefix:     o.Prefix,
			Namespace:  o.Namespace,
			Label:      o.Name,
			Version:    o.VersionString,
			SchemaFile: o.OriginalFilename,
		})
	}
	return out
}

func artifactBaseName(snap *generators.Snapshot) string {
	switch snap.RootKind {
	case generators.EntityModel:
		if snap.Model != nil {
			return firstNonEmpty(snap.Model.SemanticID, snap.Model.SystemName, snap.Model.ID)
		}
	case generators.EntityCollection:
		if snap.Collection != nil {
			return firstNonEmpty(snap.Collection.SemanticID, snap.Collection.SystemName, snap.Collection.ID)
		}
	case generators.EntityField:
		if snap.Field != nil {
			return firstNonEmpty(snap.Field.SemanticID, snap.Field.SystemName, snap.Field.ID)
		}
	}
	return "mapping"
}

func artifactExtension(format registry.Format) string {
	switch generators.Format(format) {
	case generators.FormatX3MLB:
		return ".b.x3ml"
	case generators.FormatX3ML:
		return ".a.x3ml"
	default:
		return "." + string(format)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrIntegrationNotRegistered):
		apierror.Write(w, apierror.NotFound("integration not registered"))
	case errors.Is(err, ErrConfigNotFound):
		apierror.Write(w, apierror.NotFound("integration config not found"))
	default:
		apierror.Write(w, apierror.InternalWith(err.Error()))
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// resolveLang reads the first language tag from Accept-Language and strips
// quality params. Defaults to "en". Threading the app language resolver through
// here adds surface for marginal benefit; config form labels are usually
// authored in English and translated lazily.
func resolveLang(r *http.Request) string {
	header := r.Header.Get("Accept-Language")
	if header == "" {
		return "en"
	}
	for i, c := range header {
		if c == ',' || c == ';' {
			return header[:i]
		}
	}
	return header
}
