package ids

import (
	"github.com/google/uuid"
)

// UUIDv5 returns an RFC 4122 version 5 UUID derived from namespace and key.
// The output is stable: same inputs always produce the same UUID. Use this
// for deterministic identifiers in generated artefacts (Arches resource
// graphs, RDF instance URIs) where the same Pletka data must produce the
// same external IDs across runs.
func UUIDv5(namespace uuid.UUID, key string) string {
	return uuid.NewSHA1(namespace, []byte(key)).String()
}

// ProjectNamespace derives a stable namespace UUID for a project from its
// canonical namespace URL (e.g. "https://linked.art/ns/ogee"). Generators
// scope their UUIDv5 keys under this namespace so artefacts from different
// projects never collide and the same project always yields the same root
// namespace.
func ProjectNamespace(projectNamespaceURL string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(projectNamespaceURL))
}
