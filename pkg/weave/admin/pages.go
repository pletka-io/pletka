package admin

import (
	"html/template"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/session"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// Pages renders the global admin shell page.
type Pages struct {
	logger   *slog.Logger
	renderer *weavetemplates.Renderer
	i18n     i18n.Manager
	session  *session.Manager
}

func NewPages(logger *slog.Logger, renderer *weavetemplates.Renderer, i18nManager i18n.Manager, sessionManager *session.Manager) *Pages {
	if logger == nil {
		logger = slog.Default()
	}
	return &Pages{
		logger:   logger,
		renderer: renderer,
		i18n:     i18nManager,
		session:  sessionManager,
	}
}

func (p *Pages) Mount(r chi.Router) {
	r.Get("/admin", p.AdminPage)
}

func (p *Pages) AdminPage(w http.ResponseWriter, r *http.Request) {
	snap := weaveauth.FromContext(r.Context())
	if snap == nil || !snap.IsSuperAdmin {
		errresp.Error(w, r, http.StatusNotFound, "not_found", "not found")
		return
	}

	lang := p.currentLang(r)
	page := weavetemplates.IslandPage{
		Title:     "Admin",
		Lang:      lang,
		Path:      r.URL.Path,
		Languages: p.i18n.Languages(),
		Principal: weaveauth.PrincipalFromContext(r.Context()),
		Labels:    p.renderer.ShellLabels(lang),
		Breadcrumbs: []weavetemplates.Breadcrumb{
			{Label: "Admin"},
		},
		Island: weavetemplates.IslandMount{
			Name: frontendrefs.Island("admin-shell"),
			Props: map[string]string{
				"lang": lang,
			},
			Dependencies: []string{frontendrefs.Island("admin-shell")},
			Placeholder:  placeholderHTML(),
		},
	}

	if err := p.renderer.RenderIslandPage(w, page); err != nil {
		p.logger.Error("render admin page", "err", err)
		p.renderer.RespondInternalError(w, r, p.renderer.ErrorContext(r, lang))
	}
}

func (p *Pages) currentLang(r *http.Request) string {
	if p.session != nil {
		if lang := p.session.Language(r.Context()); lang != "" {
			return lang
		}
	}
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return lang
	}
	return "en"
}

func placeholderHTML() template.HTML {
	return template.HTML(`
<div class="bg-white shadow-sm rounded-lg p-6 animate-pulse">
    <div class="h-8 bg-gray-200 rounded w-1/4 mb-4"></div>
    <div class="h-4 bg-gray-200 rounded w-1/2 mb-6"></div>
    <div class="space-y-3">
        <div class="h-10 bg-gray-100 rounded"></div>
        <div class="h-10 bg-gray-100 rounded"></div>
        <div class="h-10 bg-gray-100 rounded"></div>
    </div>
</div>`)
}
