package projectpage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	pkgdomain "github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/actorlabels"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
	"github.com/pletka-io/pletka/pkg/weave/publication"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

// LinkedOntologyReader is the cross-slice reader for "what ontology
// versions does this project have linked", populated with names + version
// strings + class/property counts. Satisfied by
// projectontologyversion.Service in production; stubbed in tests.
//
// The interface lives here (not on pkg/domain) so projectpage owns its
// own narrow contract rather than dragging in the larger slice surface.
type LinkedOntologyReader interface {
	LinkedOntologies(ctx context.Context, projectID string) ([]pkgdomain.LinkedOntology, error)
}

type ReleaseReader interface {
	ListByProject(ctx context.Context, projectID string) ([]release.Release, error)
}

type ExampleReader interface {
	List(ctx context.Context, projectID string, opts ...pkgdomain.QueryOption) ([]*pkgdomain.Example, int64, error)
}

// AttributionReader is the narrow read surface projectpage uses to
// fold project credits into the overview schema. Satisfied by the
// attribution slice's Store (and any test fake). Lives here for the
// same reason as LinkedOntologyReader above — projectpage owns its
// own narrow contract per ADR-0001.
type AttributionReader interface {
	ListForProject(ctx context.Context, projectID string) ([]pkgdomain.Attribution, error)
}

type LangResolver func(*http.Request) string

type Host struct {
	Logger           *slog.Logger
	Weave            pkgdomain.WeaveStore
	LinkedOntologies LinkedOntologyReader
	Releases         ReleaseReader
	Examples         ExampleReader
	Attributions     AttributionReader
	ActorLabels      actorlabels.Reader
	Languages        []formschema.LanguageInfo
	LangResolver     LangResolver
	I18n             i18n.Manager
	// Publication derives the project's unreleased-changes rollup for the
	// header badge. Optional; nil leaves the count zero.
	Publication *publication.Reader
}

