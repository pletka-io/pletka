package ontologyevidence

import (
	"encoding/json"
	"fmt"
	"slices"
)

type validator struct {
	preparer *Preparer
	contexts map[Origin]Document
	digests  map[Origin]string
	bodies   map[string]string
}

func decodeDocument(data, digest string) (Document, error) {
	var d Document
	if hash(data) != digest {
		return d, fmt.Errorf("legacy snapshot digest mismatch")
	}
	if err := json.Unmarshal([]byte(data), &d); err != nil {
		return d, err
	}
	if _, err := NewOrigin(d.ProjectID, d.Version); err != nil || d.FormatVersion != 1 {
		return d, fmt.Errorf("legacy snapshot identity or format mismatch")
	}
	return d, nil
}

func validateArtifact(a Artifact) (Document, error) {
	var preparer Preparer
	p, err := preparer.Prepare(a)
	if err != nil {
		return Document{}, err
	}
	// This fresh preparer cannot escape alongside the returned document.
	return p.value.contexts[p.value.root].document, nil
}

func (p *Preparer) validate(a Artifact) (Prepared, error) {
	entry, err := p.context(a)
	if err != nil {
		return Prepared{}, err
	}
	d := entry.document
	root := Origin{project: d.ProjectID, version: d.Version}
	entries := map[Origin]*preparedContext{root: entry}
	v := validator{preparer: p, contexts: map[Origin]Document{root: d}, digests: map[Origin]string{}, bodies: map[string]string{}}
	for i, source := range d.Sources {
		entry, err := p.context(Artifact(source))
		if err != nil {
			return Prepared{}, err
		}
		s := entry.document
		o := Origin{project: s.ProjectID, version: s.Version}
		if _, exists := v.contexts[o]; exists || len(s.Sources) != 0 {
			return Prepared{}, fmt.Errorf("duplicate, conflicting or nested source context")
		}
		v.contexts[o], v.digests[o] = s, source.Digest
		// Freeze exact equal strings onto the operation's shared storage before
		// this newly validated root can escape. No exported buffers are shared.
		d.Sources[i].JSON = entry.artifact.JSON
		entries[o] = entry
	}
	for origin, context := range v.contexts {
		bodies, err := v.inventory(context, !entries[origin].checked)
		if err != nil {
			return Prepared{}, err
		}
		if !entries[origin].checked {
			if err := validateAssessments(context, bodies); err != nil {
				return Prepared{}, err
			}
			local := context
			local.Sources = nil
			encoded, err := json.Marshal(local)
			if err != nil {
				return Prepared{}, err
			}
			entries[origin].localDigest = hash(string(encoded))
			entries[origin].checked = true
		}
		if err := v.dependencies(context); err != nil {
			return Prepared{}, err
		}
	}
	visited := map[Origin]bool{}
	if err := v.visit(root, visited, map[Origin]bool{}); err != nil {
		return Prepared{}, err
	}
	if len(visited) != len(v.contexts) {
		return Prepared{}, fmt.Errorf("snapshot contains unreferenced source contexts")
	}
	return Prepared{value: &preparedArtifact{artifact: a, root: root, contexts: entries, resolver: Resolver{root: root, contexts: v.contexts}}}, nil
}

func (v *validator) visit(o Origin, visited, stack map[Origin]bool) error {
	if stack[o] {
		return fmt.Errorf("snapshot source cycle")
	}
	if visited[o] {
		return nil
	}
	visited[o], stack[o] = true, true
	defer delete(stack, o)
	for _, source := range v.contexts[o].SourceRefs {
		if err := v.visit(Origin{project: source.ProjectID, version: source.Version}, visited, stack); err != nil {
			return err
		}
	}
	return nil
}

