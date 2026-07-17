package session

import (
	"net/http"

	"github.com/justinas/nosurf"

	"github.com/pletka-io/pletka/pkg/i18n"
)

// TemplateFuncs returns template functions for accessing session data
// These functions solve the gohtml limitation of needing to pass everything down the chain
func (m *Manager) TemplateFuncs() map[string]any {
	return map[string]any{
		// Session value getters
		"sessionGet": func(r *http.Request, key string) any {
			return m.Get(r.Context(), key)
		},
		
		"sessionString": func(r *http.Request, key, fallback string) string {
			return m.GetStringWithDefault(r.Context(), key, fallback)
		},
		
		"sessionInt": func(r *http.Request, key string, fallback int) int {
			return m.GetIntWithDefault(r.Context(), key, fallback)
		},
		
		"sessionBool": func(r *http.Request, key string) bool {
			return m.GetBool(r.Context(), key)
		},
		
		// Common session values
		"sessionLang": func(r *http.Request) string {
			return m.Language(r.Context())
		},
		
		"sessionTheme": func(r *http.Request) string {
			return m.Theme(r.Context())
		},
		
		"sessionPageSize": func(r *http.Request) int {
			return m.PageSize(r.Context())
		},
		
		"sessionUserID": func(r *http.Request) string {
			return m.UserID(r.Context())
		},
		
		"sessionUserEmail": func(r *http.Request) string {
			return m.UserEmail(r.Context())
		},
		
		"sessionIsAuth": func(r *http.Request) bool {
			return m.IsAuthenticated(r.Context())
		},
		
		"sessionCSRF": func(r *http.Request) string {
			return m.CSRFToken(r.Context())
		},

		// nosurf CSRF token for use in meta tags and fetch headers
		"csrfToken": func(r *http.Request) string {
			return nosurf.Token(r)
		},
		
		// Check if session key exists
		"sessionExists": func(r *http.Request, key string) bool {
			return m.Exists(r.Context(), key)
		},
		
		// Get all session keys (useful for debugging)
		"sessionKeys": func(r *http.Request) []string {
			return m.Keys(r.Context())
		},
	}
}

// ContextTemplateFuncs returns template functions that work with BaseContext
// These are more convenient when you already have BaseContext in your template data
func ContextTemplateFuncs() map[string]any {
	return map[string]any{
		// Get session value from BaseContext
		"ctxSession": func(bc *BaseContext, key string) any {
			if bc.sessionManager == nil {
				return nil
			}
			return bc.sessionManager.Get(bc.Context(), key)
		},
		
		"ctxSessionString": func(bc *BaseContext, key, fallback string) string {
			if bc.sessionManager == nil {
				return fallback
			}
			return bc.sessionManager.GetStringWithDefault(bc.Context(), key, fallback)
		},
		
		"ctxSessionInt": func(bc *BaseContext, key string, fallback int) int {
			if bc.sessionManager == nil {
				return fallback
			}
			return bc.sessionManager.GetIntWithDefault(bc.Context(), key, fallback)
		},
		
		"ctxSessionBool": func(bc *BaseContext, key string) bool {
			if bc.sessionManager == nil {
				return false
			}
			return bc.sessionManager.GetBool(bc.Context(), key)
		},
		
		// URL builders from BaseContext
		"ctxLangURL": func(bc *BaseContext, lang string) string {
			return bc.LangChangeURL(lang)
		},
		
		"ctxThemeURL": func(bc *BaseContext, theme string) string {
			return bc.ThemeChangeURL(theme)
		},
		
		"ctxSortURL": func(bc *BaseContext, field string) string {
			return bc.SortURL(field)
		},
		
		"ctxPageURL": func(bc *BaseContext, page int) string {
			return bc.PageURL(page)
		},
		
		"ctxPageSizeURL": func(bc *BaseContext, size int) string {
			return bc.PageSizeURL(size)
		},
		
		// Path helpers
		"ctxIsPath": func(bc *BaseContext, path string) bool {
			return bc.IsCurrentPath(path)
		},
		
		"ctxIsPathPrefix": func(bc *BaseContext, prefix string) bool {
			return bc.IsCurrentPathPrefix(prefix)
		},
		
		// Query helpers
		"ctxQueryParam": func(bc *BaseContext, key, value string) string {
			return bc.UpdateQueryParam(key, value)
		},
		
		"ctxRemoveParam": func(bc *BaseContext, key string) string {
			return bc.RemoveQueryParam(key)
		},
		
		"ctxAddParam": func(bc *BaseContext, key, value string) string {
			return bc.AddQueryParam(key, value)
		},
	}
}

// RequestContextFunc creates a template function to get BaseContext from request
// This allows templates to access BaseContext when only request is available
func RequestContextFunc(sm *Manager, languages []i18n.Language) func(*http.Request) *BaseContext {
	return func(r *http.Request) *BaseContext {
		return NewBaseContext(r, sm, languages)
	}
}

// Example template usage:
/*

Using session functions directly with request:
{{ $lang := sessionLang .Request }}
{{ $theme := sessionTheme .Request }}
{{ $pageSize := sessionPageSize .Request }}

Using BaseContext functions (when BaseContext is embedded in template data):
{{ $lang := .BaseContext.Language }}
{{ $sortURL := ctxSortURL .BaseContext "name" }}
{{ $isHomePage := ctxIsPath .BaseContext "/" }}

Getting arbitrary session values:
{{ $customValue := sessionString .Request "my_custom_key" "default" }}
{{ if sessionExists .Request "show_advanced" }}
    ... advanced options ...
{{ end }}

Language switcher example:
<select onchange="location.href=this.value">
    {{ range .Languages }}
        <option value="{{ ctxLangURL $.BaseContext .Code }}" 
                {{ if eq .Code $.Language }}selected{{ end }}>
            {{ .Name }}
        </option>
    {{ end }}
</select>

*/