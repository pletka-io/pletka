package session

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
)

// Manager wraps scs.SessionManager with additional functionality
type Manager struct {
	*scs.SessionManager
	config Config
	logger *slog.Logger
}

// Config holds session configuration
type Config struct {
	// Session lifetime (absolute cap; scs requires one).
	Lifetime time.Duration
	// IdleTimeout expires a session after this much inactivity; each request
	// renews the window. Sliding, so an active user is not logged out
	// mid-work. Zero disables idle expiry (absolute Lifetime only).
	IdleTimeout time.Duration

	// Cookie settings
	CookieName     string
	CookieDomain   string
	CookiePath     string
	CookieSecure   bool
	CookieHTTPOnly bool
	CookieSameSite http.SameSite
	CookiePersist  bool
	
	// Default values
	DefaultLanguage string
	DefaultPageSize int
	
	// Feature flags
	EnableCSRF bool
	DebugMode  bool
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		Lifetime:        30 * 24 * time.Hour, // absolute backstop
		IdleTimeout:     8 * time.Hour,       // sliding: active users stay in
		CookieName:      "session",
		CookiePath:      "/",
		CookieSecure:    true,
		CookieHTTPOnly:  true,
		CookieSameSite:  http.SameSiteLaxMode,
		CookiePersist:   true,
		DefaultLanguage: "en",
		DefaultPageSize: 20,
		EnableCSRF:      true,
		DebugMode:       false,
	}
}

// New creates a new session manager with the given configuration
func New(cfg Config) *Manager {
	sm := scs.New()
	
	// Configure session
	sm.Lifetime = cfg.Lifetime
	sm.IdleTimeout = cfg.IdleTimeout
	sm.Cookie.Name = cfg.CookieName
	sm.Cookie.Domain = cfg.CookieDomain
	sm.Cookie.Path = cfg.CookiePath
	sm.Cookie.Secure = cfg.CookieSecure
	sm.Cookie.HttpOnly = cfg.CookieHTTPOnly
	sm.Cookie.SameSite = cfg.CookieSameSite
	sm.Cookie.Persist = cfg.CookiePersist

	return &Manager{
		SessionManager: sm,
		config:        cfg,
		logger:        slog.Default(), // Use default logger
	}
}

// NewWithLogger creates a new session manager with the given configuration and logger
func NewWithLogger(cfg Config, logger *slog.Logger) *Manager {
	sm := scs.New()
	
	// Configure session
	sm.Lifetime = cfg.Lifetime
	sm.IdleTimeout = cfg.IdleTimeout
	sm.Cookie.Name = cfg.CookieName
	sm.Cookie.Domain = cfg.CookieDomain
	sm.Cookie.Path = cfg.CookiePath
	sm.Cookie.Secure = cfg.CookieSecure
	sm.Cookie.HttpOnly = cfg.CookieHTTPOnly
	sm.Cookie.SameSite = cfg.CookieSameSite
	sm.Cookie.Persist = cfg.CookiePersist

	return &Manager{
		SessionManager: sm,
		config:        cfg,
		logger:        logger,
	}
}

// NewManager creates a new session manager with default configuration
// This is a convenience function for simple usage
func NewManager(logger interface{}) *Manager {
	cfg := DefaultConfig()
	// In development mode, don't require secure cookies
	cfg.CookieSecure = false
	cfg.DebugMode = true // Enable debug mode for development
	
	// Try to use the provided logger if it's *slog.Logger
	if slogger, ok := logger.(*slog.Logger); ok {
		return NewWithLogger(cfg, slogger)
	}
	return New(cfg)
}

// Common session keys
const (
	KeyLanguage      = "language"
	KeyPageSize      = "page_size" 
	KeySortBy        = "sort_by"
	KeySortOrder     = "sort_order"
	KeyFlash         = "flash"
	KeyFlashError    = "flash_error"
	KeyFlashWarning  = "flash_warning"
	KeyFlashInfo     = "flash_info"
	KeyUserID        = "user_id"
	KeyUserEmail     = "user_email"
	KeyIsAuthenticated = "is_authenticated"
	KeyCSRFToken     = "csrf_token"
	KeyReturnTo      = "return_to"
	KeyTheme         = "theme"
)

// GetString retrieves a string value from session
func (m *Manager) GetString(ctx context.Context, key string) string {
	return m.SessionManager.GetString(ctx, key)
}

// GetStringWithDefault retrieves a string value from session with fallback
func (m *Manager) GetStringWithDefault(ctx context.Context, key string, fallback string) string {
	val := m.SessionManager.GetString(ctx, key)
	if val == "" {
		return fallback
	}
	return val
}

// GetInt retrieves an int value from session with fallback
func (m *Manager) GetInt(ctx context.Context, key string) int {
	return m.SessionManager.GetInt(ctx, key)
}

// GetIntWithDefault retrieves an int value from session with fallback
func (m *Manager) GetIntWithDefault(ctx context.Context, key string, fallback int) int {
	val := m.SessionManager.GetInt(ctx, key)
	if val == 0 && !m.SessionManager.Exists(ctx, key) {
		return fallback
	}
	return val
}

// GetBoolDefault retrieves a bool value from session with default
func (m *Manager) GetBoolDefault(ctx context.Context, key string, defaultVal bool) bool {
	if !m.SessionManager.Exists(ctx, key) {
		m.SessionManager.Put(ctx, key, defaultVal)
		return defaultVal
	}
	return m.SessionManager.GetBool(ctx, key)
}


