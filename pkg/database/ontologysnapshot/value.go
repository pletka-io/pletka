// Package ontologysnapshot captures and reads immutable legacy ontology evidence.
// Detached validation and semantic values live in package ontologyevidence.
package ontologysnapshot

import "github.com/pletka-io/pletka/pkg/ontologyevidence"

// Origin is the portable legacy evidence value.
type Origin = ontologyevidence.Origin

// Reference is the portable legacy evidence value.
type Reference = ontologyevidence.Reference

// Artifact is the portable legacy evidence value.
type Artifact = ontologyevidence.Artifact

// Body is the portable legacy evidence value.
type Body = ontologyevidence.Body

// Element is the portable legacy evidence value.
type Element = ontologyevidence.Element

// TermReference is the portable legacy evidence value.
type TermReference = ontologyevidence.TermReference

// Candidate is the portable legacy evidence value.
type Candidate = ontologyevidence.Candidate

// Status is the portable legacy evidence value.
type Status = ontologyevidence.Status

// Assessment is the portable legacy evidence value.
type Assessment = ontologyevidence.Assessment

// Dependency is the portable legacy evidence value.
type Dependency = ontologyevidence.Dependency

// Source is the portable legacy evidence value.
type Source = ontologyevidence.Source

// ContextReference is the portable legacy evidence value.
type ContextReference = ontologyevidence.ContextReference

// Document is the portable legacy evidence value.
type Document = ontologyevidence.Document

// UnresolvedError is the portable legacy evidence value.
type UnresolvedError = ontologyevidence.UnresolvedError

// Definition is the portable legacy evidence value.
type Definition = ontologyevidence.Definition

// Resolved preserves the corresponding assessment state.
const Resolved = ontologyevidence.Resolved

// Ambiguous preserves the corresponding assessment state.
const Ambiguous = ontologyevidence.Ambiguous

// Missing preserves the corresponding assessment state.
const Missing = ontologyevidence.Missing

// Invalid preserves the corresponding assessment state.
const Invalid = ontologyevidence.Invalid

// NewOrigin validates the project/release input to explicit capture.
func NewOrigin(project, version string) (Origin, error) {
	return ontologyevidence.NewOrigin(project, version)
}

// NewReference validates an exact snapshot binding.
func NewReference(project, version, digest string) (Reference, error) {
	return ontologyevidence.NewReference(project, version, digest)
}

// ValidateBundle validates all detached evidence before any database work.
func ValidateBundle(artifacts []Artifact) error { return ontologyevidence.ValidateBundle(artifacts) }