func (h Host) Validate() error {
	var missing []string
	if h.Weave == nil {
		missing = append(missing, "Weave")
	}
	if h.LinkedOntologies == nil {
		missing = append(missing, "LinkedOntologies")
	}
	if h.Releases == nil {
		missing = append(missing, "Releases")
	}
	if h.Attributions == nil {
		missing = append(missing, "Attributions")
	}
	if h.ActorLabels == nil {
		missing = append(missing, "ActorLabels")
	}
	if len(missing) > 0 {
		return fmt.Errorf("projectpage host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

type Handler struct {
	logger           *slog.Logger
	weave            pkgdomain.WeaveStore
	linkedOntologies LinkedOntologyReader
	releases         ReleaseReader
	examples         ExampleReader
	attributions     AttributionReader
	actorLabels      actorlabels.Reader
	languages        []formschema.LanguageInfo
	langResolver     LangResolver
	// i18n walks LocalizedText nodes inside schema responses so labels
	// resolve to Translations maps the frontend can pick from with tr()
	// — schema-driven UI rule (.claude/rules/ui-patterns.md).
	i18n i18n.Manager
	// publication derives the project's unreleased-changes rollup. Optional.
	publication *publication.Reader
}

func NewHandler(
	logger *slog.Logger,
	weaveStore pkgdomain.WeaveStore,
	linkedOntologies LinkedOntologyReader,
	releases ReleaseReader,
	examples ExampleReader,
	attributions AttributionReader,
	actorLabels actorlabels.Reader,
	languages []formschema.LanguageInfo,
	langResolver LangResolver,
	i18nMgr i18n.Manager,
) *Handler {
	return &Handler{
		logger:           logger,
		weave:            weaveStore,
		linkedOntologies: linkedOntologies,
		releases:         releases,
		examples:         examples,
		attributions:     attributions,
		actorLabels:      actorLabels,
		languages:        languages,
		langResolver:     langResolver,
		i18n:             i18nMgr,
	}
}

func Mount(r chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := NewHandler(
		host.Logger,
		host.Weave,
		host.LinkedOntologies,
		host.Releases,
		host.Examples,
		host.Attributions,
		host.ActorLabels,
		host.Languages,
		host.LangResolver,
		host.I18n,
	)
	h.publication = host.Publication
	h.Mount(r)
}

// Mount registers projectpage's cross-cutting page-schema endpoints.
// Per the pkg/weave/ module-shape ADR, projectpage is a handler-only
// module: it composes data across slices to build the page-level
// schema for a single project view. Per-entity list/form/data
// endpoints (entity-list-schema, /data, filters/institutions) are
// owned by the project slice (entity-shaped); they used to live here
// in parallel and were dropped in the alignment commit.
func (h *Handler) Mount(r chi.Router) {
	// projectRead gates on auth.ProjectRead and loads the project into
	// context (equivalent to auth.WrapProjectRead, expanded into discrete
	// middlewares so auth.ResolveContentVersion can run after the project
	// is resolved but before the handler).
	projectRead := []func(http.Handler) http.Handler{
		auth.WithProjectVersionContext,
		auth.RequireProjectRead(h.weave.Projects()),
	}
	if h.publication != nil {
		projectRead = append(projectRead, auth.ResolveContentVersion(h.publication))
	}
	r.With(projectRead...).Get("/projects/{projectID:[A-Z0-9]+}/page-schema", h.ProjectPageSchema)
	r.With(projectRead...).Get("/projects/{projectID:[A-Z0-9]+}/overview-schema", h.ProjectOverviewSchema)
	r.With(projectRead...).Get("/projects/{projectID:[A-Z0-9]+}/adoptions-tab-schema", h.ProjectAdoptionsTabSchema)
	r.With(projectRead...).Get("/projects/{projectID:[A-Z0-9]+}/adoptions/{sourceProjectID}/{sourceEntityID}/closure", h.ProjectAdoptionClosure)
	r.With(projectRead...).Get("/projects/{projectID:[A-Z0-9]+}/release-tab-schema", h.ProjectReleaseTabSchema)
}

func (h *Handler) ProjectPageSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "project ID is required")
		return
	}

	project := auth.ProjectFromContext(ctx)
	if project == nil {
		h.logger.Error("project missing from request context", "id", projectID)
		errresp.Error(w, r, http.StatusNotFound, "not_found", "project not found")
		return
	}

	statsMap, err := h.weave.Projects().StatsForProjects(ctx, []string{projectID})
	if err != nil {
		h.logger.Error("failed to compute project stats", "id", projectID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to load project stats")
		return
	}
	stats := statsMap[projectID]
	if stats == nil {
		stats = &pkgdomain.WeaveProjectStats{}
	}
	releaseItems, err := h.releases.ListByProject(ctx, projectID)
	if err != nil {
		h.logger.Debug("failed to load releases for page-schema", "project", projectID, "err", err)
	}
	exampleCount := 0
	if h.examples != nil && auth.ProjectVersionFromContext(ctx) == "" {
		if _, total, err := h.examples.List(ctx, projectID, pkgdomain.WithLimit(1)); err != nil {
			h.logger.Debug("failed to load examples for page-schema", "project", projectID, "err", err)
		} else {
			exampleCount = int(total)
		}
	}
	conceptListCount := 0
	if lists, err := h.weave.ConceptLists().List(ctx, projectID); err != nil {
		h.logger.Debug("failed to load concept lists for page-schema", "project", projectID, "err", err)
	} else {
		conceptListCount = len(lists)
	}
	adoptionOpts := []pkgdomain.QueryOption{pkgdomain.WithProjectID(projectID)}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		adoptionOpts = append(adoptionOpts, pkgdomain.WithVersion(version))
	}
	adoptionItems, err := h.weave.Adoptions().List(ctx, adoptionOpts...)
	if err != nil {
		h.logger.Debug("failed to load adoptions for page-schema", "project", projectID, "err", err)
	}

	lang := h.currentLang(r)
	resolved, rerr := h.weave.Projects().ResolvedOntologyVersions(ctx, project.ID, pkgdomain.ResolvedOntologyVersionOpts{})
	if rerr != nil {
		h.logger.Debug("failed to resolve ontology versions for page-schema warnings", "project", project.ID, "err", rerr)
	}
	setup := formschema.ProjectSetupState{HasOntology: len(resolved) > 0}
	res := auth.ProjectResource(project)
	snap := auth.FromContext(ctx)
	warnings := formschema.ComputeProjectWarnings(project.ID, setup, snap, res)

	// Unreleased-changes rollup for the header badge — live view only
	// (a release view shows the frozen snapshot, no "unreleased" notion).
	unreleased := 0
	if h.publication != nil && auth.ProjectVersionFromContext(ctx) == "" {
		if rollup, rErr := h.publication.ProjectRollup(ctx, projectID); rErr != nil {
			h.logger.Debug("project publication rollup", "project", projectID, "err", rErr)
		} else {
			unreleased = rollup.Unreleased()
		}
	}

	schema := formschema.BuildProjectPageSchema(
		project,
		int(stats.ModelCount),
		int(stats.CollectionCount),
		int(stats.FieldCount),
		exampleCount,
		conceptListCount,
		len(adoptionItems),
		len(releaseItems),
		unreleased,
		snap.Can(auth.ProjectEdit, res, nil),
		snap.IsProjectMember(res),
		warnings,
		lang,
		h.languages,
		auth.ProjectVersionFromContext(ctx),
	)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(schema); err != nil {
		h.logger.Error("failed to encode project page schema", "project", projectID, "error", err)
	}
}

