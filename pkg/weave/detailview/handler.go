package detailview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	pkgdomain "github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/integrations/registry"
	"github.com/pletka-io/pletka/pkg/session"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/publication"
	weaveroutes "github.com/pletka-io/pletka/pkg/weave/routes"
	"github.com/pletka-io/pletka/pkg/weave/templates"
)

// AutocompletePreloader warms the per-project autocomplete index in the
// background when an edit page renders for a logged-in user.
// Satisfied by pkg/weave/ontology.Service; nil-safe.
type AutocompletePreloader interface {
	PreloadAutocomplete(projectID string)
}

// IntegrationsLookup returns the enabled (integration_id, config_id, label)
// triples for a project. Used by derivativesFor to surface "Upload to 3M"
// style buttons on the X3ML diagram sub-tab. nil-safe: Handler tolerates a
// nil lookup (no integrations surfaced).
type IntegrationsLookup interface {
	EnabledForProject(ctx context.Context, projectID string) ([]EnabledIntegrationConfig, error)
}

// EnabledIntegrationConfig is one enabled (integration, config) pair the
// detailview surfaces as an action button.
type EnabledIntegrationConfig struct {
	IntegrationID string
	ConfigID      string
	Label         string
}

// Source returns a detached bounded selection; implementations finish every
// content read (including field usage) before returning.
type Source interface {
	Detail(context.Context, auth.ReadScope, string, string, string) (browserinput.Detail, error)
}

// Host is the narrow dependency surface needed to mount detailview routes.
type Host struct {
	Logger       *slog.Logger
	Weave        pkgdomain.WeaveStore
	I18n         i18n.Manager
	Session      *session.Manager
	Renderer     *templates.Renderer
	Source       Source
	Integrations *registry.Registry
	IntLookup    IntegrationsLookup
	Preloader    AutocompletePreloader
	// Publication derives live entities' draft/published/modified state for
	// the detail header badge, and (satisfying auth.LatestReleaseReader)
	// backs auth.ResolveContentVersion so a public project's non-editor
	// reader defaults to the latest release instead of the hot draft.
	// Optional; nil disables the badge and the release default (readers see
	// hot).
	Publication *publication.Reader
	// HasFormat reports whether a generator renderer for the format is
	// registered in this build. Wired by pkg/app from the same renderer
	// set that backs the generators service, so a core-only build (no
	// shacl/arches/cytoscape renderers) never advertises a derivative URL
	// that would 500 when clicked.
	HasFormat func(format generators.Format) bool
}

func (h Host) Validate() error {
	if h.Source == nil {
		return fmt.Errorf("detailview host missing bounded source")
	}
	if h.Weave == nil {
		return fmt.Errorf("detailview host missing weave store")
	}
	if h.Renderer == nil {
		return fmt.Errorf("detailview host missing renderer")
	}
	if h.I18n == nil {
		return fmt.Errorf("detailview host missing i18n manager")
	}
	if h.HasFormat == nil {
		return fmt.Errorf("detailview host missing HasFormat")
	}
	return nil
}

func (h Host) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

// Handler owns the detailview routes under the weave project base while the
// legacy compatibility API and page routes can be mounted elsewhere.
type Handler struct {
	logger       *slog.Logger
	weave        pkgdomain.WeaveStore
	i18n         i18n.Manager
	session      *session.Manager
	renderer     *templates.Renderer
	source       Source
	integrations *registry.Registry
	intLookup    IntegrationsLookup
	preloader    AutocompletePreloader
	publication  *publication.Reader
	hasFormat    func(format generators.Format) bool
}

// NewHandler constructs a Handler. preloader may be nil (no cache warming).
func NewHandler(
	logger *slog.Logger,
	weaveStore pkgdomain.WeaveStore,
	i18nManager i18n.Manager,
	sessionManager *session.Manager,
	renderer *templates.Renderer,
	source Source,
	integrations *registry.Registry,
	intLookup IntegrationsLookup,
	preloader AutocompletePreloader,
	pub *publication.Reader,
	hasFormat func(format generators.Format) bool,
) *Handler {
	return &Handler{
		logger:       logger,
		weave:        weaveStore,
		i18n:         i18nManager,
		session:      sessionManager,
		renderer:     renderer,
		source:       source,
		integrations: integrations,
		intLookup:    intLookup,
		preloader:    preloader,
		publication:  pub,
		hasFormat:    hasFormat,
	}
}

