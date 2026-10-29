// Package ontologysnapshot preserves legacy ontology evidence independently of
// verified publications. Neither stored values nor readers have a hot fallback.
package ontologysnapshot

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/namespace"
)

// Origin identifies the project archive used as capture input. It does not
// certify archive completeness or grant permission to read the project.
type Origin struct{ project, version string }

// NewOrigin rejects missing project and release identities.
func NewOrigin(project, version string) (Origin, error) {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(version) == "" {
		return Origin{}, fmt.Errorf("snapshot requires a project and release identity")
	}
	return Origin{project: project, version: version}, nil
}

// Reference binds one exact captured document, including its source contexts.
type Reference struct {
	origin Origin
	digest string
}

// NewReference validates an exact snapshot reference before opening a session.
func NewReference(project, version, digest string) (Reference, error) {
	o, err := NewOrigin(project, version)
	if err != nil {
		return Reference{}, err
	}
	if len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
		return Reference{}, fmt.Errorf("snapshot requires a SHA256 digest")
	}
	return Reference{origin: o, digest: digest}, nil
}

// Artifact retains exact UTF-8 document bytes. It is detached from the reader.
// This evidence format is not a whole-project export or restore contract.
type Artifact struct {
	JSON   string `json:"json"`
	Digest string `json:"digest"`
}

// Body retains a concrete local body and its original project selection row.
// Selection is evidence only; is_primary never resolves an ambiguity.
type Body struct {
	OntologyID string          `json:"ontology_id"`
	VersionID  string          `json:"version_id"`
	Digest     string          `json:"digest"`
	BodyJSON   string          `json:"body_json"`
	Selection  json.RawMessage `json:"selection"`
}

// Element preserves the complete original semantic input, including datatype,
// literal kind, additional types and explicit version selection.
type Element struct {
	EntityKind  string          `json:"entity_kind"`
	EntityID    string          `json:"entity_id"`
	Location    string          `json:"location"`
	DefaultKind string          `json:"default_kind"`
	Original    json.RawMessage `json:"original"`
}

// TermReference is the normalized matching input; Elements retain the originals.
type TermReference struct {
	Kind      string `json:"kind"`
	URI       string `json:"uri"`
	Prefix    string `json:"prefix"`
	LocalName string `json:"local_name"`
	VersionID string `json:"version_id"`
}

// Candidate identifies one definition in one exact retained local body.
type Candidate struct {
	OntologyID string `json:"ontology_id"`
	VersionID  string `json:"version_id"`
	Digest     string `json:"digest"`
	URI        string `json:"uri"`
}

// Status distinguishes semantic evidence from mere content preservation.
type Status string

// Assessment statuses never authorize a fallback to another context or store.
const (
	Resolved  Status = "resolved"
	Ambiguous Status = "ambiguous"
	Missing   Status = "missing"
	Invalid   Status = "invalid"
)

// Assessment preserves one semantic location and its complete candidate set.
type Assessment struct {
	EntityKind string        `json:"entity_kind"`
	EntityID   string        `json:"entity_id"`
	Location   string        `json:"location"`
	Reference  TermReference `json:"reference"`
	Status     Status        `json:"status"`
	Candidates []Candidate   `json:"candidates"`
}

// Dependency binds an imported entity to its retained source project context.
type Dependency struct {
	ProjectID     string `json:"project_id"`
	Version       string `json:"version_number"`
	EntityKind    string `json:"entity_type"`
	EntityID      string `json:"entity_id"`
	SourceProject string `json:"source_project_id"`
	SourceVersion string `json:"source_version"`
}

// Source retains one local interpretation context, without recursively embedding
// its sources. Semantic operations must use the enclosing Artifact's closure.
type Source struct {
	JSON   string `json:"json"`
	Digest string `json:"digest"`
}

// ContextReference pins a source's local context inside the flat closure.
type ContextReference struct {
	ProjectID string `json:"project_id"`
	Version   string `json:"version"`
	Digest    string `json:"digest"`
}

// Document is the independently versioned legacy evidence contract. Sources
// retain their own inventory, bindings, assessments and content identity.
type Document struct {
	FormatVersion int                 `json:"format_version"`
	ProjectID     string              `json:"project_id"`
	Version       string              `json:"version"`
	Inventory     []Body              `json:"inventory"`
	Bindings      []namespace.Binding `json:"bindings"`
	Elements      []Element           `json:"elements"`
	Assessments   []Assessment        `json:"assessments"`
	Sources       []Source            `json:"sources"`
	SourceRefs    []ContextReference  `json:"source_refs"`
	Dependencies  []Dependency        `json:"dependencies"`
}

func hash(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

// Decode checks format, identity, body hashes, source contexts, reference
// coverage and assessments. It does not promote local evidence to publication.
func (a Artifact) Decode() (Document, error) {
	return validateArtifact(a)
}

// Reference returns an exact binding only after validating the complete value.
func (a Artifact) Reference() (Reference, error) {
	d, err := a.Decode()
	if err != nil {
		return Reference{}, err
	}
	return NewReference(d.ProjectID, d.Version, a.Digest)
}

type bodyDocument struct {
	FormatVersion int                 `json:"format_version"`
	Ontology      struct{ ID string } `json:"ontology"`
	Version       struct {
		ID         string
		OntologyID string `json:"ontology_id"`
	} `json:"version"`
	Classes    []term              `json:"classes"`
	Properties []term              `json:"properties"`
	Bindings   []namespace.Binding `json:"namespace_bindings"`
}

type term struct {
	URI       string
	Prefix    string
	LocalName string `json:"local_name"`
}

func decodeBody(b Body) (bodyDocument, error) {
	var d bodyDocument
	sealed := ontologybody.Body{FormatVersion: 1, CanonicalJSON: b.BodyJSON, Digest: b.Digest}
	if err := sealed.Verify(); err != nil {
		return d, err
	}
	if err := json.Unmarshal([]byte(b.BodyJSON), &d); err != nil {
		return d, err
	}
	if d.FormatVersion != 1 || d.Ontology.ID != b.OntologyID || d.Version.ID != b.VersionID || d.Version.OntologyID != b.OntologyID {
		return d, fmt.Errorf("legacy snapshot body identity or format mismatch")
	}
	return d, nil
}
