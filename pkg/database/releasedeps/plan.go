package releasedeps

// Source is one concrete project release, never a draft or moving selector.
type Source struct {
	ProjectID string
	Version   string
}

// Plan is an all-or-error dependency selection. Its private maps cannot be
// changed by callers. An empty plan grants no source capability.
type Plan struct {
	pins     map[string]string
	entities map[entityKey]string
}

// Sources returns the complete closure in deterministic project order.
func (p Plan) Sources() []Source {
	out := make([]Source, 0, len(p.pins))
	for _, project := range sortedProjects(p.pins) {
		out = append(out, Source{ProjectID: project, Version: p.pins[project]})
	}
	return out
}

// Project resolves only a member of this plan's project closure.
func (p Plan) Project(project string) (Source, bool) {
	version, ok := p.pins[project]
	if !ok {
		return Source{}, false
	}
	return Source{ProjectID: project, Version: version}, true
}

// Entity resolves direct foreign membership; own content keeps its root session.
func (p Plan) Entity(kind, id string) (Source, bool) {
	project, ok := p.entities[entityKey{kind, id}]
	if !ok {
		return Source{}, false
	}
	return p.Project(project)
}