// ProjectAdoptionsTabSchema drives the Adoptions tab's bill-of-
// materials view. Per the adoption UX review, the tab shows only
// explicit project-level receipts (curator clicked Adopt); auto-
// receipts written by Adapt and parent-inheritance live elsewhere.
// Each item carries dependency counts up front and a ClosureURL the
// frontend uses to fetch the per-kind list on demand.
type ProjectAdoptionsTabSchema struct {
	Kind           string                   `json:"kind"`
	ProjectID      string                   `json:"project_id"`
	CurrentVersion string                   `json:"current_version,omitempty"`
	DraftURL       string                   `json:"draft_url"`
	Items          []ProjectAdoptionTabItem `json:"items"`
	EmptyMessage   pkgdomain.Localizable    `json:"empty_message,omitempty"`
	// Labels carries every user-visible string the frontend renders so
	// the Svelte component holds zero domain wording. Each value is a
	// LocalizedText resolved server-side via h.i18n.Resolve before
	// encode — same pattern as the list-row origin_label
	// (00efbaad).
	Labels ProjectAdoptionsLabels `json:"labels"`
}

type ProjectAdoptionsLabels struct {
	Title             i18n.LocalizedText `json:"title"`
	Description       i18n.LocalizedText `json:"description"`
	From              i18n.LocalizedText `json:"from"`
	SourceLink        i18n.LocalizedText `json:"source_link"`
	AdoptedPrefix     i18n.LocalizedText `json:"adopted_prefix"`
	ByPrefix          i18n.LocalizedText `json:"by_prefix"`
	Models            i18n.LocalizedText `json:"models"`
	Collections       i18n.LocalizedText `json:"collections"`
	Fields            i18n.LocalizedText `json:"fields"`
	Loading           i18n.LocalizedText `json:"loading"`
	LoadErrorPrefix   i18n.LocalizedText `json:"load_error_prefix"`
	SectionEmpty      i18n.LocalizedText `json:"section_empty"`
	SourceColumnLabel i18n.LocalizedText `json:"source_column_label"`
}

type ProjectAdoptionTabItem struct {
	EntityType      string `json:"entity_type"`
	SourceProjectID string `json:"source_project_id"`
	SourceEntityID  string `json:"source_entity_id"`
	// CurrentURL points at this project's view of the adopted entity
	// (/projects/{currentProjectID}/{plural}/{entityID}). That's where
	// overrides + fork are surfaced. SourceURL points at the source
	// project's own page for the same entity.
	CurrentURL string                 `json:"current_url,omitempty"`
	SourceURL  string                 `json:"source_url,omitempty"`
	AdoptedAt  time.Time              `json:"adopted_at"`
	CreatedBy  pkgdomain.ActorRef     `json:"created_by,omitempty"`
	Origin     pkgdomain.Origin       `json:"origin"`
	Label      pkgdomain.Translations `json:"label"`
	// Name is the source entity's UIName looked up server-side, so the
	// card header can show "Person · LAM.9" instead of just "LAM.9".
	Name pkgdomain.Translations `json:"name,omitempty"`
	// Bill of materials: counts of distinct entities reachable from
	// the receipt's source entity. Each kind ships both the direct
	// (depth-1) count and the transitive (full closure) count so the
	// curator can tell receipts apart even when their closures
	// overlap. ClosureURLTemplate returns the per-kind list when a
	// chip is expanded — caller substitutes {kind} =
	// models|collections|fields.
	ModelCount            int    `json:"model_count"`
	ModelCountDirect      int    `json:"model_count_direct"`
	CollectionCount       int    `json:"collection_count"`
	CollectionCountDirect int    `json:"collection_count_direct"`
	FieldCount            int    `json:"field_count"`
	FieldCountDirect      int    `json:"field_count_direct"`
	ClosureURLTemplate    string `json:"closure_url_template,omitempty"`
}

func (h *Handler) ProjectAdoptionsTabSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "project ID is required")
		return
	}

	// Filter to explicit project-context receipts per Q-C of the
	// adoption UX review. context_entity_type='project' AND
	// entity_type IN (model, collection, field) gives the rows the
	// curator created via the Adopt button — skipping auto-receipts
	// from Adapt (context = model/collection) and the parent-
	// inheritance category copy (entity_type='category').
	opts := []pkgdomain.QueryOption{
		pkgdomain.WithProjectID(projectID),
		pkgdomain.WithFilter("context_entity_type", "project"),
		pkgdomain.WithFilter("context_entity_id", projectID),
	}
	currentVersion := auth.ProjectVersionFromContext(ctx)
	if currentVersion != "" {
		opts = append(opts, pkgdomain.WithVersion(currentVersion))
	}
	allAdoptions, err := h.weave.Adoptions().List(ctx, opts...)
	if err != nil {
		h.logger.Error("failed to load project adoptions", "project", projectID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to load adoptions")
		return
	}
	adoptions := allAdoptions[:0]
	for _, a := range allAdoptions {
		switch a.EntityType {
		case "model", "collection", "field":
			adoptions = append(adoptions, a)
		}
	}

	actorNames := h.actorNames(ctx, adoptions)

	items := make([]ProjectAdoptionTabItem, 0, len(adoptions))
	for _, adoption := range adoptions {
		item := ProjectAdoptionTabItem{
			EntityType:         adoption.EntityType,
			SourceProjectID:    adoption.SourceProjectID,
			SourceEntityID:     adoption.SourceEntityID,
			CurrentURL:         addVersion(currentProjectEntityURL(projectID, adoption.EntityType, adoption.SourceEntityID), currentVersion),
			SourceURL:          projectEntityURL(adoption.EntityType, adoption.SourceEntityID),
			AdoptedAt:          adoption.AdoptedAt,
			CreatedBy:          actorRef(adoption.CreatedByID, actorNames),
			Origin:             adoption.Origin,
			Label:              adoptionLabel(adoption),
			ClosureURLTemplate: fmt.Sprintf("/projects/%s/adoptions/%s/%s/closure?kind={kind}", projectID, adoption.SourceProjectID, adoption.SourceEntityID),
		}
		// Look up the source entity's friendly name so the card header
		// reads "Person · LAM.9" rather than just "model · LAM.9".
		// Missing entities (deleted upstream) leave Name nil — Svelte
		// falls back to the source entity id.
		switch adoption.EntityType {
		case "model":
			if m, err := h.weave.Models().GetByID(ctx, adoption.SourceEntityID); err == nil && m != nil {
				item.Name = m.UIName
			}
		case "collection":
			if c, err := h.weave.Collections().GetByID(ctx, adoption.SourceEntityID); err == nil && c != nil {
				item.Name = c.UIName
			}
		case "field":
			if f, err := h.weave.WeaveFields().GetByID(ctx, adoption.SourceEntityID); err == nil && f != nil {
				item.Name = f.UIName
			}
		}
		// Eager counts per Q-E. Six walker invocations per receipt
		// (direct + transitive × 3 entity types); at realistic receipt
		// counts (<10) still inside the page-load budget. Both kinds
		// ship so the curator can tell receipts apart even when their
		// transitive closures fully overlap.
		if ids, err := h.weave.Models().ListReceiptModelClosure(ctx, adoption.SourceEntityID, adoption.EntityType); err == nil {
			item.ModelCount = len(ids)
		} else {
			h.logger.Warn("receipt model closure", "source", adoption.SourceEntityID, "err", err)
		}
		if ids, err := h.weave.Models().ListReceiptModelDirect(ctx, adoption.SourceEntityID, adoption.EntityType); err == nil {
			item.ModelCountDirect = len(ids)
		} else {
			h.logger.Warn("receipt model direct", "source", adoption.SourceEntityID, "err", err)
		}
		if ids, err := h.weave.Collections().ListReceiptCollectionClosure(ctx, adoption.SourceEntityID, adoption.EntityType); err == nil {
			item.CollectionCount = len(ids)
		} else {
			h.logger.Warn("receipt collection closure", "source", adoption.SourceEntityID, "err", err)
		}
		if ids, err := h.weave.Collections().ListReceiptCollectionDirect(ctx, adoption.SourceEntityID, adoption.EntityType); err == nil {
			item.CollectionCountDirect = len(ids)
		} else {
			h.logger.Warn("receipt collection direct", "source", adoption.SourceEntityID, "err", err)
		}
		if ids, err := h.weave.WeaveFields().ListReceiptFieldClosure(ctx, adoption.SourceEntityID, adoption.EntityType); err == nil {
			item.FieldCount = len(ids)
		} else {
			h.logger.Warn("receipt field closure", "source", adoption.SourceEntityID, "err", err)
		}
		if ids, err := h.weave.WeaveFields().ListReceiptFieldDirect(ctx, adoption.SourceEntityID, adoption.EntityType); err == nil {
			item.FieldCountDirect = len(ids)
		} else {
			h.logger.Warn("receipt field direct", "source", adoption.SourceEntityID, "err", err)
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].AdoptedAt.Equal(items[j].AdoptedAt) {
			return items[i].SourceEntityID < items[j].SourceEntityID
		}
		return items[i].AdoptedAt.After(items[j].AdoptedAt) // newest first
	})

	out := ProjectAdoptionsTabSchema{
		Kind:           "adoptions",
		ProjectID:      projectID,
		CurrentVersion: currentVersion,
		DraftURL:       "/projects/" + projectID,
		Items:          items,
		EmptyMessage:   i18n.L("project_page.adoptions_empty", "No adoptions yet."),
		Labels: ProjectAdoptionsLabels{
			Title:             i18n.L("project_page.adoptions.title", "Adoptions"),
			Description:       i18n.L("project_page.adoptions.description", "Explicit receipts you claimed via the Adopt button. Each card lists what that adoption transitively brings into the project — click a count to expand."),
			From:              i18n.L("project_page.adoptions.from", "from"),
			SourceLink:        i18n.L("project_page.adoptions.source_link", "source ↗"),
			AdoptedPrefix:     i18n.L("project_page.adoptions.adopted_prefix", "Adopted"),
			ByPrefix:          i18n.L("project_page.adoptions.by_prefix", "by"),
			Models:            i18n.L("common.models", "Models"),
			Collections:       i18n.L("common.collections", "Collections"),
			Fields:            i18n.L("common.fields", "Fields"),
			Loading:           i18n.L("common.loading", "Loading…"),
			LoadErrorPrefix:   i18n.L("project_page.adoptions.load_error_prefix", "Failed to load:"),
			SectionEmpty:      i18n.L("project_page.adoptions.section_empty", "Nothing in this section."),
			SourceColumnLabel: i18n.L("project_page.adoptions.source_column_label", "source"),
		},
	}
	if h.i18n != nil {
		h.i18n.Resolve(&out, h.currentLang(r))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.logger.Error("failed to encode adoptions tab schema", "project", projectID, "error", err)
	}
}

