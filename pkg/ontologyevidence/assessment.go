package ontologyevidence

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/namespace"
)

const classKind = "class"
const propertyKind = "property"

func assess(ref TermReference, inventory []Body, bodies []bodyDocument, aliases []namespace.Binding) (Status, []Candidate) {
	matches := make([]Candidate, 0)
	native := make([]Candidate, 0, 1) // matches declared under the owner's own prefix
	if invalidReference(ref, inventory) {
		return Invalid, matches
	}
	for i, body := range bodies {
		all, own := bodyMatches(ref, inventory[i], body, aliases)
		matches = append(matches, all...)
		native = append(native, own...)
	}
	// An extension may redeclare a parent's term under the parent's URI (Linked
	// Art 0.9 carries six CRM classes). The same URI is the same term, so when
	// every match shares one URI the owner declaring it under its own prefix is
	// the identity; foreign copies alone, or differing URIs, stay ambiguous.
	if len(matches) > 1 && len(native) == 1 && sameURI(matches) {
		matches = native
	}
	slices.SortFunc(matches, func(a, b Candidate) int {
		if c := strings.Compare(a.VersionID, b.VersionID); c != 0 {
			return c
		}
		return strings.Compare(a.URI, b.URI)
	})
	switch len(matches) {
	case 0:
		return Missing, matches
	case 1:
		return Resolved, matches
	default:
		return Ambiguous, matches
	}
}

// Matching is existential over the complete alias registry. Inspect both
// immutable slices without copying or collapsing conflicting namespaces.
func matchesTerm(reference string, t term, project, body []namespace.Binding) bool {
	return namespace.MatchesTerm(reference, t.URI, t.Prefix, t.LocalName, project) ||
		namespace.MatchesTerm(reference, t.URI, t.Prefix, t.LocalName, body)
}

func invalidReference(ref TermReference, inventory []Body) bool {
	return ref.Kind != classKind && ref.Kind != propertyKind ||
		(ref.Prefix == "") != (ref.LocalName == "") ||
		ref.URI == "" && ref.Prefix == "" ||
		ref.VersionID != "" && !slices.ContainsFunc(inventory, func(b Body) bool { return b.VersionID == ref.VersionID })
}

type original struct {
	Type            string            `json:"type"`
	URI             string            `json:"uri"`
	Prefix          string            `json:"prefix"`
	LocalName       string            `json:"local_name"`
	VersionID       string            `json:"ontology_version_id"`
	AdditionalTypes []json.RawMessage `json:"additional_types"`
	Datatype        string            `json:"datatype"`
}

func reference(o original, kind string) TermReference {
	return TermReference{Kind: kind, URI: o.URI, Prefix: o.Prefix, LocalName: o.LocalName, VersionID: o.VersionID}
}

func elementReferences(e Element) ([]Assessment, error) {
	var o original
	if err := json.Unmarshal(e.Original, &o); err != nil {
		return nil, fmt.Errorf("invalid semantic element %s: %w", e.Location, err)
	}
	refs := make([]Assessment, 0, 2+len(o.AdditionalTypes))
	add := func(location string, r TermReference) {
		refs = append(refs, Assessment{EntityKind: e.EntityKind, EntityID: e.EntityID, Location: location, Reference: r})
	}
	if strings.ToLower(o.Type) != "complete" && (o.Type != "" || o.URI != "" || o.Prefix != "" || o.LocalName != "" || o.VersionID != "") {
		kind := o.Type
		if kind == "" {
			kind = e.DefaultKind
		}
		kind = strings.ToLower(kind)
		if kind == "literal" {
			kind = classKind
		}
		add(e.Location, reference(o, kind))
	}
	for i, raw := range o.AdditionalTypes {
		var additional original
		if err := json.Unmarshal(raw, &additional); err != nil {
			return nil, err
		}
		add(fmt.Sprintf("%s/additional_types/%d", e.Location, i), reference(additional, classKind))
	}
	if o.Datatype != "" {
		add(e.Location+"/datatype", TermReference{Kind: classKind, URI: o.Datatype})
	}
	return refs, nil
}

