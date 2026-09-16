package formschema

import (
	"fmt"
	"net/url"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// ProjectPageSchema is the top-level schema for the project detail page.
// It describes the page header, tabs, and navigation links.
type ProjectPageSchema struct {
	Entity   ProjectPageEntity    `json:"entity"`
	Tabs     []ProjectPageTab     `json:"tabs"`
	NavLinks []ProjectPageNavLink `json:"nav_links,omitempty"`
	Warnings []SettingsWarning    `json:"warnings,omitempty"`
	Release  *ProjectReleaseView  `json:"release,omitempty"`
	UI       SchemaUI             `json:"ui"`
}

// ProjectPageEntity is the project header content shown above the tabs.
type ProjectPageEntity struct {
	ID          string              `json:"id"`
	Name        domain.Translations `json:"name"`
	Description domain.Translations `json:"description,omitempty"`
	// Status is the derived project publication state for the header badge:
	// draft (no release) | published (released, no unreleased edits) |
	// modified (released, has unreleased edits).
	Status string `json:"status"`
	// UnreleasedChanges is the count of live entities new or modified since the
	// latest release. Zero (omitted) when up to date or unreleased.
	UnreleasedChanges int `json:"unreleased_changes,omitempty"`
}

// ProjectPageTab describes one tab in the project detail page.
//
// Tabs may nest one level deep: a parent tab declares Children and omits
// ContentURL. The frontend renders the sub-row when the parent (or any
// of its children) is active. Counts on a parent are derived as a list
// (Breakdown) of the children's counts so the chip can show
// "Patterns 12·45·617" without losing the per-child signal.
//
// Align controls which side of the tab bar the chip sits on: "" or
// "left" → primary content tabs; "right" → muted meta nav (Exports,
// Settings). Variant flips chip styling: "" or "primary" → standard;
// "muted" → smaller, gray-500, separated from the left tabs by a
// vertical divider.
type ProjectPageTab struct {
	ID         string             `json:"id"`
	Label      domain.Localizable `json:"label"`
	Icon       string             `json:"icon"`
	ContentURL string             `json:"content_url,omitempty"`
	Href       string             `json:"href,omitempty"` // full-page navigation (Settings, Exports)
	Count      int                `json:"count,omitempty"`
	Breakdown  []int              `json:"breakdown,omitempty"` // per-child counts on parent tabs
	Default    bool               `json:"default,omitempty"`
	Disabled   bool               `json:"disabled,omitempty"`
	Tooltip    string             `json:"tooltip,omitempty"`
	Align      string             `json:"align,omitempty"`    // "left" (default) | "right"
	Variant    string             `json:"variant,omitempty"`  // "primary" (default) | "muted"
	Children   []ProjectPageTab   `json:"children,omitempty"` // sub-row when parent is active
}

// ProjectPageNavLink is a page-level nav link (e.g., Exports, Settings).
// Unlike tabs, nav links perform full-page navigation.
type ProjectPageNavLink struct {
	Label domain.Localizable `json:"label"`
	Href  string             `json:"href"`
	Icon  string             `json:"icon"`
}

type ProjectReleaseView struct {
	Version  string             `json:"version"`
	DraftURL string             `json:"draft_url,omitempty"`
	Label    domain.Localizable `json:"label"`
}

// BuildProjectPageSchema creates the page-level schema for a project detail view.
// canAdmin should be true when the current user has admin rights on the project
// (owner, maintainer, or superadmin role); this controls visibility of the
// Settings nav link.
// isMember should be true when the current user is a project member
// (owner / explicit role / inherited org-membership / super-admin); this
// controls visibility of the Exports nav link, since the verification CSV
// surface is members-only.
//
// warnings is the precomputed list from ComputeProjectWarnings — the handler
// owns the auth snapshot and resource needed to compute them.
func BuildProjectPageSchema(
	project *domain.Project,
	modelCount, collectionCount, fieldCount, exampleCount int,
	conceptListCount int,
	adoptionCount int,
	releaseCount int,
	unreleasedChanges int,
	canAdmin, isMember bool,
	warnings []SettingsWarning,
	lang string,
	languages []LanguageInfo,
	activeVersion string,
) *ProjectPageSchema {
	// Derive the header status: a released project reads published when its
	// live working set matches the release, modified when it has unreleased
	// edits; an unreleased project keeps its raw draft status.
	headerStatus := string(project.Status)
	if releaseCount > 0 {
		if unreleasedChanges > 0 {
			headerStatus = "modified"
		} else {
			headerStatus = "published"
		}
	}
	withVersion := func(raw string) string {
		if activeVersion == "" {
			return raw
		}
		return addVersionToURL(raw, activeVersion)
	}

	overview := ProjectPageTab{
		ID:         "overview",
		Label:      i18n.L("workspace.tabs.overview", "Overview"),
		Icon:       "home",
		ContentURL: withVersion(fmt.Sprintf("/projects/%s/overview-schema", project.ID)),
		Default:    true,
	}

	patternsChildren := []ProjectPageTab{
		{
			ID:         "models",
			Label:      i18n.L("common.models", "Models"),
			Icon:       "cube",
			ContentURL: withVersion(fmt.Sprintf("/projects/%s/entity-list-schema/model", project.ID)),
			Count:      modelCount,
		},
		{
			ID:         "collections",
			Label:      i18n.L("common.collections", "Collections"),
			Icon:       "collection",
			ContentURL: withVersion(fmt.Sprintf("/projects/%s/entity-list-schema/collection", project.ID)),
			Count:      collectionCount,
		},
		{
			ID:         "fields",
			Label:      i18n.L("common.fields", "Fields"),
			Icon:       "list",
			ContentURL: withVersion(fmt.Sprintf("/projects/%s/entity-list-schema/field", project.ID)),
			Count:      fieldCount,
		},
		{
			ID:         "concept-lists",
			Label:      i18n.L("concept_list.list.title", "Concept lists"),
			Icon:       "tag",
			ContentURL: withVersion(fmt.Sprintf("/projects/%s/entity-list-schema/concept-list", project.ID)),
			Count:      conceptListCount,
			Disabled:   activeVersion != "",
			Tooltip: func() string {
				if activeVersion != "" {
					return "Concept list browsing is currently available only in draft mode"
				}
				return ""
			}(),
		},
		{
			ID:         "examples",
			Label:      i18n.L("common.examples", "Examples"),
			Icon:       "document-text",
			ContentURL: withVersion(fmt.Sprintf("/projects/%s/entity-list-schema/example", project.ID)),
			Count:      exampleCount,
			Disabled:   activeVersion != "",
			Tooltip: func() string {
				if activeVersion != "" {
					return "Examples are currently available only in draft mode"
				}
				return ""
			}(),
		},
	}
	patterns := ProjectPageTab{
		ID:        "patterns",
		Label:     i18n.L("workspace.tabs.patterns", "Patterns"),
		Icon:      "cube",
		Breakdown: []int{modelCount, collectionCount, fieldCount, conceptListCount, exampleCount},
		Children:  patternsChildren,
	}

	lifecycleChildren := []ProjectPageTab{
		{
			ID:         "releases",
			Label:      i18n.L("project_page.releases", "Releases"),
			Icon:       "bookmark",
			ContentURL: withVersion(fmt.Sprintf("/projects/%s/release-tab-schema", project.ID)),
			Count:      releaseCount,
		},
		{
			ID:         "adoptions",
			Label:      i18n.L("project_page.adoptions", "Adoptions"),
			Icon:       "share",
			ContentURL: withVersion(fmt.Sprintf("/projects/%s/adoptions-tab-schema", project.ID)),
			Count:      adoptionCount,
		},
		{
			ID:         "activity",
			Label:      i18n.L("project_page.activity", "Activity"),
			Icon:       "clock",
			ContentURL: withVersion(fmt.Sprintf("/projects/%s/activity-schema", project.ID)),
			Disabled:   true,
			Tooltip:    "Coming soon",
		},
	}
	lifecycle := ProjectPageTab{
		ID:        "lifecycle",
		Label:     i18n.L("workspace.tabs.lifecycle", "Lifecycle"),
		Icon:      "bookmark",
		Breakdown: []int{adoptionCount, releaseCount},
		Children:  lifecycleChildren,
	}

	tabs := []ProjectPageTab{overview, patterns, lifecycle}

	// Right-aligned meta nav: Exports (members-only) + Settings (admin).
	// Rendered as muted-variant tabs with full-page Href instead of
	// ContentURL. The separator before them is drawn by the frontend.
	if isMember {
		tabs = append(tabs, ProjectPageTab{
			ID:      "exports",
			Label:   i18n.L("project_page.exports", "Exports"),
			Icon:    "download",
			Href:    withVersion(fmt.Sprintf("/projects/%s/exports", project.ID)),
			Align:   "right",
			Variant: "muted",
		})
	}
	if canAdmin {
		tabs = append(tabs, ProjectPageTab{
			ID:      "settings",
			Label:   i18n.L("workspace.org.settings", "Settings"),
			Icon:    "cog",
			Href:    withVersion(fmt.Sprintf("/projects/%s/settings", project.ID)),
			Align:   "right",
			Variant: "muted",
		})
	}

	// NavLinks remain on the schema but stay empty under the new tab
	// model — kept for wire compatibility with any older client. Newer
	// clients read the right-aligned tabs instead.
	var navLinks []ProjectPageNavLink

	return &ProjectPageSchema{
		Entity: ProjectPageEntity{
			ID:                project.ID,
			Name:              project.UIName,
			Description:       project.Description,
			Status:            headerStatus,
			UnreleasedChanges: unreleasedChanges,
		},
		Tabs:     tabs,
		NavLinks: navLinks,
		Warnings: warnings,
		Release: func() *ProjectReleaseView {
			if activeVersion == "" {
				return nil
			}
			view := &ProjectReleaseView{
				Version: activeVersion,
				Label: i18n.LF("release.viewing_version", "Viewing release {version}",
					map[string]string{"version": activeVersion}),
			}
			// canAdmin actually carries the ProjectEdit capability signal
			// (see the caller) — only editors get a return-to-draft link.
			if canAdmin {
				view.DraftURL = fmt.Sprintf("/projects/%s", project.ID)
			}
			return view
		}(),
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

func addVersionToURL(raw, activeVersion string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("version", activeVersion)
	u.RawQuery = q.Encode()
	return u.String()
}
