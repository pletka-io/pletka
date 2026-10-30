// Package ontologyrelease publishes complete ontology artifacts. Current policy
// is consulted only by Capture; bound readers see retained publications only.
package ontologyrelease

import (
	"crypto/sha256"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/ontologyartifact"
)

// Reference binds an ontology owner and stable version ID, never a version label.
type Reference struct{ body ontologybody.Reference }

// NewReference rejects zero identities before construction of a read capability.
func NewReference(owner ontologybody.OntologyID, version ontologybody.VersionID) (Reference, error) {
	b, err := ontologybody.NewReference(owner, version)
	return Reference{body: b}, err
}

// Pin retains a concrete publication identity. New value consumers should
// import pkg/ontologyartifact directly.
type Pin = ontologyartifact.Pin

// Requirement records a classified publication dependency.
type Requirement = ontologyartifact.Requirement

// Document is the exact portable publication envelope.
type Document = ontologyartifact.Document

// Artifact retains publication bytes and their digest.
type Artifact = ontologyartifact.Artifact
type bodyDocument = ontologyartifact.Body
type bodyTerm = ontologyartifact.Term

func digest(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

func decodeBody(s string) (bodyDocument, error) { return ontologyartifact.DecodeBody(s) }

type policyError struct{ message string }

func (e *policyError) Error() string { return e.message }
func (e *policyError) ValidationFields() map[string][]string {
	return map[string][]string{"publication": {e.message}}
}
func invalid(format string, args ...any) error {
	return &policyError{message: fmt.Sprintf(format, args...)}
}

type missingError struct{ owner, version string }

func (e *missingError) Error() string {
	return fmt.Sprintf("ontology %s version %s is not published", e.owner, e.version)
}
func (e *missingError) IsNotInRelease() bool { return true }
