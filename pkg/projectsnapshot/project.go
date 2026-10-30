package projectsnapshot

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/namespace"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
)

// ProjectInput is the assembler's explicit loaded inventory. Counts come from
// the same bound read; released counts must also match the capture receipt.
// Nil sections, counts, dependency lists and membership lists mean not loaded.
type ProjectInput struct {
	Content      Content
	Evidence     Evidence
	Counts       map[string]int64
	Dependencies []Identity
	Membership   []Membership
}

// CompleteProject contains all owned sections and complete selected source
// projects. Its zero value is invalid. It certifies structural value coverage,
// not authorization, semantic promotion, a serializer or a restore format.
type CompleteProject struct{ graph *graph }

type graph struct {
	root     Identity
	projects map[Identity]*project
	entities map[EntityKey]nodeKey
	nodes    map[nodeKey]record
	edges    map[nodeKey][]nodeKey
}

type project struct {
	data         string
	evidence     Evidence
	dependencies []Identity
	members      map[localKey]Identity
	rows         map[string][]nodeKey
}

type localKey struct{ kind, id string }
type nodeKey struct {
	source       Identity
	section, key string
}
type record struct {
	data   string
	fields map[string]json.RawMessage
}

// NewComplete constructs all-or-error, without exposing input buffers. Only
// assembly infrastructure should call it; serializers receive the finished value.
func NewComplete(root ProjectInput, sources []ProjectInput) (CompleteProject, error) {
	g := &graph{root: root.Evidence.id, projects: map[Identity]*project{}, entities: map[EntityKey]nodeKey{}, nodes: map[nodeKey]record{}, edges: map[nodeKey][]nodeKey{}}
	inputs := append([]ProjectInput{root}, sources...)
	versions := map[string]Identity{}
	for i, in := range inputs {
		id := in.Evidence.id
		if !id.valid() || i > 0 && id.draft {
			return CompleteProject{}, fmt.Errorf("project input has no valid evidence or selects a source draft")
		}
		if _, ok := versions[id.project]; ok {
			return CompleteProject{}, fmt.Errorf("duplicate or conflicting project selection %s", id.project)
		}
		versions[id.project] = id
		if err := g.addProject(in); err != nil {
			return CompleteProject{}, err
		}
	}
	if err := g.checkSources(); err != nil {
		return CompleteProject{}, err
	}
	if err := g.connect(); err != nil {
		return CompleteProject{}, err
	}
	return CompleteProject{graph: g}, nil
}

func (g *graph) addProject(in ProjectInput) error {
	id := in.Evidence.id
	if in.Counts == nil || in.Dependencies == nil || in.Membership == nil {
		return fmt.Errorf("project coverage and dependency inventories must be loaded")
	}
	data, err := json.Marshal(in.Content)
	if err != nil {
		return fmt.Errorf("detach project content: %w", err)
	}
	var sections map[string][]json.RawMessage
	if err := json.Unmarshal(data, &sections); err != nil {
		return err
	}
	if len(in.Counts) != len(Coverage()) || len(sections) != len(Coverage()) {
		return fmt.Errorf("project section coverage differs from value contract")
	}
	p := &project{data: string(data), evidence: in.Evidence, dependencies: slices.Clone(in.Dependencies), members: map[localKey]Identity{}, rows: map[string][]nodeKey{}}
	counts := make(map[string]int64, len(sections))
	for _, section := range Coverage() {
		rows := sections[section.Name]
		count, ok := in.Counts[section.Name]
		if rows == nil || !ok || count != int64(len(rows)) {
			return fmt.Errorf("section %s is unloaded or truncated", section.Name)
		}
		counts[section.Name] = count
		if err := g.addRows(id, p, section, rows); err != nil {
			return err
		}
	}
	if len(in.Content.Projects) != 1 || in.Content.Projects[0].ID != id.project {
		return fmt.Errorf("project metadata is absent or belongs to another project")
	}
	if err := in.Evidence.checkCounts(counts); err != nil {
		return err
	}
	if err := p.addMembership(id, in.Membership); err != nil {
		return err
	}
	g.projects[id] = p
	return nil
}

