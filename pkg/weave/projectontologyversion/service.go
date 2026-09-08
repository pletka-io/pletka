package projectontologyversion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// ProjectReader is the cross-slice reader the service uses to look up
// project metadata + walk the parent chain for inherited groups.
// Satisfied by domain.WeaveStore.Projects() in production.
type ProjectReader interface {
	GetByID(ctx context.Context, id string) (*domain.Project, error)
	ResolvedOntologyVersions(ctx context.Context, projectID string, opts domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error)
}

// OntologyReader exposes the ontology master table for the create form's
// dependent selects (base ontology list + extension lookup).
type OntologyReader interface {
	List(ctx context.Context) ([]*domain.Ontology, error)
	GetByID(ctx context.Context, id string) (*domain.Ontology, error)
}

// OntologyVersionReader exposes the ontology_versions master table.
type OntologyVersionReader interface {
	GetByID(ctx context.Context, id string) (*domain.OntologyVersion, error)
	ListByOntology(ctx context.Context, ontologyID string) ([]*domain.OntologyVersion, error)
}

// Service composes project-ontology-version business logic over a Store
// plus the cross-slice readers it needs.
type Service struct {
	store    Store
	projects ProjectReader
	ontology OntologyReader
	versions OntologyVersionReader
	log      *slog.Logger
	runner   domain.ChangeLogRunner
	bus      domain.EventBus
}

type versionedProjectOntologyStore interface {
	ListVersion(ctx context.Context, projectID, releaseVersion string) ([]*domain.ProjectOntologyVersion, error)
	GetVersion(ctx context.Context, projectID, versionID, releaseVersion string) (*domain.ProjectOntologyVersion, error)
	CountPathElementUsageVersion(ctx context.Context, projectID, versionID, releaseVersion string) (int64, error)
	SamplePathElementFieldsVersion(ctx context.Context, projectID, versionID, releaseVersion string, limit int) ([]domain.FieldUsageSample, error)
	OntologyUsageVersion(ctx context.Context, projectID, versionID, releaseVersion string) (int, int, error)
}

// NewService wires a Service. nil log → slog.Default; nil runner → noop.
func NewService(
	store Store,
	projects ProjectReader,
	ontology OntologyReader,
	versions OntologyVersionReader,
	log *slog.Logger,
	runner domain.ChangeLogRunner,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	if runner == nil {
		runner = domain.NoopChangeLogRunner()
	}
	return &Service{
		store:    store,
		projects: projects,
		ontology: ontology,
		versions: versions,
		log:      log,
		runner:   runner,
	}
}

// WithEventBus sets the bus used to publish ontology-version-change events.
// Call once at boot (from the router) after NewService. nil-safe: Publish
// calls are guarded internally. Returns s for fluent chaining.
func (s *Service) WithEventBus(bus domain.EventBus) *Service {
	s.bus = bus
	return s
}

// publishProjectOntologyVersionsChanged fires a ProjectOntologyVersionsChanged
// event when a bus is wired. Inline helper to keep publish calls DRY.
func (s *Service) publishProjectOntologyVersionsChanged(ctx context.Context, projectID string) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, domain.Event{
		Type:      domain.EventProjectOntologyVersionsChanged,
		ProjectID: projectID,
	})
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

type ErrForbidden struct {
	Capability string
	Resource   string
}

func (e *ErrForbidden) Error() string {
	return fmt.Sprintf("forbidden: requires %s on %s", e.Capability, e.Resource)
}

type ErrValidation struct {
	Fields map[string][]string
}

func (e *ErrValidation) Error() string { return "validation error" }

// ErrInUse is returned by Delete when path elements still reference the
// version (or any cascaded extension). Handlers map to 409 with the
// version_in_use payload shape.
type ErrInUse struct {
	TotalUsage int64
	Samples    []domain.FieldUsageSample
}

func (e *ErrInUse) Error() string {
	return fmt.Sprintf("ontology version in use by %d path elements", e.TotalUsage)
}

var (
	errNotFound  = errors.New("project ontology version: not found")
	errDuplicate = errors.New("project ontology version: already linked")
)

// IsNotFound reports whether err signals "not found".
func IsNotFound(err error) bool { return errors.Is(err, errNotFound) }

// IsDuplicate reports whether err signals "version already linked".
func IsDuplicate(err error) bool { return errors.Is(err, errDuplicate) }

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// CreateInput is the payload accepted by Create.
type CreateInput struct {
	OntologyID string
	VersionID  string
	Extensions []string
	IsPrimary  bool
	UsageNotes string
}

// UpdateInput captures a partial PATCH update.
type UpdateInput struct {
	IsPrimary  *bool
	UsageNotes *string
}

// StatsReport summarises a single linked version's usage.
type StatsReport struct {
	UsageCount   int64                     `json:"usage_count"`
	FieldSamples []domain.FieldUsageSample `json:"field_samples"`
}

// ListView is the composed read-side response (own grouped + inherited).
// One of Items or Groups is non-empty; the handler picks the appropriate
// JSON envelope based on which is set.
type ListView struct {
	OwnGroups       []domain.LinkedOntologyGroup
	InheritedGroups []InheritedGroup
	IsEmpty         bool
}

// InheritedGroup is one ancestor project's contribution to the linked-
// ontologies pane. Carries everything the handler needs to render without
// looking up the parent project again.
type InheritedGroup struct {
	SourceProjectID    string
	SourceProjectLabel string
	Rows               []domain.ResolvedOntologyVersion
}

// PaneView is the rich shape consumed by the Svelte settings ontology
// pane. Each group is one base ontology version + the extensions
// linked under it + the catalog of extensions still available to
// enable. Inherited groups carry the source project label.
//
// URL emission is the contract surface that lets the frontend stay
// schema-driven (.claude/rules/api-patterns.md): server alone decides
// who sees what, and the frontend never composes a project-ontology-
// versions URL. URLs are emitted only when the caller has project.edit
// AND the row belongs to an own group (inherited rows are read-only by
// definition — the user can't mutate the parent's ontology link from
// here).
type PaneView struct {
	Groups []PaneGroup `json:"groups"`
	// Actions carries top-level mutation URLs (currently: add a new
	// ontology). Nil when the caller can't edit; the frontend hides
	// the "Add ontology" affordance on a nil block.
	Actions *PaneActions `json:"actions,omitempty"`
}

// PaneActions is the top-level mutation block on PaneView.
type PaneActions struct {
	// CreateURL is the POST endpoint for adding a new ontology link to
	// the project (used by the AddOntologyModal's FormRenderer when it
	// submits the create form — but the form schema itself owns the
	// submit URL, so frontend code reads this only to decide whether
	// the affordance is enabled).
	CreateURL string `json:"create_url,omitempty"`
	// FormSchemaURL is the GET endpoint that returns the create form
	// schema. The modal fetches this directly.
	FormSchemaURL string `json:"form_schema_url,omitempty"`
}

type PaneGroup struct {
	BaseVersionID       string          `json:"base_version_id"`
	Origin              domain.Origin   `json:"origin"`
	Base                *PaneOntology   `json:"base"`
	Extensions          []PaneOntology  `json:"extensions"`
	AvailableExtensions []PaneAvailable `json:"available_extensions,omitempty"`
}

