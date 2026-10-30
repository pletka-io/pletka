// Package releasedescriptor captures identities inside the project release transaction.
package releasedescriptor

import values "github.com/pletka-io/pletka/pkg/releasedescriptor"

// Hosts provides capture-time configuration. Portable value consumers should
// import pkg/releasedescriptor directly.
type Hosts = values.Hosts

// Actor is a captured public actor descriptor.
type Actor = values.Actor

// Snapshot is a detached release value with private maps.
type Snapshot = values.Snapshot

// UnavailableError reports missing historical identity evidence.
type UnavailableError = values.UnavailableError

// InvalidError reports incomplete capture inputs.
type InvalidError = values.InvalidError
type document = values.Document

// DefaultModuleHost and DefaultOntologyHost preserve the capture defaults.
const (
	DefaultModuleHost   = values.DefaultModuleHost
	DefaultOntologyHost = values.DefaultOntologyHost
)

// Decode validates portable descriptors against a bound project release.
func Decode(data []byte, project, version string) (Snapshot, error) {
	return values.Decode(data, project, version)
}

// ProjectModule constructs an identifier from explicit capture inputs.
func ProjectModule(host string, actor Actor, project string) string {
	return values.ProjectModule(host, actor, project)
}

// OntologySlug preserves canonical ontology path naming.
func OntologySlug(prefix, name, fallback string) string {
	return values.OntologySlug(prefix, name, fallback)
}
