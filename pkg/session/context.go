package session

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/i18n"
)

// BaseContext provides common request context that can be embedded in template data structs
type BaseContext struct {
	// Request information
	Request         *http.Request `json:"-"`
	RequestID       string        `json:"request_id"`
	Method          string        `json:"method"`
	Path            string        `json:"path"`
	Host            string        `json:"host"`
	Scheme          string        `json:"scheme"`
	
	// Session information
	SessionID       string        `json:"session_id"`
	Language        string        `json:"language"`
	Languages       []i18n.Language `json:"languages"`
	Theme           string        `json:"theme"`
	
	// User information
	IsAuthenticated bool          `json:"is_authenticated"`
	UserID          string        `json:"user_id,omitempty"`
	UserEmail       string        `json:"user_email,omitempty"`
	
	// CSRF protection
	CSRFToken       string        `json:"csrf_token,omitempty"`
	
	// Flash messages
	Flash           string        `json:"flash,omitempty"`
	FlashError      string        `json:"flash_error,omitempty"`
	FlashWarning    string        `json:"flash_warning,omitempty"`
	FlashInfo       string        `json:"flash_info,omitempty"`
	
	// Preferences
	PageSize        int           `json:"page_size"`
	SortBy          string        `json:"sort_by,omitempty"`
	SortOrder       string        `json:"sort_order,omitempty"`
	
	// Debug/Development
	DevMode         bool          `json:"dev_mode"`
	RenderTime      time.Duration `json:"render_time,omitempty"`
	
	// Private fields for internal use
	sessionManager  *Manager      `json:"-"`
	startTime       time.Time     `json:"-"`
}

// NewBaseContext creates a new base context from the request
func NewBaseContext(r *http.Request, sm *Manager, availableLanguages []i18n.Language) *BaseContext {
	ctx := r.Context()
	
	// Determine scheme
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// Check common reverse proxy headers
	if r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	
	// Get flash messages
	flash, flashError, flashWarning, flashInfo := sm.PopFlash(ctx)
	
	// Get language and log it for debugging
	lang := sm.Language(ctx)
	if sm.logger != nil && sm.config.DebugMode {
		sm.logger.Debug("BaseContext language from session",
			"language", lang,
			"path", r.URL.Path,
		)
	}
	
	isAuth := sm.IsAuthenticated(ctx)
	userID := sm.UserID(ctx)

	// Debug log authentication status
	if sm.logger != nil && sm.config.DebugMode {
		sm.logger.Debug("NewBaseContext authentication check",
			"path", r.URL.Path,
			"is_authenticated", isAuth,
			"user_id", userID,
		)
	}

	bc := &BaseContext{
		Request:         r,
		RequestID:       r.Header.Get("X-Request-ID"),
		Method:          r.Method,
		Path:            r.URL.Path,
		Host:            r.Host,
		Scheme:          scheme,
		SessionID:       sm.Token(ctx),
		Language:        lang,
		Languages:       availableLanguages,
		Theme:           sm.Theme(ctx),
		IsAuthenticated: isAuth,
		UserID:          userID,
		UserEmail:       sm.UserEmail(ctx),
		CSRFToken:       sm.CSRFToken(ctx),
		Flash:           flash,
		FlashError:      flashError,
		FlashWarning:    flashWarning,
		FlashInfo:       flashInfo,
		PageSize:        sm.PageSize(ctx),
		SortBy:          sm.GetStringWithDefault(ctx, KeySortBy, ""),
		SortOrder:       sm.GetStringWithDefault(ctx, KeySortOrder, "asc"),
		DevMode:         sm.config.DebugMode,
		sessionManager:  sm,
		startTime:       time.Now(),
	}
	
	return bc
}

// StartTimer starts the render timer (call in deferred function)
func (bc *BaseContext) StartTimer() func() {
	bc.startTime = time.Now()
	return func() {
		bc.RenderTime = time.Since(bc.startTime)
	}
}

// URL returns the full URL for the current request
func (bc *BaseContext) URL() string {
	return fmt.Sprintf("%s://%s%s", bc.Scheme, bc.Host, bc.Request.URL.String())
}

// BaseURL returns the base URL without path
func (bc *BaseContext) BaseURL() string {
	return fmt.Sprintf("%s://%s", bc.Scheme, bc.Host)
}

// UpdateQueryParam returns current URL with updated query parameter
func (bc *BaseContext) UpdateQueryParam(key, value string) string {
	u, _ := url.Parse(bc.Request.URL.String())
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String()
}

// RemoveQueryParam returns current URL without specified query parameter
func (bc *BaseContext) RemoveQueryParam(key string) string {
	u, _ := url.Parse(bc.Request.URL.String())
	q := u.Query()
	q.Del(key)
	u.RawQuery = q.Encode()
	return u.String()
}

// AddQueryParam returns current URL with additional query parameter
func (bc *BaseContext) AddQueryParam(key, value string) string {
	u, _ := url.Parse(bc.Request.URL.String())
	q := u.Query()
	q.Add(key, value)
	u.RawQuery = q.Encode()
	return u.String()
}

// LangChangeURL returns URL to change language
func (bc *BaseContext) LangChangeURL(lang string) string {
	return bc.UpdateQueryParam("lang", lang)
}

// ThemeChangeURL returns URL to change theme
func (bc *BaseContext) ThemeChangeURL(theme string) string {
	return bc.UpdateQueryParam("theme", theme)
}

// SortURL returns URL with sort parameters
func (bc *BaseContext) SortURL(field string) string {
	u, _ := url.Parse(bc.Request.URL.String())
	q := u.Query()
	
	// Toggle sort order if clicking same field
	if bc.SortBy == field {
		if bc.SortOrder == "asc" {
			q.Set("sort_order", "desc")
		} else {
			q.Set("sort_order", "asc")
		}
	} else {
		q.Set("sort_order", "asc")
	}
	
	q.Set("sort_by", field)
	u.RawQuery = q.Encode()
	return u.String()
}

// PageURL returns URL for specific page number
func (bc *BaseContext) PageURL(page int) string {
	return bc.UpdateQueryParam("page", fmt.Sprintf("%d", page))
}

// PageSizeURL returns URL with different page size
func (bc *BaseContext) PageSizeURL(size int) string {
	u, _ := url.Parse(bc.Request.URL.String())
	q := u.Query()
	q.Set("page_size", fmt.Sprintf("%d", size))
	q.Del("page") // Reset to first page when changing size
	u.RawQuery = q.Encode()
	return u.String()
}

// IsCurrentPath checks if the given path matches current path
func (bc *BaseContext) IsCurrentPath(path string) bool {
	return bc.Path == path
}

// IsCurrentPathPrefix checks if current path starts with given prefix
func (bc *BaseContext) IsCurrentPathPrefix(prefix string) bool {
	return strings.HasPrefix(bc.Path, prefix)
}

// HasFlash returns true if any flash messages are present
func (bc *BaseContext) HasFlash() bool {
	return bc.Flash != "" || bc.FlashError != "" || bc.FlashWarning != "" || bc.FlashInfo != ""
}

// Session returns the session manager (for template functions)
func (bc *BaseContext) Session() *Manager {
	return bc.sessionManager
}

// Context returns the request context
func (bc *BaseContext) Context() context.Context {
	return bc.Request.Context()
}