type PaneOntology struct {
	VersionID     string `json:"version_id"`
	OntologyID    string `json:"ontology_id"`
	Name          string `json:"name"`
	Prefix        string `json:"prefix"`
	OntologyType  string `json:"ontology_type"` // "base" | "extension"
	VersionString string `json:"version_string"`
	IsPrimary     bool   `json:"is_primary"`
	UsageNotes    string `json:"usage_notes,omitempty"`
	// UpdateURL is the PATCH endpoint for this row (mark primary, edit
	// notes). Empty for inherited rows and for read-only callers.
	UpdateURL string `json:"update_url,omitempty"`
	// DeleteURL is the DELETE endpoint for this row (disable an
	// extension, remove a base). Cascade rules live server-side.
	DeleteURL string `json:"delete_url,omitempty"`
}

type PaneAvailable struct {
	OntologyID    string `json:"ontology_id"`
	Name          string `json:"name"`
	Prefix        string `json:"prefix"`
	VersionID     string `json:"version_id"`
	VersionString string `json:"version_string"`
	// EnableURL is the POST endpoint that links this extension to the
	// current project. Empty for read-only callers.
	EnableURL string `json:"enable_url,omitempty"`
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// ListView returns the composed own + inherited view used to render the
// linked-ontologies list. IsEmpty=true means render the empty state.
func (s *Service) ListView(ctx context.Context, projectID string) (*ListView, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if err := s.ensureProjectExists(ctx, projectID); err != nil {
		return nil, err
	}

	own, err := s.store.ListGrouped(ctx, projectID)
	if releaseVersion := auth.ProjectVersionFromContext(ctx); releaseVersion != "" {
		if vs, ok := s.store.(versionedProjectOntologyStore); ok {
			links, lerr := vs.ListVersion(ctx, projectID, releaseVersion)
			if lerr != nil {
				return nil, fmt.Errorf("list archived own links: %w", lerr)
			}
			own, err = s.buildOwnListGroupsVersion(ctx, projectID, releaseVersion, links)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("list own grouped: %w", err)
	}

	inherited, err := s.projects.ResolvedOntologyVersions(ctx, projectID, domain.ResolvedOntologyVersionOpts{OnlyInherited: true})
	if err != nil {
		return nil, fmt.Errorf("resolve inherited ontology versions: %w", err)
	}

	view := &ListView{OwnGroups: own}
	if len(inherited) > 0 {
		grouped, gerr := s.composeInheritedGroups(ctx, inherited)
		if gerr != nil {
			return nil, gerr
		}
		view.InheritedGroups = grouped
	}
	view.IsEmpty = len(view.OwnGroups) == 0 && len(view.InheritedGroups) == 0
	return view, nil
}

func (s *Service) buildOwnListGroupsVersion(ctx context.Context, projectID, releaseVersion string, links []*domain.ProjectOntologyVersion) ([]domain.LinkedOntologyGroup, error) {
	type richItem struct {
		Po                PaneOntology
		ExtendsOntologyID string
		UsageCount        int64
	}

	bases := make(map[string]richItem, len(links))
	extensionsByBase := make(map[string][]richItem, len(links))

	for _, link := range links {
		if link == nil {
			continue
		}
		po, ext, err := s.resolveLinkRich(ctx, link)
		if err != nil {
			return nil, err
		}
		var usageCount int64
		if vs, ok := s.store.(versionedProjectOntologyStore); ok {
			usageCount, err = vs.CountPathElementUsageVersion(ctx, projectID, link.OntologyVersionID, releaseVersion)
			if err != nil {
				return nil, fmt.Errorf("count archived ontology version usage: %w", err)
			}
		}
		ri := richItem{Po: po, ExtendsOntologyID: ext, UsageCount: usageCount}
		if po.OntologyType == string(domain.OntologyTypeBase) {
			if existing, ok := bases[po.OntologyID]; !ok || (po.IsPrimary && !existing.Po.IsPrimary) {
				bases[po.OntologyID] = ri
			}
			continue
		}
		if ext == "" {
			continue
		}
		extensionsByBase[ext] = append(extensionsByBase[ext], ri)
	}

	type baseEntry struct {
		ontologyID string
		item       richItem
	}
	ordered := make([]baseEntry, 0, len(bases))
	for id, item := range bases {
		ordered = append(ordered, baseEntry{ontologyID: id, item: item})
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].item.Po.IsPrimary != ordered[j].item.Po.IsPrimary {
			return ordered[i].item.Po.IsPrimary
		}
		return ordered[i].item.Po.Name < ordered[j].item.Po.Name
	})

	out := make([]domain.LinkedOntologyGroup, 0, len(ordered))
	for _, b := range ordered {
		items := make([]domain.ProjectOntologyVersionWithCounts, 0, 1+len(extensionsByBase[b.ontologyID]))
		items = append(items, domain.ProjectOntologyVersionWithCounts{
			Link: &domain.ProjectOntologyVersion{
				ProjectID:         projectID,
				OntologyVersionID: b.item.Po.VersionID,
				IsPrimary:         b.item.Po.IsPrimary,
				UsageNotes:        b.item.Po.UsageNotes,
			},
			UsageCount:   b.item.UsageCount,
			OntologyName: b.item.Po.Name,
		})
		exts := extensionsByBase[b.ontologyID]
		sort.SliceStable(exts, func(i, j int) bool {
			return exts[i].Po.Name < exts[j].Po.Name
		})
		for _, ext := range exts {
			items = append(items, domain.ProjectOntologyVersionWithCounts{
				Link: &domain.ProjectOntologyVersion{
					ProjectID:         projectID,
					OntologyVersionID: ext.Po.VersionID,
					IsPrimary:         ext.Po.IsPrimary,
					UsageNotes:        ext.Po.UsageNotes,
				},
				UsageCount:   ext.UsageCount,
				OntologyName: ext.Po.Name,
			})
		}
		out = append(out, domain.LinkedOntologyGroup{
			BaseVersionID: b.item.Po.VersionID,
			BaseLabel:     b.item.Po.Name,
			Primary:       b.item.Po.IsPrimary,
			Items:         items,
			Source:        "own",
		})
	}
	return out, nil
}

