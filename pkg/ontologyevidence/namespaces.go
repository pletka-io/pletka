package ontologyevidence

import "github.com/pletka-io/pletka/pkg/namespace"

// Namespaces retains the root context's declarations, without promoting tied
// aliases to an effective map or merging independently interpreted sources.
// Semantic resolution of imported entities still goes through Resolve.
func (a Artifact) Namespaces() (namespace.Registry, error) {
	d, err := a.Decode()
	if err != nil {
		return namespace.Registry{}, err
	}
	return namespace.NewRegistry(d.Bindings), nil
}
