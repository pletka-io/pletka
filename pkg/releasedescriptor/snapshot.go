// Package releasedescriptor contains detached historical actor and module values.
// It has no database, request or capture capability.
package releasedescriptor

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DefaultModuleHost and DefaultOntologyHost define the default output namespace.
const (
	DefaultModuleHost   = "pletka.io"
	DefaultOntologyHost = "ontology.pletka.io"
)

// Hosts is capture-time configuration, never a released reader's lookup policy.
type Hosts struct{ Project, Ontology string }

// Actor is the historical public descriptor used by manifests and receipts.
type Actor struct {
	ActorID     string `json:"actor_id,omitempty"`
	Type        string `json:"type,omitempty"`
	Slug        string `json:"slug,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// Document is the versioned portable representation. Decode binds and validates it.
type Document struct {
	Format          int               `json:"format"`
	ProjectID       string            `json:"project_id"`
	Version         string            `json:"version"`
	Actors          map[string]Actor  `json:"actors"`
	ProjectModules  map[string]string `json:"project_modules"`
	OntologyModules map[string]string `json:"ontology_modules"`
}

// Snapshot is a detached value bound to one release. Its maps are not exposed.
type Snapshot struct{ doc Document }

// UnavailableError reports missing historical evidence, without live substitution.
type UnavailableError struct{ ProjectID, Version string }

// InvalidError identifies a cut whose required identity evidence is incomplete.
type InvalidError struct{ Message string }

func (e *InvalidError) Error() string { return e.Message }

// ValidationFields maps invalid capture inputs through the canonical API door.
func (e *InvalidError) ValidationFields() map[string][]string {
	return map[string][]string{"descriptors": {e.Message}}
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("release %s@%s has no captured actor/module descriptors", e.ProjectID, e.Version)
}

// IsNotInRelease selects the canonical API error envelope.
func (e *UnavailableError) IsNotInRelease() bool { return true }

// Decode verifies format and ownership at the bound read boundary.
func Decode(data []byte, project, version string) (Snapshot, error) {
	var d Document
	if len(data) == 0 {
		return Snapshot{}, &UnavailableError{project, version}
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return Snapshot{}, fmt.Errorf("decode release descriptors: %w", err)
	}
	if d.Format != 1 || d.ProjectID != project || d.Version != version || d.Actors == nil || d.ProjectModules == nil || d.OntologyModules == nil || d.ProjectModules[project] == "" {
		return Snapshot{}, fmt.Errorf("invalid release descriptor format or ownership")
	}
	for id, actor := range d.Actors {
		if id == "" || actor.ActorID != id {
			return Snapshot{}, fmt.Errorf("invalid captured actor identity")
		}
	}
	return Snapshot{doc: d}, nil
}

// Actor returns a copy; optional absent references remain absent.
func (s Snapshot) Actor(id *string) (*Actor, error) {
	if id == nil || strings.TrimSpace(*id) == "" {
		return nil, nil
	}
	a, ok := s.doc.Actors[strings.TrimSpace(*id)]
	if !ok {
		return nil, fmt.Errorf("actor %s is not in captured release descriptors", *id)
	}
	return &a, nil
}

// Module returns this release's captured module identifier.
func (s Snapshot) Module() string { return s.doc.ProjectModules[s.doc.ProjectID] }

// ProjectModule returns only a captured dependency identifier or the bound owner.
func (s Snapshot) ProjectModule(project string) (string, error) {
	path, ok := s.doc.ProjectModules[project]
	if !ok || path == "" {
		return "", fmt.Errorf("project %s has no captured module identifier", project)
	}
	return path, nil
}

// OntologyModule returns only a captured ontology version's module identifier.
func (s Snapshot) OntologyModule(version string) (string, error) {
	path, ok := s.doc.OntologyModules[version]
	if !ok || path == "" {
		return "", fmt.Errorf("ontology version %s has no captured module identifier", version)
	}
	return path, nil
}

// ProjectModule constructs an identifier from explicit values at capture time.
func ProjectModule(host string, actor Actor, project string) string {
	if host == "" {
		host = DefaultModuleHost
	}
	slug := strings.TrimSpace(actor.Slug)
	if slug == "" {
		slug = actor.ActorID
	}
	kind := "actors"
	switch actor.Type {
	case "organization":
		kind = "orgs"
	case "person":
		kind = "users"
	}
	return fmt.Sprintf("%s/%s/%s/projects/%s", host, kind, slug, project)
}

// OntologySlug preserves the existing serialization contract for ontology paths.
func OntologySlug(prefix, name, fallback string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name != "" {
		name = strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ".", "-", ":", "-").Replace(name)
		if name = strings.Trim(name, "-"); name != "" {
			return name
		}
	}
	if prefix = strings.TrimSpace(prefix); prefix != "" {
		return prefix
	}
	return strings.TrimSpace(fallback)
}

// OntologyModules returns a copy of the captured version-to-module inventory.
func (s Snapshot) OntologyModules() map[string]string {
	out := make(map[string]string, len(s.doc.OntologyModules))
	for id, module := range s.doc.OntologyModules {
		out[id] = module
	}
	return out
}