// LinkedOntologies returns the flat list of ontology versions linked to
// projectID, walking the parent_project_id chain so child projects show
// the ontologies inherited from a master library (e.g. TPE inheriting
// from LA). Deduped by ontology_version_id — own rows win over
// ancestors. SourceProjectID on each entry is empty for own rows and
// set to the ancestor's project ID for inherited rows.
//
// This is the canonical reader surface: every consumer that needs
// "what ontologies does this project have linked, including inherited"
// goes through here. The projectpage overview schema, the settings
// linked-ontologies pane, and any future readers funnel through this
// one method to stop the drift that produced the bug where one path
// read empty legacy tables while another walked the live data.
//
// Sort order: own entries first, then by name + version (stable —
// overview cards rely on it).
func (s *Service) LinkedOntologies(ctx context.Context, projectID string) ([]domain.LinkedOntology, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if err := s.ensureProjectExists(ctx, projectID); err != nil {
		return nil, err
	}

	// Walk the parent chain via projects.ResolvedOntologyVersions.
	// Returns own + inherited rows, deduped by version id, with
	// SourceProjectID set on inherited entries. Honours the active
	// release version (?version=) automatically — it routes to archive
	// tables internally.
	resolved, err := s.projects.ResolvedOntologyVersions(ctx, projectID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		return nil, fmt.Errorf("resolve project ontology versions: %w", err)
	}
	releaseVersion := auth.ProjectVersionFromContext(ctx)

	out := make([]domain.LinkedOntology, 0, len(resolved))
	for _, r := range resolved {
		if r.Link == nil {
			continue
		}
		link := r.Link
		v, err := s.versions.GetByID(ctx, link.OntologyVersionID)
		if err != nil {
			s.log.Warn("get ontology version", "version_id", link.OntologyVersionID, "err", err)
			continue
		}
		if v == nil {
			continue
		}

		name := ""
		if o, oerr := s.ontology.GetByID(ctx, v.OntologyID); oerr == nil && o != nil {
			name = o.Name
		}
		if name == "" {
			if label := v.OntologyLabel.Get("en"); label != "" {
				name = label
			} else if v.OntologyURI != "" {
				name = v.OntologyURI
			} else {
				name = "Unknown"
			}
		}

		// Usage counters reflect the *current* project's field
		// references against the inherited ontology — i.e. how much of
		// the inherited vocabulary the child actually uses. That's the
		// right reading for the child's overview card.
		var classesUsed, propertiesUsed int
		var uerr error
		if releaseVersion != "" {
			if vs, ok := s.store.(versionedProjectOntologyStore); ok {
				classesUsed, propertiesUsed, uerr = vs.OntologyUsageVersion(ctx, projectID, link.OntologyVersionID, releaseVersion)
			} else {
				classesUsed, propertiesUsed, uerr = s.store.OntologyUsage(ctx, projectID, link.OntologyVersionID)
			}
		} else {
			classesUsed, propertiesUsed, uerr = s.store.OntologyUsage(ctx, projectID, link.OntologyVersionID)
		}
		if uerr != nil {
			s.log.Warn("ontology usage", "version_id", link.OntologyVersionID, "err", uerr)
			// Fall through with zero usage rather than 500 the whole
			// overview just because the usage join misfired.
		}
		out = append(out, domain.LinkedOntology{
			Name:            name,
			Version:         v.VersionString,
			URI:             v.OntologyURI,
			ClassCount:      int(v.ClassCount),
			PropertyCount:   int(v.PropertyCount),
			ClassesUsed:     classesUsed,
			PropertiesUsed:  propertiesUsed,
			SourceProjectID: r.SourceProjectID,
			Origin:          domain.OriginFromProject(projectID, r.SourceProjectID),
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		// Own rows first (SourceProjectID == ""), then inherited.
		ownI := out[i].SourceProjectID == ""
		ownJ := out[j].SourceProjectID == ""
		if ownI != ownJ {
			return ownI
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

// PaneView assembles the rich shape consumed by the Svelte settings
// ontology pane. Each PaneGroup is one base ontology + its enabled
// extensions + the catalog of extensions still available to enable.
// Inherited groups (from a parent project) come first and are
// read-only on the frontend.
//
// Grouping rule: ONE PaneGroup per linked ontology classified as
// "base" by weave_ontologies.ontology_type. Extensions attach to the
// group whose base.ontology_id matches the extension's
// extends_ontology_id. This is independent of the version-level
// compatible_base_versions field, which records compatibility, NOT
// classification — using compat for grouping conflates
// "two bases with version-compat overlap" into a single group and
// silently drops one base from display.
func (s *Service) PaneView(ctx context.Context, projectID string) (*PaneView, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if err := s.ensureProjectExists(ctx, projectID); err != nil {
		return nil, err
	}

	// Mutation URLs are only emitted on this read endpoint when the
	// caller can also write. Release-version views are immutable
	// snapshots, so suppress URLs there too — even an editor on the
	// draft can't mutate the archived release. Both checks together
	// give us the schema-driven gate for the whole pane.
	canEdit := s.canEdit(ctx) && auth.ProjectVersionFromContext(ctx) == ""

	out := &PaneView{Groups: make([]PaneGroup, 0)}

	// Inherited groups first — read-only context. Even when canEdit is
	// true these rows never get mutation URLs because the user can't
	// touch the parent project's ontology links from here.
	inheritedRows, err := s.projects.ResolvedOntologyVersions(ctx, projectID, domain.ResolvedOntologyVersionOpts{OnlyInherited: true})
	if err != nil {
		return nil, fmt.Errorf("resolve inherited: %w", err)
	}
	if len(inheritedRows) > 0 {
		inheritedGroups, ierr := s.buildInheritedPaneGroups(ctx, inheritedRows)
		if ierr != nil {
			return nil, ierr
		}
		out.Groups = append(out.Groups, inheritedGroups...)
	}

	// Own groups: load all own links, classify by ontology_type, group by
	// ontology.ID for bases and by extends_ontology_id for extensions.
	links, err := s.store.List(ctx, projectID)
	if releaseVersion := auth.ProjectVersionFromContext(ctx); releaseVersion != "" {
		if vs, ok := s.store.(versionedProjectOntologyStore); ok {
			links, err = vs.ListVersion(ctx, projectID, releaseVersion)
			if err != nil {
				return nil, fmt.Errorf("list archived own links: %w", err)
			}
		}
	} else if err != nil {
		return nil, fmt.Errorf("list own links: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("list own links: %w", err)
	}
	ownGroups, err := s.buildOwnGroups(ctx, links)
	if err != nil {
		return nil, err
	}
	if canEdit {
		decoratePaneGroupsForEdit(projectID, ownGroups)
		out.Actions = &PaneActions{
			CreateURL:     paneCreateURL(projectID),
			FormSchemaURL: paneFormSchemaURL(projectID),
		}
	}
	out.Groups = append(out.Groups, ownGroups...)

	return out, nil
}

// canEdit reports whether the request context carries a snapshot with
// project.edit on the project resource. Mirrors requireProjectWrite but
// returns a bool instead of an error — used when a missing capability
// should silently omit a URL rather than 403.
func (s *Service) canEdit(ctx context.Context) bool {
	snap := auth.FromContext(ctx)
	res := auth.ProjectResourceFromContext(ctx)
	return snap.Can(auth.ProjectEdit, res, nil)
}

// decoratePaneGroupsForEdit fills in update_url/delete_url on each own
// PaneOntology and enable_url on each available extension. Mutates the
// slice in place. Caller must already have verified canEdit.
func decoratePaneGroupsForEdit(projectID string, groups []PaneGroup) {
	for gi := range groups {
		g := &groups[gi]
		if g.Base != nil {
			g.Base.UpdateURL = paneRowURL(projectID, g.Base.VersionID)
			g.Base.DeleteURL = paneRowURL(projectID, g.Base.VersionID)
		}
		for ei := range g.Extensions {
			ext := &g.Extensions[ei]
			ext.UpdateURL = paneRowURL(projectID, ext.VersionID)
			ext.DeleteURL = paneRowURL(projectID, ext.VersionID)
		}
		for ai := range g.AvailableExtensions {
			g.AvailableExtensions[ai].EnableURL = paneCreateURL(projectID)
		}
	}
}

// paneRowURL is the canonical /{versionID} suffix used by both PATCH
// (mark primary, edit usage notes) and DELETE (disable / remove). The
// HTTP verb disambiguates the intent server-side.
func paneRowURL(projectID, versionID string) string {
	return fmt.Sprintf("/projects/%s/project-ontology-versions/%s", projectID, versionID)
}

func paneCreateURL(projectID string) string {
	return fmt.Sprintf("/projects/%s/project-ontology-versions", projectID)
}

func paneFormSchemaURL(projectID string) string {
	return fmt.Sprintf("/projects/%s/project-ontology-versions/form-schema?mode=create", projectID)
}

// buildOwnGroups classifies own links by ontology_type and produces
// one PaneGroup per base-level heading. Extensions attach to the
// heading they extend (via extends_ontology_id) when that heading is
// linked to this project. Extensions whose target isn't linked at all
// are logged and skipped (out-of-scope for the v1 pane; the user can't
// currently see what they're missing without configuring the target
// first).
//
// Base-level headings are not limited to ontology_type=="base": any
// linked extension that itself has >=1 linked child (e.g. aaao, which
// cpro extends) is promoted to its own heading too, so the pane shows
// "two bases (crm + aaao)" instead of silently orphaning cpro under a
// base it doesn't directly extend.
func (s *Service) buildOwnGroups(ctx context.Context, links []*domain.ProjectOntologyVersion) ([]PaneGroup, error) {
	type richItem struct {
		Po PaneOntology
		// ExtendsOntologyID, if any, classifies the item as an extension
		// of that base ontology.
		ExtendsOntologyID string
	}

	bases := make(map[string]richItem, len(links))              // ontology_id → base item
	extensionsByBase := make(map[string][]richItem, len(links)) // extends_ontology_id → extensions
	extensionByID := make(map[string]richItem, len(links))      // ontology_id → its own extension item (promotion lookup)

	for _, link := range links {
		po, ext, err := s.resolveLinkRich(ctx, link)
		if err != nil {
			return nil, err
		}
		ri := richItem{Po: po, ExtendsOntologyID: ext}
		if po.OntologyType == string(domain.OntologyTypeBase) {
			// Tolerate the rare "two links on the same ontology_id" case
			// (shouldn't happen with a unique index but defensive). The
			// primary one wins the slot.
			if existing, ok := bases[po.OntologyID]; !ok || (po.IsPrimary && !existing.Po.IsPrimary) {
				bases[po.OntologyID] = ri
			}
			continue
		}
		// Extension: attach to the base it extends.
		if ext == "" {
			s.log.Warn("extension without extends_ontology_id; skipping",
				"ontology_id", po.OntologyID, "version_id", po.VersionID)
			continue
		}
		extensionsByBase[ext] = append(extensionsByBase[ext], ri)
		extensionByID[po.OntologyID] = ri
	}

	// Promote any linked extension that itself has >=1 linked child to a
	// base-level heading of its own (e.g. aaao, which cpro extends), so
	// the pane shows "two bases (crm + aaao)" instead of orphaning cpro
	// under a base it doesn't directly extend. Visited-set guarded: this
	// pass is a single non-recursive sweep, but pathological extends
	// data (A extends B extends A) must not cause repeat work.
	promoted := make(map[string]richItem, len(extensionByID))
	visited := make(map[string]bool, len(extensionByID))
	for ontologyID, item := range extensionByID {
		if visited[ontologyID] {
			continue
		}
		visited[ontologyID] = true
		if _, isBase := bases[ontologyID]; isBase {
			continue // already a true base heading
		}
		if len(extensionsByBase[ontologyID]) == 0 {
			continue // no linked children -- stays a flat extension row
		}
		promoted[ontologyID] = item
	}

	// Roots: true bases + promoted extensions, each becomes one
	// PaneGroup heading. Ordering: bases first (primary, then name --
	// the pre-existing sort), then promoted headings by name.
	type rootEntry struct {
		ontologyID string
		item       richItem
	}
	baseRoots := make([]rootEntry, 0, len(bases))
	for id, item := range bases {
		baseRoots = append(baseRoots, rootEntry{ontologyID: id, item: item})
	}
	sort.SliceStable(baseRoots, func(i, j int) bool {
		if baseRoots[i].item.Po.IsPrimary != baseRoots[j].item.Po.IsPrimary {
			return baseRoots[i].item.Po.IsPrimary
		}
		return baseRoots[i].item.Po.Name < baseRoots[j].item.Po.Name
	})

	promotedRoots := make([]rootEntry, 0, len(promoted))
	for id, item := range promoted {
		promotedRoots = append(promotedRoots, rootEntry{ontologyID: id, item: item})
	}
	sort.SliceStable(promotedRoots, func(i, j int) bool {
		return promotedRoots[i].item.Po.Name < promotedRoots[j].item.Po.Name
	})

	roots := make([]rootEntry, 0, len(baseRoots)+len(promotedRoots))
	roots = append(roots, baseRoots...)
	roots = append(roots, promotedRoots...)

	groups := make([]PaneGroup, 0, len(roots))
	for _, root := range roots {
		base := root.item.Po

		// Children of this heading, minus any that are themselves
		// promoted (those get their own heading below, not a duplicate
		// listing here).
		rawChildren := extensionsByBase[root.ontologyID]
		exts := make([]richItem, 0, len(rawChildren))
		for _, ri := range rawChildren {
			if _, isPromoted := promoted[ri.Po.OntologyID]; isPromoted {
				continue
			}
			exts = append(exts, ri)
		}

		// Sort extensions stably by name.
		sort.SliceStable(exts, func(i, j int) bool {
			return exts[i].Po.Name < exts[j].Po.Name
		})
		extPos := make([]PaneOntology, 0, len(exts))
		enabled := make(map[string]struct{}, len(exts))
		for _, ri := range exts {
			extPos = append(extPos, ri.Po)
			enabled[ri.Po.OntologyID] = struct{}{}
		}

		avail, err := s.availableExtensionsFor(ctx, base.OntologyID, base.VersionString, enabled)
		if err != nil {
			s.log.Warn("compute available extensions", "base_ontology_id", base.OntologyID, "err", err)
		}
		if avail == nil {
			avail = []PaneAvailable{}
		}

		groups = append(groups, PaneGroup{
			BaseVersionID:       base.VersionID,
			Origin:              domain.OwnOrigin(),
			Base:                &base,
			Extensions:          extPos,
			AvailableExtensions: avail,
		})
	}

	// Orphan extensions (target not linked at all, and not itself a
	// promoted heading): collect under a synthetic group at the end so
	// the user at least sees them in the pane and isn't surprised when
	// the data round-trips. The synthetic group has Base=nil; the
	// frontend renders an "Unattached extensions" header.
	orphans := []PaneOntology{}
	for extendsID, items := range extensionsByBase {
		if _, ok := bases[extendsID]; ok {
			continue
		}
		if _, ok := promoted[extendsID]; ok {
			continue
		}
		for _, it := range items {
			orphans = append(orphans, it.Po)
		}
	}
	if len(orphans) > 0 {
		sort.SliceStable(orphans, func(i, j int) bool {
			return orphans[i].Name < orphans[j].Name
		})
		groups = append(groups, PaneGroup{
			Origin:              domain.OwnOrigin(),
			Extensions:          orphans,
			AvailableExtensions: []PaneAvailable{},
		})
	}

	return groups, nil
}

// resolveLinkRich joins a project-ontology link with its ontology +
// version metadata. Returns the PaneOntology view + the
// extends_ontology_id (empty when not an extension).
func (s *Service) resolveLinkRich(ctx context.Context, link *domain.ProjectOntologyVersion) (PaneOntology, string, error) {
	po := PaneOntology{
		VersionID:  link.OntologyVersionID,
		IsPrimary:  link.IsPrimary,
		UsageNotes: link.UsageNotes,
	}

	v := link.OntologyVersion
	if v == nil {
		fetched, err := s.versions.GetByID(ctx, link.OntologyVersionID)
		if err != nil {
			return po, "", fmt.Errorf("get version %s: %w", link.OntologyVersionID, err)
		}
		v = fetched
	}
	if v == nil {
		return po, "", nil
	}
	po.OntologyID = v.OntologyID
	po.VersionString = v.VersionString

	o, err := s.ontology.GetByID(ctx, v.OntologyID)
	if err != nil {
		return po, "", fmt.Errorf("get ontology %s: %w", v.OntologyID, err)
	}
	if o == nil {
		return po, "", nil
	}
	po.Name = o.Name
	po.Prefix = o.Prefix
	po.OntologyType = string(o.OntologyType)
	extends := ""
	if o.ExtendsOntologyID != nil {
		extends = *o.ExtendsOntologyID
	}
	return po, extends, nil
}

// buildInheritedPaneGroups turns inherited rows into pane groups while keeping
// source-project ordering. Unlike the simpler list view, the pane must group by
// ontology base family as well as by source project; otherwise a parent project
// with more than one linked base collapses unrelated rows under the wrong base.
func (s *Service) buildInheritedPaneGroups(ctx context.Context, rows []domain.ResolvedOntologyVersion) ([]PaneGroup, error) {
	type sourceBucket struct {
		projectID    string
		projectLabel string
		rows         []domain.ResolvedOntologyVersion
	}
	ordered := make([]sourceBucket, 0)
	seenSource := make(map[string]int)
	for _, r := range rows {
		if r.SourceProjectID == "" {
			continue
		}
		if idx, ok := seenSource[r.SourceProjectID]; ok {
			ordered[idx].rows = append(ordered[idx].rows, r)
			continue
		}
		label := r.SourceProjectID
		if parent, err := s.projects.GetByID(ctx, r.SourceProjectID); err == nil && parent != nil {
			if name := parent.UIName.Get("en"); name != "" {
				label = name
			}
		}
		seenSource[r.SourceProjectID] = len(ordered)
		ordered = append(ordered, sourceBucket{
			projectID:    r.SourceProjectID,
			projectLabel: label,
			rows:         []domain.ResolvedOntologyVersion{r},
		})
	}

	type richItem struct {
		Po                PaneOntology
		ExtendsOntologyID string
	}
	out := make([]PaneGroup, 0)
	for _, bucket := range ordered {
		bases := make(map[string]richItem)
		extensionsByBase := make(map[string][]richItem)
		for _, row := range bucket.rows {
			po, ext, err := s.resolveLinkRich(ctx, row.Link)
			if err != nil {
				return nil, err
			}
			ri := richItem{Po: po, ExtendsOntologyID: ext}
			if po.OntologyType == string(domain.OntologyTypeBase) {
				if existing, ok := bases[po.OntologyID]; !ok || (po.IsPrimary && !existing.Po.IsPrimary) {
					bases[po.OntologyID] = ri
				}
				continue
			}
			if ext == "" {
				s.log.Warn("inherited extension without extends_ontology_id; skipping",
					"project_id", bucket.projectID, "ontology_id", po.OntologyID, "version_id", po.VersionID)
				continue
			}
			extensionsByBase[ext] = append(extensionsByBase[ext], ri)
		}

		type baseEntry struct {
			ontologyID string
			item       richItem
		}
		baseOrder := make([]baseEntry, 0, len(bases))
		for id, item := range bases {
			baseOrder = append(baseOrder, baseEntry{ontologyID: id, item: item})
		}
		sort.SliceStable(baseOrder, func(i, j int) bool {
			if baseOrder[i].item.Po.IsPrimary != baseOrder[j].item.Po.IsPrimary {
				return baseOrder[i].item.Po.IsPrimary
			}
			return baseOrder[i].item.Po.Name < baseOrder[j].item.Po.Name
		})

		for _, baseEntry := range baseOrder {
			base := baseEntry.item.Po
			exts := extensionsByBase[baseEntry.ontologyID]
			sort.SliceStable(exts, func(i, j int) bool {
				return exts[i].Po.Name < exts[j].Po.Name
			})
			extPos := make([]PaneOntology, 0, len(exts))
			for _, ri := range exts {
				extPos = append(extPos, ri.Po)
			}
			out = append(out, PaneGroup{
				BaseVersionID: base.VersionID,
				Origin: domain.Origin{
					Kind:               domain.OriginInherited,
					SourceProjectID:    bucket.projectID,
					SourceProjectLabel: bucket.projectLabel,
				},
				Base:                &base,
				Extensions:          extPos,
				AvailableExtensions: []PaneAvailable{},
			})
		}

		orphans := []PaneOntology{}
		for extendsID, items := range extensionsByBase {
			if _, ok := bases[extendsID]; ok {
				continue
			}
			for _, it := range items {
				orphans = append(orphans, it.Po)
			}
		}
		if len(orphans) > 0 {
			sort.SliceStable(orphans, func(i, j int) bool {
				return orphans[i].Name < orphans[j].Name
			})
			out = append(out, PaneGroup{
				Origin: domain.Origin{
					Kind:               domain.OriginInherited,
					SourceProjectID:    bucket.projectID,
					SourceProjectLabel: bucket.projectLabel,
				},
				Extensions:          orphans,
				AvailableExtensions: []PaneAvailable{},
			})
		}
	}
	return out, nil
}

// availableExtensionsFor returns the catalog of extension ontologies
// that target baseOntologyID and aren't already enabled in the project.
//
// Version selection: the active version of each extension wins. If no
// active version exists, fall back to the first version.
//
// Note on the dropped CompatibleBaseVersions filter: extensions
// classify by extends_ontology_id, NOT by version-level compat. Real-
// world data (e.g. AAAo extending CRM, with AAAo's own extensions
// listing compatible_base_versions=[CRM's version_string] rather than
// AAAo's) breaks any naive compat match: AAAo's extensions list CRM's
// "7.1.3" because CRM is the root base of the chain. Strict matching
// against the immediate parent's version_string returned an empty
// catalog. Fix is structural — classify by ontology graph
// (extends_ontology_id), let version compat be advisory metadata
// elsewhere.
func (s *Service) availableExtensionsFor(ctx context.Context, baseOntologyID, _baseVersionString string, enabled map[string]struct{}) ([]PaneAvailable, error) {
	all, err := s.ontology.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list ontologies: %w", err)
	}
	out := make([]PaneAvailable, 0)
	for _, o := range all {
		if o == nil || !o.IsExtension() {
			continue
		}
		if o.ExtendsOntologyID == nil || *o.ExtendsOntologyID != baseOntologyID {
			continue
		}
		if _, already := enabled[o.ID]; already {
			continue
		}
		versions, verr := s.versions.ListByOntology(ctx, o.ID)
		if verr != nil {
			s.log.Warn("list versions for extension", "ontology_id", o.ID, "err", verr)
			continue
		}
		var pick *domain.OntologyVersion
		for _, v := range versions {
			if v == nil {
				continue
			}
			if v.IsActive {
				pick = v
				break
			}
		}
		if pick == nil {
			for _, v := range versions {
				if v != nil {
					pick = v
					break
				}
			}
		}
		if pick == nil {
			continue
		}
		out = append(out, PaneAvailable{
			OntologyID:    o.ID,
			Name:          o.Name,
			Prefix:        o.Prefix,
			VersionID:     pick.ID,
			VersionString: pick.VersionString,
		})
	}
	return out, nil
}

// Get returns a single own link, or (nil, nil) when not found.
func (s *Service) Get(ctx context.Context, projectID, versionID string) (*domain.ProjectOntologyVersion, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vs, ok := s.store.(versionedProjectOntologyStore); ok {
			return vs.GetVersion(ctx, projectID, versionID, version)
		}
	}
	return s.store.Get(ctx, projectID, versionID)
}

// Stats returns usage count and up to 10 field samples for a single linked
// version (no cascade).
func (s *Service) Stats(ctx context.Context, projectID, versionID string) (*StatsReport, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vs, ok := s.store.(versionedProjectOntologyStore); ok {
			count, err := vs.CountPathElementUsageVersion(ctx, projectID, versionID, version)
			if err != nil {
				return nil, err
			}
			samples, err := vs.SamplePathElementFieldsVersion(ctx, projectID, versionID, version, 10)
			if err != nil {
				return nil, err
			}
			return &StatsReport{UsageCount: count, FieldSamples: samples}, nil
		}
	}
	count, err := s.store.CountPathElementUsage(ctx, projectID, versionID)
	if err != nil {
		return nil, err
	}
	samples, err := s.store.SamplePathElementFields(ctx, projectID, versionID, 10)
	if err != nil {
		return nil, err
	}
	return &StatsReport{UsageCount: count, FieldSamples: samples}, nil
}

// ListBaseOntologies returns the base ontologies offered as the first
// dependent select in the create form.
func (s *Service) ListBaseOntologies(ctx context.Context) ([]*domain.Ontology, error) {
	all, err := s.ontology.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list ontologies: %w", err)
	}
	bases := make([]*domain.Ontology, 0, len(all))
	for _, o := range all {
		if o == nil {
			continue
		}
		if o.IsBase() {
			bases = append(bases, o)
		}
	}
	return bases, nil
}

// VersionOption is a {value, label} pair for a dependent select.
type VersionOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ListAvailableVersions returns the versions of `baseOntologyID` not yet
// linked to projectID. Used by the version_id dependent select.
func (s *Service) ListAvailableVersions(ctx context.Context, projectID, baseOntologyID string) ([]VersionOption, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if baseOntologyID == "" {
		return nil, &ErrValidation{Fields: map[string][]string{"base": {"required"}}}
	}

	all, err := s.versions.ListByOntology(ctx, baseOntologyID)
	if err != nil {
		return nil, fmt.Errorf("list ontology versions: %w", err)
	}
	linked, err := s.store.List(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list linked: %w", err)
	}
	linkedSet := make(map[string]struct{}, len(linked))
	for _, l := range linked {
		if l != nil {
			linkedSet[l.OntologyVersionID] = struct{}{}
		}
	}

	out := make([]VersionOption, 0, len(all))
	for _, v := range all {
		if v == nil {
			continue
		}
		if _, already := linkedSet[v.ID]; already {
			continue
		}
		label := v.VersionString
		if label == "" {
			label = v.ID
		}
		out = append(out, VersionOption{Value: v.ID, Label: label})
	}
	return out, nil
}

// ExtensionTreeNode is one node in the extends-hierarchy of compatible
// extensions offered for a chosen base version. Children are the node's
// direct extends-children; HasChildren lets the widget show an expander.
type ExtensionTreeNode struct {
	Value       string              `json:"value"`
	Label       string              `json:"label"`
	Prefix      string              `json:"prefix"`
	HasChildren bool                `json:"has_children"`
	Children    []ExtensionTreeNode `json:"children,omitempty"`
}

// ListAvailableExtensions returns the extends-hierarchy forest of extension
// ontology versions under baseVersionID's ontology, excluding any already
// linked to projectID. Roots are the base ontology's direct
// extends-children; each root's Children are that extension's own direct
// extends-children, and so on. Each node offers the extension's active
// version (fallback: first available).
//
// Inclusion is purely STRUCTURAL — an ontology appears because it extends
// the node above it (extends_ontology_id). compatible_base_versions is
// deliberately NOT a gate: real data populates it inconsistently (every
// AAAo extension declares the root crm version {7.1.3}, and some declare
// nothing), so gating on it silently hid every extension under any non-crm
// base. This matches the "enable extension" path (availableExtensionsFor).
func (s *Service) ListAvailableExtensions(ctx context.Context, projectID, baseVersionID string) ([]ExtensionTreeNode, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if baseVersionID == "" {
		return nil, &ErrValidation{Fields: map[string][]string{"base_version": {"required"}}}
	}

	baseRow, err := s.versions.GetByID(ctx, baseVersionID)
	if err != nil {
		return nil, fmt.Errorf("get base version: %w", err)
	}
	if baseRow == nil {
		return nil, errNotFound
	}

	all, err := s.ontology.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list ontologies: %w", err)
	}
	linked, err := s.store.List(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list linked: %w", err)
	}
	linkedSet := make(map[string]struct{}, len(linked))
	for _, l := range linked {
		if l != nil {
			linkedSet[l.OntologyVersionID] = struct{}{}
		}
	}

	// Structural forest: which ontologies extend which, via extends_ontology_id.
	// This is the sole basis for the tree — compatible_base_versions is NOT a
	// gate here. That field is populated inconsistently (usually the root CRM
	// version, e.g. AAAo's own extensions all declare {7.1.3} not AAAo's
	// version, and some declare nothing), so gating on it silently hid every
	// extension under any non-CRM base. The extends relationship is the truth,
	// matching the "enable extension" path (availableExtensionsFor).
	childOntologies := make(map[string][]*domain.Ontology)
	for _, o := range all {
		if o == nil || !o.IsExtension() {
			continue
		}
		parent := ""
		if o.ExtendsOntologyID != nil {
			parent = *o.ExtendsOntologyID
		}
		childOntologies[parent] = append(childOntologies[parent], o)
	}

	return s.buildExtensionForest(ctx, childOntologies, linkedSet, baseRow.OntologyID, map[string]bool{})
}

// buildExtensionForest recursively assembles the extends-forest under
// parentOntologyID from the structural childOntologies map, offering each
// ontology's active version (fallback: first) and skipping versions already
// linked to the project. Cycle-guarded via seen (an ontology cannot extend
// its own ancestor).
func (s *Service) buildExtensionForest(ctx context.Context, childOntologies map[string][]*domain.Ontology, linkedSet map[string]struct{}, parentOntologyID string, seen map[string]bool) ([]ExtensionTreeNode, error) {
	candidates := childOntologies[parentOntologyID]
	nodes := make([]ExtensionTreeNode, 0, len(candidates))
	for _, o := range candidates {
		if seen[o.ID] {
			continue // cycle guard
		}
		versions, listErr := s.versions.ListByOntology(ctx, o.ID)
		if listErr != nil {
			return nil, fmt.Errorf("list extension versions: %w", listErr)
		}
		// Offer the active version (fallback: first available); the ontology
		// is included because it extends this node, not because of any
		// compatibility declaration. Mirrors availableExtensionsFor.
		pick := firstActiveOrAny(versions)
		if pick == nil {
			continue
		}
		if _, already := linkedSet[pick.ID]; already {
			continue
		}

		next := make(map[string]bool, len(seen)+1)
		for k, val := range seen {
			next[k] = val
		}
		next[o.ID] = true

		children, buildErr := s.buildExtensionForest(ctx, childOntologies, linkedSet, o.ID, next)
		if buildErr != nil {
			return nil, buildErr
		}

		verStr := pick.VersionString
		if verStr == "" {
			verStr = pick.ID
		}
		name := o.Name
		if name == "" {
			name = o.ID
		}
		nodes = append(nodes, ExtensionTreeNode{
			Value:       pick.ID,
			Label:       fmt.Sprintf("%s %s", name, verStr),
			Prefix:      o.Prefix,
			HasChildren: len(children) > 0,
			Children:    children,
		})
	}
	return nodes, nil
}

// firstActiveOrAny returns the active version, else the first non-nil version,
// else nil.
func firstActiveOrAny(versions []*domain.OntologyVersion) *domain.OntologyVersion {
	for _, v := range versions {
		if v != nil && v.IsActive {
			return v
		}
	}
	for _, v := range versions {
		if v != nil {
			return v
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

// resolveExtensionChain expands the selected extension version IDs to also
// include every ancestor extension version (walking extends_ontology_id up
// to, but not including, a base ontology), so linking a deep extension pulls
// its intermediate bases along. Deduped; cycle-guarded. The base itself is
// not included here — Create links it separately via in.VersionID.
func (s *Service) resolveExtensionChain(ctx context.Context, selected []string) ([]string, error) {
	out := make([]string, 0, len(selected))
	seenVer := map[string]bool{}
	add := func(id string) {
		if id != "" && !seenVer[id] {
			seenVer[id] = true
			out = append(out, id)
		}
	}
	for _, verID := range selected {
		add(verID)
		ver, err := s.versions.GetByID(ctx, verID)
		if err != nil {
			return nil, fmt.Errorf("resolve chain: get version: %w", err)
		}
		if ver == nil {
			continue
		}
		ont, err := s.ontology.GetByID(ctx, ver.OntologyID)
		if err != nil {
			return nil, fmt.Errorf("resolve chain: get ontology: %w", err)
		}
		seenOnt := map[string]bool{ver.OntologyID: true}
		for ont != nil && ont.ExtendsOntologyID != nil && *ont.ExtendsOntologyID != "" {
			parentID := *ont.ExtendsOntologyID
			if seenOnt[parentID] {
				break // cycle guard
			}
			seenOnt[parentID] = true
			parent, err := s.ontology.GetByID(ctx, parentID)
			if err != nil {
				return nil, fmt.Errorf("resolve chain: get parent: %w", err)
			}
			if parent == nil || parent.IsBase() {
				break // reached (or past) the base; base is linked via in.VersionID
			}
			pv, err := s.activeVersionForOntology(ctx, parent.ID)
			if err != nil {
				return nil, fmt.Errorf("resolve chain: active parent version: %w", err)
			}
			if pv != nil {
				add(pv.ID)
			} else {
				// No active version for this intermediate ancestor: it
				// can't be linked, so the chain gets a hole and the deep
				// extension will surface as an orphan in the pane. Don't
				// fail the create, but don't do it silently either.
				s.log.Warn("extension chain: ancestor has no active version; skipping (linked extension may appear unattached)",
					"ancestor_ontology_id", parent.ID, "ancestor_prefix", parent.Prefix)
			}
			ont = parent
		}
	}
	return out, nil
}

// activeVersionForOntology returns the ACTIVE version row for ontologyID, or
// nil if none is active. OntologyVersionReader has no direct "active"
// lookup, so this scans ListByOntology for the IsActive row.
func (s *Service) activeVersionForOntology(ctx context.Context, ontologyID string) (*domain.OntologyVersion, error) {
	versions, err := s.versions.ListByOntology(ctx, ontologyID)
	if err != nil {
		return nil, err
	}
	for _, v := range versions {
		if v != nil && v.IsActive {
			return v, nil
		}
	}
	return nil, nil
}

// Create links a base version (and optional extensions) to projectID.
// IsPrimary triggers an atomic primary flip on success.
func (s *Service) Create(ctx context.Context, projectID string, in CreateInput) (*domain.ProjectOntologyVersion, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, err
	}
	if err := s.ensureProjectExists(ctx, projectID); err != nil {
		return nil, err
	}
	if in.VersionID == "" {
		return nil, &ErrValidation{Fields: map[string][]string{"version_id": {"required"}}}
	}

	existing, err := s.store.Get(ctx, projectID, in.VersionID)
	if err != nil {
		return nil, fmt.Errorf("check existing: %w", err)
	}
	if existing != nil {
		return nil, errDuplicate
	}

	chain, err := s.resolveExtensionChain(ctx, in.Extensions)
	if err != nil {
		return nil, err
	}

	addedByID := s.actorIDFromContext(ctx)
	now := time.Now()

	base := &domain.ProjectOntologyVersion{
		ProjectID:         projectID,
		OntologyVersionID: in.VersionID,
		AddedAt:           now,
		AddedByID:         addedByID,
		IsPrimary:         false, // SetPrimary handles flip atomically
		UsageNotes:        in.UsageNotes,
	}

	if err := s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Create(ctx, base); err != nil {
			return fmt.Errorf("create base link: %w", err)
		}
		if err := rec.Record(ctx, domain.ChangeLogEntry{
			EntityType: "project-ontology-version",
			EntityID:   in.VersionID,
			Operation:  "create",
			ProjectID:  projectID,
			Payload:    marshalLink(base),
		}); err != nil {
			return err
		}

		// Best-effort extensions — duplicates are skipped silently. chain is
		// in.Extensions expanded to include every intermediate ancestor
		// extension (resolveExtensionChain); the base is already linked
		// above via in.VersionID.
		for _, extID := range chain {
			if extID == "" || extID == in.VersionID {
				continue
			}
			existingExt, getErr := s.store.Get(ctx, projectID, extID)
			if getErr != nil {
				s.log.Warn("check existing extension failed", "err", getErr, "project_id", projectID, "extension_version_id", extID)
				continue
			}
			if existingExt != nil {
				continue
			}
			ext := &domain.ProjectOntologyVersion{
				ProjectID:         projectID,
				OntologyVersionID: extID,
				AddedAt:           now,
				AddedByID:         addedByID,
				IsPrimary:         false,
				UsageNotes:        "",
			}
			if err := s.store.Create(ctx, ext); err != nil {
				s.log.Warn("create extension failed", "err", err, "project_id", projectID, "extension_version_id", extID)
				continue
			}
			if err := rec.Record(ctx, domain.ChangeLogEntry{
				EntityType: "project-ontology-version",
				EntityID:   extID,
				Operation:  "create",
				ProjectID:  projectID,
				Payload:    marshalLink(ext),
			}); err != nil {
				return err
			}
		}

		if in.IsPrimary {
			if err := s.store.SetPrimary(ctx, projectID, in.VersionID); err != nil {
				return fmt.Errorf("set primary: %w", err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	final, err := s.store.Get(ctx, projectID, in.VersionID)
	if err != nil || final == nil {
		return nil, fmt.Errorf("re-read created link: %w", err)
	}
	s.publishProjectOntologyVersionsChanged(ctx, projectID)
	return final, nil
}

// Update applies a PATCH to a single link. is_primary=true triggers a
// primary flip; is_primary=false flips off (when the row was primary).
func (s *Service) Update(ctx context.Context, projectID, versionID string, in UpdateInput) (*domain.ProjectOntologyVersion, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, err
	}

	link, err := s.store.Get(ctx, projectID, versionID)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return nil, errNotFound
	}
	prev := *link

	if err := s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		setPrimary := in.IsPrimary != nil && *in.IsPrimary && !link.IsPrimary
		if setPrimary {
			if err := s.store.SetPrimary(ctx, projectID, versionID); err != nil {
				return fmt.Errorf("set primary: %w", err)
			}
			reloaded, rerr := s.store.Get(ctx, projectID, versionID)
			if rerr != nil || reloaded == nil {
				return fmt.Errorf("reload after set primary: %w", rerr)
			}
			link = reloaded
		}
		if in.IsPrimary != nil && !*in.IsPrimary {
			link.IsPrimary = false
		}
		if in.UsageNotes != nil {
			link.UsageNotes = *in.UsageNotes
		}
		if err := s.store.Update(ctx, link); err != nil {
			return fmt.Errorf("update link: %w", err)
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "project-ontology-version",
			EntityID:        versionID,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalLink(&prev),
			Payload:         marshalLink(link),
		})
	}); err != nil {
		return nil, err
	}

	final, err := s.store.Get(ctx, projectID, versionID)
	if err != nil || final == nil {
		return nil, fmt.Errorf("re-read updated link: %w", err)
	}
	s.publishProjectOntologyVersionsChanged(ctx, projectID)
	return final, nil
}

// Delete unlinks an ontology version with cascade: any extension whose
// CompatibleBaseVersions includes this version's version_string is also
// removed. If any path element still references the target or any cascade
// row, ErrInUse is returned with up to 10 field samples.
func (s *Service) Delete(ctx context.Context, projectID, versionID string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}

	target, err := s.store.Get(ctx, projectID, versionID)
	if err != nil {
		return err
	}
	if target == nil {
		return errNotFound
	}

	cascadeIDs, err := s.collectCascadeExtensionIDs(ctx, projectID, versionID)
	if err != nil {
		return err
	}

	var totalUsage int64
	samples := make([]domain.FieldUsageSample, 0, 10)
	allIDs := append([]string{versionID}, cascadeIDs...)
	for _, id := range allIDs {
		count, cerr := s.store.CountPathElementUsage(ctx, projectID, id)
		if cerr != nil {
			return fmt.Errorf("count usage: %w", cerr)
		}
		totalUsage += count
		if count > 0 && len(samples) < 10 {
			limit := 10 - len(samples)
			got, serr := s.store.SamplePathElementFields(ctx, projectID, id, limit)
			if serr != nil {
				return fmt.Errorf("sample fields: %w", serr)
			}
			samples = append(samples, got...)
		}
	}
	if totalUsage > 0 {
		return &ErrInUse{TotalUsage: totalUsage, Samples: samples}
	}

	if err := s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		// Extensions first, then target.
		for _, id := range cascadeIDs {
			ext, _ := s.store.Get(ctx, projectID, id)
			if err := s.store.Delete(ctx, projectID, id); err != nil {
				return fmt.Errorf("delete extension: %w", err)
			}
			if ext != nil {
				if err := rec.Record(ctx, domain.ChangeLogEntry{
					EntityType:      "project-ontology-version",
					EntityID:        id,
					Operation:       "delete",
					ProjectID:       projectID,
					PreviousPayload: marshalLink(ext),
				}); err != nil {
					return err
				}
			}
		}
		if err := s.store.Delete(ctx, projectID, versionID); err != nil {
			return fmt.Errorf("delete target: %w", err)
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "project-ontology-version",
			EntityID:        versionID,
			Operation:       "delete",
			ProjectID:       projectID,
			PreviousPayload: marshalLink(target),
		})
	}); err != nil {
		return err
	}
	s.publishProjectOntologyVersionsChanged(ctx, projectID)
	return nil
}

