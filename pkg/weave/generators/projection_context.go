package generators

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

type semanticLocation struct {
	source         projectsnapshot.Identity
	kind, id, path string
}
type retainedTerm struct{ uri, version string }
type projection struct {
	consumer   projectsnapshot.Identity
	root       projectsnapshot.EntityKey
	content    map[projectsnapshot.Identity]projectsnapshot.Content
	terms      map[semanticLocation]retainedTerm
	namespaces NamespaceSet
	ontologies []domain.ResolvedOntologyVersion
	bundle     []domain.OntologyBundleEntry
	targets    []X3MLTarget
}

func newProjection(f projectsnapshot.EntityFragment) (*projection, error) {
	consumer, err := f.Consumer()
	if err != nil {
		return nil, err
	}
	root, err := f.Root()
	if err != nil {
		return nil, err
	}
	p := &projection{consumer: consumer, root: root, content: map[projectsnapshot.Identity]projectsnapshot.Content{}, terms: map[semanticLocation]retainedTerm{}}
	ids, err := f.Projects()
	if err != nil {
		return nil, err
	}
	// Prefer the consumer's direct link policy when a body is shared.
	slices.SortStableFunc(ids, func(a, b projectsnapshot.Identity) int {
		if a == b {
			return 0
		}
		if a == consumer {
			return -1
		}
		if b == consumer {
			return 1
		}
		return 0
	})
	seen := map[string]bool{}
	for _, id := range ids {
		c, err := f.Content(id)
		if err != nil {
			return nil, err
		}
		p.content[id] = c
		evidence, err := f.Evidence(id)
		if err != nil {
			return nil, err
		}
		if err := p.retainTerms(id, c, evidence); err != nil {
			return nil, err
		}
		if err := p.retainOntologies(id, evidence, seen); err != nil {
			return nil, err
		}
	}
	c := p.content[consumer]
	bindings := make([]*domain.NamespaceBinding, 0, len(c.Namespaces))
	for _, b := range c.Namespaces {
		bindings = append(bindings, &domain.NamespaceBinding{Prefix: b.Prefix, Namespace: b.Namespace, Weight: b.Weight, Source: b.Source})
	}
	project, err := projectEntity(c.Projects[0])
	if err != nil {
		return nil, err
	}
	p.namespaces, err = BuildNamespaceSet(project, bindings)
	if err != nil {
		return nil, err
	}
	// Resolve aliases in a stable order, independently of traversal/renderer order.
	keys := make([]semanticLocation, 0, len(p.terms))
	for k := range p.terms {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b semanticLocation) int {
		return strings.Compare(a.source.ProjectID()+"\x00"+a.kind+"\x00"+a.id+"\x00"+a.path, b.source.ProjectID()+"\x00"+b.kind+"\x00"+b.id+"\x00"+b.path)
	})
	for _, k := range keys {
		if _, _, err := p.alias(p.terms[k].uri); err != nil {
			return nil, err
		}
	}
	return p, nil
}
func (p *projection) retainTerms(id projectsnapshot.Identity, c projectsnapshot.Content, e projectsnapshot.Evidence) error {
	elements, err := projectsnapshot.SemanticElements(c)
	if err != nil {
		return err
	}
	inputs, err := ontologyevidence.InputReferences(elements)
	if err != nil {
		return err
	}
	if e.Contract() == 2 {
		legacy, err := e.Legacy()
		if err != nil {
			return err
		}
		resolver, err := legacy.Resolver()
		if err != nil {
			return err
		}
		for _, input := range inputs {
			def, err := resolver.Resolve(input.EntityKind, input.EntityID, input.Location)
			if err != nil {
				return fmt.Errorf("generator semantic input %s %s%s: %w", input.EntityKind, input.EntityID, input.Location, err)
			}
			term, _, err := def.Evidence()
			if err != nil {
				return err
			}
			p.terms[semanticLocation{id, input.EntityKind, input.EntityID, input.Location}] = retainedTerm{term.URI, term.VersionID}
		}
		return nil
	}
	refs, err := e.References()
	if err != nil {
		return err
	}
	wanted := map[semanticLocation]bool{}
	for _, input := range inputs {
		wanted[semanticLocation{id, input.EntityKind, input.EntityID, input.Location}] = true
	}
	for _, r := range refs {
		if !wanted[semanticLocation{id, r.EntityKind, r.EntityID, r.Location}] {
			continue
		}
		p.terms[semanticLocation{id, r.EntityKind, r.EntityID, r.Location}] = retainedTerm{r.URI, r.VersionID}
	}
	return nil
}
func (p *projection) retainOntologies(id projectsnapshot.Identity, e projectsnapshot.Evidence, seen map[string]bool) error {
	type retained struct {
		content ontologyartifact.Content
		primary bool
	}
	bodies := []retained{}
	if e.Contract() == 2 {
		a, err := e.Legacy()
		if err != nil {
			return err
		}
		d, err := a.Decode()
		if err != nil {
			return err
		}
		for _, b := range d.Inventory {
			var c ontologyartifact.Content
			if err := json.Unmarshal([]byte(b.BodyJSON), &c); err != nil {
				return err
			}
			bodies = append(bodies, retained{content: c})
		}
	} else {
		publications, err := e.Publications()
		if err != nil {
			return err
		}
		for _, a := range publications {
			c, err := a.Artifact.Content()
			if err != nil {
				return err
			}
			bodies = append(bodies, retained{c, a.Primary})
		}
	}
	for _, b := range bodies {
		c := b.content
		if seen[c.Version.ID] {
			continue
		}
		seen[c.Version.ID] = true
		p.ontologies = append(p.ontologies, domain.ResolvedOntologyVersion{Link: &domain.ProjectOntologyVersion{ProjectID: id.ProjectID(), OntologyVersionID: c.Version.ID, IsPrimary: b.primary}, SourceProjectID: id.ProjectID(), OntologyName: c.Ontology.Name, OntologyPrefix: c.Ontology.Prefix, VersionString: c.Version.Version})
		p.bundle = append(p.bundle, domain.OntologyBundleEntry{Prefix: c.Ontology.Prefix, Namespace: c.Ontology.Namespace, Name: c.Ontology.Name, VersionString: c.Version.Version, OriginalFilename: c.Version.Filename, RDFContent: c.Version.RDF})
		p.targets = append(p.targets, X3MLTarget{Prefix: c.Ontology.Prefix, Namespace: c.Ontology.Namespace, Label: c.Ontology.Name, Version: c.Version.Version, SchemaFile: c.Version.Filename})
	}
	return nil
}

