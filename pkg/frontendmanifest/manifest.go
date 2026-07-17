package frontendmanifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Kind string

const (
	KindGlobalEntry            Kind = "globalEntries"
	KindIsland                 Kind = "islands"
	KindFormWidget             Kind = "formWidgets"
	KindEntityListRowWidget    Kind = "entityListRowWidgets"
	KindEntityListEditorWidget Kind = "entityListEditorWidgets"
	KindContentWidget          Kind = "contentWidgets"
)

var Kinds = []Kind{
	KindGlobalEntry,
	KindIsland,
	KindFormWidget,
	KindEntityListRowWidget,
	KindEntityListEditorWidget,
	KindContentWidget,
}

type Manifest struct {
	ID                      string  `json:"id,omitempty"`
	Name                    string  `json:"name,omitempty"`
	Description             string  `json:"description,omitempty"`
	GlobalEntries           []Entry `json:"globalEntries,omitempty"`
	Islands                 []Entry `json:"islands,omitempty"`
	FormWidgets             []Entry `json:"formWidgets,omitempty"`
	EntityListRowWidgets    []Entry `json:"entityListRowWidgets,omitempty"`
	EntityListEditorWidgets []Entry `json:"entityListEditorWidgets,omitempty"`
	ContentWidgets          []Entry `json:"contentWidgets,omitempty"`

	Path string `json:"-"`
	Core bool   `json:"-"`
}

type Entry struct {
	Name        string `json:"name"`
	Source      string `json:"source,omitempty"`
	Description string `json:"description,omitempty"`
	Overrides   string `json:"overrides,omitempty"`

	ManifestPath string `json:"-"`
	SourcePath   string `json:"-"`
	Core         bool   `json:"-"`
}

type Catalog struct {
	entries map[Kind]map[string]Entry
}

type Reference struct {
	Kind   Kind
	Name   string
	Source string
}

func LoadFile(path string, core bool) (Manifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	return LoadBytes(path, body, core)
}

func LoadBytes(path string, body []byte, core bool) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	manifest.Path = path
	manifest.Core = core
	return manifest, nil
}

func NewCatalog(manifests ...Manifest) (*Catalog, error) {
	c := &Catalog{entries: make(map[Kind]map[string]Entry, len(Kinds))}
	for _, kind := range Kinds {
		c.entries[kind] = map[string]Entry{}
	}
	for _, manifest := range manifests {
		if err := c.addManifest(manifest); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *Catalog) Has(kind Kind, name string) bool {
	_, ok := c.entries[kind][name]
	return ok
}

func (c *Catalog) Names(kind Kind) []string {
	names := make([]string, 0, len(c.entries[kind]))
	for name := range c.entries[kind] {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *Catalog) ValidateFiles() error {
	for _, kind := range Kinds {
		for _, entry := range c.entries[kind] {
			if strings.HasPrefix(entry.ManifestPath, "embedded:") {
				continue
			}
			if entry.SourcePath == "" {
				return fmt.Errorf("%s %q has no source", kind, entry.Name)
			}
			if _, err := os.Stat(entry.SourcePath); err != nil {
				return fmt.Errorf("%s %q source %s: %w", kind, entry.Name, entry.SourcePath, err)
			}
		}
	}
	return nil
}

func (c *Catalog) ValidateReferences(refs []Reference) error {
	var missing []string
	for _, ref := range refs {
		if ref.Name == "" {
			continue
		}
		if !c.Has(ref.Kind, ref.Name) {
			if ref.Source != "" {
				missing = append(missing, fmt.Sprintf("%s %q from %s", ref.Kind, ref.Name, ref.Source))
			} else {
				missing = append(missing, fmt.Sprintf("%s %q", ref.Kind, ref.Name))
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("frontend manifest missing references:\n- %s", strings.Join(missing, "\n- "))
	}
	return nil
}

func (c *Catalog) addManifest(manifest Manifest) error {
	for _, kind := range Kinds {
		for _, entry := range manifest.entries(kind) {
			if entry.Name == "" {
				return fmt.Errorf("%s: %s entry has empty name", manifest.Path, kind)
			}
			entry.ManifestPath = manifest.Path
			entry.SourcePath = resolveSource(manifest.Path, entry.Source)
			entry.Core = manifest.Core
			existing, exists := c.entries[kind][entry.Name]
			if exists {
				if existing.Core && !entry.Core && entry.Overrides == "core" {
					c.entries[kind][entry.Name] = entry
					continue
				}
				msg := fmt.Sprintf("%s: duplicate %s %q", manifest.Path, kind, entry.Name)
				if existing.Core && !entry.Core {
					msg += `; add "overrides": "core" to replace the core entry intentionally`
				}
				return errors.New(msg)
			}
			c.entries[kind][entry.Name] = entry
		}
	}
	return nil
}

func (m Manifest) entries(kind Kind) []Entry {
	switch kind {
	case KindGlobalEntry:
		return m.GlobalEntries
	case KindIsland:
		return m.Islands
	case KindFormWidget:
		return m.FormWidgets
	case KindEntityListRowWidget:
		return m.EntityListRowWidgets
	case KindEntityListEditorWidget:
		return m.EntityListEditorWidgets
	case KindContentWidget:
		return m.ContentWidgets
	default:
		return nil
	}
}

func resolveSource(manifestPath, source string) string {
	if source == "" {
		return ""
	}
	if filepath.IsAbs(source) {
		return source
	}
	return filepath.Clean(filepath.Join(filepath.Dir(manifestPath), source))
}