type ProjectReleaseTabSchema struct {
	Kind                    string                           `json:"kind"`
	ProjectID               string                           `json:"project_id"`
	CurrentVersion          string                           `json:"current_version,omitempty"`
	CanCreate               bool                             `json:"can_create"`
	CreateFormSchemaURL     string                           `json:"create_form_schema_url,omitempty"`
	CreateBlockedMessage    pkgdomain.Localizable            `json:"create_blocked_message,omitempty"`
	DependencySettingsURL   string                           `json:"dependency_settings_url,omitempty"`
	DraftParentDependencies []ProjectReleaseParentDependency `json:"draft_parent_dependencies,omitempty"`
	DraftURL                string                           `json:"draft_url"`
	Items                   []ProjectReleaseTabItem          `json:"items"`
	EmptyMessage            pkgdomain.Localizable            `json:"empty_message,omitempty"`
}

type ProjectReleaseTabItem struct {
	Version     string                 `json:"version"`
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	CreatedByID string                 `json:"created_by_id,omitempty"`
	ViewURL     string                 `json:"view_url"`
	Active      bool                   `json:"active"`
	Label       pkgdomain.Translations `json:"label"`
}

type ProjectReleaseParentDependency struct {
	ParentProjectID string                 `json:"parent_project_id"`
	Label           pkgdomain.Translations `json:"label"`
	Primary         bool                   `json:"primary"`
}

