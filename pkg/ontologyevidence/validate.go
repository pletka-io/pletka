package ontologyevidence

import (
	"encoding/json"
	"fmt"
	"slices"
)

type validator struct {
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
	d, err := decodeDocument(a.JSON, a.Digest)
	if err != nil {
		return d, err
	}
	root := Origin{project: d.ProjectID, version: d.Version}
	v := validator{contexts: map[Origin]Document{root: d}, digests: map[Origin]string{}, bodies: map[string]string{}}
	for _, source := range d.Sources {
		s, err := decodeDocument(source.JSON, source.Digest)
		if err != nil {
			return d, err
		}
		o := Origin{project: s.ProjectID, version: s.Version}
		if _, exists := v.contexts[o]; exists || len(s.Sources) != 0 {
			return d, fmt.Errorf("duplicate, conflicting or nested source context")
		}
		v.contexts[o], v.digests[o] = s, source.Digest
	}
	for _, context := range v.contexts {
		bodies, err := v.inventory(context)
		if err != nil {
			return d, err
		}
		if err := validateAssessments(context, bodies); err != nil {
			return d, err
		}
		if err := v.dependencies(context); err != nil {
			return d, err
		}
	}
	visited := map[Origin]bool{}
	if err := v.visit(root, visited, map[Origin]bool{}); err != nil {
		return d, err
	}
	if len(visited) != len(v.contexts) {
		return d, fmt.Errorf("snapshot contains unreferenced source contexts")
	}
	return d, nil
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

func (v *validator) inventory(d Document) ([]bodyDocument, error) {
	bodies := make([]bodyDocument, 0, len(d.Inventory))
	previous := ""
	for _, b := range d.Inventory {
		if b.VersionID <= previous || b.OntologyID == "" {
			return nil, fmt.Errorf("duplicate, unordered or invalid concrete body identity")
		}
		previous = b.VersionID
		if digest := v.bodies[b.VersionID]; digest != "" && digest != b.Digest {
			return nil, fmt.Errorf("conflicting bytes for concrete ontology version %s", b.VersionID)
		}
		v.bodies[b.VersionID] = b.Digest
		body, err := decodeBody(b)
		if err != nil {
			return nil, err
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
		status, candidates := assess(ref, d.Inventory, bodies, d.Bindings)
		if status != a.Status || !slices.Equal(candidates, a.Candidates) {
			return fmt.Errorf("assessment differs from retained candidate evidence")
		}
	}
	if len(expected) != 0 {
		return fmt.Errorf("snapshot omits semantic assessments")
	}
	return nil
}
