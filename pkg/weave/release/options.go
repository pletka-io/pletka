package release

import "github.com/pletka-io/pletka/pkg/database/releasedescriptor"

// Option configures release creation at the application boundary.
type Option func(*Service)

// WithModuleHosts freezes the same identifiers that the host's materializer uses.
func WithModuleHosts(projectHost, ontologyHost string) Option {
	return func(s *Service) { s.hosts = releasedescriptor.Hosts{Project: projectHost, Ontology: ontologyHost} }
}
