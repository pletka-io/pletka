package session

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/justinas/nosurf"
)

// ParamSync is middleware that syncs URL parameters to session
// It handles common parameters like lang, theme, page_size, sort_by, etc.
func (m *Manager) ParamSync(params ...string) func(http.Handler) http.Handler {
	// Default parameters to sync if none provided
	if len(params) == 0 {
		params = []string{"lang", "theme", "page_size", "sort_by", "sort_order"}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			query := r.URL.Query()

			for _, param := range params {
				if value := query.Get(param); value != "" {
					switch param {
					case "lang", "language":
						m.SetLanguage(ctx, value)
					case "theme":
						m.SetTheme(ctx, value)
					case "page_size", "rows", "per_page":
						if size, err := strconv.Atoi(value); err == nil && size > 0 && size <= 100 {
							m.SetPageSize(ctx, size)
						}
					case "sort_by":
						m.Put(ctx, KeySortBy, value)
					case "sort_order", "order":
						// Normalize to asc/desc
						if strings.ToLower(value) == "desc" {
							m.Put(ctx, KeySortOrder, "desc")
						} else {
							m.Put(ctx, KeySortOrder, "asc")
						}
					default:
						// Store other parameters as-is
						m.Put(ctx, param, value)
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth is middleware that ensures user is authenticated
func (m *Manager) RequireAuth(redirectTo string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !m.IsAuthenticated(r.Context()) {
				// Remember where they were trying to go
				m.RememberReturnTo(r.Context(), r.URL.String())
				http.Redirect(w, r, redirectTo, http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// LoadAndSave provides middleware which automatically loads and saves session
// data for the current request, and communicates the session token to and from
// the client in a cookie.
func (m *Manager) LoadAndSave(next http.Handler) http.Handler {
	return m.SessionManager.LoadAndSave(next)
}

// LanguageFromCookie middleware reads the language from query params or cookie and updates the session
func (m *Manager) LanguageFromCookie(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		currentLang := m.Language(ctx)
		newLang := ""
		source := ""

		// First check query parameter (takes precedence)
		if lang := r.URL.Query().Get("lang"); lang != "" {
			newLang = lang
			source = "query"
			// Also set a cookie for persistence
			http.SetCookie(w, &http.Cookie{
				Name:     "language",
				Value:    lang,
				Path:     "/",
				MaxAge:   31536000, // 1 year
				HttpOnly: false,    // Allow JavaScript to read it for the selector
				SameSite: http.SameSiteLaxMode,
			})
		} else if cookie, err := r.Cookie("language"); err == nil && cookie.Value != "" {
			// Fall back to cookie if no query param
			newLang = cookie.Value
			source = "cookie"
		}

		// Update session if language changed
		if newLang != "" && newLang != currentLang {
			m.SetLanguage(ctx, newLang)
			if m.logger != nil {
				m.logger.Debug("Language updated",
					"previous", currentLang,
					"new", newLang,
					"source", source,
					"path", r.URL.Path,
				)
			}
		}

		// Debug log current language
		if m.logger != nil && m.config.DebugMode {
			finalLang := m.Language(ctx)
			m.logger.Debug("Language middleware result",
				"language", finalLang,
				"path", r.URL.Path,
				"hasQuery", r.URL.Query().Get("lang") != "",
				"hasCookie", func() bool {
					_, err := r.Cookie("language")
					return err == nil
				}(),
			)
		}

		next.ServeHTTP(w, r)
	})
}

// CSRFProtect returns CSRF protection middleware if enabled
func (m *Manager) CSRFProtect() func(http.Handler) http.Handler {
	if !m.config.EnableCSRF {
		// Return no-op middleware if CSRF is disabled
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return m.noSurf()
}

// noSurf creates the CSRF protection middleware with config-based settings
func (m *Manager) noSurf() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		csrfHandler := nosurf.New(next)
		csrfHandler.SetBaseCookie(http.Cookie{
			HttpOnly: true,
			Path:     "/",
			Secure:   m.config.CookieSecure, // Use session config's Secure setting
			SameSite: http.SameSiteLaxMode,
		})

		// Exempt JSON API calls from CSRF. These are called from JavaScript
		// fetch() with explicit Content-Type or X-Requested-With headers.
		// Traditional form POSTs (multipart/form-data, application/x-www-form-urlencoded)
		// still require CSRF tokens.
		csrfHandler.ExemptFunc(func(r *http.Request) bool {
			if r.Method == "GET" || r.Method == "HEAD" {
				return false // safe methods, nosurf skips them anyway
			}

			// Always exempt /api/ routes (pure JSON API)
			if strings.HasPrefix(r.URL.Path, "/api/") {
				return true
			}

			// The MCP endpoint is called by MCP clients, not the browser —
			// there is no session-carried CSRF cookie/token to present.
			// Auth is a bearer API key (auth.RequireAPIKey), checked after
			// this middleware; an unauthenticated POST must reach that
			// check and get a 401 envelope, not a CSRF-layer rejection.
			if r.URL.Path == "/mcp" {
				return true
			}

			// Client error reporter — fire-and-forget telemetry POSTed from
			// JS, often when no CSRF token is available (errors during early
			// load, or with no session). It's rate-limited server-side; without
			// this exemption nosurf 400s the report and the error is lost —
			// which is exactly how the ReuseTab crash stayed invisible.
			if r.URL.Path == "/errors/client" {
				return true
			}

			// The app is schema-driven: every mutation is a JSON (or XHR)
			// request issued by JavaScript, authenticated by session cookie and
			// protected cross-origin by CORS preflight (application/json is a
			// non-simple content type, so a forged cross-site POST can't set
			// it). Exempt those regardless of path. Traditional HTML form posts
			// (login, multipart, x-www-form-urlencoded) carry no JSON/XHR marker
			// and stay CSRF-protected. Previously only /projects/ JSON was
			// exempt, so JSON mutations elsewhere — e.g. POST /profile/orgs
			// (create organization) — were 400'd by nosurf.
			ct := r.Header.Get("Content-Type")
			if strings.HasPrefix(ct, "application/json") {
				return true
			}
			if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
				return true
			}

			return false
		})

		return csrfHandler
	}
}