// UnresolvedError retains the actual assessment; callers must not retry against
// live policy or treat ambiguous candidates as equivalent definitions.
type UnresolvedError struct{ Status Status }

func (e *UnresolvedError) Error() string { return "legacy semantic reference is " + string(e.Status) }

// Definition is a checked semantic capability. Its zero value is unusable.
type Definition struct {
	candidate Candidate
	body      string
}

// Evidence returns the exact definition identity and sealed body after a unique
// match. It does not assert that the ontology's own dependency closure is valid.
func (d Definition) Evidence() (Candidate, string, error) {
	if d.body == "" {
		return Candidate{}, "", fmt.Errorf("unbound semantic definition")
	}
	return d.candidate, d.body, nil
}

// Resolve routes imported entities through their recorded source contexts.
// Callers cannot accidentally interpret them using the importing inventory.
func (a Artifact) Resolve(entityKind, entityID, location string) (Definition, error) {
	r, err := a.Resolver()
	if err != nil {
		return Definition{}, err
	}
	return r.Resolve(entityKind, entityID, location)
}

// Resolver holds validated, private interpretation contexts for a batch of
// lookups. It has no reader or mutable exported buffers; its zero value fails.
type Resolver struct {
	root     Origin
	contexts map[Origin]Document
}

// Resolver validates the entire artifact once, before any definition is served.
// It does not promote unresolved assessments or change source context routing.
func (a Artifact) Resolver() (Resolver, error) {
	var preparer Preparer
	p, err := preparer.Prepare(a)
	if err != nil {
		return Resolver{}, err
	}
	return p.Resolver()
}

// Resolve uses the already validated context graph and preserves exact
// resolved/missing/ambiguous/invalid outcomes for each location.
func (r Resolver) Resolve(entityKind, entityID, location string) (Definition, error) {
	d, ok := r.contexts[r.root]
	if !ok {
		return Definition{}, fmt.Errorf("semantic resolver is uninitialized")
	}
	// Decode has already verified every edge and ruled out cycles.
	for {
		found := false
		for _, dep := range d.Dependencies {
			if dep.EntityKind == entityKind && dep.EntityID == entityID {
				d = r.contexts[Origin{project: dep.SourceProject, version: dep.SourceVersion}]
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	for _, r := range d.Assessments {
		if r.EntityKind != entityKind || r.EntityID != entityID || r.Location != location {
			continue
		}
		if r.Status != Resolved {
			return Definition{}, &UnresolvedError{Status: r.Status}
		}
		c := r.Candidates[0]
		for _, b := range d.Inventory {
			if b.VersionID == c.VersionID {
				return Definition{candidate: c, body: b.BodyJSON}, nil
			}
		}
	}
	return Definition{}, &UnresolvedError{Status: Missing}
}

func sameURI(matches []Candidate) bool {
	for _, m := range matches[1:] {
		if m.URI != matches[0].URI {
			return false
		}
	}
	return true
}

// bodyMatches lists the body's terms the reference matches (one candidate per
// URI), and separately those the owner declares under its own prefix.
func bodyMatches(ref TermReference, pin Body, body bodyDocument, aliases []namespace.Binding) (all, native []Candidate) {
	if ref.VersionID != "" && ref.VersionID != pin.VersionID {
		return nil, nil
	}
	terms := body.Classes
	if ref.Kind == propertyKind {
		terms = body.Properties
	}
	seen, own := map[string]bool{}, map[string]bool{}
	for _, t := range terms {
		if ref.URI != "" && !matchesTerm(ref.URI, t, aliases, body.Bindings) {
			continue
		}
		if ref.Prefix != "" && !matchesTerm(ref.Prefix+":"+ref.LocalName, t, aliases, body.Bindings) {
			continue
		}
		c := Candidate{OntologyID: pin.OntologyID, VersionID: pin.VersionID, Digest: pin.Digest, URI: t.URI}
		if !seen[t.URI] {
			seen[t.URI] = true
			all = append(all, c)
		}
		// Any declaration of the URI under the owner's own prefix makes it native.
		if t.Prefix != "" && t.Prefix == body.Ontology.Prefix && !own[t.URI] {
			own[t.URI] = true
			native = append(native, c)
		}
	}
	return all, native
}
