// Package ontologyartifact contains exact, detached ontology publication values.
// Decoding and validation require no database or current selection policy.
package ontologyartifact

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/namespace"
)

// Pin retains the content identity of one member of the dependency closure.
type Pin struct {
	OntologyID string `json:"ontology_id"`
	VersionID  string `json:"version_id"`
	Digest     string `json:"digest"`
}

// Requirement records how a direct dependency was classified. Kind is base,
// defaults, import, explicit, or reference; disposition is pinned, external,
// local, absent, or self. External references never authorize network access.
type Requirement struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Disposition string `json:"disposition"`
	OntologyID  string `json:"ontology_id,omitempty"`
}

// Document is format 1. BodyJSON preserves the exact sealed UTF-8 bytes, rather
// than requiring another database or serializer to reproduce JSONB formatting.
// Pins and requirements are sorted. Policy and publication timestamps are absent.
type Document struct {
	FormatVersion int           `json:"format_version"`
	OntologyID    string        `json:"ontology_id"`
	VersionID     string        `json:"version_id"`
	BodyJSON      string        `json:"body_json"`
	BodyDigest    string        `json:"body_digest"`
	Pins          []Pin         `json:"pins"`
	Requirements  []Requirement `json:"requirements"`
}

// Artifact is an immutable value when stored. Copies can escape reader callbacks.
// Preserve JSON verbatim when transporting it; Digest hashes its UTF-8 bytes.
type Artifact struct {
	JSON   string
	Digest string
}

func digest(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

// Decode verifies both layers of content identity and the supported format.
func (a Artifact) Decode() (Document, error) {
	var d Document
	if digest(a.JSON) != a.Digest {
		return d, fmt.Errorf("ontology artifact digest mismatch")
	}
	if err := json.Unmarshal([]byte(a.JSON), &d); err != nil {
		return d, err
	}
	if d.FormatVersion != 1 || strings.TrimSpace(d.OntologyID) == "" || strings.TrimSpace(d.VersionID) == "" || digest(d.BodyJSON) != d.BodyDigest {
		return d, fmt.Errorf("ontology artifact format, identity or body digest mismatch")
	}
	b, err := DecodeBody(d.BodyJSON)
	if err != nil {
		return d, err
	}
	if b.FormatVersion != 1 || b.Ontology.ID != d.OntologyID || b.Version.ID != d.VersionID || b.Version.OntologyID != d.OntologyID {
		return d, fmt.Errorf("ontology artifact body ownership mismatch")
	}
	previous := ""
	for _, p := range d.Pins {
		if p.OntologyID <= previous || p.OntologyID == d.OntologyID || p.VersionID == "" || len(p.Digest) != 64 {
			return d, fmt.Errorf("ontology artifact contains invalid, conflicting or unordered pins")
		}
		previous = p.OntologyID
	}
	return d, nil
}

// Body is the typed publication-policy projection of sealed body JSON.
// Document.BodyJSON remains the complete, exact portable body.
type Body struct {
	FormatVersion int `json:"format_version"`
	Ontology      struct {
		ID      string
		Prefix  string
		Extends string `json:"extends_ontology_id"`
	} `json:"ontology"`
	Version struct {
		ID            string
		OntologyID    string   `json:"ontology_id"`
		VersionString string   `json:"version_string"`
		Compatible    []string `json:"compatible_base_versions"`
		Imports       []string `json:"imported_ontologies"`
	} `json:"version"`
	Classes    []Term `json:"classes"`
	Properties []Term `json:"properties"`
	Relations  []struct {
		Target string `json:"target_qname"`
	} `json:"relations"`
	Bindings []namespace.Binding `json:"namespace_bindings"`
}

// Term is the semantic identity used by publication validation.
type Term struct {
	URI       string `json:"uri"`
	Prefix    string
	LocalName string `json:"local_name"`
	Inverse   string `json:"inverse_property_uri"`
}

// DecodeBody parses a body projection; it does not certify publication.
// Read consumers should use Artifact.Decode or Artifact.Content instead.
func DecodeBody(s string) (Body, error) {
	var b Body
	err := json.Unmarshal([]byte(s), &b)
	return b, err
}
