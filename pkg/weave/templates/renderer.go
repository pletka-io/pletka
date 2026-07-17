package templates

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/usermenu"
)

// AnalyticsConfig configures the optional Matomo tracking snippet rendered
// once in the shared shell. Disabled unless Enabled and SiteID + MatomoBaseURL
// are set. Product analytics only — no raw user/project IDs; query strings are
// stripped unless TrackQueryString is set (privacy default).
type AnalyticsConfig struct {
	Enabled          bool
	MatomoBaseURL    string // e.g. https://analytics.pletka.io
	SiteID           string // Matomo idsite (per-instance: prod=1, dev=3, beta=4, alpha=5)
	TrackQueryString bool
}

// matomoSnippet returns the Matomo tracker script, or "" when analytics is off
// or not fully configured.
func matomoSnippet(cfg AnalyticsConfig) template.HTML {
	if !cfg.Enabled || cfg.SiteID == "" || cfg.MatomoBaseURL == "" {
		return ""
	}
	base := strings.TrimRight(cfg.MatomoBaseURL, "/") + "/"
	customURL := ""
	if !cfg.TrackQueryString {
		// path-only page URL — keep arbitrary query strings out of analytics.
		customURL = "_paq.push(['setCustomUrl', location.origin + location.pathname]);"
	}
	js := fmt.Sprintf(`<script>
var _paq=window._paq=window._paq||[];
%s
_paq.push(['trackPageView']);_paq.push(['enableLinkTracking']);
(function(){var u=%q;_paq.push(['setTrackerUrl',u+'matomo.php']);_paq.push(['setSiteId',%q]);
var d=document,g=d.createElement('script'),s=d.getElementsByTagName('script')[0];
g.async=true;g.src=u+'matomo.js';s.parentNode.insertBefore(g,s);})();
</script>`, customURL, base, cfg.SiteID)
	return template.HTML(js) //nolint:gosec // base/siteID are trusted deployment config
}

//go:embed *.gohtml
var templateFS embed.FS

// Renderer renders the minimal weave island page shell. The optional
// i18n.Manager is used to resolve LocalizedText nodes inside the page
// (currently only the UserMenu) before render so the wire shape sent
// to the frontend matches the active language.
type Renderer struct {
	tmpl *template.Template
	i18n i18n.Manager // optional; nil disables LocalizedText resolution
}

// NewRenderer builds a renderer with the minimal function set required by the
// schema-driven island wrapper pages. Pass a non-nil i18n.Manager to enable
// LocalizedText resolution on UserMenu and any future schema-bearing fields.
func NewRenderer(islandScripts func(...string) template.HTML, i18nMgr i18n.Manager, analytics AnalyticsConfig) (*Renderer, error) {
	funcs := template.FuncMap{
		"islandScripts": islandScripts,
		"urlquery":      template.URLQueryEscaper,
		// matomo renders the tracking snippet once in the shell (or nothing).
		"matomo": func() template.HTML { return matomoSnippet(analytics) },
		// toJSON marshals a value to JSON suitable for embedding in a
		// data-prop-* attribute (single-quoted in the template).
		"toJSON": func(v any) (template.JS, error) {
			b, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return template.JS(b), nil
		},
	}

	tmpl, err := template.New("layout.gohtml").Funcs(funcs).ParseFS(templateFS, "*.gohtml")
	if err != nil {
		return nil, fmt.Errorf("parse weave templates: %w", err)
	}

	return &Renderer{tmpl: tmpl, i18n: i18nMgr}, nil
}

// RenderIslandPage renders the shared page shell with one island mount.
//
// Auto-fills page.UserMenu from page.Principal when the caller didn't
// set it (the common case — every page handler benefits from the
// dropdown without per-handler boilerplate). When an i18n.Manager is
// configured, the menu's LocalizedText labels resolve to the active
// language before render.
func (r *Renderer) RenderIslandPage(w http.ResponseWriter, page IslandPage) error {
	if page.UserMenu == nil && page.Principal != nil {
		page.UserMenu = usermenu.Build(page.Principal, page.Path)
	}
	if r.i18n != nil && page.UserMenu != nil {
		r.i18n.Resolve(page.UserMenu, page.Lang)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	view := pageView{
		IslandPage: page,
		PropAttrs:  toPropAttrs(page.Island.Props),
	}
	return r.tmpl.ExecuteTemplate(w, "layout", view)
}

func toPropAttrs(props map[string]string) []template.HTMLAttr {
	if len(props) == 0 {
		return nil
	}

	keys := make([]string, 0, len(props))
	for key := range props {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]template.HTMLAttr, 0, len(keys))
	for _, key := range keys {
		out = append(out, template.HTMLAttr(fmt.Sprintf(`data-prop-%s="%s"`, key, template.HTMLEscapeString(props[key]))))
	}
	return out
}
