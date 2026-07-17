package integrations

import (
	"github.com/pletka-io/pletka/pkg/integrations/registry"
	"github.com/pletka-io/pletka/pkg/integrations/threem"
)

// CoreIntegrations returns the public core integrations. These are suitable for
// an OSS build and are intentionally independent from hosted platform services.
//
// No init() and no package globals: registration is an explicit constructor
// call every time. Adding a new core integration is done by appending its
// constructor here.
func CoreIntegrations() []registry.Integration {
	return []registry.Integration{
		threem.New(),
	}
}

// BuildRegistry composes the public core integrations with any extras supplied
// by the caller and returns a ready-to-use Registry.
//
// Deprecated: prefer CoreRegistry or a build-surface-specific registry
// constructor at composition boundaries so the intended build surface is visible
// at the call site.
func BuildRegistry(extra ...registry.Integration) (*registry.Registry, error) {
	return registry.NewRegistry(append(CoreIntegrations(), extra...)...)
}

// CoreRegistry returns a registry with only public core integrations.
func CoreRegistry() (*registry.Registry, error) {
	return registry.NewRegistry(CoreIntegrations()...)
}
