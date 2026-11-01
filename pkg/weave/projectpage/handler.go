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
	"github.com/pletka-io/pletka/pkg/browserinput"
	pkgdomain "github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
	"github.com/pletka-io/pletka/pkg/weave/publication"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

type ReleaseReader interface {
	ListByProject(ctx context.Context, projectID string) ([]release.Release, error)
}

// Source returns all display content after closing its bound read session.
type Source interface {
	Summary(context.Context, auth.ReadScope, string) (browserinput.ProjectSummary, error)
	Adoptions(context.Context, auth.ReadScope, string) (browserinput.Adoptions, error)
}

type LangResolver func(*http.Request) string

type Host struct {
	Logger       *slog.Logger
	Weave        pkgdomain.WeaveStore
	Source       Source
	Releases     ReleaseReader
	Languages    []formschema.LanguageInfo
	LangResolver LangResolver
	I18n         i18n.Manager
	// Publication derives the project's unreleased-changes rollup for the
	// header badge. Optional; nil leaves the count zero.
	Publication *publication.Reader
}

func (h Host) Validate() error {
	var missing []string
	if h.Weave == nil {
		missing = append(missing, "Weave")
	}
	if h.Source == nil {
		missing = append(missing, "Source")
	}
	if h.Releases == nil {
		missing = append(missing, "Releases")
	}
	if len(missing) > 0 {
		return fmt.Errorf("projectpage host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

type Handler struct {
	logger       *slog.Logger
	weave        pkgdomain.WeaveStore
	source       Source
	releases     ReleaseReader
	languages    []formschema.LanguageInfo
	langResolver LangResolver
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
	source Source,
	releases ReleaseReader,
	languages []formschema.LanguageInfo,
	langResolver LangResolver,
	i18nMgr i18n.Manager,
) *Handler {
	return &Handler{
		logger:       logger,
		weave:        weaveStore,
		source:       source,
		releases:     releases,
		languages:    languages,
		langResolver: langResolver,
		i18n:         i18nMgr,
	}
}

func Mount(r chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := NewHandler(
		host.Logger,
		host.Weave,
		host.Source,
		host.Releases,
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

type ProjectReleaseTabSchema struct {
	Kind                    string                           `json:"kind"`
	ProjectID               string                           `json:"project_id"`
	CurrentVersion          string                           `json:"current_version,omitempty"`
	CanCreate               bool                             `json:"can_create"`
	CreateFormSchemaURL     string                           `json:"create_form_schema_url,omitempty"`
	CreateBlockedMessage    pkgdomain.Localizable            `json:"create_blocked_message,omitempty"`
	DependencySettingsURL   string                           `json:"dependency_settings_url,omitempty"`
	DraftParentDependencies []ProjectReleaseParentDependency `json:"draft_parent_dependencies,omitempty"`
	DraftURL                string                           `json:"draft_url,omitempty"`
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
		errresp.InternalWith(w, r, err, "failed to load releases")
		return
	}

	canEdit := auth.FromContext(ctx).Can(auth.ProjectEdit, auth.ProjectResource(auth.ProjectFromContext(ctx)), nil)
	canCreate := canEdit && currentVersion == ""
	out := ProjectReleaseTabSchema{
		Kind:           "releases",
		ProjectID:      projectID,
		CurrentVersion: currentVersion,
		CanCreate:      canCreate,
		Items:          make([]ProjectReleaseTabItem, 0, len(items)),
		EmptyMessage:   i18n.L("project_page.releases_empty", "No releases yet."),
	}
	if canEdit {
		out.DraftURL = "/projects/" + projectID
	}
	if canCreate {
		links, ierr := h.weave.ProjectInheritances().List(ctx, projectID)
		if ierr != nil {
			h.logger.Error("failed to load parent dependencies for release tab", "project", projectID, "err", ierr)
			errresp.InternalWith(w, r, ierr, "failed to load releases")
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