// Mount builds and registers detailview routes under the weave project base.
func Mount(r chi.Router, host Host) error {
	if err := host.Validate(); err != nil {
		return err
	}
	NewHandler(
		host.logger(),
		host.Weave,
		host.I18n,
		host.Session,
		host.Renderer,
		host.Source,
		host.Integrations,
		host.IntLookup,
		host.Preloader,
		host.Publication,
		host.HasFormat,
	).Mount(r)
	return nil
}

// Mount registers the parallel detailview routes under the weave project base.
func (h *Handler) Mount(r chi.Router) {
	// RequireProjectRead both loads the project into context (same
	// auth.WithProject attachment auth.ProjectFromContext/
	// auth.ResolveContentVersion read) and 404s a caller without
	// ProjectRead on it — anonymous on a private project. That replaces
	// the previous auth.WithProjectResource, which loaded+attached the
	// project but never enforced read access, leaving API/StatsAPI open
	// to anonymous reads of private project content.
	mw := []func(http.Handler) http.Handler{auth.WithProjectVersionContext, auth.RequireProjectRead(h.weave.Projects())}
	if h.publication != nil {
		// ResolveContentVersion needs the project loaded into context by
		// RequireProjectRead, so it must come after it in the chain.
		mw = append(mw, auth.ResolveContentVersion(h.publication))
	}
	r.With(mw...).Get("/projects/{projectID:[A-Z0-9]+}/entity-view/{entityType}/{entityID}", h.API)
	r.With(mw...).Get("/projects/{projectID:[A-Z0-9]+}/entity-view/{entityType}/{entityID}/stats", h.StatsAPI)
	r.With(mw...).Get("/projects/{projectID:[A-Z0-9]+}/entity-view/{entityType}/{entityID}/reuse", h.ReuseAPI)
	r.With(mw...).Get("/projects/{projectID:[A-Z0-9]+}/models/{modelID:[^/]+\\.[^/]+(?:_[^/]+)?}", h.Page("model"))
	r.With(mw...).Get("/projects/{projectID:[A-Z0-9]+}/collections/{collectionID:[^/]+\\.[^/]+(?:_[^/]+)?}", h.Page("collection"))
	r.With(mw...).Get("/projects/{projectID:[A-Z0-9]+}/fields/{fieldID:[^/]+\\.[^/]+(?:_[^/]+)?}", h.Page("field"))
	r.With(mw...).Get("/projects/{projectID:[A-Z0-9]+}/concept-lists/{conceptListID:[^/]+}", h.Page("concept-list"))
}

// Page renders the detailview page shell using the compatibility island name,
// "entity-view", but with the new weave project route base.
func (h *Handler) Page(entityType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		projectID := chi.URLParam(r, "projectID")
		activeVersion := auth.ProjectVersionFromContext(ctx)

		var entityID string
		switch entityType {
		case "model":
			entityID = chi.URLParam(r, "modelID")
		case "collection":
			entityID = chi.URLParam(r, "collectionID")
		case "field":
			entityID = chi.URLParam(r, "fieldID")
		case "concept-list":
			entityID = chi.URLParam(r, "conceptListID")
		}

		if projectID == "" || entityID == "" {
			errresp.Error(w, r, http.StatusBadRequest, "bad_request", "project and entity ID are required")
			return
		}

		entityID = strings.SplitN(entityID, "_", 2)[0]

		project := auth.ProjectFromContext(ctx)
		if project == nil {
			var err error
			project, err = h.weave.Projects().GetByID(ctx, projectID)
			if err != nil || project == nil {
				errresp.Error(w, r, http.StatusNotFound, "not_found", "project not found")
				return
			}
		}

		// Gate page access on the same project-read capability the
		// /gen/* derivatives endpoints (visualization slice) gate on.
		// Without this, anonymous viewers could reach the entity-view
		// page on a private project but every derivatives request
		// silently 404'd, so the Diagram tab looked broken. 404 (not
		// 403) so we don't leak project existence.
		snap := auth.FromContext(ctx)
		if !snap.Can(auth.ProjectRead, auth.ProjectResource(project), nil) {
			errresp.Error(w, r, http.StatusNotFound, "not_found", "project not found")
			return
		}

		// Warm the autocomplete index in the background so the first
		// path-builder keystroke hits a hot cache. Skip anonymous users —
		// they cannot open the path builder anyway.
		if h.preloader != nil && !snap.IsAnonymous && activeVersion == "" {
			h.preloader.PreloadAutocomplete(projectID)
		}

		lang := h.currentLang(r)
		content, err := h.loadDetail(ctx, auth.ReadScopeFromContext(ctx), projectID, entityType, entityID)
		if err != nil {
			e := apierror.FromError(err)
			if e.Code == apierror.CodeInternal {
				errresp.Internal(w, r, err)
			} else {
				errresp.Error(w, r, e.Status, string(e.Code), e.Message)
			}
			return
		}
		projectName := content.ProjectName.Get(lang, projectID)
		entityName := content.Response.Entity.Name.Get(lang, entityID)
		entityTypeLabel := map[string]string{"model": "Models", "collection": "Collections", "field": "Fields", "concept-list": "Concept lists"}[entityType]

		page := templates.IslandPage{
			Title:       entityID + " " + entityName + " · " + projectID,
			Lang:        lang,
			Path:        requestPathWithVersion(r, activeVersion),
			Languages:   h.i18n.Languages(),
			Principal:   auth.PrincipalFromContext(r.Context()),
			Labels:      h.renderer.ShellLabels(lang),
			Breadcrumbs: h.breadcrumbs(projectID, projectName, entityType, entityTypeLabel, entityName, activeVersion),
			Island: templates.IslandMount{
				Name: frontendrefs.Island("entity-view"),
				Props: map[string]string{
					"project-id":  projectID,
					"entity-type": entityType,
					"entity-id":   entityID,
					"lang":        lang,
					"route-base":  weaveroutes.ProjectBase,
				},
				Dependencies: []string{frontendrefs.Island("entity-view")},
				Placeholder:  placeholderHTML(),
			},
		}

		if err := h.renderer.RenderIslandPage(w, page); err != nil {
			h.logger.Error("render detailview page", "project_id", projectID, "entity_type", entityType, "entity_id", entityID, "err", err)
			h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang), err)
		}
	}
}