// ---------------------------------------------------------------------------
// Internal composition + helpers
// ---------------------------------------------------------------------------

// collectCascadeExtensionIDs returns version IDs in the project whose
// underlying ontology_version's CompatibleBaseVersions includes target's
// version_string. Excludes the target itself.
// collectCascadeExtensionIDs returns the version IDs of extensions
// linked to the project whose ontology extends the target's ontology.
// Cascade fires only when the target is a BASE ontology — extensions
// don't have transitive dependents in the link table.
//
// Matching rule mirrors PaneView's grouping: extensions classify by
// ontology_type='extension' and extends_ontology_id == targetOntologyID.
// The previous heuristic used CompatibleBaseVersions (a version-level
// compatibility hint) which produced false positives when an unrelated
// base shared a version string with a real extension's compat list.
func (s *Service) collectCascadeExtensionIDs(ctx context.Context, projectID, targetVersionID string) ([]string, error) {
	targetVersion, err := s.versions.GetByID(ctx, targetVersionID)
	if err != nil || targetVersion == nil {
		// Without the target's ontology we can't compute cascade.
		return nil, nil
	}
	targetOntology, err := s.ontology.GetByID(ctx, targetVersion.OntologyID)
	if err != nil || targetOntology == nil {
		return nil, nil
	}
	// Cascade only applies when the target is a base. Extensions don't
	// have dependents in this slice's link table.
	if !targetOntology.IsBase() {
		return nil, nil
	}
	targetOntologyID := targetOntology.ID

	linked, err := s.store.List(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list linked for cascade: %w", err)
	}

	out := make([]string, 0)
	for _, row := range linked {
		if row == nil || row.OntologyVersionID == targetVersionID {
			continue
		}
		v, verr := s.versions.GetByID(ctx, row.OntologyVersionID)
		if verr != nil || v == nil {
			continue
		}
		o, oerr := s.ontology.GetByID(ctx, v.OntologyID)
		if oerr != nil || o == nil {
			continue
		}
		// Extension AND extends the target base.
		if !o.IsExtension() {
			continue
		}
		if o.ExtendsOntologyID == nil || *o.ExtendsOntologyID != targetOntologyID {
			continue
		}
		out = append(out, row.OntologyVersionID)
	}
	return out, nil
}