func (r record) checkJSON(section Section) error {
	for _, column := range section.JSONColumns {
		raw, ok := r.fields[column]
		if !ok || string(raw) == jsonNull {
			continue
		}
		var data []byte
		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}
		if !json.Valid(data) {
			return fmt.Errorf("column %s has invalid authored JSON", column)
		}
	}
	return nil
}

func (r record) key(id Identity, section Section) (nodeKey, error) {
	// Namespace project_id describes declaration provenance, not release ownership.
	owner := r.text(columnProjectID)
	if section.Name == sectionReleaseNamespaceBindings {
		owner = r.text(columnReleaseProjectID)
	}
	if _, hasOwner := r.fields[columnProjectID]; hasOwner && owner != id.project {
		return nodeKey{}, fmt.Errorf("row in %s escapes project ownership", section.Name)
	}
	if !id.draft && r.text(columnVersionNumber) != id.version {
		return nodeKey{}, fmt.Errorf("row in %s escapes release selection", section.Name)
	}
	parts := make([]string, 0, len(section.Key))
	for _, field := range section.Key {
		v, ok := r.fields[field]
		if !ok || string(v) == jsonNull {
			return nodeKey{}, fmt.Errorf("row in %s lacks key %s", section.Name, field)
		}
		if field == columnID && (r.text(field) == "" || r.text(field) == "0") {
			return nodeKey{}, fmt.Errorf("row in %s lacks an identity", section.Name)
		}
		parts = append(parts, string(v))
	}
	// JSON encoding avoids collisions between composite key components.
	encoded, err := json.Marshal(parts)
	if err != nil {
		return nodeKey{}, err
	}
	return nodeKey{source: id, section: section.Name, key: string(encoded)}, nil
}

func (r record) text(column string) string {
	raw := r.fields[column]
	if len(raw) == 0 || string(raw) == jsonNull {
		return ""
	}
	if raw[0] != '"' {
		return string(raw)
	}
	var value string
	// Fields were encoded from typed records; a string decoding error cannot occur.
	_ = json.Unmarshal(raw, &value)
	return value
}

func (g *graph) checkSources() error {
	legacy := []ontologyevidence.Artifact{}
	for _, id := range g.identities() {
		p := g.projects[id]
		if err := g.checkProjectSources(id, p); err != nil {
			return err
		}
		if p.evidence.contract == 2 {
			legacy = append(legacy, p.evidence.artifact)
		}
	}
	if err := ontologyevidence.ValidateBundle(legacy); err != nil {
		return err
	}
	if err := g.checkSharedOntologies(); err != nil {
		return err
	}
	visited := map[Identity]bool{}
	if err := g.visit(g.root, visited, map[Identity]bool{}); err != nil {
		return err
	}
	if len(visited) != len(g.projects) {
		return fmt.Errorf("unreferenced source project content")
	}
	return nil
}

func (g *graph) visit(id Identity, visited, active map[Identity]bool) error {
	if active[id] {
		return fmt.Errorf("project dependency cycle")
	}
	if visited[id] {
		return nil
	}
	active[id], visited[id] = true, true
	p := g.projects[id]
	closure := map[Identity]bool{}
	for _, dep := range p.dependencies {
		closure[dep] = true
	}
	for _, dep := range p.dependencies {
		for _, transitive := range g.projects[dep].dependencies {
			if !closure[transitive] {
				return fmt.Errorf("project inventory omits transitive source")
			}
		}
		if err := g.visit(dep, visited, active); err != nil {
			return err
		}
	}
	delete(active, id)
	return nil
}

func (g *graph) identities() []Identity {
	ids := make([]Identity, 0, len(g.projects))
	for id := range g.projects {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b Identity) int { return strings.Compare(a.project, b.project) })
	return ids
}

