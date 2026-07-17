package assets

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pletka-io/pletka/pkg/frontendmanifest"
)

// ViteManifestEntry represents a single entry in the Vite manifest
type ViteManifestEntry struct {
	File    string   `json:"file"`
	Name    string   `json:"name"`
	Src     string   `json:"src"`
	IsEntry bool     `json:"isEntry"`
	CSS     []string `json:"css,omitempty"`
}

// IslandResolver resolves island names to their hashed JS file paths from the Vite manifest.
type IslandResolver struct {
	mu                sync.RWMutex
	manifest          map[string]ViteManifestEntry
	devMode           bool
	basePath          string // filesystem path to static/dist in dev mode
	staticFilesystems []fs.FS
}

// FrontendManifestValidation configures startup validation for frontend
// contribution manifests. PlatformManifestPaths are the platform/private
// manifests to compose with the core manifest when it is available.
type FrontendManifestValidation struct {
	CoreManifestPath      string
	PlatformManifestPaths []string
	PlatformManifests     []frontendmanifest.Manifest
	ValidateSourceFiles   bool
}

const defaultCoreFrontendManifestPath = "frontend/core.frontend.json"
const embeddedCoreFrontendManifestPath = "embedded:frontend/core.frontend.json"

var frontendManifestEnvNames = []string{
	"PLETKA_FRONTEND_MANIFESTS",
	"PLETKA_FRONTEND_CONTRIBUTIONS",
}

// NewIslandResolver creates a resolver. In devMode it reads from filesystem on every call.
func NewIslandResolver(devMode bool) *IslandResolver {
	return NewIslandResolverWithStaticFS(devMode)
}

// NewIslandResolverWithStaticFS creates a resolver that reads Vite manifests
// from contributed /static filesystems before falling back to the core bundle.
// The order matches the /static overlay: earlier filesystems win.
func NewIslandResolverWithStaticFS(devMode bool, staticFilesystems ...fs.FS) *IslandResolver {
	r := &IslandResolver{
		devMode:           devMode,
		basePath:          "pkg/assets/static/dist",
		staticFilesystems: append([]fs.FS{}, staticFilesystems...),
	}
	if !devMode {
		// In production, load once from embedded FS
		r.loadFromEmbed()
	}
	return r
}

// FrontendManifestPathsFromEnv returns platform/private manifest paths declared
// through the same environment variables used by the Vite build.
func FrontendManifestPathsFromEnv() []string {
	var paths []string
	for _, name := range frontendManifestEnvNames {
		for _, path := range filepath.SplitList(os.Getenv(name)) {
			path = strings.TrimSpace(path)
			if path != "" {
				paths = append(paths, frontendManifestEnvPath(path))
			}
		}
	}
	return paths
}

func frontendManifestEnvPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(filepath.Join("frontend", path))
}

func (r *IslandResolver) loadFromEmbed() {
	r.manifest = make(map[string]ViteManifestEntry)
	for _, staticFS := range r.staticFilesystems {
		data, err := fs.ReadFile(staticFS, "dist/.vite/manifest.json")
		if err == nil {
			r.mergeManifest(data)
			continue
		}
		if !errors.Is(err, fs.ErrNotExist) {
			continue
		}
	}
	if data, err := ReadStaticFile("dist/.vite/manifest.json"); err == nil {
		r.mergeManifest(data)
	}
}

func (r *IslandResolver) loadFromFilesystem() {
	r.manifest = make(map[string]ViteManifestEntry)
	for _, staticFS := range r.staticFilesystems {
		if data, err := fs.ReadFile(staticFS, "dist/.vite/manifest.json"); err == nil {
			r.mergeManifest(data)
		}
	}
	path := filepath.Join(r.basePath, ".vite", "manifest.json")
	if data, err := os.ReadFile(path); err == nil {
		r.mergeManifest(data)
	}
}

func (r *IslandResolver) parseManifest(data []byte) {
	r.manifest = make(map[string]ViteManifestEntry)
	r.mergeManifest(data)
}

func (r *IslandResolver) mergeManifest(data []byte) {
	var raw map[string]ViteManifestEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	if r.manifest == nil {
		r.manifest = make(map[string]ViteManifestEntry)
	}
	for key, entry := range raw {
		if _, exists := r.manifest[key]; exists {
			continue
		}
		r.manifest[key] = entry
	}
}