// API serves the detailview API under the new weave compatibility path.
func (h *Handler) API(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	entityType := chi.URLParam(r, "entityType")
	entityID := chi.URLParam(r, "entityID")

	if projectID == "" || entityType == "" || entityID == "" {
		h.writeAPIError(w, "projectID, entityType, and entityID are required", http.StatusBadRequest)
		return
	}

	content, err := h.loadDetail(ctx, auth.ReadScopeFromContext(ctx), projectID, entityType, entityID)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	resp := h.decorateDetail(ctx, content)
	h.writeLocalizedJSON(w, r, http.StatusOK, resp)
}

// StatsAPI serves detailview statistics under the new weave path.
func (h *Handler) StatsAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	entityType := chi.URLParam(r, "entityType")
	entityID := chi.URLParam(r, "entityID")

	if projectID == "" || entityType == "" || entityID == "" {
		h.writeAPIError(w, "projectID, entityType, and entityID are required", http.StatusBadRequest)
		return
	}

	if entityType != "model" && entityType != "collection" && entityType != "field" {
		apierror.Write(w, r, apierror.BadRequest("unsupported statistics entity type"))
		return
	}
	content, err := h.loadDetail(ctx, auth.ReadScopeFromContext(ctx), projectID, entityType, entityID)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	h.writeJSON(w, http.StatusOK, content.Stats)
}