// composeInheritedGroups builds one InheritedGroup per ancestor project
// that contributes at least one row, in walk order.
func (s *Service) composeInheritedGroups(ctx context.Context, rows []domain.ResolvedOntologyVersion) ([]InheritedGroup, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	type bucket struct {
		projectID string
		rows      []domain.ResolvedOntologyVersion
	}
	var ordered []bucket
	seen := make(map[string]int)
	for _, r := range rows {
		if r.SourceProjectID == "" {
			continue
		}
		if idx, ok := seen[r.SourceProjectID]; ok {
			ordered[idx].rows = append(ordered[idx].rows, r)
			continue
		}
		seen[r.SourceProjectID] = len(ordered)
		ordered = append(ordered, bucket{projectID: r.SourceProjectID, rows: []domain.ResolvedOntologyVersion{r}})
	}

	out := make([]InheritedGroup, 0, len(ordered))
	for _, b := range ordered {
		parent, err := s.projects.GetByID(ctx, b.projectID)
		label := b.projectID
		if err == nil && parent != nil {
			if name := parent.UIName.Get("en"); name != "" {
				label = name
			}
		}
		out = append(out, InheritedGroup{
			SourceProjectID:    b.projectID,
			SourceProjectLabel: label,
			Rows:               b.rows,
		})
	}
	return out, nil
}