func (h *Handler) ProjectReleaseTabSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "project ID is required")
		return
	}

	currentVersion := auth.ProjectVersionFromContext(ctx)
	items, err := h.releases.ListByProject(ctx, projectID)
	if err != nil {
		h.logger.Error("failed to load project releases", "project", projectID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to load releases")
		return
	}

	canEdit := auth.FromContext(ctx).Can(auth.ProjectEdit, auth.ProjectResource(auth.ProjectFromContext(ctx)), nil)
	canCreate := canEdit && currentVersion == ""
	out := ProjectReleaseTabSchema{
		Kind:           "releases",
		ProjectID:      projectID,
		CurrentVersion: currentVersion,
		CanCreate:      canCreate,
		DraftURL:       "/projects/" + projectID,
		Items:          make([]ProjectReleaseTabItem, 0, len(items)),
		EmptyMessage:   i18n.L("project_page.releases_empty", "No releases yet."),
	}
	if canCreate {
		links, ierr := h.weave.ProjectInheritances().List(ctx, projectID)
		if ierr != nil {
			h.logger.Error("failed to load parent dependencies for release tab", "project", projectID, "err", ierr)
			errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to load releases")
			return
		}
		for _, link := range links {
			if normalizeReleaseDependencySource(link.SourceMode) != pkgdomain.DependencySourceDraft {
				continue
			}
			label := pkgdomain.Translations{
				"en": link.ParentProjectID,
				"nl": link.ParentProjectID,
			}
			if parent, perr := h.weave.Projects().GetByID(ctx, link.ParentProjectID); perr == nil && parent != nil {
				label = parent.UIName
			}
			out.DraftParentDependencies = append(out.DraftParentDependencies, ProjectReleaseParentDependency{
				ParentProjectID: link.ParentProjectID,
				Label:           label,
				Primary:         link.IsPrimary,
			})
		}
		sort.Slice(out.DraftParentDependencies, func(i, j int) bool {
			if out.DraftParentDependencies[i].Primary != out.DraftParentDependencies[j].Primary {
				return out.DraftParentDependencies[i].Primary
			}
			return out.DraftParentDependencies[i].ParentProjectID < out.DraftParentDependencies[j].ParentProjectID
		})
		if len(out.DraftParentDependencies) > 0 {
			out.CanCreate = false
			out.DependencySettingsURL = "/projects/" + projectID + "/settings#ontology"
			out.CreateBlockedMessage = i18n.L("project_page.releases_blocked_draft_parents", "Pin every parent dependency to a named release before creating a child release.")
		}
	}
	if out.CanCreate {
		out.CreateFormSchemaURL = "/projects/" + projectID + "/releases/form-schema"
	}
	for _, item := range items {
		viewURL := "/projects/" + projectID + "?version=" + url.QueryEscape(item.Version) + "#tab=releases"
		out.Items = append(out.Items, ProjectReleaseTabItem{
			Version:     item.Version,
			Title:       item.Title,
			Description: item.Description,
			CreatedAt:   item.CreatedAt,
			CreatedByID: item.CreatedByID,
			ViewURL:     viewURL,
			Active:      currentVersion == item.Version,
			Label: pkgdomain.Translations{
				"en": item.Version,
				"nl": item.Version,
			},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.logger.Error("failed to encode release tab schema", "project", projectID, "error", err)
	}
}

func normalizeReleaseDependencySource(mode pkgdomain.DependencySourceMode) pkgdomain.DependencySourceMode {
	if mode == pkgdomain.DependencySourceRelease {
		return pkgdomain.DependencySourceRelease
	}
	return pkgdomain.DependencySourceDraft
}

func groupContextIDs(adoptions []pkgdomain.Adoption, entityType string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, adoption := range adoptions {
		if adoption.ContextEntityType != entityType || adoption.ContextEntityID == "" {
			continue
		}
		if _, ok := seen[adoption.ContextEntityID]; ok {
			continue
		}
		seen[adoption.ContextEntityID] = struct{}{}
		out = append(out, adoption.ContextEntityID)
	}
	return out
}

// ProjectAdoptionClosureItem is a single row in the per-receipt
// bill-of-materials drill-down. SourceProjectID lets the frontend
// label "from LA / from SARI" so cross-project deps in the closure
// are visible.
type ProjectAdoptionClosureItem struct {
	ID              string                 `json:"id"`
	SemanticID      string                 `json:"semantic_id,omitempty"`
	Name            pkgdomain.Translations `json:"name,omitempty"`
	SourceProjectID string                 `json:"source_project_id"`
	CurrentURL      string                 `json:"current_url,omitempty"`
	SourceURL       string                 `json:"source_url,omitempty"`
}

// ProjectAdoptionClosureResponse drives a single section of the
// per-receipt bill-of-materials.
type ProjectAdoptionClosureResponse struct {
	Kind  string                       `json:"kind"`
	Count int                          `json:"count"`
	Items []ProjectAdoptionClosureItem `json:"items"`
}

// ProjectAdoptionClosure handles
//
//	GET /projects/{projectID}/adoptions/{sourceProjectID}/{sourceEntityID}/closure?kind=models|collections|fields
//
// Returns the transitive closure of the requested kind rooted at the
// receipt's source entity. Used by the Adoptions tab when the curator
// clicks a count chip to expand one section.
func (h *Handler) ProjectAdoptionClosure(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	sourceProjectID := chi.URLParam(r, "sourceProjectID")
	sourceEntityID := chi.URLParam(r, "sourceEntityID")
	if projectID == "" || sourceProjectID == "" || sourceEntityID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "projectID, sourceProjectID, and sourceEntityID are required")
		return
	}
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	switch kind {
	case "models", "collections", "fields":
	default:
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "kind must be one of models, collections, fields")
		return
	}

	// Resolve the seed's entity type from the receipt. Look the row up
	// directly rather than threading more URL params; same receipt
	// shape the tab schema reads from.
	receipts, err := h.weave.Adoptions().List(ctx,
		pkgdomain.WithProjectID(projectID),
		pkgdomain.WithFilter("context_entity_type", "project"),
		pkgdomain.WithFilter("context_entity_id", projectID),
		pkgdomain.WithFilter("source_project_id", sourceProjectID),
		pkgdomain.WithFilter("source_entity_id", sourceEntityID),
	)
	if err != nil {
		h.logger.Error("load receipt for closure", "project", projectID, "source", sourceEntityID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to load receipt")
		return
	}
	if len(receipts) == 0 {
		errresp.Error(w, r, http.StatusNotFound, "not_found", "receipt not found")
		return
	}
	seedKind := receipts[0].EntityType
	currentVersion := auth.ProjectVersionFromContext(ctx)

	resp := ProjectAdoptionClosureResponse{Kind: kind}
	switch kind {
	case "models":
		ids, err := h.weave.Models().ListReceiptModelClosure(ctx, sourceEntityID, seedKind)
		if err != nil {
			h.logger.Error("receipt model closure", "err", err)
			errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to compute closure")
			return
		}
		resp.Items = make([]ProjectAdoptionClosureItem, 0, len(ids))
		for _, id := range ids {
			m, err := h.weave.Models().GetByID(ctx, id)
			if err != nil || m == nil {
				continue
			}
			resp.Items = append(resp.Items, ProjectAdoptionClosureItem{
				ID: m.ID, SemanticID: m.SemanticID, Name: m.UIName,
				SourceProjectID: m.ProjectID,
				CurrentURL:      addVersion(currentProjectEntityURL(projectID, "model", m.ID), currentVersion),
				SourceURL:       projectEntityURL("model", m.ID),
			})
		}
	case "collections":
		ids, err := h.weave.Collections().ListReceiptCollectionClosure(ctx, sourceEntityID, seedKind)
		if err != nil {
			h.logger.Error("receipt collection closure", "err", err)
			errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to compute closure")
			return
		}
		resp.Items = make([]ProjectAdoptionClosureItem, 0, len(ids))
		for _, id := range ids {
			c, err := h.weave.Collections().GetByID(ctx, id)
			if err != nil || c == nil {
				continue
			}
			resp.Items = append(resp.Items, ProjectAdoptionClosureItem{
				ID: c.ID, SemanticID: c.SemanticID, Name: c.UIName,
				SourceProjectID: c.ProjectID,
				CurrentURL:      addVersion(currentProjectEntityURL(projectID, "collection", c.ID), currentVersion),
				SourceURL:       projectEntityURL("collection", c.ID),
			})
		}
	case "fields":
		ids, err := h.weave.WeaveFields().ListReceiptFieldClosure(ctx, sourceEntityID, seedKind)
		if err != nil {
			h.logger.Error("receipt field closure", "err", err)
			errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to compute closure")
			return
		}
		resp.Items = make([]ProjectAdoptionClosureItem, 0, len(ids))
		for _, id := range ids {
			f, err := h.weave.WeaveFields().GetByID(ctx, id)
			if err != nil || f == nil {
				continue
			}
			resp.Items = append(resp.Items, ProjectAdoptionClosureItem{
				ID: f.ID, SemanticID: f.SemanticID, Name: f.UIName,
				SourceProjectID: f.ProjectID,
				CurrentURL:      addVersion(currentProjectEntityURL(projectID, "field", f.ID), currentVersion),
				SourceURL:       projectEntityURL("field", f.ID),
			})
		}
	}
	resp.Count = len(resp.Items)
	sort.Slice(resp.Items, func(i, j int) bool {
		if resp.Items[i].SourceProjectID != resp.Items[j].SourceProjectID {
			return resp.Items[i].SourceProjectID < resp.Items[j].SourceProjectID
		}
		return resp.Items[i].SemanticID < resp.Items[j].SemanticID
	})

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("encode adoption closure", "err", err)
	}
}