// alias emits only QNames that expand to the retained absolute URI. A stable
// generated alias handles source-prefix collisions without reinterpreting terms.
func (p *projection) alias(uri string) (string, string, error) {
	for _, b := range p.namespaces.Bindings {
		if rest, ok := strings.CutPrefix(uri, b.Namespace); ok && rest != "" && !strings.ContainsAny(rest, "/#:") {
			return b.Prefix, rest, nil
		}
	}
	i := strings.LastIndexAny(uri, "/#:")
	if i < 0 || i == len(uri)-1 {
		return "", "", fmt.Errorf("cannot represent retained term URI %q", uri)
	}
	ns, local := uri[:i+1], uri[i+1:]
	hash := sha256.Sum256([]byte(ns))
	prefix := fmt.Sprintf("ns_%x", hash[:8])
	if existing, ok := p.namespaces.ByPrefix[prefix]; ok && existing != ns {
		return "", "", fmt.Errorf("generated namespace alias collision")
	}
	if _, ok := p.namespaces.ByPrefix[prefix]; !ok {
		p.namespaces.ByPrefix[prefix] = ns
		p.namespaces.Bindings = append(p.namespaces.Bindings, NamespaceBinding{Prefix: prefix, Namespace: ns, Source: "retained-term"})
	}
	return prefix, local, nil
}
func (p *projection) pathElement(key projectsnapshot.EntityKey, location string, raw []byte) (domain.PathElement, error) {
	var out domain.PathElement
	if err := decodeProjection(raw, &out); err != nil {
		return out, err
	}
	err := p.resolveElement(key, location, &out)
	return out, err
}
func (p *projection) term(key projectsnapshot.EntityKey, location string) (retainedTerm, error) {
	t, ok := p.terms[semanticLocation{key.Source, key.Kind, key.ID, location}]
	if !ok {
		return t, fmt.Errorf("semantic location %s %s%s has no retained definition", key.Kind, key.ID, location)
	}
	return t, nil
}
func (p *projection) resolveElement(key projectsnapshot.EntityKey, location string, e *domain.PathElement) error {
	if e.Type != "complete" && (e.URI != "" || e.Prefix != "" || e.LocalName != "" || e.OntologyVersionID != "") {
		t, err := p.term(key, location)
		if err != nil {
			return err
		}
		e.URI = t.uri
		e.OntologyVersionID = t.version
		e.Prefix, e.LocalName, err = p.alias(t.uri)
		if err != nil {
			return err
		}
	}
	if e.Datatype != "" {
		t, err := p.term(key, location+"/datatype")
		if err != nil {
			return err
		}
		prefix, local, err := p.alias(t.uri)
		if err != nil {
			return err
		}
		e.Datatype = prefix + ":" + local
	}
	for i := range e.AdditionalTypes {
		t, err := p.term(key, fmt.Sprintf("%s/additional_types/%d", location, i))
		if err != nil {
			return err
		}
		e.AdditionalTypes[i].URI = t.uri
		e.AdditionalTypes[i].Prefix, e.AdditionalTypes[i].LocalName, err = p.alias(t.uri)
		if err != nil {
			return err
		}
	}
	return nil
}
func conceptURI(uri, ns string) (string, error) {
	if local, ok := strings.CutPrefix(uri, "pletka:"); ok {
		if ns == "" || local == "" {
			return "", fmt.Errorf("local concept has no captured namespace")
		}
		return ns + local, nil
	}
	if !strings.Contains(uri, ":") {
		return "", fmt.Errorf("concept has no absolute URI")
	}
	return uri, nil
}