// ReuseAPI serves current global usage after validating working-view membership.
// A release cannot select the historical state of unrelated consuming projects,
// so released views omit this capability and direct versioned requests fail.
func (h *Handler) ReuseAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	entityType := chi.URLParam(r, "entityType")
	entityID := chi.URLParam(r, "entityID")

	if projectID == "" || entityType == "" || entityID == "" {
		h.writeAPIError(w, "projectID, entityType, and entityID are required", http.StatusBadRequest)
		return
	}

	if auth.ReadScopeFromContext(ctx).IsRelease() {
		apierror.Write(w, r, apierror.BadRequest("global reuse is available only in the working view"))
		return
	}
	if _, err := h.loadDetail(ctx, auth.ReadScopeFromContext(ctx), projectID, entityType, entityID); err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}

	project, err := h.weave.Projects().GetByID(ctx, projectID)
	if err != nil || project == nil {
		h.writeAPIError(w, "project not found", http.StatusNotFound)
		return
	}
	if !auth.FromContext(ctx).Can(auth.ProjectRead, auth.ProjectResource(project), nil) {
		h.writeAPIError(w, "project not found", http.StatusNotFound)
		return
	}

	resp, err := BuildReuse(ctx, h.weave, projectID, entityType, entityID)
	if err != nil {
		if errors.Is(err, errUnsupportedReuseEntityType) {
			h.writeAPIError(w, err.Error(), http.StatusBadRequest)
			return
		}
		var ue *reuseUsageError
		if errors.As(err, &ue) {
			h.logger.Error(ue.op, ue.idKey, entityID, "project_id", projectID, "err", ue.err)
		}
		apierror.Write(w, r, apierror.InternalWith(err.Error(), err))
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// errUnsupportedReuseEntityType marks a BuildReuse failure the caller should
// surface as 400 Bad Request; every other BuildReuse error is a store
// failure the caller surfaces as 500.
var errUnsupportedReuseEntityType = errors.New("reuse not supported for entity type")

// reuseUsageError wraps a ListUsage failure inside BuildReuse with enough
// context (slog op name + id field key) for the HTTP handler to log with the
// same fidelity as the pre-refactor per-branch log calls, without BuildReuse
// itself depending on a logger.
type reuseUsageError struct {
	op    string
	idKey string
	msg   string // exact text surfaced to the HTTP caller via writeAPIError
	err   error  // underlying store error, logged but not exposed in the response
}

func (e *reuseUsageError) Error() string { return e.msg }
func (e *reuseUsageError) Unwrap() error { return e.err }

// BuildReuse builds the included_in/referenced_by payload for an entity —
// the models and collections in the project that reference it. Shared by
// the ReuseAPI HTTP handler and the MCP entity_reuse tool; this is the
// single source of truth for the reuse payload (per the dispatcher rule,
// MCP must not rebuild it in parallel).
func BuildReuse(ctx context.Context, weave pkgdomain.WeaveStore, projectID, entityType, entityID string) (*FieldReuseResponse, error) {
	var resp FieldReuseResponse

	switch entityType {
	case "field":
		// A field is only ever a member (included in models/collections). It
		// cannot be a value target. → IncludedIn only.
		usage, err := weave.WeaveFields().ListUsage(ctx, entityID, projectID)
		if err != nil {
			return nil, &reuseUsageError{op: "list field usage", idKey: "field_id", msg: "failed to list field usage", err: err}
		}
		resp.IncludedIn = partitionFieldUsage(ctx, weave, projectID, usage)
	case "model":
		// A model is never bundled as a member; it is targeted as a value type
		// by fields. → ReferencedBy only.
		usage, err := weave.Models().ListUsage(ctx, entityID)
		if err != nil {
			return nil, &reuseUsageError{op: "list model usage", idKey: "model_id", msg: "failed to list model usage", err: err}
		}
		resp.ReferencedBy = partitionFieldUsage(ctx, weave, projectID, usage)
	case "collection":
		// A collection has both relationships: bundled by a model
		// (part_of_collection → IncludedIn) and targeted as a field value type
		// (weave_override_refs collection_model → ReferencedBy). The
		// value-target query is entity-agnostic, so reuse the model store's
		// ListUsage with the collection's id.
		bundling, err := weave.Collections().ListUsage(ctx, entityID, projectID)
		if err != nil {
			return nil, &reuseUsageError{op: "list collection usage", idKey: "collection_id", msg: "failed to list collection usage", err: err}
		}
		target, err := weave.Models().ListUsage(ctx, entityID)
		if err != nil {
			return nil, &reuseUsageError{op: "list collection target usage", idKey: "collection_id", msg: "failed to list collection usage", err: err}
		}
		resp.IncludedIn = partitionFieldUsage(ctx, weave, projectID, pkgdomain.FieldUsageList{Models: bundling})
		resp.ReferencedBy = partitionFieldUsage(ctx, weave, projectID, target)
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedReuseEntityType, entityType)
	}

	return &resp, nil
}

// pubState returns the live entity's publication state (draft/published/
// modified/new) for the header badge. Returns "" when browsing a release
// (activeVersion set — the archived status already reads published) or when no
// publication reader is wired.
func (h *Handler) pubState(ctx context.Context, projectID, entityType, entityID, activeVersion string) string {
	if h.publication == nil || activeVersion != "" {
		return ""
	}
	m, err := h.publication.BatchState(ctx, projectID, entityType, []string{entityID})
	if err != nil {
		h.logger.Warn("publication state", "entity_type", entityType, "entity_id", entityID, "err", err)
		return ""
	}
	return string(m[entityID])
}

// dedupeUsageRefs drops duplicate usage refs, keyed by project+semantic id,
// preserving first-seen order.
func dedupeUsageRefs(refs []pkgdomain.FieldUsageRef) []pkgdomain.FieldUsageRef {
	seen := make(map[string]struct{}, len(refs))
	out := refs[:0]
	for _, r := range refs {
		k := r.ProjectID + "|" + r.SemanticID
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, r)
	}
	return out
}

