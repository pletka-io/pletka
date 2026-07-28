package app

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"github.com/pletka-io/pletka/pkg/assets"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/mergedfs"
	"github.com/pletka-io/pletka/pkg/session"
	"github.com/pletka-io/pletka/pkg/weave/errortracking"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
)

type rootContextKey string

const devModeKey rootContextKey = "dev_mode"

type rootDependencies struct {
	Weave   domain.WeaveStore
	Pool    *pgxpool.Pool
	Logger  *slog.Logger
	Session *session.Manager
	DevMode bool

	LogStaticFiles bool
	AllowedOrigins []string
	StaticAssets   []StaticAssetSet

	// ObsMiddleware instruments requests for Prometheus (nil = no-op).
	ObsMiddleware func(http.Handler) http.Handler

	// Error500 renders the branded 500 page/JSON when a panic is recovered
	// (nil = plain-text "Internal Server Error" fallback).
	Error500 func(http.ResponseWriter, *http.Request)

	// ErrResponder holds the global negotiated-error responder, set once the
	// error-page host is assembled (see app.go). StashMiddleware installs it
	// on every request's context before routes are mounted; the concrete
	// responder can be wired in after Mount without violating chi's
	// no-middleware-after-routes rule.
	ErrResponder *errresp.Holder
}

func buildRootMux(d rootDependencies) *chi.Mux {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Pool == nil {
		d.Logger.Error("database pool is nil - cannot continue")
		panic("app.buildRootMux: database pool is required but was nil")
	}

	r := chi.NewRouter()
	setupGlobalMiddleware(r, d)
	mountStaticAssets(r, d)
	return r
}

func isDevMode(ctx context.Context) bool {
	if isDev, ok := ctx.Value(devModeKey).(bool); ok {
		return isDev
	}
	return false
}

