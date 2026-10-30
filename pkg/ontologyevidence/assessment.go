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
	if invalidReference(ref, inventory) {
		return Invalid, matches
	}
	for i, body := range bodies {
		pin := inventory[i]
		if ref.VersionID != "" && ref.VersionID != pin.VersionID {
			continue
		}
		bindings := append(slices.Clone(aliases), body.Bindings...)
		terms := body.Classes
		if ref.Kind == propertyKind {
			terms = body.Properties
		}
		seen := map[string]bool{}
		for _, t := range terms {
			if ref.URI != "" && !namespace.MatchesTerm(ref.URI, t.URI, t.Prefix, t.LocalName, bindings) {
				continue
			}
			if ref.Prefix != "" && !namespace.MatchesTerm(ref.Prefix+":"+ref.LocalName, t.URI, t.Prefix, t.LocalName, bindings) {
				continue
			}
			if !seen[t.URI] {
				matches = append(matches, Candidate{OntologyID: pin.OntologyID, VersionID: pin.VersionID, Digest: pin.Digest, URI: t.URI})
				seen[t.URI] = true
			}
		}
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
	d, err := a.Decode()
	if err != nil {
		return Definition{}, err
	}
	contexts := map[Origin]Document{}
	for _, source := range d.Sources {
		context, err := decodeDocument(source.JSON, source.Digest)
		if err != nil {
			return Definition{}, err
		}
		contexts[Origin{project: context.ProjectID, version: context.Version}] = context
	}
	// Decode has already verified every edge and ruled out cycles.
	for {
		found := false
		for _, dep := range d.Dependencies {
			if dep.EntityKind == entityKind && dep.EntityID == entityID {
				d = contexts[Origin{project: dep.SourceProject, version: dep.SourceVersion}]
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