// partitionFieldUsage splits one relationship's usages into a ReuseSection:
// same-project buckets (Models/Collections — the "This project" sub-tab) and
// per-project groups (OtherProjects — the "Other projects" sub-tab), resolving
// each other project's display name. Groups preserve first-seen order (the
// store sorts by project_id).
func partitionFieldUsage(ctx context.Context, weave pkgdomain.WeaveStore, projectID string, usage pkgdomain.FieldUsageList) *ReuseSection {
	// Dedupe: the same entity can reach a target via multiple override rows →
	// duplicate ref ids crash the frontend's keyed {#each}. Key by project+id.
	usage.Models = dedupeUsageRefs(usage.Models)
	usage.Collections = dedupeUsageRefs(usage.Collections)
	sec := &ReuseSection{
		Models:      []pkgdomain.FieldUsageRef{},
		Collections: []pkgdomain.FieldUsageRef{},
	}
	groups := map[string]*ProjectUsageGroup{}
	var order []string
	other := func(ref pkgdomain.FieldUsageRef, isModel bool) {
		g := groups[ref.ProjectID]
		if g == nil {
			name := ref.ProjectID
			if p, err := weave.Projects().GetByID(ctx, ref.ProjectID); err == nil && p != nil {
				name = p.UIName.Get("en", p.SystemName)
			}
			g = &ProjectUsageGroup{ProjectID: ref.ProjectID, ProjectName: name}
			groups[ref.ProjectID] = g
			order = append(order, ref.ProjectID)
		}
		if isModel {
			g.Models = append(g.Models, ref)
		} else {
			g.Collections = append(g.Collections, ref)
		}
	}
	for _, m := range usage.Models {
		if m.ProjectID == "" || m.ProjectID == projectID {
			sec.Models = append(sec.Models, m)
		} else {
			other(m, true)
		}
	}
	for _, c := range usage.Collections {
		if c.ProjectID == "" || c.ProjectID == projectID {
			sec.Collections = append(sec.Collections, c)
		} else {
			other(c, false)
		}
	}
	for _, pid := range order {
		sec.OtherProjects = append(sec.OtherProjects, *groups[pid])
	}
	return sec
}

// breadcrumbs uses the selected project/entity display names supplied by the
// bounded projection and preserves the selection in navigation links.
func (h *Handler) breadcrumbs(projectID, projectName, entityType, entityTypeLabel, entityName, activeVersion string) []templates.Breadcrumb {
	proj := withVersion(fmt.Sprintf("%s/%s", weaveroutes.ProjectBase, projectID), activeVersion)
	// Pivots always carry their Href — the active one is rendered with
	// a stronger style but stays clickable so users can jump back to
	// the list of the current entity type (bold breadcrumb wasn't a
	// link previously).
	pivot := func(kind, label string) templates.Pivot {
		return templates.Pivot{
			Label:  label,
			Href:   proj + "#tab=" + kind + "s",
			Active: entityType == kind,
		}
	}
	_ = entityTypeLabel
	return []templates.Breadcrumb{
		{Label: h.i18n.T("projects.title", "en"), Href: weaveroutes.ProjectBase},
		{Label: projectName, Href: proj},
		{Pivots: []templates.Pivot{
			pivot("model", "Models"),
			pivot("collection", "Collections"),
			pivot("field", "Fields"),
			pivot("concept-list", "Concept lists"),
		}},
		{Label: entityName},
	}
}

