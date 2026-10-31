package projectsnapshot

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/releasecontent"
)

// Input returns a detached inventory for portable encoding. It conveys no
// database provenance or authorization. Only complete values expose this method;
// fragments cannot be passed off as portable whole-project content.
func (c CompleteProject) Input(id Identity) (ProjectInput, error) {
	content, err := c.Content(id)
	if err != nil {
		return ProjectInput{}, err
	}
	p := c.graph.projects[id]
	in := ProjectInput{Content: content, Evidence: p.evidence,
		Counts: map[string]int64{}, Dependencies: slices.Clone(p.dependencies),
		Membership: make([]Membership, 0, len(p.members))}
	for _, section := range Coverage() {
		in.Counts[section.Name] = int64(len(p.rows[section.Name]))
	}
	slices.SortFunc(in.Dependencies, func(a, b Identity) int { return strings.Compare(a.project, b.project) })
	for key, source := range p.members {
		in.Membership = append(in.Membership, Membership{Kind: key.kind, ID: key.id, Source: source})
	}
	slices.SortFunc(in.Membership, func(a, b Membership) int {
		if cmp := strings.Compare(a.Kind, b.Kind); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ID, b.ID)
	})
	return in, nil
}

// Source returns an already retained immutable source and its complete closure.
// It cannot discover, retarget or promote a draft into a release.
func (c CompleteProject) Source(id Identity) (CompleteProject, error) {
	if id.IsDraft() {
		return CompleteProject{}, fmt.Errorf("source must be an immutable release")
	}
	root, err := c.Input(id)
	if err != nil {
		return CompleteProject{}, err
	}
	sources := make([]ProjectInput, 0, len(root.Dependencies))
	for _, dep := range root.Dependencies {
		in, err := c.Input(dep)
		if err != nil {
			return CompleteProject{}, err
		}
		sources = append(sources, in)
	}
	return NewComplete(root, sources)
}

// CaptureSections returns a copy of the original storage receipt inventory.
// Its hashes describe PostgreSQL rows, not portable file bytes. Drafts have no
// receipt; parsing a receipt never authenticates imported content as a local cut.
func (e Evidence) CaptureSections() (map[string]releasecontent.Section, error) {
	if !e.id.valid() || e.contract == 0 {
		return nil, fmt.Errorf("capture receipt is unavailable")
	}
	sections := make(map[string]releasecontent.Section)
	for _, name := range releasecontent.RequiredSections() {
		if e.contract == 1 {
			sections[name], _ = e.verified.Section(name)
		} else {
			sections[name], _ = e.legacy.Section(name)
		}
	}
	return sections, nil
}
