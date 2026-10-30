package releasedeps

import (
	"slices"
	"strings"
)

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

// Member is one direct foreign entity and its immutable selected source.
type Member struct {
	Kind, ID string
	Source   Source
}

// Membership returns detached direct membership in deterministic kind/ID order.
func (p Plan) Membership() []Member {
	out := make([]Member, 0, len(p.entities))
	for key, project := range p.entities {
		out = append(out, Member{Kind: key.kind, ID: key.id, Source: Source{ProjectID: project, Version: p.pins[project]}})
	}
	slices.SortFunc(out, func(a, b Member) int {
		if order := strings.Compare(a.Kind, b.Kind); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}
