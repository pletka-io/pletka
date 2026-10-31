package draftcapture

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Patch derives ownership only from this committed transaction receipt. It never
// accepts arbitrary current snapshots or entity fragments. Final value/encoding
// equality ensures every captured section and interpretation change survived.
func (r Receipt) Patch() (projectsnapshot.DraftPatch, error) {
	if r.revision < 1 || r.project == "" {
		return projectsnapshot.DraftPatch{}, fmt.Errorf("uninitialized mutation receipt")
	}
	id, err := r.before.Root()
	if err != nil {
		return projectsnapshot.DraftPatch{}, err
	}
	afterID, err := r.after.Root()
	if err != nil {
		return projectsnapshot.DraftPatch{}, err
	}
	if !id.IsDraft() || id != afterID || id.ProjectID() != r.project {
		return projectsnapshot.DraftPatch{}, fmt.Errorf("receipt snapshot ownership differs")
	}
	before, err := r.before.Content(id)
	if err != nil {
		return projectsnapshot.DraftPatch{}, err
	}
	after, err := r.after.Content(id)
	if err != nil {
		return projectsnapshot.DraftPatch{}, err
	}
	oldRows, newRows, err := changedRows(before, after)
	if err != nil {
		return projectsnapshot.DraftPatch{}, err
	}
	prior, err := r.before.DraftContext()
	if err != nil {
		return projectsnapshot.DraftPatch{}, err
	}
	next, err := r.after.DraftContext()
	if err != nil {
		return projectsnapshot.DraftPatch{}, err
	}
	patch := projectsnapshot.DraftPatch{Target: id, Before: oldRows, After: newRows, Context: &projectsnapshot.DraftContextChange{Before: prior, After: next}}
	for _, dep := range next.Dependencies {
		if slices.Contains(prior.Dependencies, dep) {
			continue
		}
		source, err := r.after.Source(dep)
		if err != nil {
			return projectsnapshot.DraftPatch{}, err
		}
		patch.Sources = append(patch.Sources, source)
	}
	merged, err := projectsnapshot.PatchDraft(r.before, patch)
	if err != nil {
		return projectsnapshot.DraftPatch{}, err
	}
	if err := verifyReplay(merged, r.after); err != nil {
		return projectsnapshot.DraftPatch{}, err
	}
	return patch, nil
}

func verifyReplay(merged, expectedValue projectsnapshot.CompleteProject) error {
	actual, err := projectbundle.Encode(merged)
	if err != nil {
		return err
	}
	expected, err := projectbundle.Encode(expectedValue)
	if err != nil {
		return err
	}
	a, err := json.Marshal(actual)
	if err != nil {
		return err
	}
	b, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	if string(a) != string(b) {
		return fmt.Errorf("mutation patch does not reproduce its captured after value")
	}
	return nil
}

func changedRows(before, after projectsnapshot.Content) (projectsnapshot.Content, projectsnapshot.Content, error) {
	old, err := sectionRows(before)
	if err != nil {
		return projectsnapshot.Content{}, projectsnapshot.Content{}, err
	}
	next, err := sectionRows(after)
	if err != nil {
		return projectsnapshot.Content{}, projectsnapshot.Content{}, err
	}
	removed, added := map[string][]json.RawMessage{}, map[string][]json.RawMessage{}
	for _, section := range projectsnapshot.Coverage() {
		a, err := keyedRows(old[section.Name], section)
		if err != nil {
			return projectsnapshot.Content{}, projectsnapshot.Content{}, err
		}
		b, err := keyedRows(next[section.Name], section)
		if err != nil {
			return projectsnapshot.Content{}, projectsnapshot.Content{}, err
		}
		for key, row := range a {
			if string(row) != string(b[key]) {
				removed[section.Name] = append(removed[section.Name], row)
			}
		}
		for key, row := range b {
			if string(row) != string(a[key]) {
				added[section.Name] = append(added[section.Name], row)
			}
		}
	}
	x, err := contentRows(removed)
	if err != nil {
		return projectsnapshot.Content{}, projectsnapshot.Content{}, err
	}
	y, err := contentRows(added)
	return x, y, err
}
func sectionRows(c projectsnapshot.Content) (map[string][]json.RawMessage, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var result map[string][]json.RawMessage
	err = json.Unmarshal(raw, &result)
	return result, err
}
func keyedRows(rows []json.RawMessage, section projectsnapshot.Section) (map[string]json.RawMessage, error) {
	result := make(map[string]json.RawMessage, len(rows))
	for _, row := range rows {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(row, &fields); err != nil {
			return nil, err
		}
		key := make([]json.RawMessage, 0, len(section.Key))
		for _, column := range section.Key {
			key = append(key, fields[column])
		}
		raw, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		result[string(raw)] = row
	}
	return result, nil
}
func contentRows(rows map[string][]json.RawMessage) (projectsnapshot.Content, error) {
	for name := range rows {
		slices.SortFunc(rows[name], func(a, b json.RawMessage) int {
			if string(a) < string(b) {
				return -1
			}
			if string(a) > string(b) {
				return 1
			}
			return 0
		})
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return projectsnapshot.Content{}, err
	}
	var c projectsnapshot.Content
	err = json.Unmarshal(raw, &c)
	return c, err
}
