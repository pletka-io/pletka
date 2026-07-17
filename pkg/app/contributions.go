package app

import (
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/integrations"
	"github.com/pletka-io/pletka/pkg/integrations/registry"
	"github.com/pletka-io/pletka/pkg/session"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// Host is the explicit app-level contract passed to host route contributions.
// Add fields only when a contribution truly needs them.
type Host struct {
	Pool                *pgxpool.Pool
	Logger              *slog.Logger
	Templates           *weavetemplates.Renderer
	I18n                i18n.Manager
	Session             *session.Manager
	IntegrationRegistry *registry.Registry
	IntegrationCipher   *integrations.Cipher
}

// RouteContribution lets a host add routes around the core app without routing
// through a broad app dependency aggregate.
type RouteContribution struct {
	ID    string
	Mount func(parent chi.Router, host Host)
}

// Validate checks the minimum fields needed to mount a route contribution.
func (c RouteContribution) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("route contribution id is required")
	}
	if c.Mount == nil {
		return fmt.Errorf("route contribution %q mount function is required", c.ID)
	}
	return nil
}

// StaticAssetSet contributes files to the app-wide /static asset overlay.
// Earlier sets take precedence over later sets; all contributed sets take
// precedence over the core embedded static assets.
type StaticAssetSet struct {
	ID string
	FS fs.FS
}

func (s StaticAssetSet) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("static asset set id is required")
	}
	if s.FS == nil {
		return fmt.Errorf("static asset set %q filesystem is required", s.ID)
	}
	return nil
}

// FrontendManifestSet contributes a schema-driven frontend registry manifest
// from an embedded or otherwise virtual filesystem. Use this with the matching
// StaticAssetSet when a host owns islands, widgets, or global frontend entries.
type FrontendManifestSet struct {
	ID   string
	FS   fs.FS
	Path string
}

func (s FrontendManifestSet) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("frontend manifest set id is required")
	}
	if s.FS == nil {
		return fmt.Errorf("frontend manifest set %q filesystem is required", s.ID)
	}
	if s.Path == "" {
		return fmt.Errorf("frontend manifest set %q path is required", s.ID)
	}
	return nil
}

// Contributions are host-provided app surfaces. Registration and activation are
// separate concerns; this value only states what this app instance should mount.
type Contributions struct {
	AdminSections     []formschema.AdminSection
	Routes            []RouteContribution
	StaticAssets      []StaticAssetSet
	FrontendManifests []FrontendManifestSet
}

func (c Contributions) validate() error {
	for _, route := range c.Routes {
		if err := route.Validate(); err != nil {
			return err
		}
	}
	for _, assets := range c.StaticAssets {
		if err := assets.Validate(); err != nil {
			return err
		}
	}
	for _, manifest := range c.FrontendManifests {
		if err := manifest.Validate(); err != nil {
			return err
		}
	}
	return nil
}
