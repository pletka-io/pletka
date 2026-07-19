package authpages

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/session"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

type LangResolver func(*http.Request) string

type Host struct {
	Logger       *slog.Logger
	Templates    *weavetemplates.Renderer
	I18n         i18n.Manager
	Session      *session.Manager
	LangResolver LangResolver

	RegistrationEnabled bool
	SSOLoginURL         string
}

func (h Host) Validate() error {
	var missing []string
	if h.Templates == nil {
		missing = append(missing, "Templates")
	}
	if h.I18n == nil {
		missing = append(missing, "I18n")
	}
	if len(missing) > 0 {
		return fmt.Errorf("authpages host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Handler renders auth pages through the weave shell.
type Handler struct {
	logger   *slog.Logger
	renderer *weavetemplates.Renderer
	i18n     i18n.Manager
	session  *session.Manager
	lang     LangResolver

	registrationEnabled bool
	ssoLoginURL         string
}

func Mount(r chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := &Handler{
		logger:              host.Logger,
		renderer:            host.Templates,
		i18n:                host.I18n,
		session:             host.Session,
		lang:                host.LangResolver,
		registrationEnabled: host.RegistrationEnabled,
		ssoLoginURL:         host.SSOLoginURL,
	}
	h.Mount(r)
}

// Mount registers the site-root auth page routes.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/login", h.LoginPage)
	r.Get("/login/local", h.LocalLoginPage)
	r.Get("/register", h.RegisterPage)
	r.Get("/logout", h.LogoutPage)
}

// LoginPage renders the sign-in form. Already-authenticated users are sent to
// the safe return destination immediately. When SSO is enabled, this renders
// the SSO-only card instead of the password form — LocalLoginPage remains the
// unlinked break-glass path to the password form.
func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	redirectURL := h.safeRedirect(r)
	if auth.PrincipalFromContext(r.Context()) != nil {
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	content := loginHTML(redirectURL, h.registrationEnabled)
	if h.ssoLoginURL != "" {
		content = ssoLoginHTML(h.ssoLoginURL, redirectURL)
	}
	h.render(w, r, renderInput{
		Title:      h.t("auth.login.title", h.currentLang(r), "Sign in"),
		ActivePath: "/login",
		Content:    content,
	})
}

// LocalLoginPage always renders the password form — the unlinked break-glass
// path when SSO is enabled. API-side gating (super-admin only) lives in
// pkg/auth; this page is just the form.
func (h *Handler) LocalLoginPage(w http.ResponseWriter, r *http.Request) {
	redirectURL := h.safeRedirect(r)
	if auth.PrincipalFromContext(r.Context()) != nil {
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	h.render(w, r, renderInput{
		Title:      h.t("auth.login.title", h.currentLang(r), "Sign in"),
		ActivePath: "/login",
		Content:    loginHTML(redirectURL, false), // no register link on break-glass
	})
}

// RegisterPage renders the registration form when registration is enabled.
func (h *Handler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	if !h.registrationEnabled {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if auth.PrincipalFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	h.render(w, r, renderInput{
		Title:      h.t("auth.register.title", h.currentLang(r), "Create account"),
		ActivePath: "/register",
		Content:    registerHTML(),
	})
}

// LogoutPage destroys the current session and redirects to the home page.
func (h *Handler) LogoutPage(w http.ResponseWriter, r *http.Request) {
	if h.session != nil {
		_ = h.session.Destroy(r.Context())
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// ProfilePage renders a small profile page from the weave auth principal.
func (h *Handler) ProfilePage(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		http.Redirect(w, r, "/login?next=/profile", http.StatusFound)
		return
	}

	h.render(w, r, renderInput{
		Title:      "Profile",
		ActivePath: "/profile",
		Content:    profileHTML(principal),
	})
}

type renderInput struct {
	Title      string
	ActivePath string
	Content    template.HTML
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, in renderInput) {
	lang := h.currentLang(r)
	page := weavetemplates.IslandPage{
		Title:       in.Title,
		Lang:        lang,
		Path:        in.ActivePath,
		Languages:   h.i18n.Languages(),
		Principal:   auth.PrincipalFromContext(r.Context()),
		IsAnonymous: auth.FromContext(r.Context()).IsAnonymous,
		Labels:      h.renderer.ShellLabels(lang),
		Island: weavetemplates.IslandMount{
			Placeholder: in.Content,
		},
	}

	if err := h.renderer.RenderIslandPage(w, page); err != nil {
		if h.logger != nil {
			h.logger.Error("render auth page", "path", r.URL.Path, "err", err)
		}
		h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang))
	}
}

func (h *Handler) currentLang(r *http.Request) string {
	if h.lang != nil {
		if lang := h.lang(r); lang != "" {
			return lang
		}
	}
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

func (h *Handler) t(key, lang, fallback string) string {
	if h.i18n == nil {
		return fallback
	}
	if text := h.i18n.T(key, lang); text != "" && text != key {
		return text
	}
	return fallback
}

func (h *Handler) safeRedirect(r *http.Request) string {
	redirectURL := r.URL.Query().Get("next")
	if redirectURL == "" {
		redirectURL = r.URL.Query().Get("redirect")
	}
	if !isSafeReturnPath(redirectURL) {
		redirectURL = ""
	}
	if redirectURL == "" {
		ref := r.Referer()
		if ref != "" {
			if u, err := url.Parse(ref); err == nil && u.Host == r.Host {
				candidate := u.Path
				if u.RawQuery != "" {
					candidate += "?" + u.RawQuery
				}
				// Skip the landing page so it doesn't override the
				// /profile fallback when a user clicks Sign In from "/"
				// (customer landed back on "/" instead of
				// /profile because Referer was the landing page).
				if isSafeReturnPath(candidate) && u.Path != "/" {
					redirectURL = candidate
				}
			}
		}
	}
	if redirectURL == "" {
		// George flagged that the old /dashboard landed on a stale
		// page. /profile is the user workspace overview (memberships
		// + owned/created/collaborating tabs), which matches the
		// dashboard role. Anonymous next= and explicit redirect
		// params still win; this is just the fallback when neither
		// is set.
		return "/profile"
	}
	return redirectURL
}

func isSafeReturnPath(p string) bool {
	if p == "" || !strings.HasPrefix(p, "/") {
		return false
	}
	if strings.HasPrefix(p, "//") {
		return false
	}
	return !strings.HasPrefix(p, "/login") &&
		!strings.HasPrefix(p, "/logout") &&
		!strings.HasPrefix(p, "/register")
}

func jsString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
