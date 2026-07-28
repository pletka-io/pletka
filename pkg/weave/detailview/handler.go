package detailview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	pkgdomain "github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/integrations/registry"
	"github.com/pletka-io/pletka/pkg/session"
	weavepkg "github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/actorlabels"
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

// Host is the narrow dependency surface needed to mount detailview routes.
type Host struct {
	Logger       *slog.Logger
	Weave        pkgdomain.WeaveStore
	I18n         i18n.Manager
	Session      *session.Manager
	Renderer     *templates.Renderer
	Actors       actorlabels.Reader
	Integrations *registry.Registry
	IntLookup    IntegrationsLookup
	Preloader    AutocompletePreloader
	// Publication derives live entities' draft/published/modified state for
	// the detail header badge. Optional; nil disables the badge.
	Publication *publication.Reader
	// HasFormat reports whether a generator renderer for the format is
	// registered in this build. Wired by pkg/app from the same renderer
	// set that backs the generators service, so a core-only build (no
	// shacl/arches/cytoscape renderers) never advertises a derivative URL
	// that would 500 when clicked.
	HasFormat func(format generators.Format) bool
}

func (h Host) Validate() error {
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
	actors       actorlabels.Reader
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
	actors actorlabels.Reader,
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
		actors:       actors,
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
		host.Actors,
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
	r.With(auth.WithProjectVersionContext, auth.WithProjectResource(h.weave)).Get("/projects/{projectID:[A-Z0-9]+}/entity-view/{entityType}/{entityID}", h.API)
	r.With(auth.WithProjectVersionContext, auth.WithProjectResource(h.weave)).Get("/projects/{projectID:[A-Z0-9]+}/entity-view/{entityType}/{entityID}/stats", h.StatsAPI)
	r.With(auth.WithProjectVersionContext, auth.WithProjectResource(h.weave)).Get("/projects/{projectID:[A-Z0-9]+}/entity-view/{entityType}/{entityID}/reuse", h.ReuseAPI)
	r.With(auth.WithProjectVersionContext, auth.WithProjectResource(h.weave)).Get("/projects/{projectID:[A-Z0-9]+}/models/{modelID:[^/]+\\.[^/]+(?:_[^/]+)?}", h.Page("model"))
	r.With(auth.WithProjectVersionContext, auth.WithProjectResource(h.weave)).Get("/projects/{projectID:[A-Z0-9]+}/collections/{collectionID:[^/]+\\.[^/]+(?:_[^/]+)?}", h.Page("collection"))
	r.With(auth.WithProjectVersionContext, auth.WithProjectResource(h.weave)).Get("/projects/{projectID:[A-Z0-9]+}/fields/{fieldID:[^/]+\\.[^/]+(?:_[^/]+)?}", h.Page("field"))
	r.With(auth.WithProjectVersionContext, auth.WithProjectResource(h.weave)).Get("/projects/{projectID:[A-Z0-9]+}/concept-lists/{conceptListID:[^/]+}", h.Page("concept-list"))
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
		if h.preloader != nil && !snap.IsAnonymous {
			h.preloader.PreloadAutocomplete(projectID)
		}

		lang := h.currentLang(r)
		projectName := project.UIName.Get(lang, projectID)
		entityName, entityTypeLabel, err := h.entityPageMeta(ctx, entityType, entityID, lang)
		if err != nil {
			errresp.Error(w, r, http.StatusNotFound, "not_found", err.Error())
			return
		}

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
			h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang))
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

	var (
		resp *Response
		err  error
	)

	switch entityType {
	case "model":
		resp, err = h.buildModel(ctx, projectID, entityID)
	case "collection":
		resp, err = h.buildCollection(ctx, projectID, entityID)
	case "field":
		resp, err = h.buildField(ctx, projectID, entityID)
	case "concept-list":
		resp, err = h.buildConceptList(ctx, projectID, entityID)
	default:
		h.writeAPIError(w, fmt.Sprintf("unsupported entity type: %s", entityType), http.StatusBadRequest)
		return
	}

	if err != nil {
		h.logger.Error("build detailview response", "entity_type", entityType, "entity_id", entityID, "err", err)
		h.writeAPIError(w, "Failed to build detailview response", http.StatusInternalServerError)
		return
	}

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

	var stats pkgdomain.ModelViewStats
	var err error

	switch entityType {
	case "model":
		var view *pkgdomain.ModelView
		view, err = h.weave.ModelView(ctx, entityID, projectID)
		if err == nil {
			stats = view.Stats
		}
	case "collection":
		var rawFields []pkgdomain.ResolvedField
		rawFields, err = h.weave.CollectionView(ctx, entityID, projectID)
		if err == nil {
			// Hidden fields must not be counted in
			// the read-only Stats tab either — same filter buildCollection
			// applies before grouping (handler.go ~line 694).
			fields := filterVisibleFields(rawFields)
			grouped := make(map[string][]pkgdomain.ResolvedField)
			for _, f := range fields {
				grouped[f.CategoryID] = append(grouped[f.CategoryID], f)
			}
			sections := make([]ViewSection, 0, len(grouped))
			for catID, catFields := range grouped {
				name := pkgdomain.Translations{"en": catID}
				if catID == "" {
					name = pkgdomain.Translations{"en": "Uncategorized"}
				}
				sections = append(sections, ViewSection{
					ID:   catID,
					Name: name,
					Items: []ViewItem{{
						Fields: convertFields(catFields, ViewRefs{}, projectID, auth.ProjectVersionFromContext(ctx)),
					}},
				})
			}
			stats = computeCollectionStats(fields, sections)
		}
	case "field":
		// Field stats are usage signals — how many models / collections
		// in this project reference this field, plus the total override
		// row count. Frontend StatsTab branches on
		// entity_type to render the right card set.
		var counts pkgdomain.FieldUsageCounts
		counts, err = h.weave.WeaveFields().CountUsage(ctx, entityID, projectID)
		if err == nil {
			stats = pkgdomain.ModelViewStats{
				ModelsUsing:        counts.ModelsUsing,
				CollectionsUsing:   counts.CollectionsUsing,
				FieldOverrideCount: counts.OverrideRowCount,
			}
		}
	default:
		h.writeAPIError(w, fmt.Sprintf("unsupported entity type: %s", entityType), http.StatusBadRequest)
		return
	}

	if err != nil {
		h.logger.Error("build detailview stats", "entity_type", entityType, "entity_id", entityID, "err", err)
		h.writeAPIError(w, "failed to compute stats", http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusOK, stats)
}