func (h *Handler) currentLang(r *http.Request) string {
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

// hasProjectEdit uses current permissions even while selected content is read-only.
func (h *Handler) hasProjectEdit(ctx context.Context, projectID string) bool {
	return auth.FromContext(ctx).Can(auth.ProjectEdit, h.projectResource(ctx, projectID), nil)
}

func capabilityURL(enabled bool, url string) string {
	if !enabled {
		return ""
	}
	return url
}

// lifecycleCaps returns the delete/deprecate/activate URLs for the detail-page
// "More actions" kebab. All empty unless the entity is editable and we're on
// the current version (lifecycle actions don't apply to a pinned historical
// version). base is the entity's collection route, e.g.
// "/projects/{p}/models/{id}": delete is DELETE base, and exactly one of
// deprecate/activate is set to match the entity's current status. Mirrors the
// list-row actions so both surfaces offer the same lifecycle.
func lifecycleCaps(editable, inUse bool, activeVersion, base string, deprecated bool) (del, deprecate, activate string) {
	if !editable || activeVersion != "" {
		return "", "", ""
	}
	// An in-use entity can't be deleted (the endpoint refuses with 409) — offer
	// Deprecate only, matching the list row-action gating (owned && !in_use).
	if !inUse {
		del = base
	}
	if deprecated {
		activate = base + "/activate"
	} else {
		deprecate = base + "/deprecate"
	}
	return del, deprecate, activate
}

// projectResource loads projectID and returns the auth.Resource that
// drives capability checks for surfaces composed off this project.
// Returns a zero-value Resource (ScopeType="project", everything else
// empty) if the project can't be loaded — callers' Can() checks will
// fail closed in that case.
func (h *Handler) projectResource(ctx context.Context, projectID string) auth.Resource {
	resource := auth.ProjectResourceFromContext(ctx)
	if resource.ID != "" {
		return resource
	}
	project, err := h.weave.Projects().GetByID(ctx, projectID)
	if err != nil || project == nil {
		return auth.ProjectResource(nil)
	}
	return auth.ProjectResource(project)
}

// derivativesFor returns the URL block the visualization slice exposes
// for an entity of the given kind ("models", "collections", "fields")
// and ID. Single source of truth for the /gen/* URL shape — frontend
// (DiagramTab) reads from these instead of constructing URLs itself,
// matching .claude/rules/api-patterns.md (schema is the complete
// contract).
//
// Per-format gating: each URL is emitted only when the caller (per
// snap) has the right to use it. URLs the caller can't use come back
// empty; the frontend's DiagramTab hides the corresponding sub-tab
// when its URL is empty. This is what schema-driven URLs buy us — the
// server alone decides who sees what, no client-side role logic to
// keep in sync.
//
// Current policy:
//   - Diagram, Turtle, JSON-LD, SHACL: any reader who passed the
//     project-read gate already (no per-format capability needed)
//   - Export Graph: gated on auth.DerivativeExportGraphRead — granted
//     to superadmin only today via the IsSuperAdmin bypass in
//     snap.Can(). Loosen by adding the capability to project-role
//     capability sets in pkg/auth/capabilities.go.
//
// Loosen or tighten policy by editing the capability mappings, NOT
// this function. This handler stays a thin "for each format, ask
// snap.Can()" loop. Future plan: move the capability/role mapping
// from code to TOML / DB so admins can re-tune without redeploying.
func (h *Handler) derivativesFor(ctx context.Context, snap *auth.AuthSnapshot, projectResource auth.Resource, kind, id, activeVersion string) *DerivativesCap {
	// Keep the consumer context even when the entity belongs to a pinned source.
	// A release's root need not exist in the live entity tables.
	if projectResource.ID == "" {
		return nil
	}
	base := "/projects/" + projectResource.ID + "/gen/" + kind + "/" + id

	urlIfCan := func(cap auth.Capability, suffix string) string {
		if snap.Can(cap, projectResource, nil) {
			return withVersion(base+suffix, activeVersion)
		}
		return ""
	}

	// CSV uses the per-entity exports route, NOT /gen/. Member-only
	// gating mirrors the project-level verification CSV: the sub-tab
	// is for project participants who want to see what their entity
	// looks like in the dump, not for drive-by readers.
	csvURL := ""
	if snap.IsProjectMember(projectResource) && projectResource.ID != "" {
		csvURL = withVersion(fmt.Sprintf("/projects/%s/exports/%s/%s.csv", projectResource.ID, kind, id), activeVersion)
	}

	return &DerivativesCap{
		DiagramURL:         h.urlIfFormat(generators.FormatMermaid, withVersion(base+"/diagram", activeVersion)),
		TurtleURL:          h.urlIfFormat(generators.FormatTurtle, withVersion(base+"/turtle", activeVersion)),
		JSONLDURL:          h.urlIfFormat(generators.FormatJSONLD, withVersion(base+"/jsonld", activeVersion)),
		SHACLURL:           h.urlIfFormat(generators.FormatSHACL, urlIfCan(auth.DerivativeSHACLRead, "/shacl")),
		SPARQLURL:          h.urlIfFormat(generators.FormatSPARQL, urlIfCan(auth.DerivativeSPARQLRead, "/sparql")),
		X3MLAURL:           h.urlIfFormat(generators.FormatX3ML, withVersion(base+"/x3ml-a", activeVersion)),
		X3MLBURL:           h.urlIfFormat(generators.FormatX3MLB, withVersion(base+"/x3ml-b", activeVersion)),
		X3MLAZipURL:        h.urlIfFormat(generators.FormatX3ML, withVersion(base+"/x3ml-a?bundle=zip", activeVersion)),
		X3MLBZipURL:        h.urlIfFormat(generators.FormatX3MLB, withVersion(base+"/x3ml-b?bundle=zip", activeVersion)),
		ResearchSpaceURL:   h.urlIfFormat(generators.FormatResearchSpace, researchSpaceURLFor(snap, projectResource, kind, base, activeVersion)),
		ArchesURL:          h.urlIfFormat(generators.FormatArches, archesURLFor(snap, projectResource, kind, base, activeVersion)),
		SnapshotURL:        snapshotURLFor(snap, projectResource, kind, base, activeVersion),
		ASCIITreeURL:       asciiTreeURLFor(snap, projectResource, kind, base, activeVersion),
		ExportGraphURL:     h.urlIfFormat(generators.FormatExportGraph, urlIfCan(auth.DerivativeExportGraphRead, "/exportgraph")),
		CytoscapeURL:       h.urlIfFormat(generators.FormatCytoscape, withVersion(base+"/cytoscape", activeVersion)),
		CSVURL:             csvURL,
		IntegrationActions: h.integrationActionsFor(ctx, snap, projectResource, kind, id),
	}
}

// urlIfFormat returns url only when a generator renderer for format is
// registered in this build (see Host.HasFormat). It keeps the existing
// role/capability gating AND-ed: callers pass a url that is already ""
// when the auth gate failed, so an empty url stays empty regardless of
// format availability.
func (h *Handler) urlIfFormat(format generators.Format, url string) string {
	if url == "" {
		return ""
	}
	if h.hasFormat != nil && h.hasFormat(format) {
		return url
	}
	return ""
}

// integrationActionsFor enumerates ActionSchema entries for the
// per-project integrations the entity's X3ML formats apply to. The
// frontend's DiagramTab renders these in the X3ML sub-tab via
// ActionButton.svelte — no integration-specific Svelte code.
//
// Gated on auth.ProjectEdit: integration actions modify external state
// (uploading mappings), so they match the hub's own write gate.
// Drive-by readers see no buttons.
//
// Errors during lookup are logged but not propagated — a misbehaving
// integration store should not break the detail page; the buttons
// simply do not appear.
func (h *Handler) integrationActionsFor(ctx context.Context, snap *auth.AuthSnapshot, projectResource auth.Resource, kind, entityID string) []formschema.ActionGroupSchema {
	if h.integrations == nil || h.intLookup == nil {
		return nil
	}
	if !snap.Can(auth.ProjectEdit, projectResource, nil) {
		return nil
	}
	enabled, err := h.intLookup.EnabledForProject(ctx, projectResource.ID)
	if err != nil {
		h.logger.Warn("integrations lookup failed", "project", projectResource.ID, "err", err)
		return nil
	}
	if len(enabled) == 0 {
		return nil
	}
	// The hub's action endpoint expects EntityKind in the singular
	// (matches generators.EntityKind values); derivatives use the
	// plural URL segment. Convert.
	entityKind := singularKind(kind)

	// Group enabled configs by integration so we can emit one
	// ActionGroupSchema per (integration, action) with all targets the
	// project has enabled.
	configsByIntegration := map[string][]EnabledIntegrationConfig{}
	for _, e := range enabled {
		configsByIntegration[e.IntegrationID] = append(configsByIntegration[e.IntegrationID], e)
	}

	out := make([]formschema.ActionGroupSchema, 0)
	for integID, configs := range configsByIntegration {
		integ, ok := h.integrations.Integration(integID)
		if !ok {
			continue
		}
		// Only surface integrations whose AppliesTo covers x3ml-b —
		// that's the rendered mapping the hub will hand off on this
		// surface.
		appliesToX3MLB := false
		for _, f := range integ.AppliesTo() {
			if f == "x3ml-b" {
				appliesToX3MLB = true
				break
			}
		}
		if !appliesToX3MLB {
			continue
		}
		for _, act := range integ.Actions() {
			targets := make([]formschema.ActionTarget, 0, len(configs))
			for _, cfg := range configs {
				label := cfg.Label
				if label == "" {
					label = cfg.ConfigID
				}
				targets = append(targets, formschema.ActionTarget{
					ID:    cfg.ConfigID,
					Label: label,
					Endpoint: formschema.SchemaEndpoint{
						Method:  "POST",
						URL:     fmt.Sprintf("/projects/%s/integrations/%s/configs/%s/actions/%s?kind=%s&id=%s&format=x3ml-b", projectResource.ID, integID, cfg.ConfigID, act.ID, entityKind, entityID),
						Confirm: act.Confirm,
					},
				})
			}
			out = append(out, formschema.ActionGroupSchema{
				Kind:    "action-group",
				ID:      integID + "." + act.ID,
				Label:   act.Label,
				Help:    act.Help,
				Theme:   act.Theme,
				Targets: targets,
			})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// singularKind converts the URL-segment kind ("models", "collections",
// "fields") into the generators.EntityKind value ("model", etc.) the
// hub's action endpoint expects via the ?kind= query param.
func singularKind(kind string) string {
	switch kind {
	case "models":
		return "model"
	case "collections":
		return "collection"
	case "fields":
		return "field"
	default:
		return kind
	}
}

// researchSpaceURLFor returns the ResearchSpace YAML endpoint when the
// entity kind is "models" — the Python original only exposes RS for
// model exports, so we mirror that scope. Gated through the shared
// derivative policy (super_admin-only today, fail closed) so the emit
// matches visualization.canDerivativeFormat. Other entity kinds get an
// empty URL and the schema's omitempty drops the field.
func researchSpaceURLFor(snap *auth.AuthSnapshot, r auth.Resource, kind, base, activeVersion string) string {
	if snap == nil || !snap.CanDerivative(string(generators.FormatResearchSpace), r) {
		return ""
	}
	if kind != "models" {
		return ""
	}
	return withVersion(base+"/researchspace", activeVersion)
}

// archesURLFor exposes the Arches Resource Graph endpoint only for
// super-admins and only on models. Anyone else sees an empty URL and the
// DiagramTab hides the sub-view.
func archesURLFor(snap *auth.AuthSnapshot, r auth.Resource, kind, base, activeVersion string) string {
	if kind != "models" {
		return ""
	}
	if !snap.CanDerivative(string(generators.FormatArches), r) {
		return ""
	}
	return withVersion(base+"/arches", activeVersion)
}

// snapshotURLFor exposes the renderer-neutral Snapshot JSON endpoint —
// super-admins only, models only — for verifying the generated
// path_node / path_node_id identities.
func snapshotURLFor(snap *auth.AuthSnapshot, r auth.Resource, kind, base, activeVersion string) string {
	if kind != "models" {
		return ""
	}
	if snap == nil || !snap.CanDerivative(string(generators.FormatSnapshot), r) {
		return ""
	}
	return withVersion(base+"/snapshot", activeVersion)
}

// asciiTreeURLFor exposes the Snapshot rendered as a human-readable
// ASCII tree — super-admins only, models only. Same audience as
// snapshotURLFor; the ascii tree is the curator-facing view for
// reviewing how every semantic intermediate is named.
func asciiTreeURLFor(snap *auth.AuthSnapshot, r auth.Resource, kind, base, activeVersion string) string {
	if kind != "models" {
		return ""
	}
	if snap == nil || !snap.CanDerivative(string(generators.FormatASCIITree), r) {
		return ""
	}
	return withVersion(base+"/ascii-tree", activeVersion)
}

func requestPathWithVersion(r *http.Request, activeVersion string) string {
	if activeVersion == "" {
		return r.URL.Path
	}
	return withVersion(r.URL.Path, activeVersion)
}

func releaseView(projectID, activeVersion string, canEdit bool) *ReleaseView {
	if activeVersion == "" {
		return nil
	}
	view := &ReleaseView{
		Version: activeVersion,
		Label: i18n.LF("release.viewing_version", "Viewing release {version}",
			map[string]string{"version": activeVersion}),
	}
	if canEdit {
		view.DraftURL = fmt.Sprintf("%s/%s", weaveroutes.ProjectBase, projectID)
	}
	return view
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

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.logger.Error("encode detailview response", "err", err)
	}
}

// writeLocalizedJSON walks v with h.i18n.Resolve before encoding so any
// LocalizedText nodes inside (origin_label, fork_label, …) ship as
// Translations maps the frontend can pick from with tr(). Use this for
// detail-view responses; the plain writeJSON stays for error/stat
// payloads that carry no LocalizedText.
func (h *Handler) writeLocalizedJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if h.i18n != nil {
		h.i18n.Resolve(v, h.currentLang(r))
	}
	h.writeJSON(w, status, v)
}

func (h *Handler) writeAPIError(w http.ResponseWriter, msg string, status int) {
	h.writeJSON(w, status, map[string]string{"error": msg})
}

// originDetailLabel + originDetailHelp build the LocalizedText nodes
// the detail page's provenance badge + tooltip render. Mirror the
// list-row label family (common.origin_state.*) plus a help/tooltip
// family (common.origin_help.*). Kept in pkg/weave/detailview so the
// detail-view package owns its full schema — no leakage to the slice
// handlers.
