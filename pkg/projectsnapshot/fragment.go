package projectsnapshot

import (
	"encoding/json"
	"fmt"
)

// EntityFragment is an explicitly partial project view with a closed structural
// entity graph. It cannot be converted to CompleteProject. Evidence retains its
// original complete interpretation context; that does not make these rows complete.
type EntityFragment struct {
	graph    *graph
	root     EntityKey
	selected map[nodeKey]bool
	projects map[Identity]bool
}

// NewFragment validates explicitly paged subsets and returns only their closed
// entity graph. Counts describe loaded subsets, never complete receipt counts.
// Only assembly infrastructure may use this constructor. Interpretation contexts
// and source inventories remain complete even when structural rows are partial.
func NewFragment(root ProjectInput, sources []ProjectInput, kind, id string) (EntityFragment, error) {
	g, err := buildGraph(root, sources, true)
	if err != nil {
		return EntityFragment{}, err
	}
	return g.entity(kind, id)
}

// Consumer returns the selection whose contextual overrides are being viewed.
func (f EntityFragment) Consumer() (Identity, error) {
	if f.graph == nil {
		return Identity{}, fmt.Errorf("entity fragment is uninitialized")
	}
	return f.graph.root, nil
}

// Entity selects a closed fragment through the root's recorded membership. It
// accepts no source version override. This initial value-level operation uses an
// already complete input; bounded database assembly belongs to the assembler.
func (c CompleteProject) Entity(kind, id string) (EntityFragment, error) {
	return c.graph.entity(kind, id)
}

func (g *graph) entity(kind, id string) (EntityFragment, error) {
	if g == nil || !selectable(kind) {
		return EntityFragment{}, fmt.Errorf("entity fragment requires an initialized graph and non-project root")
	}
	root, err := g.resolve(g.root, kind, id)
	if err != nil {
		return EntityFragment{}, err
	}
	f := EntityFragment{graph: g, root: EntityKey{Source: root.source, Kind: kind, ID: id}, selected: map[nodeKey]bool{}, projects: map[Identity]bool{}}
	queue := []nodeKey{root}
	queue = f.addContext(g.root, queue)
	for len(queue) > 0 {
		for len(queue) > 0 {
			key := queue[0]
			queue = queue[1:]
			if f.selected[key] {
				continue
			}
			f.selected[key] = true
			queue = f.addContext(key.source, queue)
			queue = append(queue, f.graph.edges[key]...)
		}
		queue, err = f.contextSupplements()
		if err != nil {
			return EntityFragment{}, err
		}
	}
	return f, nil
}

// Consumer base overrides are not children of a foreign field globally. Add
// them only for selected fields visible in contexts this fragment actually uses.
func (f *EntityFragment) contextSupplements() ([]nodeKey, error) {
	queue := []nodeKey{}
	for id := range f.projects {

		for _, section := range []string{sectionAdoptions, "weave_entity_forks_archive"} {
			for _, key := range f.graph.projects[id].rows[section] {
				if f.selected[key] {
					continue
				}
				r := f.graph.nodes[key]
				entityID := r.text(columnForkEntityID)
				if section == sectionAdoptions {
					if r.text(columnContextEntityType) != "" && r.text(columnContextEntityType) != kindProject {
						continue
					}
					entityID = r.text(columnSourceEntityID)
				}
				entity, err := f.graph.resolve(id, r.text(columnEntityType), entityID)
				if err == nil && f.selected[entity] {
					queue = append(queue, key)
				}
			}
		}
		for _, key := range f.graph.projects[id].rows[sectionFieldOverrides] {
			r := f.graph.nodes[key]
			if f.selected[key] || r.text(columnEntityType) != "" {
				continue
			}
			field, err := f.graph.resolve(id, kindField, r.text(columnFieldID))
			if err != nil {
				return nil, err
			}
			if f.selected[field] {
				queue = append(queue, key)
			}
		}
	}
	return queue, nil
}

func (f *EntityFragment) addContext(id Identity, queue []nodeKey) []nodeKey {
	if f.projects[id] {
		return queue
	}
	f.projects[id] = true
	for _, section := range []string{sectionProjects, sectionReleaseNamespaceBindings, "weave_project_actors_archive", "weave_project_attributions_archive", "weave_project_ontology_versions_archive"} {
		queue = append(queue, f.graph.projects[id].rows[section]...)
	}
	return queue
}

// Root returns the actual source identity of the requested entity.
func (f EntityFragment) Root() (EntityKey, error) {
	if f.graph == nil {
		return EntityKey{}, fmt.Errorf("entity fragment is uninitialized")
	}
	return f.root, nil
}

// Projects returns only projects needed by this fragment, in stable order.
func (f EntityFragment) Projects() ([]Identity, error) {
	if f.graph == nil {
		return nil, fmt.Errorf("entity fragment is uninitialized")
	}
	ids := []Identity{}
	for _, id := range f.graph.identities() {
		if f.projects[id] {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// Content returns detached selected rows, preserving their original relative
// order. All sections are explicitly present, but these are fragment counts.
func (f EntityFragment) Content(id Identity) (Content, error) {
	if f.graph == nil || !f.projects[id] {
		return Content{}, fmt.Errorf("project is outside entity fragment")
	}
	sections := map[string][]json.RawMessage{}
	for _, section := range Coverage() {
		rows := []json.RawMessage{}
		for _, key := range f.graph.projects[id].rows[section.Name] {
			if f.selected[key] {
				rows = append(rows, json.RawMessage(f.graph.nodes[key].data))
			}
		}
		sections[section.Name] = rows
	}
	raw, err := json.Marshal(sections)
	if err != nil {
		return Content{}, err
	}
	var content Content
	err = json.Unmarshal(raw, &content)
	return content, err
}

// Evidence retains the selected project's original interpretation context. It
// cannot be used to claim that this fragment contains every captured row.
func (f EntityFragment) Evidence(id Identity) (Evidence, error) {
	if f.graph == nil || !f.projects[id] {
		return Evidence{}, fmt.Errorf("project is outside entity fragment")
	}
	return f.graph.projects[id].evidence, nil
}
