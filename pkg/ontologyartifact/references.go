package ontologyartifact

// TermReference preserves the semantic input at one archived JSON location.
// Kind is class (including datatypes) or property. VersionID is optional input;
// the enclosing TermResolution always carries the concrete publication identity.
type TermReference struct {
	Kind      string `json:"kind"`
	URI       string `json:"uri"`
	Prefix    string `json:"prefix"`
	LocalName string `json:"local_name"`
	VersionID string `json:"version_id"`
}

// TermResolution is detached release evidence. Foreign entities keep their
// source project's inventory; consumers must not re-resolve them in their scope.
type TermResolution struct {
	EntityKind string        `json:"entity_kind"`
	EntityID   string        `json:"entity_id"`
	Location   string        `json:"location"`
	Reference  TermReference `json:"reference"`
	OntologyID string        `json:"ontology_id"`
	VersionID  string        `json:"version_id"`
	Digest     string        `json:"digest"`
	URI        string        `json:"uri"`
}