func (h *Handler) entityNamesByID(ctx context.Context, entityType string, ids []string) map[string]pkgdomain.Translations {
	out := make(map[string]pkgdomain.Translations, len(ids))
	if len(ids) == 0 {
		return out
	}
	switch entityType {
	case "model":
		for _, id := range ids {
			item, err := h.weave.Models().GetByID(ctx, id)
			if err == nil && item != nil {
				out[id] = item.UIName
			}
		}
	case "collection":
		for _, id := range ids {
			item, err := h.weave.Collections().GetByID(ctx, id)
			if err == nil && item != nil {
				out[id] = item.UIName
			}
		}
	}
	return out
}

func projectEntityURL(entityType, entityID string) string {
	switch entityType {
	case "model", "collection", "field", "category":
		return pkgdomain.ParseSemanticID(entityID).URL()
	default:
		return ""
	}
}

// currentProjectEntityURL builds the detail URL under the CURRENT
// project (i.e. the one viewing the adoption receipt), not the source
// project. Used for the primary "open" link on each adoption row so
// curators land on the version where overrides + fork live.
func currentProjectEntityURL(currentProjectID, entityType, entityID string) string {
	if currentProjectID == "" || entityID == "" {
		return ""
	}
	plural, ok := map[string]string{
		"model":      "models",
		"collection": "collections",
		"field":      "fields",
		"category":   "categories",
	}[entityType]
	if !ok {
		return ""
	}
	return fmt.Sprintf("/projects/%s/%s/%s", currentProjectID, plural, entityID)
}