func (v *validator) dependencies(d Document) error {
	sources := map[Origin]bool{}
	for _, ref := range d.SourceRefs {
		o := Origin{project: ref.ProjectID, version: ref.Version}
		if ref.Digest == "" || v.digests[o] != ref.Digest || sources[o] {
			return fmt.Errorf("missing, duplicate or conflicting source context reference")
		}
		sources[o] = true
	}
	dependencies := map[location]bool{}
	for _, dep := range d.Dependencies {
		if dep.ProjectID != d.ProjectID || dep.Version != d.Version || !sources[Origin{project: dep.SourceProject, version: dep.SourceVersion}] {
			return fmt.Errorf("entity dependency escapes its source context")
		}
		key := location{kind: dep.EntityKind, id: dep.EntityID}
		if dependencies[key] {
			return fmt.Errorf("duplicate entity dependency")
		}
		dependencies[key] = true
		for _, e := range d.Elements {
			if e.EntityKind == dep.EntityKind && e.EntityID == dep.EntityID {
				return fmt.Errorf("entity has both local and source interpretation contexts")
			}
		}
	}
	return nil
}

func (v *validator) inventory(d Document, freeze bool) ([]bodyDocument, error) {
	bodies := make([]bodyDocument, 0, len(d.Inventory))
	previous := ""
	for i, b := range d.Inventory {
		if b.VersionID <= previous || b.OntologyID == "" {
			return nil, fmt.Errorf("duplicate, unordered or invalid concrete body identity")
		}
		previous = b.VersionID
		if digest := v.bodies[b.VersionID]; digest != "" && digest != b.Digest {
			return nil, fmt.Errorf("conflicting bytes for concrete ontology version %s", b.VersionID)
		}
		v.bodies[b.VersionID] = b.Digest
		body, err := v.preparer.body(b)
		if err != nil {
			return nil, err
		}
		if freeze {
			d.Inventory[i].BodyJSON = v.preparer.bodies[b.Digest].json
		}
		var selection struct {
			ProjectID string `json:"project_id"`
			Version   string `json:"version_number"`
			VersionID string `json:"ontology_version_id"`
		}
		if err := json.Unmarshal(b.Selection, &selection); err != nil {
			return nil, err
		}
		if selection.ProjectID != d.ProjectID || selection.Version != d.Version || selection.VersionID != b.VersionID {
			return nil, fmt.Errorf("body selection escapes its project context")
		}
		bodies = append(bodies, body)
	}
	return bodies, nil
}

type location struct{ kind, id, path string }

func validateAssessments(d Document, bodies []bodyDocument) error {
	type result struct {
		status     Status
		candidates []Candidate
	}
	// Locations must each be checked, but identical references share the same
	// immutable inventory and bindings within this context.
	assessed := map[TermReference]result{}
	expected := map[location]TermReference{}
	elements := map[location]bool{}
	for _, e := range d.Elements {
		key := location{e.EntityKind, e.EntityID, e.Location}
		if elements[key] {
			return fmt.Errorf("duplicate original semantic element")
		}
		elements[key] = true
		refs, err := elementReferences(e)
		if err != nil {
			return err
		}
		for _, r := range refs {
			key := location{r.EntityKind, r.EntityID, r.Location}
			if _, exists := expected[key]; exists {
				return fmt.Errorf("duplicate semantic location")
			}
			expected[key] = r.Reference
		}
	}
	for _, a := range d.Assessments {
		key := location{a.EntityKind, a.EntityID, a.Location}
		ref, ok := expected[key]
		if !ok || ref != a.Reference {
			return fmt.Errorf("assessment omits, duplicates or changes original input")
		}
		delete(expected, key)
		value, ok := assessed[ref]
		if !ok {
			value.status, value.candidates = assess(ref, d.Inventory, bodies, d.Bindings)
			assessed[ref] = value
		}
		if value.status != a.Status || !slices.Equal(value.candidates, a.Candidates) {
			return fmt.Errorf("assessment differs from retained candidate evidence")
		}
	}
	if len(expected) != 0 {
		return fmt.Errorf("snapshot omits semantic assessments")
	}
	return nil
}
