package ontologyartifact

import "github.com/pletka-io/pletka/pkg/namespace"

// Namespaces exposes retained body declarations separately from a checked
// effective output map. It never changes the publication bytes or digest.
func (a Artifact) Namespaces() (namespace.Registry, error) {
	c, err := a.Content()
	if err != nil {
		return namespace.Registry{}, err
	}
	return namespace.NewRegistry(c.Bindings), nil
}