func adoptionLabel(adoption pkgdomain.Adoption) pkgdomain.Translations {
	label := adoption.SourceEntityID
	if adoption.EntityType != "" {
		label = adoption.EntityType + " · " + adoption.SourceEntityID
	}
	return pkgdomain.Translations{"en": label, "nl": label}
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (h *Handler) actorNames(ctx context.Context, adoptions []pkgdomain.Adoption) map[string]string {
	if h.actorLabels == nil {
		return map[string]string{}
	}
	ids := make([]string, 0, len(adoptions))
	for _, adoption := range adoptions {
		if adoption.CreatedByID != nil && *adoption.CreatedByID != "" {
			ids = append(ids, *adoption.CreatedByID)
		}
	}
	return h.actorLabels.LabelsByID(ctx, ids)
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

func addVersion(raw, currentVersion string) string {
	if currentVersion == "" || raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("version", currentVersion)
	u.RawQuery = q.Encode()
	return u.String()
}

func (h *Handler) ProjectOverviewSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "project ID is required")
		return
	}

	project := auth.ProjectFromContext(ctx)
	if project == nil {
		h.logger.Error("project missing from request context", "id", projectID)
		errresp.Error(w, r, http.StatusNotFound, "not_found", "project not found")
		return
	}

	statsMap, err := h.weave.Projects().StatsForProjects(ctx, []string{projectID})
	if err != nil {
		h.logger.Error("failed to compute project stats", "id", projectID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to load project stats")
		return
	}
	stats := statsMap[projectID]
	if stats == nil {
		stats = &pkgdomain.WeaveProjectStats{}
	}

	linkedOntologies, err := h.linkedOntologies.LinkedOntologies(ctx, projectID)
	if err != nil {
		h.logger.Debug("failed to load linked ontologies", "project", projectID, "err", err)
	}
	ontoInfos := make([]formschema.OntologyInfo, 0, len(linkedOntologies))
	for _, o := range linkedOntologies {
		ontoInfos = append(ontoInfos, formschema.OntologyInfo{
			Name:           o.Name,
			Version:        o.Version,
			URI:            o.URI,
			ClassCount:     o.ClassCount,
			PropertyCount:  o.PropertyCount,
			ClassesUsed:    o.ClassesUsed,
			PropertiesUsed: o.PropertiesUsed,
			Origin:         o.Origin,
		})
	}

	var attributions []pkgdomain.Attribution
	if h.attributions != nil {
		rows, attribErr := h.attributions.ListForProject(ctx, projectID)
		if attribErr != nil {
			h.logger.Debug("failed to load project attributions", "project", projectID, "err", attribErr)
		} else {
			attributions = rows
		}
	}

	schema := formschema.BuildProjectOverviewSchema(
		project,
		int(stats.ModelCount),
		int(stats.CollectionCount),
		int(stats.FieldCount),
		int(stats.CategoryCount),
		ontoInfos,
		attributions,
		h.currentLang(r),
		h.languages,
	)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(schema); err != nil {
		h.logger.Error("failed to encode project overview schema", "project", projectID, "error", err)
	}
}

func (h *Handler) currentLang(r *http.Request) string {
	if h.langResolver != nil {
		if lang := h.langResolver(r); lang != "" {
			return lang
		}
	}
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return lang
	}
	return "en"
}