func (m *Manager) FlashError(ctx context.Context, message string) {
	m.SessionManager.Put(ctx, KeyFlashError, message)
}

func (m *Manager) FlashWarning(ctx context.Context, message string) {
	m.SessionManager.Put(ctx, KeyFlashWarning, message)
}

func (m *Manager) FlashInfo(ctx context.Context, message string) {
	m.SessionManager.Put(ctx, KeyFlashInfo, message)
}

// PopFlash retrieves and removes flash messages
func (m *Manager) PopFlash(ctx context.Context) (flash, flashError, flashWarning, flashInfo string) {
	flash = m.SessionManager.PopString(ctx, KeyFlash)
	flashError = m.SessionManager.PopString(ctx, KeyFlashError)
	flashWarning = m.SessionManager.PopString(ctx, KeyFlashWarning)
	flashInfo = m.SessionManager.PopString(ctx, KeyFlashInfo)
	return
}

// IsAuthenticated checks if user is authenticated
func (m *Manager) IsAuthenticated(ctx context.Context) bool {
	return m.SessionManager.GetBool(ctx, KeyIsAuthenticated)
}

// EstablishAuthenticatedSession marks the session authenticated for the given
// actor — the single sequence every login path (password, SSO) must use, so
// no caller can half-set a session. Rotates the session token against fixation.
func (m *Manager) EstablishAuthenticatedSession(ctx context.Context, actorID, email string) error {
	m.Put(ctx, KeyUserID, actorID)
	m.Put(ctx, KeyUserEmail, email)
	m.Put(ctx, KeyIsAuthenticated, true)
	return m.RenewToken(ctx)
}

// SetAuthenticated sets authentication status and user info
func (m *Manager) SetAuthenticated(ctx context.Context, userID, email string) {
	m.SessionManager.Put(ctx, KeyIsAuthenticated, true)
	m.SessionManager.Put(ctx, KeyUserID, userID)
	m.SessionManager.Put(ctx, KeyUserEmail, email)
	m.SessionManager.RenewToken(ctx) // Security: rotate session ID on login
}

// ClearAuthentication removes authentication data
func (m *Manager) ClearAuthentication(ctx context.Context) {
	m.SessionManager.Remove(ctx, KeyIsAuthenticated)
	m.SessionManager.Remove(ctx, KeyUserID)
	m.SessionManager.Remove(ctx, KeyUserEmail)
	m.SessionManager.RenewToken(ctx) // Security: rotate session ID on logout
}

// UserID returns the authenticated user's ID
func (m *Manager) UserID(ctx context.Context) string {
	return m.SessionManager.GetString(ctx, KeyUserID)
}

// UserEmail returns the authenticated user's email
func (m *Manager) UserEmail(ctx context.Context) string {
	return m.SessionManager.GetString(ctx, KeyUserEmail)
}

// CSRFToken returns the CSRF token for the current session
func (m *Manager) CSRFToken(ctx context.Context) string {
	// For now, return a placeholder token
	// In production, this should integrate with a proper CSRF library
	token := m.SessionManager.GetString(ctx, KeyCSRFToken)
	if token == "" {
		token = "csrf_token_placeholder"
		m.SessionManager.Put(ctx, KeyCSRFToken, token)
	}
	return token
}

// ValidateCSRF validates the provided CSRF token
func (m *Manager) ValidateCSRF(ctx context.Context, token string) bool {
	sessionToken := m.SessionManager.GetString(ctx, KeyCSRFToken)
	return sessionToken != "" && sessionToken == token
}

// AddFlash adds a flash message to the session
func (m *Manager) AddFlash(ctx context.Context, key string, value string) {
	m.SessionManager.Put(ctx, key, value)
}

// Flash retrieves and removes a flash message from the session
func (m *Manager) Flash(ctx context.Context, key string) string {
	return m.SessionManager.PopString(ctx, key)
}

// Language returns the user's language preference
func (m *Manager) Language(ctx context.Context) string {
	return m.GetStringWithDefault(ctx, KeyLanguage, m.config.DefaultLanguage)
}

// SetLanguage sets the user's language preference
func (m *Manager) SetLanguage(ctx context.Context, lang string) {
	m.SessionManager.Put(ctx, KeyLanguage, lang)
}

// PageSize returns the user's preferred page size
func (m *Manager) PageSize(ctx context.Context) int {
	return m.GetIntWithDefault(ctx, KeyPageSize, m.config.DefaultPageSize)
}

// SetPageSize sets the user's preferred page size
func (m *Manager) SetPageSize(ctx context.Context, size int) {
	m.SessionManager.Put(ctx, KeyPageSize, size)
}

// Remember last visited URL (useful for login redirects)
func (m *Manager) RememberReturnTo(ctx context.Context, url string) {
	m.SessionManager.Put(ctx, KeyReturnTo, url)
}

// GetReturnTo retrieves and removes the return URL
func (m *Manager) PopReturnTo(ctx context.Context, fallback string) string {
	url := m.SessionManager.PopString(ctx, KeyReturnTo)
	if url == "" {
		return fallback
	}
	return url
}

// Theme management
func (m *Manager) Theme(ctx context.Context) string {
	return m.GetStringWithDefault(ctx, KeyTheme, "light")
}

func (m *Manager) SetTheme(ctx context.Context, theme string) {
	m.SessionManager.Put(ctx, KeyTheme, theme)
}