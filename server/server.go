// Package server is Pletka's HTTP layer. It owns the standalone HTTP server
// shell, schema rendering, static renderer assets, and route composition.
package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/pletka-io/pletka/server/assets"
	"github.com/pletka-io/pletka/server/templates"
)

// BuildInfo describes the binary that is serving requests.
type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// Config controls the standalone Pletka HTTP server shell.
type Config struct {
	Addr              string
	Logger            *slog.Logger
	BuildInfo         BuildInfo
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// Server owns the public Pletka HTTP routes.
type Server struct {
	addr      string
	logger    *slog.Logger
	buildInfo BuildInfo
	handler   http.Handler
	cssFile   string
	jsFile    string
	timeouts  timeouts
}

type timeouts struct {
	readHeader time.Duration
	read       time.Duration
	write      time.Duration
	idle       time.Duration
}

// New constructs a standalone server. This first migration slice is
// intentionally small: it proves routing, embedded assets, and page-schema
// rendering before the Weave store and vertical slices move in.
func New(cfg Config) (*Server, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	if cfg.Addr == "" {
		cfg.Addr = "localhost:8080"
	}
	t := timeouts{
		readHeader: defaultDuration(cfg.ReadHeaderTimeout, 10*time.Second),
		read:       defaultDuration(cfg.ReadTimeout, 30*time.Second),
		write:      defaultDuration(cfg.WriteTimeout, 30*time.Second),
		idle:       defaultDuration(cfg.IdleTimeout, 120*time.Second),
	}

	dist, err := fs.Sub(assets.Dist, "dist")
	if err != nil {
		return nil, fmt.Errorf("renderer dist fs: %w", err)
	}

	s := &Server{
		addr:      cfg.Addr,
		logger:    logger,
		buildInfo: cfg.BuildInfo,
		cssFile:   assetNameIfExists(dist, "index.css"),
		jsFile:    assetNameIfExists(dist, "index.js"),
		timeouts:  t,
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(dist))))
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("GET /version", s.handleVersion)
	mux.HandleFunc("GET /api/v1/version", s.handleVersion)
	mux.HandleFunc("GET /", s.handleHome)

	s.handler = recoverer(logger, requestLogger(logger, mux))
	return s, nil
}

// Handler returns the configured HTTP handler for tests or embedding.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// HTTPServer returns a configured stdlib server for command wiring.
func (s *Server) HTTPServer() *http.Server {
	return &http.Server{
		Addr:              s.addr,
		Handler:           s.handler,
		ReadHeaderTimeout: s.timeouts.readHeader,
		ReadTimeout:       s.timeouts.read,
		WriteTimeout:      s.timeouts.write,
		IdleTimeout:       s.timeouts.idle,
	}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	s.logger.Info("starting pletka server", "addr", s.addr)
	return s.HTTPServer().ListenAndServe()
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ready": true,
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.buildInfo)
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	schema := map[string]any{
		"schemaVersion": "0.1.0",
		"title":         "Pletka",
		"lang":          "en",
		"body": map[string]any{
			"type": "server-shell",
			"id":   "home",
			"text": "Pletka core server is running.",
		},
		"links": []map[string]string{
			{"rel": "self", "href": "/"},
			{"rel": "version", "href": "/version"},
			{"rel": "health", "href": "/healthz"},
		},
		"meta": map[string]any{
			"version": s.buildInfo,
		},
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		http.Error(w, "failed to encode page schema", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.RenderPage(w, templates.PageData{
		Lang:       "en",
		Title:      "Pletka",
		SchemaJSON: string(raw),
		CSSFile:    s.cssFile,
		JSFile:     s.jsFile,
	}); err != nil {
		s.logger.Error("render page", "err", err)
	}
}

func assetNameIfExists(dist fs.FS, name string) string {
	if _, err := fs.Stat(dist, name); err != nil {
		return ""
	}
	return name
}

func defaultDuration(v, def time.Duration) time.Duration {
	if v == 0 {
		return def
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"duration", time.Since(start),
		)
	})
}

func recoverer(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				logger.Error("request panic", "method", r.Method, "path", r.URL.Path, "panic", v, "stack", string(debug.Stack()))
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}
