package projectsnapshot

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// DraftPatch names exactly the owned rows changed by one operation. Before and
// After contain row images, not entity closures or whole-section replacements.
// A row only in Before is deleted; only in After is inserted; in both is updated.
// Nil/empty sections change nothing. Every existing row must match Before exactly,
// including SQL nulls and authored JSON bytes. No current-state read is performed.
//
// Context is a separate explicit compare-and-swap for interpretation and source
// selection. Sources supplies complete immutable releases needed by a new pin;
// existing source bodies can never be replaced. The write boundary must capture
// these inputs atomically with the mutation. This value does not prove attribution
// or authorize a caller to derive a patch from today's database or a fragment.
type DraftPatch struct {
	Target  Identity
	Before  Content
	After   Content
	Context *DraftContextChange
	Sources []CompleteProject
}

// DraftContext is the root's complete interpretation and dependency inventory.
// It contains no structural rows. A context change is explicit because an actor,
// publication policy or source-pin edit can affect more than one entity.
type DraftContext struct {
	Evidence     Evidence
	Dependencies []Identity
	Membership   []Membership
}

// DraftContextChange requires an exact prior context, never a live fallback.
type DraftContextChange struct{ Before, After DraftContext }

// DraftContext returns detached inventories for an explicit draft baseline.
func (c CompleteProject) DraftContext() (DraftContext, error) {
	id, err := c.Root()
	if err != nil {
		return DraftContext{}, err
	}
	if !id.IsDraft() {
		return DraftContext{}, fmt.Errorf("draft context requires a draft baseline")
	}
	in, err := c.Input(id)
	if err != nil {
		return DraftContext{}, err
	}
	return DraftContext{in.Evidence, in.Dependencies, in.Membership}, nil
}

// PatchDraft applies an attributed row/context delta to a validated complete
// baseline. Referenced bodies remain unchanged unless explicitly named as rows.
// Cascades and child deletions must be supplied; missing references fail complete
// graph validation. Failure leaves the baseline intact and returns no value.
// Replays with stale Before images fail; delivery deduplication belongs to the
// writer's durable change-set checkpoint, not a heuristic here.
func PatchDraft(base CompleteProject, patch DraftPatch) (CompleteProject, error) {
	id, err := base.Root()
	if err != nil {
		return CompleteProject{}, err
	}
	if !id.IsDraft() || patch.Target != id {
		return CompleteProject{}, fmt.Errorf("patch target must match the explicit draft baseline")
	}
	root, err := base.Input(id)
	if err != nil {
		return CompleteProject{}, err
	}
	root.Content, root.Counts, err = patchRows(base, patch)
	if err != nil {
		return CompleteProject{}, err
	}
	if err := patchContext(base, &root, patch.Context); err != nil {
		return CompleteProject{}, err
	}
	sources, err := patchSources(base, root.Dependencies, patch.Sources)
	if err != nil {
		return CompleteProject{}, err
	}
	result, err := NewComplete(root, sources)
	if err != nil {
		return CompleteProject{}, fmt.Errorf("patched draft is invalid: %w", err)
	}
	return result, nil
}

