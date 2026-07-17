package templates

import (
	"bytes"
	"html/template"
	"net/http"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// ErrorPageDeps carries the per-request shell context needed to render
// any styled error page (404, 405, 403, 500). Resolved by the chi
// dispatcher (or any handler that wants to render an in-shell error)
// so each call site stays a thin invocation.
type ErrorPageDeps struct {
	Lang      string
	Languages []i18n.Language
	Principal *auth.Principal
	Labels    ShellLabels
}

// ErrorPageContent describes the body of one error page. Status code
// goes on the response; Code is the stylised badge ("404", "405",
// etc.) shown above the heading.
type ErrorPageContent struct {
	StatusCode  int
	Code        string
	Heading     string
	Body        string
	RequestPath string
	Links       []ErrorLink
}

// ErrorLink is one CTA on a generic error page. Primary lights up the
// current primary button style; the rest render as outline buttons.
type ErrorLink struct {
	Label   string
	Href    string
	Primary bool
}

// errorBodyView is the data shape passed to the error_body template
// fragment. Same field names as ErrorPageContent so the template stays
// agnostic to which error it's rendering.
type errorBodyView struct {
	Code        string
	Heading     string
	Body        string
	RequestPath string
	Links       []ErrorLink
}

// RenderErrorPage renders any error status with the shared weave
// shell. Status is set BEFORE the template streams bytes — Go's
// ResponseWriter only respects WriteHeader if it precedes the first
// body byte, and ExecuteTemplate writes as it goes.
//
// Use the dedicated helpers (RenderNotFound / RenderMethodNotAllowed)
// when the defaults fit; reach for this directly when a handler needs
// a custom heading/body for a specific surface.
func (r *Renderer) RenderErrorPage(w http.ResponseWriter, req *http.Request, deps ErrorPageDeps, content ErrorPageContent) {
	body := r.errorBody(content)
	page := IslandPage{
		Title:     content.Heading,
		Lang:      deps.Lang,
		Path:      req.URL.Path,
		Languages: deps.Languages,
		Principal: deps.Principal,
		Labels:    deps.Labels,
		Island: IslandMount{
			// No island — pure server-rendered. The shell template
			// falls through to render Placeholder when Island.Name
			// is empty.
			Placeholder: body,
		},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(content.StatusCode)

	view := pageView{
		IslandPage: page,
		PropAttrs:  toPropAttrs(page.Island.Props),
	}
	_ = r.tmpl.ExecuteTemplate(w, "layout", view) // best-effort: headers already sent, write failure isn't actionable
}

// RenderNotFound writes a 404 with the friendly default body — "page
// not found" + Go-home / Browse-projects CTAs. For custom 404 copy on
// a specific surface (e.g. "project not found, choose another") call
// RenderErrorPage directly.
func (r *Renderer) RenderNotFound(w http.ResponseWriter, req *http.Request, deps ErrorPageDeps) {
	r.RenderErrorPage(w, req, deps, ErrorPageContent{
		StatusCode: http.StatusNotFound,
		Code:       "404",
		Heading:    r.t(deps.Lang, "errors.not_found.heading", "Page not found"),
		Body: r.t(deps.Lang, "errors.not_found.body",
			"The page you were looking for doesn't exist or has moved. Try one of the links below."),
		RequestPath: req.URL.Path,
		Links:       defaultErrorLinks(r, deps.Lang),
	})
}

// RenderMethodNotAllowed writes a 405 with the request's actual method
// surfaced in the body — helps curators diagnose "I clicked something
// wrong" vs a real bug. Same default CTAs as 404.
func (r *Renderer) RenderMethodNotAllowed(w http.ResponseWriter, req *http.Request, deps ErrorPageDeps) {
	body := r.t(deps.Lang, "errors.method_not_allowed.body",
		"This page doesn't accept that kind of request. The link or form you followed may be out of date.")
	if req.Method != "" {
		body = body + " (" + req.Method + ")"
	}
	r.RenderErrorPage(w, req, deps, ErrorPageContent{
		StatusCode:  http.StatusMethodNotAllowed,
		Code:        "405",
		Heading:     r.t(deps.Lang, "errors.method_not_allowed.heading", "Method not allowed"),
		Body:        body,
		RequestPath: req.URL.Path,
		Links:       defaultErrorLinks(r, deps.Lang),
	})
}

// RenderForbidden writes a 403 — the caller is authenticated (or
// anonymous) but lacks the capability the resource requires. CTAs
// branch on auth state: anon viewers get a Login link with a `next=`
// return path; signed-in users get the same home/projects links as
// 404. Body stays vague — leaking exactly which capability is
// missing helps attackers map the surface.
//
// Note: not all forbidden cases should reach this page. Project-scope
// surfaces 404 instead of 403 to avoid leaking project existence
// (see settings handler). Use 403 for surfaces where the resource is
// already known to exist (org pages, public ontologies, profile
// pages of other users).
func (r *Renderer) RenderForbidden(w http.ResponseWriter, req *http.Request, deps ErrorPageDeps) {
	r.RenderErrorPage(w, req, deps, ErrorPageContent{
		StatusCode: http.StatusForbidden,
		Code:       "403",
		Heading:    r.t(deps.Lang, "errors.forbidden.heading", "Access denied"),
		Body: r.t(deps.Lang, "errors.forbidden.body",
			"You don't have permission to view this page. Sign in with an account that has access, or head back to a public area."),
		RequestPath: req.URL.Path,
		Links:       forbiddenLinks(r, deps),
	})
}

// RenderInternalError writes a 500 — something blew up server-side.
// Body is intentionally generic; the actual error must already be
// logged by the calling handler (this renderer doesn't see the
// original). The page exists so curators get the styled shell
// instead of "Internal Server Error" plain text.
func (r *Renderer) RenderInternalError(w http.ResponseWriter, req *http.Request, deps ErrorPageDeps) {
	r.RenderErrorPage(w, req, deps, ErrorPageContent{
		StatusCode: http.StatusInternalServerError,
		Code:       "500",
		Heading:    r.t(deps.Lang, "errors.internal.heading", "Something went wrong"),
		Body: r.t(deps.Lang, "errors.internal.body",
			"An unexpected error occurred. The team has been notified — please try again in a moment, or head back to a known-good page."),
		RequestPath: req.URL.Path,
		Links:       defaultErrorLinks(r, deps.Lang),
	})
}

// defaultErrorLinks builds the standard CTA pair (home, projects)
// used by 404 + 405 + 500. Custom error surfaces can build their own.
func defaultErrorLinks(r *Renderer, lang string) []ErrorLink {
	return []ErrorLink{
		{
			Label:   r.t(lang, "errors.actions.home", "Go home"),
			Href:    "/",
			Primary: true,
		},
		{
			Label: r.t(lang, "errors.actions.projects", "Browse projects"),
			Href:  "/projects",
		},
	}
}

// forbiddenLinks surfaces a Login link for anonymous viewers (with
// next= preserved so login bounces them back to the gated surface)
// and falls through to the default home/projects links for signed-in
// users hitting a capability gap.
func forbiddenLinks(r *Renderer, deps ErrorPageDeps) []ErrorLink {
	if deps.Principal == nil {
		// Anonymous — surface the login affordance first.
		return []ErrorLink{
			{
				Label:   r.t(deps.Lang, "errors.actions.login", "Sign in"),
				Href:    "/login", // base.gohtml's data-login-link handler injects next= on click for HTML pages; this is the static fallback
				Primary: true,
			},
			{
				Label: r.t(deps.Lang, "errors.actions.home", "Go home"),
				Href:  "/",
			},
		}
	}
	return defaultErrorLinks(r, deps.Lang)
}

// errorBody renders the error_body template fragment to a
// template.HTML the shell can drop into Island.Placeholder. Falls back
// to a minimal escaped string on template failure so the visitor still
// gets the shell with a useful message.
func (r *Renderer) errorBody(content ErrorPageContent) template.HTML {
	view := errorBodyView{
		Code:        content.Code,
		Heading:     content.Heading,
		Body:        content.Body,
		RequestPath: content.RequestPath,
		Links:       content.Links,
	}
	var buf bytes.Buffer
	if err := r.tmpl.ExecuteTemplate(&buf, "error_body", view); err != nil {
		return template.HTML(template.HTMLEscapeString(view.Heading + " — " + view.Body))
	}
	return template.HTML(buf.String())
}

// t looks up an i18n key, falling back to a hard-coded English
// default when the key is missing or the manager is nil. Keeps the
// renderer usable in tests and in early-boot paths where the i18n
// manager isn't yet attached.
func (r *Renderer) t(lang, key, fallback string) string {
	if r.i18n == nil {
		return fallback
	}
	got := r.i18n.T(key, lang)
	if got == "" || got == key {
		return fallback
	}
	return got
}

// ShellLabels builds the per-language shell-nav labels using the
// renderer's i18n manager. Handlers call this once per render rather
// than maintaining their own copy of the same key set. Returns an
// empty struct when i18n isn't wired (test harness, early boot).
func (r *Renderer) ShellLabels(lang string) ShellLabels {
	tr := func(key string) string {
		if r.i18n == nil {
			return ""
		}
		return r.i18n.T(key, lang)
	}
	return ShellLabels{
		NavOrgs:       tr("nav.orgs"),
		NavProjects:   tr("nav.projects"),
		NavOntologies: tr("nav.ontologies"),
		NavAbout:      tr("nav.about"),
		NavVision:     tr("nav.vision"),
		NavCommunity:  tr("nav.community"),
		NavLogin:      tr("nav.login"),
		NavLogout:     tr("nav.logout"),
		FooterTagline: tr("footer.made_with_love"),
	}
}

// ErrorContext builds an ErrorPageDeps from a request + language.
// Convenience wrapper handlers use to get a one-liner Respond* call.
//
// Usage:
//
//	if err := load(...); err != nil {
//	    h.logger.Error("load X", "err", err)
//	    h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang))
//	    return
//	}
func (r *Renderer) ErrorContext(req *http.Request, lang string) ErrorPageDeps {
	deps := ErrorPageDeps{
		Lang:      lang,
		Principal: auth.PrincipalFromContext(req.Context()),
		Labels:    r.ShellLabels(lang),
	}
	if r.i18n != nil {
		deps.Languages = r.i18n.Languages()
	}
	return deps
}

// RespondNotFound triages by Accept/path and writes either an
// apierror.Error JSON 404 or a styled in-shell HTML 404. Handlers
// call this instead of http.Error(w, "...", 404) when they want
// consistent error UX across the app.
func (r *Renderer) RespondNotFound(w http.ResponseWriter, req *http.Request, deps ErrorPageDeps) {
	if WantsJSON(req) {
		apierror.Write(w, apierror.NotFound(""))
		return
	}
	r.RenderNotFound(w, req, deps)
}

// RespondForbidden — same triage for 403.
func (r *Renderer) RespondForbidden(w http.ResponseWriter, req *http.Request, deps ErrorPageDeps) {
	if WantsJSON(req) {
		apierror.Write(w, apierror.Forbidden(""))
		return
	}
	r.RenderForbidden(w, req, deps)
}

// RespondInternalError — same triage for 500.
func (r *Renderer) RespondInternalError(w http.ResponseWriter, req *http.Request, deps ErrorPageDeps) {
	if WantsJSON(req) {
		apierror.Write(w, apierror.Internal())
		return
	}
	r.RenderInternalError(w, req, deps)
}

// WantsJSON reports whether the request is best answered with a JSON
// error rather than an HTML error page. Used by the global error
// dispatchers to keep API surfaces machine-readable.
//
// Detection rules (any one match → JSON):
//   - URL path is under /api/.
//   - Path ends in -schema, /schema, or /pane (settings/pane data).
//   - Accept header contains application/json AND does NOT prefer
//     text/html (avoids forcing JSON on browsers, which send both).
func WantsJSON(r *http.Request) bool {
	p := r.URL.Path
	if strings.HasPrefix(p, "/api/") {
		return true
	}
	if strings.HasSuffix(p, "-schema") ||
		strings.HasSuffix(p, "/schema") ||
		strings.HasSuffix(p, "/pane") {
		return true
	}
	accept := r.Header.Get("Accept")
	if accept == "" {
		return false
	}
	if strings.Contains(accept, "text/html") {
		return false
	}
	return strings.Contains(accept, "application/json")
}