func mountStaticAssets(r *chi.Mux, d rootDependencies) {
	staticFS := mergedfs.Merge(staticAssetFilesystems(d)...)
	staticServer := http.FileServer(http.FS(staticFS))
	r.Route("/static", func(r chi.Router) {
		r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
			http.StripPrefix("/static/", staticServer).ServeHTTP(w, req)
		})
	})

	r.Get("/favicon.ico", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/static/img/favicon.png", http.StatusMovedPermanently)
	})
	r.Get("/robots.txt", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
	})
	r.Get("/sitemap.xml", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
</urlset>
`))
	})
}

func staticAssetFilesystems(d rootDependencies) []fs.FS {
	filesystems := make([]fs.FS, 0, len(d.StaticAssets)+2)
	if d.DevMode {
		filesystems = append(filesystems, os.DirFS("pkg/assets/static"))
	}
	for _, set := range d.StaticAssets {
		filesystems = append(filesystems, set.FS)
	}
	filesystems = append(filesystems, assets.GetStaticFS())
	return filesystems
}

func setupGlobalMiddleware(r *chi.Mux, d rootDependencies) {
	r.Use(chimiddleware.RedirectSlashes)
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	if d.ObsMiddleware != nil {
		r.Use(d.ObsMiddleware) // request metrics; outer so it times the full chain
	}
	r.Use(devModeMiddleware(d.DevMode))
	r.Use(loggerMiddleware(d.Logger, d.DevMode, d.LogStaticFiles))
	r.Use(errortracking.Middleware(errortracking.NewStore(d.Pool), d.Logger))
	r.Use(recoverMiddleware(d.Error500, d.Logger))
	r.Use(securityHeaders)
	r.Use(corsHeaders(d.AllowedOrigins, d.DevMode))

	if d.Session != nil {
		r.Use(d.Session.LoadAndSave)
		r.Use(weaveauth.NewMiddleware(d.Session, d.Weave))
		r.Use(errresp.StashMiddleware(d.ErrResponder))
		r.Use(d.Session.LanguageFromCookie)
		r.Use(d.Session.ParamSync("theme", "page_size", "sort_by", "sort_order"))
		r.Use(d.Session.CSRFProtect())
	}

	// Brute-force protection lives on the auth route group (authRateLimit,
	// wired in app.go), not here — the rest of the app has no login/register
	// surface that needs per-IP throttling.
	r.Use(chimiddleware.Timeout(60 * time.Second))
	r.Use(chimiddleware.Compress(5))
}

func loggerMiddleware(logger *slog.Logger, devMode bool, logStaticFiles bool) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

			var debugInfo map[string]any

			if devMode && logger.Enabled(r.Context(), slog.LevelDebug) {
				debugInfo = requestDebugInfo(r)
			}

			defer func() {
				fullPath := r.URL.Path
				if r.URL.RawQuery != "" {
					fullPath += "?" + r.URL.RawQuery
				}
				if isStaticFileRequest(r.URL.Path) && !logStaticFiles {
					return
				}

				attrs := []slog.Attr{
					slog.String("method", r.Method),
					slog.String("path", fullPath),
					slog.Int("status", ww.Status()),
					slog.Int("size", ww.BytesWritten()),
					slog.Duration("duration", time.Since(start)),
					slog.String("ip", r.RemoteAddr),
					slog.String("user_agent", r.UserAgent()),
					slog.String("request_id", chimiddleware.GetReqID(r.Context())),
				}

				logger.LogAttrs(r.Context(), slog.LevelInfo, "HTTP Request", attrs...)
				if len(debugInfo) > 0 {
					logger.Debug("Request Debug Info",
						"request_id", chimiddleware.GetReqID(r.Context()),
						"debug", debugInfo,
					)
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

func requestDebugInfo(r *http.Request) map[string]any {
	debugInfo := make(map[string]any)
	if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
		bodyBytes, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		if len(bodyBytes) > 0 {
			bodyStr := string(bodyBytes)
			if len(bodyStr) > 1024 {
				bodyStr = bodyStr[:1024] + "... (truncated)"
			}
			debugInfo["body"] = bodyStr
		}
	}

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err == nil && r.MultipartForm != nil {
			fileInfo := make([]map[string]string, 0)
			for fieldName, files := range r.MultipartForm.File {
				for _, fileHeader := range files {
					info := map[string]string{
						"field":    fieldName,
						"filename": fileHeader.Filename,
						"size":     slog.Int64Value(fileHeader.Size).String(),
					}
					if file, err := fileHeader.Open(); err == nil {
						buffer := make([]byte, 512)
						n, _ := file.Read(buffer)
						info["mime_type"] = http.DetectContentType(buffer[:n])
						_ = file.Close()
					}
					fileInfo = append(fileInfo, info)
				}
			}
			if len(fileInfo) > 0 {
				debugInfo["files"] = fileInfo
			}
			if len(r.MultipartForm.Value) > 0 {
				debugInfo["form_values"] = r.MultipartForm.Value
			}
		}
	} else if len(r.Form) > 0 {
		debugInfo["form_values"] = r.Form
	}
	debugInfo["headers"] = r.Header
	return debugInfo
}

func corsHeaders(origins []string, devMode bool) func(http.Handler) http.Handler {
	allowedOrigins := defaultAllowedOrigins(origins, devMode)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				for _, allowed := range allowedOrigins {
					if matchOrigin(origin, allowed) {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						break
					}
				}
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token, X-Request-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func defaultAllowedOrigins(origins []string, devMode bool) []string {
	if len(origins) > 0 {
		return append([]string(nil), origins...)
	}
	if devMode {
		return []string{"http://localhost:*", "http://127.0.0.1:*"}
	}
	return []string{"https://pletka.io"}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// ponytail: in-memory map + global mutex; move to a shared store if
// multi-instance rate coordination is ever needed.
type ipLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func newIPLimiter() *ipLimiter { return &ipLimiter{limiters: map[string]*rate.Limiter{}} }

func (l *ipLimiter) get(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.limiters[ip]
	if !ok {
		lim = rate.NewLimiter(rate.Every(6*time.Second), 5) // ~10/min, burst 5
		l.limiters[ip] = lim
	}
	return lim
}

var authLimiter = newIPLimiter()

// authRateLimit throttles auth routes (login/register) per client IP to
// blunt brute-force and credential-stuffing attempts.
func authRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !authLimiter.get(host).Allow() {
			// Deliberately plain: this fires under brute-force load, and 429 has
			// no canonical envelope code — rendering the branded page/envelope
			// here would add work under exactly the attack it defends against.
			http.Error(w, "too many requests", http.StatusTooManyRequests) //nolint:forbidigo // defensive rate-limit response stays plain
			return
		}
		next.ServeHTTP(w, r)
	})
}

func devModeMiddleware(isDev bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), devModeKey, isDev)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func matchOrigin(origin, pattern string) bool {
	if pattern == origin {
		return true
	}
	// Support a single leading "scheme://*." wildcard: match the exact base
	// domain OR a subdomain of it (any depth), nothing else.
	if i := strings.Index(pattern, "://*."); i != -1 {
		scheme := pattern[:i+3]          // "https://"
		base := pattern[i+len("://*."):] // "pletka.io"
		if origin == scheme+base {       // apex
			return true
		}
		suffix := "." + base
		host := strings.TrimPrefix(origin, scheme)
		return strings.HasPrefix(origin, scheme) &&
			strings.HasSuffix(host, suffix) &&
			!strings.Contains(strings.TrimSuffix(host, suffix), "/")
	}
	// Keep the localhost dev pattern "http://localhost:*" working.
	if strings.HasSuffix(pattern, ":*") {
		return strings.HasPrefix(origin, strings.TrimSuffix(pattern, "*"))
	}
	return false
}

func isStaticFileRequest(path string) bool {
	staticPrefixes := []string{
		"/static/",
		"/assets/",
		"/css/",
		"/js/",
		"/images/",
		"/img/",
		"/favicon.ico",
		"/robots.txt",
		"/manifest.json",
	}
	staticExtensions := []string{
		".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico",
		".woff", ".woff2", ".ttf", ".eot", ".map", ".webp",
	}

	for _, prefix := range staticPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	for _, ext := range staticExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}