func patchRows(base CompleteProject, patch DraftPatch) (Content, map[string]int64, error) {
	before, err := patchRecords(patch.Target, patch.Before)
	if err != nil {
		return Content{}, nil, fmt.Errorf("patch before: %w", err)
	}
	after, err := patchRecords(patch.Target, patch.After)
	if err != nil {
		return Content{}, nil, fmt.Errorf("patch after: %w", err)
	}
	for key, row := range before {
		old, exists := base.graph.nodes[key]
		if !exists || old.data != row.data {
			return Content{}, nil, fmt.Errorf("patch before row differs from baseline in %s (%s)", key.section, key.key)
		}
	}
	for key := range after {
		if _, exists := base.graph.nodes[key]; exists {
			if _, compared := before[key]; !compared {
				return Content{}, nil, fmt.Errorf("patch would overwrite an unclaimed row in %s (%s)", key.section, key.key)
			}
		}
	}
	sections := make(map[string][]json.RawMessage, len(Coverage()))
	counts := make(map[string]int64, len(Coverage()))
	for _, section := range Coverage() {
		rows := []json.RawMessage{}
		for _, key := range base.graph.projects[patch.Target].rows[section.Name] {
			row := base.graph.nodes[key]
			if _, changed := before[key]; changed {
				var kept bool
				row, kept = after[key]
				if !kept {
					continue
				}
			}
			rows = append(rows, json.RawMessage(row.data))
		}
		inserted := []nodeKey{}
		for key := range after {
			if _, existed := before[key]; !existed && key.section == section.Name {
				inserted = append(inserted, key)
			}
		}
		slices.SortFunc(inserted, func(a, b nodeKey) int { return strings.Compare(a.key, b.key) })
		for _, key := range inserted {
			rows = append(rows, json.RawMessage(after[key].data))
		}
		sections[section.Name], counts[section.Name] = rows, int64(len(rows))
	}
	raw, err := json.Marshal(sections)
	if err != nil {
		return Content{}, nil, err
	}
	var content Content
	err = json.Unmarshal(raw, &content)
	return content, counts, err
}

func patchRecords(id Identity, content Content) (map[nodeKey]record, error) {
	raw, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	var sections map[string][]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		return nil, err
	}
	out := map[nodeKey]record{}
	for _, section := range Coverage() {
		for _, raw := range sections[section.Name] {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				return nil, err
			}
			row := record{data: string(raw), fields: fields}
			if err := row.checkJSON(section); err != nil {
				return nil, err
			}
			key, err := row.key(id, section)
			if err != nil {
				return nil, err
			}
			if _, duplicate := out[key]; duplicate {
				return nil, fmt.Errorf("duplicate patch row in %s", section.Name)
			}
			out[key] = row
		}
	}
	return out, nil
}

func patchContext(base CompleteProject, root *ProjectInput, change *DraftContextChange) error {
	if change == nil {
		return nil
	}
	prior, err := base.DraftContext()
	if err != nil {
		return err
	}
	// Compare all retained interpretation inputs, including unused descriptors.
	// Exact order is intentional: callers capture Before from the baseline, not
	// by reconstructing an approximately equivalent current context.
	if !reflect.DeepEqual(prior, change.Before) {
		return fmt.Errorf("patch before context differs from baseline")
	}
	next := change.After
	if next.Evidence.id != root.Evidence.id || next.Evidence.contract != 0 || next.Dependencies == nil || next.Membership == nil {
		return fmt.Errorf("patch after context is unloaded or not the selected draft")
	}
	root.Evidence, root.Dependencies, root.Membership = next.Evidence, next.Dependencies, next.Membership
	return nil
}

func patchSources(base CompleteProject, deps []Identity, additions []CompleteProject) ([]ProjectInput, error) {
	available := make(map[Identity]ProjectInput)
	for _, id := range base.graph.identities() {
		if id == base.graph.root {
			continue
		}
		in, err := base.Input(id)
		if err != nil {
			return nil, err
		}
		available[id] = in
	}
	selected := make(map[Identity]bool, len(deps))
	for _, id := range deps {
		selected[id] = true
	}
	for _, source := range additions {
		id, err := source.Root()
		if err != nil || id.IsDraft() {
			return nil, fmt.Errorf("patch source must be a complete immutable release")
		}
		for _, id := range source.graph.identities() {
			if !selected[id] {
				return nil, fmt.Errorf("patch supplies an unselected source %s@%s", id.project, id.version)
			}
			in, err := source.Input(id)
			if err != nil {
				return nil, err
			}
			if old, exists := available[id]; exists && !reflect.DeepEqual(old, in) {
				return nil, fmt.Errorf("patch cannot replace immutable source %s@%s", id.project, id.version)
			}
			available[id] = in
		}
	}
	out := make([]ProjectInput, 0, len(deps))
	for _, id := range deps {
		in, ok := available[id]
		if !ok {
			return nil, fmt.Errorf("patch lacks complete source %s@%s", id.project, id.version)
		}
		out = append(out, in)
	}
	return out, nil
}
