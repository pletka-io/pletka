package projectsnapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/pletka-io/pletka/pkg/namespace"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
)

// SemanticElements retains each authored location, including null scopes and
// elements without a term. Ordinals match the capture inventory's JSON pointers.
func SemanticElements(c Content) ([]ontologyevidence.Element, error) {
	result := []ontologyevidence.Element{}
	add := func(kind, id, location, defaultKind string, raw []byte) {
		if len(raw) == 0 {
			raw = []byte(jsonNull)
		}
		result = append(result, ontologyevidence.Element{EntityKind: kind, EntityID: id, Location: location, DefaultKind: defaultKind, Original: slices.Clone(raw)})
	}
	for _, row := range c.Fields {
		add(kindField, row.ID, "/ontology_scope", kindClass, row.OntologyScope)
		paths, err := pathElements(row.PathElements)
		if err != nil {
			return nil, err
		}
		for i, raw := range paths {
			add(kindField, row.ID, fmt.Sprintf("/path_elements/%d", i), "", raw)
		}
		var subfields []struct {
			PathElements json.RawMessage `json:"path_elements"`
		}
		if len(row.SubfieldPaths) > 0 {
			if err := json.Unmarshal(row.SubfieldPaths, &subfields); err != nil {
				return nil, err
			}
		}
		for i, subfield := range subfields {
			paths, err := pathElements(subfield.PathElements)
			if err != nil {
				return nil, err
			}
			for j, raw := range paths {
				add(kindField, row.ID, fmt.Sprintf("/subfield_paths/%d/path_elements/%d", i, j), "", raw)
			}
		}
	}
	for _, row := range c.Models {
		add(kindModel, row.ID, "/ontology_scope", kindClass, row.OntologyScope)
	}
	for _, row := range c.Collections {
		add(kindCollection, row.ID, "/ontology_scope", kindClass, row.OntologyScope)
	}
	return result, nil
}

func pathElements(raw []byte) ([]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var result []json.RawMessage
	err := json.Unmarshal(raw, &result)
	return result, err
}

func (g *graph) checkInterpretation(id Identity) error {
	p := g.projects[id]
	var content Content
	if err := json.Unmarshal([]byte(p.data), &content); err != nil {
		return err
	}
	elements, err := SemanticElements(content)
	if err != nil {
		return fmt.Errorf("decode authored semantic locations: %w", err)
	}
	bindings := make([]namespace.Binding, len(content.Namespaces))
	for i, row := range content.Namespaces {
		bindings[i] = namespace.Binding{Prefix: row.Prefix, Namespace: row.Namespace, Weight: row.Weight, Source: row.Source}
	}
	if p.evidence.contract == 2 {
		return g.checkLegacy(id, content, elements, bindings)
	}
	return p.checkPublished(content, elements, bindings)
}

func (p *project) checkPublished(c Content, elements []ontologyevidence.Element, bindings []namespace.Binding) error {
	expected, err := ontologyevidence.InputReferences(elements)
	if err != nil {
		return err
	}
	refs := map[semanticKey]ontologyevidence.InputReference{}
	for _, ref := range expected {
		refs[semanticKey{ref.EntityKind, ref.EntityID, ref.Location}] = ref
	}
	if len(refs) != len(p.evidence.references) {
		return fmt.Errorf("semantic reference coverage differs from authored content")
	}
	inventory := make([]ontologyevidence.Body, 0, len(p.evidence.publications))
	if err := p.checkPublicationLinks(c); err != nil {
		return err
	}
	for _, publication := range p.evidence.publications {
		d, err := publication.Artifact.Decode()
		if err != nil {
			return err
		}
		inventory = append(inventory, ontologyevidence.Body{OntologyID: d.OntologyID, VersionID: d.VersionID, Digest: d.BodyDigest, BodyJSON: d.BodyJSON})
	}
	assessor, err := ontologyevidence.NewAssessor(inventory, bindings)
	if err != nil {
		return err
	}
	for _, r := range p.evidence.references {
		key := semanticKey{r.EntityKind, r.EntityID, r.Location}
		input, ok := refs[key]
		if !ok || input.Reference != ontologyevidence.TermReference(r.Reference) {
			return fmt.Errorf("semantic input differs from recorded resolution")
		}
		status, candidates, err := assessor.Assess(input.Reference)
		if err != nil {
			return err
		}
		if status != ontologyevidence.Resolved || candidates[0].VersionID != r.VersionID || candidates[0].URI != r.URI {
			return fmt.Errorf("published semantic resolution is not uniquely supported by retained evidence")
		}
	}
	return nil
}