// Root returns the complete value's root selection, rejecting zero values.
func (c CompleteProject) Root() (Identity, error) {
	if c.graph == nil {
		return Identity{}, fmt.Errorf("complete project is uninitialized")
	}
	return c.graph.root, nil
}

// Projects returns the stable complete project closure.
func (c CompleteProject) Projects() ([]Identity, error) {
	if c.graph == nil {
		return nil, fmt.Errorf("complete project is uninitialized")
	}
	return c.graph.identities(), nil
}

// Content returns independent typed records in their original persisted order.
func (c CompleteProject) Content(id Identity) (Content, error) {
	if c.graph == nil || c.graph.projects[id] == nil {
		return Content{}, fmt.Errorf("project is outside complete value")
	}
	var data Content
	err := json.Unmarshal([]byte(c.graph.projects[id].data), &data)
	return data, err
}

// Evidence returns only the selected project's immutable interpretation context.
func (c CompleteProject) Evidence(id Identity) (Evidence, error) {
	if c.graph == nil || c.graph.projects[id] == nil {
		return Evidence{}, fmt.Errorf("project is outside complete value")
	}
	return c.graph.projects[id].evidence, nil
}

// Namespaces retains this project's declarations without merging source aliases.
func (c CompleteProject) Namespaces(id Identity) (namespace.Registry, error) {
	data, err := c.Content(id)
	if err != nil {
		return namespace.Registry{}, err
	}
	bindings := make([]namespace.Binding, len(data.Namespaces))
	for i, row := range data.Namespaces {
		bindings[i] = namespace.Binding{Prefix: row.Prefix, Namespace: row.Namespace, Weight: row.Weight, Source: row.Source}
	}
	return namespace.NewRegistry(bindings), nil
}

func (g *graph) addRows(id Identity, p *project, section Section, rows []json.RawMessage) error {
	for _, raw := range rows {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		r := record{data: string(raw), fields: fields}
		if err := r.checkJSON(section); err != nil {
			return err
		}
		key, err := r.key(id, section)
		if err != nil {
			return err
		}
		if _, exists := g.nodes[key]; exists {
			return fmt.Errorf("duplicate row identity in %s", section.Name)
		}
		g.nodes[key] = r
		p.rows[section.Name] = append(p.rows[section.Name], key)
		if section.Kind != "" {
			g.entities[EntityKey{Source: id, Kind: section.Kind, ID: r.text(columnID)}] = key
		}
	}
	return nil
}

func (p *project) addMembership(id Identity, members []Membership) error {
	for _, m := range members {
		key := localKey{m.Kind, m.ID}
		if _, ok := p.members[key]; ok || key.kind == "" || key.id == "" || !m.Source.valid() || m.Source.draft || m.Source == id {
			return fmt.Errorf("invalid or duplicate foreign membership")
		}
		p.members[key] = m.Source
	}
	return nil
}

func (g *graph) checkProjectSources(id Identity, p *project) error {
	seen := map[Identity]bool{}
	for _, source := range p.dependencies {
		if !source.valid() || source.draft || source == id || seen[source] || g.projects[source] == nil {
			return fmt.Errorf("missing, duplicate or invalid source project")
		}
		seen[source] = true
		if !id.draft && p.evidence.contract != g.projects[source].evidence.contract {
			return fmt.Errorf("source content has a different capture contract")
		}
		module, err := p.evidence.descriptors.ProjectModule(source.project)
		if err != nil {
			return err
		}
		if module != g.projects[source].evidence.descriptors.Module() {
			return fmt.Errorf("source module differs from its captured descriptor")
		}
	}
	for entity, source := range p.members {
		if !seen[source] {
			return fmt.Errorf("foreign membership escapes selected project closure")
		}
		if _, own := g.entities[EntityKey{Source: id, Kind: entity.kind, ID: entity.id}]; own {
			return fmt.Errorf("entity has both owned and foreign membership")
		}
		if _, ok := g.entities[EntityKey{Source: source, Kind: entity.kind, ID: entity.id}]; !ok {
			return fmt.Errorf("foreign entity is absent from selected source")
		}
	}
	return nil
}
