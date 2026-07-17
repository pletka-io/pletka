package templates

import (
	"html/template"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/usermenu"
)

// Breadcrumb is a single breadcrumb item in the thin weave page shell.
// When Pivots is non-empty, the shell renders the crumb as an inline
// "Models | Collections | Fields"-style pivot group instead of a single
// chevron segment — used on the entity-view detail pages so curators
// can jump sideways between entity types without going back to the
// project landing page.
type Breadcrumb struct {
	Label  string
	Href   string
	Pivots []Pivot
}

// Pivot is one alternative target inside a pivot-group breadcrumb. The
// active pivot has Active=true and an empty Href; siblings keep their
// hrefs and render as muted links.
type Pivot struct {
	Label  string
	Href   string
	Active bool
}

// PropAttr is a single data-prop-* attribute for an island mount.
type PropAttr struct {
	Key   string
	Value string
}

// IslandMount describes one Svelte island mount point.
type IslandMount struct {
	Name         string
	Props        map[string]string
	Dependencies []string
	Placeholder  template.HTML
}

// IslandPage is the minimal page contract for schema-driven island wrappers.
type IslandPage struct {
	Title       string
	Lang        string
	Path        string
	Languages   []i18n.Language
	Principal   *auth.Principal
	IsAnonymous bool
	Labels      ShellLabels
	Heading     string
	Subheading  string
	Breadcrumbs []Breadcrumb
	UserMenu    *usermenu.UserMenu
	Island      IslandMount
}

// ShellLabels contains translated strings used by the shared weave shell.
type ShellLabels struct {
	NavOrgs       string
	NavProjects   string
	NavOntologies string
	NavVision     string
	NavCommunity  string
	NavLogin      string
	NavLogout     string
	NavAbout      string
	FooterTagline string
}

type pageView struct {
	IslandPage
	PropAttrs []template.HTMLAttr
}