// resolveEntry looks up a manifest entry by trying multiple key patterns.
// This supports both island entries (src/islands/<name>.ts) and
// global entries (src/global/<name>.ts, src/global/<name>.css).
func (r *IslandResolver) resolveEntry(name string) (ViteManifestEntry, bool) {
	if r.devMode {
		r.mu.Lock()
		r.loadFromFilesystem()
		r.mu.Unlock()
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := []string{
		fmt.Sprintf("src/islands/%s.ts", name),
		fmt.Sprintf("src/global/%s.ts", name),
		fmt.Sprintf("src/global/%s.css", name),
	}
	for _, key := range keys {
		if entry, ok := r.manifest[key]; ok {
			return entry, true
		}
	}
	for _, entry := range r.manifest {
		if entry.Name == name && entry.IsEntry {
			return entry, true
		}
	}
	return ViteManifestEntry{}, false
}

// ValidateFrontendManifests validates frontend contribution manifests against
// the runtime Vite manifest. It is intended for startup checks: manifest syntax,
// duplicate/override rules, optional source-file existence, and global/island
// Vite entry resolution fail before a page renders.
func (r *IslandResolver) ValidateFrontendManifests(cfg FrontendManifestValidation) error {
	manifests, err := loadFrontendManifests(cfg.CoreManifestPath, cfg.PlatformManifests, cfg.PlatformManifestPaths)
	if err != nil {
		return err
	}
	if len(manifests) == 0 {
		return nil
	}

	if _, err := FrontendManifestCatalog(cfg); err != nil {
		return err
	}
	if err := r.validateViteEntries(manifests); err != nil {
		return err
	}
	return nil
}

// FrontendManifestCatalog loads and composes the core and configured
// platform/private frontend manifests. It validates duplicate/override rules
// and optionally source-file existence, but does not check the Vite build.
func FrontendManifestCatalog(cfg FrontendManifestValidation) (*frontendmanifest.Catalog, error) {
	manifests, err := loadFrontendManifests(cfg.CoreManifestPath, cfg.PlatformManifests, cfg.PlatformManifestPaths)
	if err != nil {
		return nil, err
	}
	catalog, err := frontendmanifest.NewCatalog(manifests...)
	if err != nil {
		return nil, err
	}
	if cfg.ValidateSourceFiles {
		if err := catalog.ValidateFiles(); err != nil {
			return nil, err
		}
	}
	return catalog, nil
}

func loadFrontendManifests(corePath string, platformManifests []frontendmanifest.Manifest, platformPaths []string) ([]frontendmanifest.Manifest, error) {
	var manifests []frontendmanifest.Manifest
	manifest, err := loadCoreFrontendManifest(corePath)
	if err != nil {
		return nil, err
	}
	manifests = append(manifests, manifest)
	manifests = append(manifests, platformManifests...)
	for _, path := range platformPaths {
		manifest, err := frontendmanifest.LoadFile(path, false)
		if err != nil {
			return nil, fmt.Errorf("load platform frontend manifest %s: %w", path, err)
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

func loadCoreFrontendManifest(corePath string) (frontendmanifest.Manifest, error) {
	if corePath != "" {
		manifest, err := frontendmanifest.LoadFile(corePath, true)
		if err != nil {
			return frontendmanifest.Manifest{}, fmt.Errorf("load core frontend manifest %s: %w", corePath, err)
		}
		return manifest, nil
	}

	manifest, err := frontendmanifest.LoadFile(defaultCoreFrontendManifestPath, true)
	if err == nil {
		return manifest, nil
	}
	if !os.IsNotExist(err) {
		return frontendmanifest.Manifest{}, fmt.Errorf("load core frontend manifest %s: %w", defaultCoreFrontendManifestPath, err)
	}

	body, err := staticFiles.ReadFile("frontend/core.frontend.json")
	if err != nil {
		return frontendmanifest.Manifest{}, fmt.Errorf("load embedded core frontend manifest: %w", err)
	}
	manifest, err = frontendmanifest.LoadBytes(embeddedCoreFrontendManifestPath, body, true)
	if err != nil {
		return frontendmanifest.Manifest{}, err
	}
	return manifest, nil
}

func (r *IslandResolver) validateViteEntries(manifests []frontendmanifest.Manifest) error {
	var missing []string
	for _, manifest := range manifests {
		for _, entry := range manifest.GlobalEntries {
			if _, ok := r.resolveEntry(entry.Name); !ok {
				missing = append(missing, fmt.Sprintf("%s: globalEntries %q", manifest.Path, entry.Name))
			}
		}
		for _, entry := range manifest.Islands {
			if _, ok := r.resolveEntry(entry.Name); !ok {
				missing = append(missing, fmt.Sprintf("%s: islands %q", manifest.Path, entry.Name))
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("frontend manifest entries missing from Vite build:\n- %s", strings.Join(missing, "\n- "))
	}
	return nil
}

// Resolve returns the hashed JS file path for an entry name.
// Returns empty string if not found.
func (r *IslandResolver) Resolve(island string) string {
	entry, ok := r.resolveEntry(island)
	if !ok {
		return ""
	}
	return "/static/dist/" + entry.File
}

// ResolveCSS returns any CSS file paths associated with an entry.
func (r *IslandResolver) ResolveCSS(island string) []string {
	entry, ok := r.resolveEntry(island)
	if !ok {
		return nil
	}
	paths := make([]string, len(entry.CSS))
	for i, css := range entry.CSS {
		paths[i] = "/static/dist/" + css
	}
	return paths
}

// IslandScriptTags generates HTML script/link tags for loading islands.
// Used as a template function.
func (r *IslandResolver) IslandScriptTags(islands ...string) template.HTML {
	var html string
	for _, island := range islands {
		// CSS from the css array (e.g., Svelte component styles)
		for _, css := range r.ResolveCSS(island) {
			html += fmt.Sprintf(`<link rel="stylesheet" href="%s">`+"\n", css)
		}
		// Main entry file
		if src := r.Resolve(island); src != "" {
			if strings.HasSuffix(src, ".css") {
				// CSS-only entry (e.g., global-css) — emit as <link>
				html += fmt.Sprintf(`<link rel="stylesheet" href="%s">`+"\n", src)
			} else {
				html += fmt.Sprintf(`<script type="module" src="%s"></script>`+"\n", src)
			}
		}
	}
	return template.HTML(html)
}
