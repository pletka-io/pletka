package content

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/go-chi/chi/v5"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// Host is the narrow app contract content pages need to render markdown-backed
// schemas through the weave shell.
type Host struct {
	Logger       *slog.Logger
	Templates    *weavetemplates.Renderer
	I18n         i18n.Manager
	LangResolver func(*http.Request) string
	OverlayPath  string
}

func (h Host) Validate() error {
	if h.Templates == nil {
		return fmt.Errorf("content host missing templates renderer")
	}
	return nil
}

func (h Host) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

// Handler serves content pages assembled from markdown sources.
// Routes register at boot with one chi.Get per discovered slug.
//
// The schema is JSON-encoded into a single data-prop on the
// "content" island; the frontend dispatcher walks the blocks and
// renders each via the matching widget. Header / footer / build
// badge come from the weave shell unchanged.
type Handler struct {
	renderer *weavetemplates.Renderer
	loader   *Loader
	i18n     i18n.Manager
	lang     func(*http.Request) string
	logger   *slog.Logger

	mu      sync.RWMutex
	pages   map[string]map[string]*PageSchema // slug → lang → schema
	sources []ContentSource
}

// NewHandler builds a handler. Refresh runs once at construction so
// the route registry can ask for the slug list immediately.
func NewHandler(host Host, sources []ContentSource) (*Handler, error) {
	if err := host.Validate(); err != nil {
		return nil, err
	}
	h := &Handler{
		renderer: host.Templates,
		loader:   NewLoader(host.I18n),
		i18n:     host.I18n,
		lang:     host.LangResolver,
		logger:   host.logger(),
		sources:  sources,
	}
	if err := h.Refresh(context.Background()); err != nil {
		return nil, err
	}
	return h, nil
}

// Refresh re-loads every source. Called at boot and on demand for
// content edits during dev (?refresh=1).
func (h *Handler) Refresh(ctx context.Context) error {
	pages, err := h.loader.LoadFromSources(ctx, h.sources)
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.pages = pages
	h.mu.Unlock()
	return nil
}

// Slugs returns every registered slug. Used by the route registrar
// to decide which chi paths to mount.
func (h *Handler) Slugs() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]string, 0, len(h.pages))
	for slug := range h.pages {
		out = append(out, slug)
	}
	return out
}

// Serve renders the page at this slug for the request's language.
// Falls back to the default language when the requested one is not
// available.
func (h *Handler) Serve(slug string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := "en"
		if h.lang != nil {
			lang = h.lang(r)
		}
		schema := h.lookup(slug, lang)
		if schema == nil {
			errresp.Error(w, r, http.StatusNotFound, "not_found", "not found")
			return
		}

		schemaJSON, err := json.Marshal(schema)
		if err != nil {
			h.logger.Error("content: marshal schema", "err", err, "slug", slug)
			h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang))
			return
		}

		page := weavetemplates.IslandPage{
			Title:       schema.SEO.Title,
			Lang:        lang,
			Path:        r.URL.Path,
			Languages:   languagesFromManager(h.i18n),
			Principal:   weaveauth.PrincipalFromContext(r.Context()),
			IsAnonymous: weaveauth.FromContext(r.Context()).IsAnonymous,
			Labels:      h.renderer.ShellLabels(lang),
			Subheading:  schema.SEO.Description,
			Island: weavetemplates.IslandMount{
				Name: frontendrefs.Island("content"),
				Props: map[string]string{
					"schema": string(schemaJSON),
				},
				Dependencies: []string{frontendrefs.Island("content")},
			},
		}

		if err := h.renderer.RenderIslandPage(w, page); err != nil {
			h.logger.Error("content: render", "err", err, "slug", slug)
		}
	}
}

func (h *Handler) lookup(slug, lang string) *PageSchema {
	h.mu.RLock()
	defer h.mu.RUnlock()
	bySlug, ok := h.pages[slug]
	if !ok {
		return nil
	}
	if s, ok := bySlug[lang]; ok {
		return s
	}
	if s, ok := bySlug["en"]; ok {
		return s
	}
	for _, s := range bySlug {
		return s
	}
	return nil
}

// Mount registers content routes from the given sources on parent.
// Slug "home" maps to "/"; every other slug maps to "/<slug>".
//
// Auth-related GET pages (/login, /register, /logout, /profile) are
// owned by pkg/weave/authpages and render through the weave shell.
//
// Sources:
//   - Embed (always): pkg/weave/content/pages/<slug>.<lang>.md
//   - Overlay (when set): Host.OverlayPath — files at
//     `<path>/pages/<slug>.<lang>.md`.
//     Overlay slugs shadow embed slugs; overlay-only slugs add new
//     routes at boot.
func Mount(parent chi.Router, host Host) error {
	if err := host.Validate(); err != nil {
		return err
	}
	logger := host.logger()
	sources, err := ConfiguredSources(logger, host.OverlayPath)
	if err != nil {
		return err
	}
	return MountWithSources(parent, host, sources)
}

// MountWithSources registers content routes with an explicit source set. App
// assembly uses this after validating those same sources against the frontend
// manifest catalog.
func MountWithSources(parent chi.Router, host Host, sources []ContentSource) error {
	if err := host.Validate(); err != nil {
		return err
	}
	h, err := NewHandler(host, sources)
	if err != nil {
		return fmt.Errorf("content: handler: %w", err)
	}

	for _, slug := range h.Slugs() {
		path := "/" + slug
		if slug == "home" {
			path = "/"
		}
		parent.Get(path, h.Serve(slug))
	}

	return nil
}

// ConfiguredSources returns the content sources the app will load at startup.
// It always includes the embedded core pages and appends the optional overlay
// directory configured by the caller.
func ConfiguredSources(logger *slog.Logger, overlayPath string) ([]ContentSource, error) {
	sources := []ContentSource{NewEmbedSource()}
	if overlayPath != "" {
		info, err := os.Stat(overlayPath)
		if err != nil {
			return nil, fmt.Errorf("content: overlay path not usable %q: %w", overlayPath, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("content: overlay path not usable %q: not a directory", overlayPath)
		}
		sources = append(sources, NewOverlaySource(os.DirFS(overlayPath)))
		if logger != nil {
			logger.Info("content: overlay enabled", "path", overlayPath)
		}
	}
	return sources, nil
}

// languagesFromManager exposes the configured languages to the shell
// so the language selector knows what to offer.
func languagesFromManager(mgr i18n.Manager) []i18n.Language {
	if mgr == nil {
		return nil
	}
	return mgr.Languages()
}