func (s *Service) ensureProjectExists(ctx context.Context, projectID string) error {
	p, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	if p == nil {
		return errNotFound
	}
	return nil
}

func (s *Service) actorIDFromContext(ctx context.Context) *string {
	p := auth.PrincipalFromContext(ctx)
	if p == nil || p.ActorID == "" {
		return nil
	}
	id := p.ActorID
	return &id
}

func (s *Service) requireProjectRead(ctx context.Context, projectID string) error {
	snap := auth.FromContext(ctx)
	res := auth.ProjectResourceFromContext(ctx)
	if !snap.Can(auth.ProjectRead, res, nil) {
		return &ErrForbidden{Capability: string(auth.ProjectRead), Resource: "project:" + projectID}
	}
	return nil
}

func (s *Service) requireProjectWrite(ctx context.Context, projectID string) error {
	snap := auth.FromContext(ctx)
	res := auth.ProjectResourceFromContext(ctx)
	if !snap.Can(auth.ProjectEdit, res, nil) {
		return &ErrForbidden{Capability: string(auth.ProjectEdit), Resource: "project:" + projectID}
	}
	return nil
}

func marshalLink(l *domain.ProjectOntologyVersion) []byte {
	if l == nil {
		return nil
	}
	b, _ := json.Marshal(l)
	return b
}