func (p *project) checkPublicationLinks(c Content) error {
	linked := map[string]bool{}
	for _, publication := range p.evidence.publications {
		d, err := publication.Artifact.Decode()
		if err != nil {
			return err
		}
		if _, err := p.evidence.descriptors.OntologyModule(d.VersionID); err != nil {
			return err
		}
		if publication.Linked {
			linked[d.VersionID] = publication.Primary
		}
	}
	if len(linked) != len(c.OntologyLinks) {
		return fmt.Errorf("direct ontology links differ from publication inventory")
	}
	for _, link := range c.OntologyLinks {
		primary, ok := linked[link.OntologyVersionID]
		if !ok || primary != (link.IsPrimary != nil && *link.IsPrimary) {
			return fmt.Errorf("ontology link flags differ from captured selection")
		}
	}
	return nil
}

func (g *graph) checkLegacy(id Identity, c Content, elements []ontologyevidence.Element, bindings []namespace.Binding) error {
	p := g.projects[id]
	d, err := p.evidence.artifact.Decode()
	if err != nil {
		return err
	}
	if err := checkLegacyElements(d.Elements, elements); err != nil {
		return err
	}
	if !sameBindings(d.Bindings, bindings) {
		return fmt.Errorf("legacy namespace evidence differs from retained declarations")
	}
	if len(d.Inventory) != len(c.OntologyLinks) {
		return fmt.Errorf("legacy ontology link inventory differs from content")
	}
	if err := p.checkLegacyLinks(c.OntologyLinks, d.Inventory); err != nil {
		return err
	}
	if len(d.SourceRefs) != len(p.dependencies) || len(d.Dependencies) != len(p.members) {
		return fmt.Errorf("legacy source context coverage differs from selected graph")
	}
	for _, source := range d.SourceRefs {
		sourceID, err := Release(source.ProjectID, source.Version)
		if err != nil {
			return err
		}
		if !slices.Contains(p.dependencies, sourceID) {
			return fmt.Errorf("legacy evidence escapes selected project closure")
		}
	}
	for _, dep := range d.Dependencies {
		selected, ok := p.members[localKey{dep.EntityKind, dep.EntityID}]
		if !ok || selected.project != dep.SourceProject || selected.version != dep.SourceVersion {
			return fmt.Errorf("legacy entity source differs from selected membership")
		}
	}
	return nil
}

func normalizedJSON(raw []byte) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	// Arbitrary authored JSON stays lossless; no float conversion or field filter.
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	data, err := json.Marshal(value)
	return string(data), err
}

func sameBindings(a, b []namespace.Binding) bool {
	if len(a) != len(b) {
		return false
	}
	counts := map[namespace.Binding]int{}
	for _, binding := range a {
		counts[binding]++
	}
	for _, binding := range b {
		counts[binding]--
		if counts[binding] < 0 {
			return false
		}
	}
	return true
}

func checkLegacyElements(retained, elements []ontologyevidence.Element) error {
	if len(retained) != len(elements) {
		return fmt.Errorf("legacy semantic element coverage differs from authored content")
	}
	expected := map[semanticKey]ontologyevidence.Element{}
	for _, e := range elements {
		expected[semanticKey{e.EntityKind, e.EntityID, e.Location}] = e
	}
	for _, e := range retained {
		original, ok := expected[semanticKey{e.EntityKind, e.EntityID, e.Location}]
		if !ok || original.DefaultKind != e.DefaultKind {
			return fmt.Errorf("legacy semantic location differs from authored content")
		}
		a, err := normalizedJSON(original.Original)
		if err != nil {
			return err
		}
		b, err := normalizedJSON(e.Original)
		if err != nil {
			return err
		}
		if a != b {
			return fmt.Errorf("legacy semantic input differs from authored content")
		}
	}

	return nil
}

func (p *project) checkLegacyLinks(links []OntologyLinkRecord, inventory []ontologyevidence.Body) error {
	for _, link := range links {
		i := slices.IndexFunc(inventory, func(b ontologyevidence.Body) bool { return b.VersionID == link.OntologyVersionID })
		if i < 0 {
			return fmt.Errorf("legacy selected body is absent")
		}
		var selection struct {
			IsPrimary *bool `json:"is_primary"`
		}
		if err := json.Unmarshal(inventory[i].Selection, &selection); err != nil {
			return err
		}
		if (selection.IsPrimary == nil) != (link.IsPrimary == nil) || selection.IsPrimary != nil && *selection.IsPrimary != *link.IsPrimary {
			return fmt.Errorf("legacy ontology link flags differ from retained selection")
		}
		if _, err := p.evidence.descriptors.OntologyModule(link.OntologyVersionID); err != nil {
			return err
		}
	}
	return nil
}
