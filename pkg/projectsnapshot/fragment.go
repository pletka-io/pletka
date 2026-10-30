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

// Entity selects a closed fragment through the root's recorded membership. It
// accepts no source version override. This initial value-level operation uses an
// already complete input; bounded database assembly belongs to the assembler.
func (c CompleteProject) Entity(kind, id string) (EntityFragment, error) {
	if c.graph == nil || kind == kindProject {
		return EntityFragment{}, fmt.Errorf("entity fragment requires an initialized graph and non-project root")
	}
	root, err := c.graph.resolve(c.graph.root, kind, id)
	if err != nil {
		return EntityFragment{}, err
	}
	f := EntityFragment{graph: c.graph, root: EntityKey{Source: root.source, Kind: kind, ID: id}, selected: map[nodeKey]bool{}, projects: map[Identity]bool{}}
	queue := []nodeKey{root}
	queue = f.addContext(c.graph.root, queue)
	// Base adoption and fork receipts are evidence attached to this requested
	// entity. Contextual receipts are reached through their selected model/collection.
	for _, section := range []string{sectionAdoptions, "weave_entity_forks_archive"} {
		for _, key := range c.graph.projects[c.graph.root].rows[section] {
			r := c.graph.nodes[key]
			entityID := r.text(columnForkEntityID)
			if section == sectionAdoptions {
				if r.text(columnContextEntityType) != "" {
					continue
				}
				entityID = r.text(columnSourceEntityID)
			}
			if r.text(columnEntityType) == kind && entityID == id {
				queue = append(queue, key)
			}
		}
	}
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
		queue, err = f.fieldDefaults()
		if err != nil {
			return EntityFragment{}, err
		}
	}
	return f, nil
}

// Consumer base overrides are not children of a foreign field globally. Add
// them only for selected fields visible in contexts this fragment actually uses.
func (f *EntityFragment) fieldDefaults() ([]nodeKey, error) {
	queue := []nodeKey{}
	for id := range f.projects {
		for _, key := range f.graph.projects[id].rows["weave_field_overrides_archive"] {
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