// ReuseAPI serves the reuse list for an entity — the models and
// collections in the project that reference it. Today supports field
// and collection. Models are deferred (overlaps with
// the model-in-context graph).
//
// Same-project only today. Cross-project reuse arrives later via the
// explicit-adoption / parent-inheritance plans; this endpoint will
// extend its payload (e.g. a `projects` slice) when those land. The
// URL stays stable; frontend keeps reading from
// capabilities.reuse_url.
//
// Response shape is always {models, collections} — collection-reuse
// only fills `models` (collections never include other collections),
// `collections` stays empty. Frontend ReuseTab branches on entity
// type for column labels.
func (h *Handler) ReuseAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	entityType := chi.URLParam(r, "entityType")
	entityID := chi.URLParam(r, "entityID")

	if projectID == "" || entityType == "" || entityID == "" {
		h.writeAPIError(w, "projectID, entityType, and entityID are required", http.StatusBadRequest)
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
		h.writeAPIError(w, err.Error(), http.StatusInternalServerError)
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

func (h *Handler) entityPageMeta(ctx context.Context, entityType, entityID, lang string) (name, label string, err error) {
	switch entityType {
	case "model":
		m, e := h.weave.Models().GetByID(ctx, entityID)
		if e != nil || m == nil {
			return "", "", fmt.Errorf("model not found")
		}
		return m.UIName.Get(lang, entityID), "Models", nil
	case "collection":
		c, e := h.weave.Collections().GetByID(ctx, entityID)
		if e != nil || c == nil {
			return "", "", fmt.Errorf("collection not found")
		}
		return c.UIName.Get(lang, entityID), "Collections", nil
	case "field":
		f, e := h.weave.WeaveFields().GetByID(ctx, entityID)
		if e != nil || f == nil {
			return "", "", fmt.Errorf("field not found")
		}
		return f.UIName.Get(lang, entityID), "Fields", nil
	case "concept-list":
		list, e := h.weave.ConceptLists().GetByID(ctx, entityID)
		if e != nil || list == nil {
			return "", "", fmt.Errorf("concept list not found")
		}
		return list.UIName.Get(lang, entityID), "Concept lists", nil
	default:
		return "", "", fmt.Errorf("unsupported entity type")
	}
}

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

func (h *Handler) buildModel(ctx context.Context, projectID, modelID string) (*Response, error) {
	projectResource := h.projectResource(ctx, projectID)
	canEdit := h.canEditProject(ctx, projectID)
	activeVersion := auth.ProjectVersionFromContext(ctx)
	forkedOrigins, err := weavepkg.ForkOriginsForProject(ctx, h.weave, projectID, "model")
	if err != nil {
		return nil, fmt.Errorf("list model forks: %w", err)
	}
	adoptedOrigins, err := weavepkg.AdoptionOriginsForProject(ctx, h.weave, projectID, "model")
	if err != nil {
		return nil, fmt.Errorf("list model adoptions: %w", err)
	}
	model, err := h.weave.Models().GetByID(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("get model: %w", err)
	}
	if model == nil {
		return nil, fmt.Errorf("model not found: %s", modelID)
	}
	// Cross-project adopted/inherited entities have no current-project
	// overrides until Adapt is invoked. Building the view in the
	// current project's scope would return zero fields. Resolve in the
	// model's owning project's scope so the curator sees what the
	// adopted entity actually looks like upstream (read-only). Once
	// the entity is Adapted, model.ProjectID == projectID and this
	// branch is a no-op.
	overrideScope := projectID
	if model.ProjectID != "" && model.ProjectID != projectID {
		overrideScope = model.ProjectID
	}
	view, err := h.weave.ModelView(ctx, modelID, overrideScope)
	if err != nil {
		return nil, fmt.Errorf("build model view: %w", err)
	}

	refs := ViewRefs{
		Categories:  make(map[string]ViewRefEntry),
		Collections: make(map[string]ViewRefEntry),
		Models:      make(map[string]ViewRefEntry),
	}

	var sections []ViewSection
	for _, cat := range view.Categories {
		refs.Categories[cat.ID] = ViewRefEntry{Name: cat.Name, Order: cat.Position, URL: withVersion(pkgdomain.ParseSemanticID(cat.ID).URL(), activeVersion)}
		var items []ViewItem
		for _, coll := range cat.Collections {
			// A collection group hidden at placement level is
			// fully removed from the read-only view (same policy as hidden
			// fields). The override editor still receives it.
			if coll.Placement != nil && coll.Placement.IsHidden {
				continue
			}
			refs.Collections[coll.ID] = ViewRefEntry{Name: coll.Name, URL: withVersion(pkgdomain.ParseSemanticID(coll.ID).URL(), activeVersion)}
			// Hidden fields are dropped before FieldCount is
			// derived so the read-only view never renders or counts them.
			fields := convertFields(filterVisibleFields(coll.Fields), refs, projectID, activeVersion)
			// SourceURL: skip the synthetic 'Direct Fields' bucket
			// (collectionWidget returns "field-group" for that one) — it
			// has no standalone detail page.
			sourceURL := ""
			if coll.ID != "" && collectionWidget(coll.ID) == "collection-group" {
				sourceURL = withVersion(pkgdomain.ParseSemanticID(coll.ID).URL(), activeVersion)
			}
			items = append(items, ViewItem{
				Widget:       collectionWidget(coll.ID),
				ID:           coll.ID,
				Name:         coll.Name,
				FieldCount:   len(fields),
				Fields:       fields,
				SharedPrefix: coll.SharedPathPrefix,
				Placement:    coll.Placement,
				SourceURL:    sourceURL,
			})
		}
		sections = append(sections, ViewSection{
			Widget:         "category-group",
			ID:             cat.ID,
			Name:           cat.Name,
			CanonicalOrder: cat.Position,
			Items:          items,
		})
	}

	sortSectionsByOrder(sections)
	entityOrigin := weavepkg.ResolveReuseOrigin(forkedOrigins, adoptedOrigins, projectID, model.ProjectID, model.ID)
	// Upgrade inherited → adopted_reference when the current project's
	// overrides reference this model. Matches the four-state taxonomy
	// from the list rule (task 3b) so the provenance card on the detail
	// page reads consistently with the list-row badge.
	if entityOrigin.Kind == pkgdomain.OriginInherited {
		if refModels, err := h.weave.Models().ListReferenceAdopted(ctx, projectID); err == nil {
			for _, m := range refModels {
				if m.ID == model.ID {
					entityOrigin = pkgdomain.Origin{
						Kind:            pkgdomain.OriginAdoptedReference,
						SourceProjectID: model.ProjectID,
						SourceEntityID:  model.ID,
					}
					break
				}
			}
		}
	}
	entityEditable := canEdit && (entityOrigin.Kind == pkgdomain.OriginOwn || entityOrigin.Kind == pkgdomain.OriginForked)
	adoptURL := ""
	forkURL := ""
	if canEdit && activeVersion == "" {
		switch entityOrigin.Kind {
		case pkgdomain.OriginInherited:
			adoptURL = fmt.Sprintf("/projects/%s/models/%s/adopt", projectID, modelID)
		case pkgdomain.OriginAdopted:
			forkURL = fmt.Sprintf("/projects/%s/models/%s/fork", projectID, modelID)
		}
	}
	adoptions, err := h.adoptionItemsForContext(ctx, projectID, "model", modelID, activeVersion)
	if err != nil {
		return nil, fmt.Errorf("list model adoptions: %w", err)
	}
	// In use = referenced as a value target by a field in any project
	// (weave_override_refs) — the same relationship the model Reuse tab shows.
	// An in-use model gates to Deprecate-only; the delete endpoint also refuses
	// with 409 as a server-side backstop.
	modelInUse := false
	if usage, uErr := h.weave.Models().ListUsage(ctx, modelID); uErr == nil {
		modelInUse = len(usage.Models) > 0 || len(usage.Collections) > 0
	}
	modelDeleteURL, modelDeprecateURL, modelActivateURL := lifecycleCaps(entityEditable, modelInUse, activeVersion, fmt.Sprintf("/projects/%s/models/%s", projectID, modelID), model.Deprecated)
	return &Response{
		Entity: EntityViewMeta{
			ID:               model.ID,
			Type:             "model",
			Name:             model.UIName,
			Description:      model.Description,
			SystemName:       model.SystemName,
			Status:           string(model.Status),
			PublicationState: h.pubState(ctx, projectID, "model", modelID, activeVersion),
			Deprecated:       model.Deprecated,
			Origin:           entityOrigin,
			OriginLabel:      originDetailLabel(entityOrigin),
			OriginHelp:       originDetailHelp(entityOrigin),
			Scope:            &ViewScopeInfo{LocalName: model.OntologyScope.LocalName, Prefix: model.OntologyScope.Prefix},
			CreatedAt:        model.CreatedAt,
			UpdatedAt:        model.UpdatedAt,
			ModelType:        model.ModelType,
		},
		Capabilities: ViewCapabilities{
			Editable:          entityEditable,
			CanAddFields:      entityEditable,
			CanReorder:        entityEditable,
			SaveURL:           capabilityURL(entityEditable, withVersion(fmt.Sprintf("/projects/%s/models/%s/overrides", projectID, modelID), activeVersion)),
			MetadataURL:       capabilityURL(entityEditable, withVersion(fmt.Sprintf("/projects/%s/form-schema/model?mode=edit&entity_id=%s", projectID, modelID), activeVersion)),
			AdoptURL:          adoptURL,
			AdoptLabel:        i18n.L("common.capabilities.adopt", "Adopt"),
			ForkURL:           forkURL,
			ForkLabel:         i18n.L("common.capabilities.adapt", "Adapt to edit"),
			ForkPending:       i18n.L("common.capabilities.adapt_pending", "Adapting…"),
			SearchFieldsURL:   withVersion(fmt.Sprintf("/projects/%s/fields/search", projectID), activeVersion),
			ExamplesSchemaURL: capabilityURL(activeVersion == "", withVersion(fmt.Sprintf("/projects/%s/entity-list-schema/example?entity_type=model&entity_id=%s", projectID, modelID), activeVersion)),
			ReuseURL:          withVersion(fmt.Sprintf("%s/%s/entity-view/model/%s/reuse", weaveroutes.ProjectBase, projectID, modelID), activeVersion),
			Derivatives:       h.derivativesFor(ctx, auth.FromContext(ctx), projectResource, "models", modelID, activeVersion),
			DeleteURL:         modelDeleteURL,
			DeprecateURL:      modelDeprecateURL,
			ActivateURL:       modelActivateURL,
		},
		ViewMode:  ViewModeDetailed,
		Sections:  sections,
		Adoptions: adoptions,
		Refs:      refs,
		StatsURL:  withVersion(fmt.Sprintf("%s/%s/entity-view/model/%s/stats", weaveroutes.ProjectBase, projectID, modelID), activeVersion),
		Release:   releaseView(projectID, activeVersion),
	}, nil
}

func (h *Handler) buildCollection(ctx context.Context, projectID, collectionID string) (*Response, error) {
	projectResource := h.projectResource(ctx, projectID)
	canEdit := h.canEditProject(ctx, projectID)
	activeVersion := auth.ProjectVersionFromContext(ctx)
	forkedOrigins, err := weavepkg.ForkOriginsForProject(ctx, h.weave, projectID, "collection")
	if err != nil {
		return nil, fmt.Errorf("list collection forks: %w", err)
	}
	adoptedOrigins, err := weavepkg.AdoptionOriginsForProject(ctx, h.weave, projectID, "collection")
	if err != nil {
		return nil, fmt.Errorf("list collection adoptions: %w", err)
	}
	collection, err := h.weave.Collections().GetByID(ctx, collectionID)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	if collection == nil {
		return nil, fmt.Errorf("collection not found: %s", collectionID)
	}

	// Cross-project read-only view — see buildModel for the rationale.
	collOverrideScope := projectID
	if collection.ProjectID != "" && collection.ProjectID != projectID {
		collOverrideScope = collection.ProjectID
	}
	resolvedFields, err := h.weave.CollectionView(ctx, collectionID, collOverrideScope)
	if err != nil {
		return nil, fmt.Errorf("build collection view: %w", err)
	}
	categories, err := h.weave.WeaveCategories().List(ctx, pkgdomain.WithProjectID(projectID))
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	categoryOrigins, err := weavepkg.CategoryAdoptionOriginsBySystemName(ctx, h.weave, projectID, categories)
	if err != nil {
		return nil, fmt.Errorf("list category adoptions: %w", err)
	}

	categoryMap := make(map[string]*categoryInfo, len(categories))
	for _, cat := range categories {
		origin := pkgdomain.OwnOrigin()
		if adopted, ok := categoryOrigins[cat.SystemName]; ok {
			origin = adopted
		}
		categoryMap[cat.ID] = &categoryInfo{name: cat.UIName, order: cat.CanonicalOrder, origin: origin}
	}

	// Hidden fields are dropped before grouping so the
	// read-only view never renders or counts them (and a category made up
	// entirely of hidden fields emits no section at all).
	grouped := make(map[string][]pkgdomain.ResolvedField)
	for _, f := range filterVisibleFields(resolvedFields) {
		grouped[f.CategoryID] = append(grouped[f.CategoryID], f)
	}

	refs := ViewRefs{
		Categories:  make(map[string]ViewRefEntry),
		Collections: make(map[string]ViewRefEntry),
		Models:      make(map[string]ViewRefEntry),
	}

	sections := make([]ViewSection, 0)
	for catID, fields := range grouped {
		catName := pkgdomain.Translations{"en": catID}
		catOrder := 0
		if info, ok := categoryMap[catID]; ok {
			catName = info.name
			catOrder = info.order
		}
		catOrigin := pkgdomain.OriginFromSemanticID(projectID, catID)
		if info, ok := categoryMap[catID]; ok && info.origin.Kind != "" {
			catOrigin = info.origin
		}
		refs.Categories[catID] = ViewRefEntry{
			Name:   catName,
			Order:  catOrder,
			URL:    withVersion(pkgdomain.ParseSemanticID(catID).URL(), activeVersion),
			Origin: catOrigin,
		}
		convertedFields := convertFields(fields, refs, projectID, activeVersion)
		// This section always renders as widget "field-group" — the same
		// "direct fields" branch the frontend uses for the model page's
		// __direct__ bucket (see group-kind.ts isDirectFieldGroup). These
		// category buckets share only a display category, not an ontology
		// root, so — same as the model view direct bucket — no shared
		// prefix is hoisted here; each field renders its own full path.
		sections = append(sections, ViewSection{
			Widget:         "category-group",
			ID:             catID,
			Name:           catName,
			CanonicalOrder: catOrder,
			Items: []ViewItem{{
				Widget:     "field-group",
				ID:         collectionID,
				FieldCount: len(convertedFields),
				Fields:     convertedFields,
			}},
		})
	}

	sortSectionsByOrder(sections)
	entityOrigin := weavepkg.ResolveReuseOrigin(forkedOrigins, adoptedOrigins, projectID, collection.ProjectID, collection.ID)
	// Upgrade inherited → adopted_reference when the current project's
	// overrides reference this collection — see model detail builder.
	if entityOrigin.Kind == pkgdomain.OriginInherited {
		if refColls, err := h.weave.Collections().ListReferenceAdopted(ctx, projectID); err == nil {
			for _, c := range refColls {
				if c.ID == collection.ID {
					entityOrigin = pkgdomain.Origin{
						Kind:            pkgdomain.OriginAdoptedReference,
						SourceProjectID: collection.ProjectID,
						SourceEntityID:  collection.ID,
					}
					break
				}
			}
		}
	}
	entityEditable := canEdit && (entityOrigin.Kind == pkgdomain.OriginOwn || entityOrigin.Kind == pkgdomain.OriginForked)
	forkURL := ""
	if canEdit && activeVersion == "" && entityOrigin.Kind == pkgdomain.OriginAdopted {
		forkURL = fmt.Sprintf("/projects/%s/collections/%s/fork", projectID, collectionID)
	}
	adoptions, err := h.adoptionItemsForContext(ctx, projectID, "collection", collectionID, activeVersion)
	if err != nil {
		return nil, fmt.Errorf("list collection context adoptions: %w", err)
	}
	// In use = bundled by a model (part_of_collection) OR targeted as a field
	// value type across any project (weave_override_refs collection_model) —
	// same two paths the reuse tab shows. Either blocks delete.
	collInUse := false
	if usage, uErr := h.weave.Collections().ListUsage(ctx, collectionID, projectID); uErr == nil {
		collInUse = len(usage) > 0
	}
	if !collInUse {
		if target, uErr := h.weave.Models().ListUsage(ctx, collectionID); uErr == nil {
			collInUse = len(target.Models) > 0 || len(target.Collections) > 0
		}
	}
	collDeleteURL, collDeprecateURL, collActivateURL := lifecycleCaps(entityEditable, collInUse, activeVersion, fmt.Sprintf("/projects/%s/collections/%s", projectID, collectionID), collection.Deprecated)
	return &Response{
		Entity: EntityViewMeta{
			ID:               collection.ID,
			Type:             "collection",
			Name:             collection.UIName,
			Description:      collection.Description,
			SystemName:       collection.SystemName,
			Status:           string(collection.Status),
			PublicationState: h.pubState(ctx, projectID, "collection", collectionID, activeVersion),
			Deprecated:       collection.Deprecated,
			Origin:           entityOrigin,
			OriginLabel:      originDetailLabel(entityOrigin),
			OriginHelp:       originDetailHelp(entityOrigin),
			Scope:            &ViewScopeInfo{LocalName: collection.OntologyScope.LocalName, Prefix: collection.OntologyScope.Prefix},
			CreatedAt:        collection.CreatedAt,
			UpdatedAt:        collection.UpdatedAt,
		},
		Capabilities: ViewCapabilities{
			Editable:        entityEditable,
			CanAddFields:    entityEditable,
			CanReorder:      entityEditable,
			SaveURL:         capabilityURL(entityEditable, withVersion(fmt.Sprintf("/projects/%s/collections/%s/overrides", projectID, collectionID), activeVersion)),
			MetadataURL:     capabilityURL(entityEditable, withVersion(fmt.Sprintf("/projects/%s/form-schema/collection?mode=edit&entity_id=%s", projectID, collectionID), activeVersion)),
			ForkURL:         forkURL,
			ForkLabel:       i18n.L("common.capabilities.adapt", "Adapt to edit"),
			ForkPending:     i18n.L("common.capabilities.adapt_pending", "Adapting…"),
			SearchFieldsURL: withVersion(fmt.Sprintf("/projects/%s/fields/search", projectID), activeVersion),
			Derivatives:     h.derivativesFor(ctx, auth.FromContext(ctx), projectResource, "collections", collectionID, activeVersion),
			ReuseURL:        withVersion(fmt.Sprintf("%s/%s/entity-view/collection/%s/reuse", weaveroutes.ProjectBase, projectID, collectionID), activeVersion),
			DeleteURL:       collDeleteURL,
			DeprecateURL:    collDeprecateURL,
			ActivateURL:     collActivateURL,
		},
		ViewMode:  ViewModeDetailed,
		Sections:  sections,
		Adoptions: adoptions,
		Refs:      refs,
		StatsURL:  withVersion(fmt.Sprintf("%s/%s/entity-view/collection/%s/stats", weaveroutes.ProjectBase, projectID, collectionID), activeVersion),
		Release:   releaseView(projectID, activeVersion),
	}, nil
}

func (h *Handler) buildField(ctx context.Context, projectID, fieldID string) (*Response, error) {
	projectResource := h.projectResource(ctx, projectID)
	canEdit := h.canEditProject(ctx, projectID)
	activeVersion := auth.ProjectVersionFromContext(ctx)
	forkedOrigins, err := weavepkg.ForkOriginsForProject(ctx, h.weave, projectID, "field")
	if err != nil {
		return nil, fmt.Errorf("list field forks: %w", err)
	}
	adoptedOrigins, err := weavepkg.AdoptionOriginsForProject(ctx, h.weave, projectID, "field")
	if err != nil {
		return nil, fmt.Errorf("list field adoptions: %w", err)
	}
	field, err := h.weave.WeaveFields().GetByID(ctx, fieldID)
	if err != nil {
		return nil, fmt.Errorf("get field: %w", err)
	}
	if field == nil {
		return nil, fmt.Errorf("field not found: %s", fieldID)
	}

	refs := ViewRefs{
		Categories:  make(map[string]ViewRefEntry),
		Collections: make(map[string]ViewRefEntry),
		Models:      make(map[string]ViewRefEntry),
	}

	// CategoryID + SetValue moved to weave_field_overrides (base
	// override row, entity_type='') in migration 027. Read them from
	// there for the entity-view response.
	categoryID := ""
	setValue := ""
	if base, _ := h.weave.Overrides().GetBase(ctx, fieldID, projectID); base != nil {
		categoryID = base.CategoryID
		setValue = base.SetValue
	}
	if categoryID != "" {
		catOrigin := pkgdomain.OriginFromSemanticID(projectID, categoryID)
		if cat, err := h.weave.WeaveCategories().GetByID(ctx, categoryID); err == nil && cat != nil {
			if origins, err := weavepkg.CategoryAdoptionOriginsBySystemName(ctx, h.weave, projectID, []*pkgdomain.Category{cat}); err == nil {
				if adopted, ok := origins[cat.SystemName]; ok {
					catOrigin = adopted
				}
			}
		}
		refs.Categories[categoryID] = ViewRefEntry{
			URL:    withVersion(pkgdomain.ParseSemanticID(categoryID).URL(), activeVersion),
			Origin: catOrigin,
		}
	}

	entityOrigin := weavepkg.ResolveReuseOrigin(forkedOrigins, adoptedOrigins, projectID, field.ProjectID, field.ID)
	// Upgrade inherited → adopted_reference when the current project's
	// overrides reference this field — same logic as model + collection
	// detail builders.
	if entityOrigin.Kind == pkgdomain.OriginInherited {
		if refFields, err := h.weave.WeaveFields().ListReferenceAdopted(ctx, projectID); err == nil {
			for _, f := range refFields {
				if f.ID == field.ID {
					entityOrigin = pkgdomain.Origin{
						Kind:            pkgdomain.OriginAdoptedReference,
						SourceProjectID: field.ProjectID,
						SourceEntityID:  field.ID,
					}
					break
				}
			}
		}
	}
	entityEditable := canEdit && (entityOrigin.Kind == pkgdomain.OriginOwn || entityOrigin.Kind == pkgdomain.OriginForked)
	fieldInUse := false
	if uc, uErr := h.weave.WeaveFields().CountUsage(ctx, fieldID, projectID); uErr == nil {
		fieldInUse = uc.ModelsUsing+uc.CollectionsUsing > 0
	}
	fieldDeleteURL, fieldDeprecateURL, fieldActivateURL := lifecycleCaps(entityEditable, fieldInUse, activeVersion, fmt.Sprintf("/projects/%s/fields/%s", projectID, fieldID), field.Deprecated)
	return &Response{
		Entity: EntityViewMeta{
			ID:                field.ID,
			Type:              "field",
			Name:              field.UIName,
			Description:       field.Description,
			SystemName:        field.SystemName,
			Status:            string(field.Status),
			PublicationState:  h.pubState(ctx, projectID, "field", fieldID, activeVersion),
			Deprecated:        field.Deprecated,
			Origin:            entityOrigin,
			OriginLabel:       originDetailLabel(entityOrigin),
			OriginHelp:        originDetailHelp(entityOrigin),
			Scope:             &ViewScopeInfo{LocalName: field.OntologyScope.LocalName, Prefix: field.OntologyScope.Prefix},
			CreatedAt:         field.CreatedAt,
			UpdatedAt:         field.UpdatedAt,
			OntologyPath:      field.OntologyPath(),
			PathElements:      field.PathElements,
			ExpectedValueType: field.ExpectedValueType,
			SetValue:          setValue,
			CategoryID:        categoryID,
		},
		Capabilities: ViewCapabilities{
			Editable:        entityEditable,
			MetadataURL:     capabilityURL(entityEditable, withVersion(fmt.Sprintf("/projects/%s/form-schema/field?mode=edit&entity_id=%s", projectID, fieldID), activeVersion)),
			SearchFieldsURL: withVersion(fmt.Sprintf("/projects/%s/fields/search", projectID), activeVersion),
			Derivatives:     h.derivativesFor(ctx, auth.FromContext(ctx), projectResource, "fields", fieldID, activeVersion),
			DeleteURL:       fieldDeleteURL,
			DeprecateURL:    fieldDeprecateURL,
			ActivateURL:     fieldActivateURL,
			ReuseURL:        withVersion(fmt.Sprintf("%s/%s/entity-view/field/%s/reuse", weaveroutes.ProjectBase, projectID, fieldID), activeVersion),
		},
		ViewMode: ViewModeDetailed,
		Sections: []ViewSection{},
		Refs:     refs,
		StatsURL: withVersion(fmt.Sprintf("%s/%s/entity-view/field/%s/stats", weaveroutes.ProjectBase, projectID, fieldID), activeVersion),
		Release:  releaseView(projectID, activeVersion),
	}, nil
}

func (h *Handler) buildConceptList(ctx context.Context, projectID, conceptListID string) (*Response, error) {
	activeVersion := auth.ProjectVersionFromContext(ctx)
	canEdit := h.canEditProject(ctx, projectID)
	list, err := h.weave.ConceptLists().GetByID(ctx, conceptListID)
	if err != nil {
		return nil, fmt.Errorf("get concept list: %w", err)
	}
	if list == nil || list.ProjectID != projectID {
		return nil, fmt.Errorf("concept list not found: %s", conceptListID)
	}

	vocabularyID := ""
	vocabularyLabel := ""
	var sourceVocabulary *pkgdomain.VocabularyRef
	if list.VocabularyID != nil && *list.VocabularyID != "" {
		vocabularyID = *list.VocabularyID
		vocab, err := h.weave.Vocabularies().GetVocabulary(ctx, vocabularyID)
		if err != nil {
			return nil, fmt.Errorf("get concept list vocabulary: %w", err)
		}
		if vocab != nil {
			vocabularyLabel = vocab.UIName.Get("en", vocab.SystemName)
			if vocabularyLabel == "" {
				vocabularyLabel = vocab.ID
			}
			sourceVocabulary = &pkgdomain.VocabularyRef{
				ID:         vocab.ID,
				SemanticID: vocab.SemanticID,
				SystemName: vocab.SystemName,
				Name:       vocab.UIName,
				BaseURI:    vocab.BaseURI,
			}
		}
	}

	rows, err := h.weave.ConceptLists().ListEntries(ctx, conceptListID)
	if err != nil {
		return nil, fmt.Errorf("list concept list entries: %w", err)
	}
	entries := make([]ConceptListEntryView, 0, len(rows))
	for _, row := range rows {
		item := ConceptListEntryView{
			ID:                row.ID,
			VocabularyEntryID: row.VocabularyEntryID,
			CustomLabel:       row.CustomLabel,
			Position:          row.Position,
		}
		entry, err := h.weave.Vocabularies().GetEntry(ctx, row.VocabularyEntryID)
		if err != nil {
			return nil, fmt.Errorf("get vocabulary entry %s: %w", row.VocabularyEntryID, err)
		}
		if entry != nil {
			item.URI = entry.URI
			item.Label = entry.Label
			item.ScopeNote = entry.ScopeNote
			item.ExternalID = entry.ExternalID
			item.BroaderURI = entry.BroaderURI
			item.BroaderPathItems = entry.BroaderPath
		}
		entries = append(entries, item)
	}

	listType := ""
	listTypeURI := ""
	listTypeLabel := ""
	var parentTerm *pkgdomain.VocabularyEntryRef
	if list.ListType != nil {
		listType = *list.ListType
		entry, err := h.weave.Vocabularies().GetEntry(ctx, *list.ListType)
		if err != nil {
			return nil, fmt.Errorf("get concept list type entry %s: %w", *list.ListType, err)
		}
		if entry != nil {
			listTypeURI = entry.URI
			listTypeLabel = entry.Label.Get("en", "")
			if listTypeLabel == "" {
				listTypeLabel = entry.URI
			}
			parentTerm = &pkgdomain.VocabularyEntryRef{
				ID:           entry.ID,
				VocabularyID: entry.VocabularyID,
				URI:          entry.URI,
				Label:        entry.Label,
				ScopeNote:    entry.ScopeNote,
				BroaderURI:   entry.BroaderURI,
				ExternalID:   entry.ExternalID,
			}
		}
	}

	return &Response{
		Entity: EntityViewMeta{
			ID:               list.ID,
			Type:             "concept-list",
			Name:             list.UIName,
			Description:      list.Description,
			SystemName:       list.SystemName,
			Status:           string(list.Status),
			Origin:           pkgdomain.OriginFromSemanticID(projectID, list.ID),
			CreatedAt:        list.CreatedAt,
			UpdatedAt:        list.UpdatedAt,
			ListType:         listType,
			VocabularyID:     vocabularyID,
			VocabularyLabel:  vocabularyLabel,
			EntryCount:       len(entries),
			ListTypeURI:      listTypeURI,
			ParentTermURI:    listTypeURI,
			ListTypeLabel:    listTypeLabel,
			ParentTerm:       parentTerm,
			SourceVocabulary: sourceVocabulary,
		},
		Capabilities: ViewCapabilities{
			Editable:    canEdit,
			MetadataURL: capabilityURL(canEdit, withVersion(fmt.Sprintf("/api/v2/projects/%s/concept-lists/%s/form-schema", projectID, list.ID), activeVersion)),
			ConceptList: &ConceptListCap{
				SearchEntriesURL: capabilityURL(canEdit, withVersion(fmt.Sprintf("/api/v2/projects/%s/concept-lists/%s/source-entries/search", projectID, list.ID), activeVersion)),
				AddEntryURL:      capabilityURL(canEdit, withVersion(fmt.Sprintf("/api/v2/projects/%s/concept-lists/%s/entries", projectID, list.ID), activeVersion)),
				UpdateEntryURL:   capabilityURL(canEdit, withVersion(fmt.Sprintf("/api/v2/projects/%s/concept-lists/%s/entries/{id}", projectID, list.ID), activeVersion)),
				RemoveEntryURL:   capabilityURL(canEdit, withVersion(fmt.Sprintf("/api/v2/projects/%s/concept-lists/%s/entries/{id}", projectID, list.ID), activeVersion)),
				ReorderURL:       capabilityURL(canEdit, withVersion(fmt.Sprintf("/api/v2/projects/%s/concept-lists/%s/entries/reorder", projectID, list.ID), activeVersion)),
			},
		},
		ViewMode: ViewModeDetailed,
		Sections: []ViewSection{},
		Entries:  entries,
		Refs:     ViewRefs{},
		Release:  releaseView(projectID, activeVersion),
	}, nil
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

func (h *Handler) canEditProject(ctx context.Context, projectID string) bool {
	if auth.ProjectVersionFromContext(ctx) != "" {
		return false
	}
	resource := auth.ProjectResourceFromContext(ctx)
	if resource.ID == "" {
		project, err := h.weave.Projects().GetByID(ctx, projectID)
		if err != nil || project == nil {
			return false
		}
		resource = auth.ProjectResource(project)
	}
	return auth.FromContext(ctx).Can(auth.ProjectEdit, resource, nil)
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

func (h *Handler) adoptionItemsForContext(ctx context.Context, projectID, contextEntityType, contextEntityID, activeVersion string) ([]AdoptionItem, error) {
	opts := []pkgdomain.QueryOption{
		pkgdomain.WithProjectID(projectID),
		pkgdomain.WithFilter("context_entity_type", contextEntityType),
		pkgdomain.WithFilter("context_entity_id", contextEntityID),
	}
	if activeVersion != "" {
		opts = append(opts, pkgdomain.WithVersion(activeVersion))
	}
	adoptions, err := h.weave.Adoptions().List(ctx, opts...)
	if err != nil {
		return nil, err
	}
	actorNames := h.actorNames(ctx, adoptions)
	items := make([]AdoptionItem, 0, len(adoptions))
	for _, adoption := range adoptions {
		items = append(items, AdoptionItem{
			EntityType:      adoption.EntityType,
			SourceProjectID: adoption.SourceProjectID,
			SourceEntityID:  adoption.SourceEntityID,
			SourceURL:       projectEntityURL(adoption.EntityType, adoption.SourceEntityID),
			AdoptedAt:       adoption.AdoptedAt,
			CreatedBy:       actorRef(adoption.CreatedByID, actorNames),
			Origin:          adoption.Origin,
			Label:           adoptionLabel(adoption),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].EntityType != items[j].EntityType {
			return items[i].EntityType < items[j].EntityType
		}
		return items[i].SourceEntityID < items[j].SourceEntityID
	})
	return items, nil
}

func (h *Handler) actorNames(ctx context.Context, adoptions []pkgdomain.Adoption) map[string]string {
	if h.actors == nil {
		return map[string]string{}
	}
	ids := make([]string, 0, len(adoptions))
	for _, adoption := range adoptions {
		if adoption.CreatedByID != nil && *adoption.CreatedByID != "" {
			ids = append(ids, *adoption.CreatedByID)
		}
	}
	return h.actors.LabelsByID(ctx, ids)
}

func actorRef(id *string, labels map[string]string) pkgdomain.ActorRef {
	if id == nil || *id == "" {
		return pkgdomain.ActorRef{}
	}
	ref := pkgdomain.ActorRef{ID: *id}
	if label := labels[*id]; label != "" {
		ref.Label = label
	}
	return ref
}

func projectEntityURL(entityType, entityID string) string {
	switch entityType {
	case "model", "collection", "field", "category":
		return pkgdomain.ParseSemanticID(entityID).URL()
	default:
		return ""
	}
}

func adoptionLabel(adoption pkgdomain.Adoption) pkgdomain.Translations {
	label := adoption.SourceEntityID
	if adoption.EntityType != "" {
		label = adoption.EntityType + " · " + adoption.SourceEntityID
	}
	return pkgdomain.Translations{"en": label, "nl": label}
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
		return auth.Resource{ScopeType: "project"}
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
	base := "/gen/" + kind + "/" + id

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

func withVersion(raw, activeVersion string) string {
	if activeVersion == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("version", activeVersion)
	u.RawQuery = q.Encode()
	return u.String()
}

func requestPathWithVersion(r *http.Request, activeVersion string) string {
	if activeVersion == "" {
		return r.URL.Path
	}
	return withVersion(r.URL.Path, activeVersion)
}

func releaseView(projectID, activeVersion string) *ReleaseView {
	if activeVersion == "" {
		return nil
	}
	return &ReleaseView{
		Version:  activeVersion,
		DraftURL: fmt.Sprintf("%s/%s", weaveroutes.ProjectBase, projectID),
		Label: i18n.LF("release.viewing_version", "Viewing release {version}",
			map[string]string{"version": activeVersion}),
	}
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
func originDetailLabel(origin pkgdomain.Origin) i18n.LocalizedText {
	source := origin.SourceProjectID
	if origin.SourceEntityID != "" {
		if source != "" {
			source += " · "
		}
		source += origin.SourceEntityID
	}
	switch origin.Kind {
	case pkgdomain.OriginForked:
		if source == "" {
			return i18n.L("common.origin_state.adapted", "Adapted")
		}
		return i18n.LF("common.origin_state.adapted_from", "Adapted from {source}", map[string]string{"source": source})
	case pkgdomain.OriginAdopted:
		if source == "" {
			return i18n.L("common.origin_state.adopted", "Adopted")
		}
		return i18n.LF("common.origin_state.adopted_from", "Adopted from {source}", map[string]string{"source": source})
	case pkgdomain.OriginAdoptedReference:
		if source == "" {
			return i18n.L("common.origin_state.adopted_reference", "Adopted (by reference)")
		}
		return i18n.LF("common.origin_state.adopted_reference_from", "Adopted by reference from {source}", map[string]string{"source": source})
	case pkgdomain.OriginInherited:
		if source == "" {
			return i18n.L("common.origin_state.inherited", "Inherited")
		}
		return i18n.LF("common.origin_state.inherited_from", "Inherited from {source}", map[string]string{"source": source})
	case pkgdomain.OriginOwn:
		return i18n.L("common.origin_state.own", "Own")
	default:
		return i18n.LocalizedText{}
	}
}

func originDetailHelp(origin pkgdomain.Origin) i18n.LocalizedText {
	switch origin.Kind {
	case pkgdomain.OriginInherited:
		return i18n.L("common.origin_help.inherited", "This entity is available through parent inheritance. Adopt it to pin the upstream version into this project.")
	case pkgdomain.OriginAdopted:
		return i18n.L("common.origin_help.adopted", "This entity is pinned from upstream and stays read-only here. Adapt it to make local changes without changing the source.")
	case pkgdomain.OriginAdoptedReference:
		return i18n.L("common.origin_help.adopted_reference", "This entity is referenced by overrides in this project but has no explicit receipt. Adopt it for a deliberate claim, or Adapt it to make local changes.")
	case pkgdomain.OriginForked:
		return i18n.L("common.origin_help.adapted", "This is your local editable copy. Upstream provenance is still kept so you can trace where it came from.")
	default:
		return i18n.LocalizedText{}
	}
}

type statEntry struct {
	name  string
	count int
}

type categoryInfo struct {
	name   pkgdomain.Translations
	order  int
	origin pkgdomain.Origin
}

// filterVisibleFields drops fields marked IsHidden before they're rendered
// into the read-only detail/browse contract. Applied before
// any derived computation — field counts, ComputeSharedPathPrefix — so
// those stay consistent with what's actually rendered. The override EDITOR
// (pkg/weave/project/override_read.go) intentionally does not use this: it
// keeps showing hidden fields (strikethrough + Hidden badge) so curators
// can unhide them.
func filterVisibleFields(fields []pkgdomain.ResolvedField) []pkgdomain.ResolvedField {
	visible := make([]pkgdomain.ResolvedField, 0, len(fields))
	for _, f := range fields {
		if f.IsHidden {
			continue
		}
		visible = append(visible, f)
	}
	return visible
}

func convertFields(fields []pkgdomain.ResolvedField, refs ViewRefs, projectID, activeVersion string) []ViewField {
	result := make([]ViewField, 0, len(fields))
	for _, f := range fields {
		result = append(result, convertField(f, refs, projectID, activeVersion))
	}
	return result
}

func convertField(f pkgdomain.ResolvedField, refs ViewRefs, projectID, activeVersion string) ViewField {
	if refs.Models == nil {
		refs.Models = make(map[string]ViewRefEntry)
	}
	if refs.Collections == nil {
		refs.Collections = make(map[string]ViewRefEntry)
	}
	for _, ref := range f.ResourceModels {
		ref.URL = withVersionForProject(ref.URL, projectID, activeVersion)
		if _, ok := refs.Models[ref.ID]; !ok {
			refs.Models[ref.ID] = ViewRefEntry{
				Name:   ref.Name,
				URL:    ref.URL,
				Origin: pkgdomain.OriginFromSemanticID(projectID, ref.SemanticID),
			}
		}
	}
	for _, ref := range f.CollectionModels {
		ref.URL = withVersionForProject(ref.URL, projectID, activeVersion)
		if _, ok := refs.Collections[ref.ID]; !ok {
			refs.Collections[ref.ID] = ViewRefEntry{
				Name:   ref.Name,
				URL:    ref.URL,
				Origin: pkgdomain.OriginFromSemanticID(projectID, ref.SemanticID),
			}
		}
	}
	for _, ref := range f.ConceptLists {
		ref.URL = withVersionForProject(ref.URL, projectID, activeVersion)
	}
	// SourceFieldURL / SourceCollectionURL — schema-driven click-through
	// to the standalone source detail. The frontend never
	// constructs these URLs (api-patterns.md). Built via SemanticID.URL()
	// which encodes the owning project's prefix, so inherited entities
	// link to their source project.
	sourceFieldURL := withVersionForProject(pkgdomain.ParseSemanticID(f.SemanticID).URL(), projectID, activeVersion)
	sourceCollectionURL := ""
	if f.PartOfCollectionID != "" {
		sourceCollectionURL = withVersionForProject(pkgdomain.ParseSemanticID(f.PartOfCollectionID).URL(), projectID, activeVersion)
	}
	return ViewField{
		Widget:              "field-override",
		OverrideID:          f.OverrideID,
		FieldID:             f.ID,
		FieldSemanticID:     f.SemanticID,
		FieldSystemName:     f.SystemName,
		Origin:              pkgdomain.OriginFromProject(projectID, f.ProjectID),
		Position:            f.Position,
		DisplayName:         f.DisplayName,
		Description:         f.Description,
		ExpectedValueType:   f.ExpectedValueType,
		SetValue:            f.SetValue,
		IsRequired:          f.IsRequired,
		IsHidden:            f.IsHidden,
		OntologyPath:        f.OntologyPath,
		PathElements:        f.PathElements,
		ResourceModelRefs:   versionRefsForProject(f.ResourceModels, projectID, activeVersion),
		CollectionModelRefs: versionRefsForProject(f.CollectionModels, projectID, activeVersion),
		ConceptListRefs:     versionRefsForProject(f.ConceptLists, projectID, activeVersion),
		CategoryID:          f.CategoryID,
		CollectionID:        f.PartOfCollectionID,
		SourceFieldURL:      sourceFieldURL,
		SourceCollectionURL: sourceCollectionURL,
	}
}

func withVersionForProject(raw, projectID, activeVersion string) string {
	if activeVersion == "" || raw == "" {
		return raw
	}
	prefix := fmt.Sprintf("/projects/%s/", projectID)
	if !strings.HasPrefix(raw, prefix) {
		return raw
	}
	return withVersion(raw, activeVersion)
}

func versionRefsForProject(refs []pkgdomain.EntityRef, projectID, activeVersion string) []pkgdomain.EntityRef {
	if len(refs) == 0 {
		return refs
	}
	out := make([]pkgdomain.EntityRef, len(refs))
	copy(out, refs)
	for i := range out {
		out[i].URL = withVersionForProject(out[i].URL, projectID, activeVersion)
	}
	return out
}

func computeCollectionStats(fields []pkgdomain.ResolvedField, sections []ViewSection) pkgdomain.ModelViewStats {
	required := 0
	valueTypeCounts := make(map[string]int)
	for _, f := range fields {
		if f.IsRequired {
			required++
		}
		if f.ExpectedValueType != "" {
			valueTypeCounts[f.ExpectedValueType]++
		}
	}
	catBreakdown, fieldBreakdown, fieldScopeBreakdown, classBreakdown, propBreakdown, scopesCount := computeCollectionBreakdowns(fields, sections)
	return pkgdomain.ModelViewStats{
		TotalFields:          len(fields),
		TotalCategories:      len(sections),
		RequiredFields:       required,
		OptionalFields:       len(fields) - required,
		ValueTypeCounts:      valueTypeCounts,
		ScopesCount:          scopesCount,
		CategoriesBreakdown:  catBreakdown,
		FieldsBreakdown:      fieldBreakdown,
		FieldScopesBreakdown: fieldScopeBreakdown,
		ClassesBreakdown:     classBreakdown,
		PropertiesBreakdown:  propBreakdown,
	}
}

func computeCollectionBreakdowns(fields []pkgdomain.ResolvedField, sections []ViewSection) (
	catBreakdown, fieldBreakdown, fieldScopeBreakdown, classBreakdown, propBreakdown []pkgdomain.StatItem,
	scopesCount int,
) {
	catItems := make([]pkgdomain.StatItem, 0, len(sections))
	maxCatCount := 0
	for _, s := range sections {
		count := 0
		for _, item := range s.Items {
			count += len(item.Fields)
		}
		if count > maxCatCount {
			maxCatCount = count
		}
		catItems = append(catItems, pkgdomain.StatItem{ID: s.ID, Name: s.Name.Get("en", s.ID), Count: count})
	}
	for i := range catItems {
		if maxCatCount > 0 {
			catItems[i].Percentage = (catItems[i].Count * 100) / maxCatCount
		}
	}
	sort.Slice(catItems, func(i, j int) bool {
		return catItems[i].Count > catItems[j].Count
	})
	if len(catItems) > 0 {
		catBreakdown = catItems
	}

	fieldCounts := make(map[string]statEntry)
	fieldScopes := make(map[string]int)
	classUsage := make(map[string]statEntry)
	propUsage := make(map[string]statEntry)

	for _, f := range fields {
		fc := fieldCounts[f.ID]
		fc.count++
		if fc.name == "" {
			fc.name = f.DisplayName.Get("en", f.SemanticID)
		}
		fieldCounts[f.ID] = fc

		firstClass := true
		seenClasses := make(map[string]bool)
		seenProps := make(map[string]bool)

		for _, elem := range f.PathElements {
			key := elem.LocalName
			if elem.Prefix != "" {
				key = elem.Prefix + ":" + elem.LocalName
			}

			switch elem.Type {
			case "class":
				if !seenClasses[key] {
					seenClasses[key] = true
					e := classUsage[key]
					e.count++
					if e.name == "" {
						e.name = elem.LocalName
					}
					classUsage[key] = e
				}
				if firstClass {
					firstClass = false
					fieldScopes[key]++
				}
			case "property":
				if !seenProps[key] {
					seenProps[key] = true
					e := propUsage[key]
					e.count++
					if e.name == "" {
						e.name = elem.LocalName
					}
					propUsage[key] = e
				}
			}
		}
	}

	scopesCount = len(fieldScopes)

	fieldBreakdown = buildStatItems(fieldCounts)
	fieldScopeBreakdown = buildStatItemsFromCounts(fieldScopes)
	classBreakdown = buildStatItems(classUsage)
	propBreakdown = buildStatItems(propUsage)

	return
}

func buildStatItems(m map[string]statEntry) []pkgdomain.StatItem {
	if len(m) == 0 {
		return nil
	}

	maxCount := 0
	for _, v := range m {
		if v.count > maxCount {
			maxCount = v.count
		}
	}

	items := make([]pkgdomain.StatItem, 0, len(m))
	for id, v := range m {
		pct := 0
		if maxCount > 0 {
			pct = (v.count * 100) / maxCount
		}
		items = append(items, pkgdomain.StatItem{
			ID:         id,
			Name:       v.name,
			Count:      v.count,
			Percentage: pct,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Count > items[j].Count
	})

	return items
}

func buildStatItemsFromCounts(m map[string]int) []pkgdomain.StatItem {
	if len(m) == 0 {
		return nil
	}

	maxCount := 0
	for _, c := range m {
		if c > maxCount {
			maxCount = c
		}
	}

	items := make([]pkgdomain.StatItem, 0, len(m))
	for name, count := range m {
		pct := 0
		if maxCount > 0 {
			pct = (count * 100) / maxCount
		}
		items = append(items, pkgdomain.StatItem{
			ID:         name,
			Name:       name,
			Count:      count,
			Percentage: pct,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Count > items[j].Count
	})

	return items
}

func sortSectionsByOrder(sections []ViewSection) {
	sort.Slice(sections, func(i, j int) bool {
		return sections[i].CanonicalOrder < sections[j].CanonicalOrder
	})
}

func collectionWidget(collectionID string) string {
	if collectionID == "__direct__" || collectionID == "" {
		return "field-group"
	}
	return "collection-group"
}
