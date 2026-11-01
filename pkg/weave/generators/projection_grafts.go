package generators

import (
	"fmt"
	"slices"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Grafts may use already-present placements or explicit collection references.
// A global anchor search would add an unpinned dependency to a delayed export.
func (p *projection) grafts(fragment projectsnapshot.EntityFragment, view projectsnapshot.ContextView, loaded map[string]*domain.Collection, constraints map[constraintKey]fieldConstraint, opts Options) (map[string]ExtraAnchoredCollection, error) {
	extra := map[string]ExtraAnchoredCollection{}
	if !opts.ComposeCollections {
		return extra, nil
	}
	queue := []projectsnapshot.ContextView{view}
	seen := map[projectsnapshot.EntityKey]bool{}
	anchors := map[string]bool{}
	stubs := []string{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, group := range current.Groups {
			fields := make([]domain.ResolvedField, 0, len(group.Fields))
			for _, f := range group.Fields {
				rf, choices, err := p.resolved(f)
				if err != nil {
					return nil, err
				}
				constraints[constraintKey{rf.OverrideProjectID, rf.OverrideID}] = choices
				fields = append(fields, rf)
				if isCollectionStub(rf) {
					stubs = append(stubs, terminalClassURI(rf.PathElements))
				}
				for _, ref := range f.Refs {
					if ref.Target.Kind != string(EntityCollection) || seen[ref.Target] {
						continue
					}
					seen[ref.Target] = true
					if loaded[ref.Target.ID] != nil {
						continue
					}
					definition, err := fragment.ComposeContext(ref.Target.Source, string(EntityCollection), ref.Target.ID)
					if err != nil {
						return nil, err
					}
					groups, _, _, err := p.groups(definition)
					if err != nil {
						return nil, err
					}
					coll, err := p.collection(ref.Target)
					if err != nil {
						return nil, err
					}
					leaves := flattenGroups(groups)
					prefix := computeSharedPathPrefix(leaves)
					extra[coll.ID] = ExtraAnchoredCollection{Collection: &coll, Fields: leaves, SharedPathPrefix: prefix}
					if len(prefix) > 0 {
						anchors[prefix[len(prefix)-1].URI] = true
					}
					queue = append(queue, definition)
				}
			}
			if group.Collection != nil {
				prefix := computeSharedPathPrefix(fields)
				if len(prefix) > 0 {
					anchors[prefix[len(prefix)-1].URI] = true
				}
			}
		}
	}
	slices.Sort(stubs)
	for _, anchor := range stubs {
		if anchor == "" || !anchors[anchor] {
			return nil, fmt.Errorf("collection composition has no retained target for anchor %q; add an explicit collection reference", anchor)
		}
	}
	return extra, nil
}
