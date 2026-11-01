package ontologyevidence

import (
	"fmt"
	"slices"
)

// Preparer shares exact document/body validation within one operation. It owns
// no reader, is not safe for concurrent use, and must not be kept globally.
// Its zero value is ready for use. Only successful complete closures are returned.
type Preparer struct {
	documents map[string]*preparedContext
	bodies    map[string]preparedBody
	artifacts map[string]Prepared
}

type preparedContext struct {
	artifact    Artifact
	document    Document
	checked     bool
	localDigest string
}
type preparedBody struct {
	json     string
	document bodyDocument
}
type preparedArtifact struct {
	artifact Artifact
	root     Origin
	contexts map[Origin]*preparedContext
	resolver Resolver
}

// Prepared is a validated, immutable interpretation capability. It has no
// database/lazy-loading capability and can outlive its preparer and read session.
// All exported collections are independent copies; the zero value fails closed.
type Prepared struct{ value *preparedArtifact }

// Prepare validates hashes, body selections, source closure and every assessment.
// Reuse requires exact original bytes as well as their digest; an edited artifact
// cannot acquire a previously validated capability by retaining its old digest.
func (p *Preparer) Prepare(a Artifact) (Prepared, error) {
	if p == nil {
		return Prepared{}, fmt.Errorf("interpretation preparer is uninitialized")
	}
	if p.artifacts == nil {
		p.artifacts = map[string]Prepared{}
		p.documents = map[string]*preparedContext{}
		p.bodies = map[string]preparedBody{}
	}
	if old, ok := p.artifacts[a.Digest]; ok {
		if old.value.artifact != a {
			return Prepared{}, fmt.Errorf("legacy snapshot digest mismatch")
		}
		return old, nil
	}
	value, err := p.validate(a)
	if err != nil {
		return Prepared{}, err
	}
	p.artifacts[a.Digest] = value
	return value, nil
}

func (p *Preparer) context(a Artifact) (*preparedContext, error) {
	if old, ok := p.documents[a.Digest]; ok {
		if old.artifact != a {
			return nil, fmt.Errorf("legacy snapshot digest mismatch")
		}
		return old, nil
	}
	d, err := decodeDocument(a.JSON, a.Digest)
	if err != nil {
		return nil, err
	}
	value := &preparedContext{artifact: a, document: d}
	p.documents[a.Digest] = value
	return value, nil
}

func (p *Preparer) body(b Body) (bodyDocument, error) {
	if old, ok := p.bodies[b.Digest]; ok {
		if old.json != b.BodyJSON || old.document.Ontology.ID != b.OntologyID || old.document.Version.ID != b.VersionID {
			return bodyDocument{}, fmt.Errorf("legacy snapshot body identity or digest mismatch")
		}
		return old.document, nil
	}
	d, err := decodeBody(b)
	if err != nil {
		return bodyDocument{}, err
	}
	p.bodies[b.Digest] = preparedBody{json: b.BodyJSON, document: d}
	return d, nil
}

// Artifact returns the exact original byte strings, without normalizing evidence.
func (p Prepared) Artifact() (Artifact, error) {
	if p.value == nil {
		return Artifact{}, fmt.Errorf("prepared interpretation is uninitialized")
	}
	return p.value.artifact, nil
}

// Reference binds this capability to its exact project, release and digest.
func (p Prepared) Reference() (Reference, error) {
	if p.value == nil {
		return Reference{}, fmt.Errorf("prepared interpretation is uninitialized")
	}
	return NewReference(p.value.root.project, p.value.root.version, p.value.artifact.Digest)
}

// Resolver reuses the already checked context graph without validation or I/O.
func (p Prepared) Resolver() (Resolver, error) {
	if p.value == nil {
		return Resolver{}, fmt.Errorf("prepared interpretation is uninitialized")
	}
	return p.value.resolver, nil
}

// Document returns independent mutable collections. Large immutable byte strings
// are shared safely; no JSON round trip or revalidation is needed for this copy.
func (p Prepared) Document() (Document, error) {
	if p.value == nil {
		return Document{}, fmt.Errorf("prepared interpretation is uninitialized")
	}
	d := p.value.contexts[p.value.root].document
	d.Inventory = slices.Clone(d.Inventory)
	for i := range d.Inventory {
		d.Inventory[i].Selection = slices.Clone(d.Inventory[i].Selection)
	}
	d.Bindings = slices.Clone(d.Bindings)
	d.Elements = slices.Clone(d.Elements)
	for i := range d.Elements {
		d.Elements[i].Original = slices.Clone(d.Elements[i].Original)
	}
	d.Assessments = slices.Clone(d.Assessments)
	for i := range d.Assessments {
		d.Assessments[i].Candidates = slices.Clone(d.Assessments[i].Candidates)
	}
	d.Sources = slices.Clone(d.Sources)
	d.SourceRefs = slices.Clone(d.SourceRefs)
	d.Dependencies = slices.Clone(d.Dependencies)
	return d, nil
}
