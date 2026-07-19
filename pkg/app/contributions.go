package app

import (
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/app/observability"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/integrations"
	"github.com/pletka-io/pletka/pkg/integrations/registry"
	"github.com/pletka-io/pletka/pkg/session"
	"github.com/pletka-io/pletka/pkg/weave/category"
	"github.com/pletka-io/pletka/pkg/weave/collection"
	"github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/model"
	"github.com/pletka-io/pletka/pkg/weave/namespacebinding"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/project"
	"github.com/pletka-io/pletka/pkg/weave/projectontologyversion"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
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
	// Services exposes core's assembled slice services; see ADR-0008.
	Services *Services
}

// Services is the read-only surface of core's assembled slice services,
// handed to route contributions so hosting binaries can compose features
// (e.g. commercial modules) over them. One instance per slice —
// contributions must never construct duplicates. Nil only in tests that
// bypass full assembly; app.New always populates every field. Additions
// require a composition-catalog entry (see ADR-0008).
type Services struct {
	Weave             domain.WeaveStore
	APIKeys           auth.APIKeyVerifier
	Projects          *project.Service
	ProjectOntologies *projectontologyversion.Service
	Namespaces        *namespacebinding.Service
	Fields            *field.Service
	Models            *model.Service
	Collections       *collection.Service
	Categories        *category.Service
	Ontology          *weaveontology.Service
	Vocabulary        *vocabulary.Service
	Languages         []formschema.LanguageInfo
	Obs               *observability.Runtime
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
