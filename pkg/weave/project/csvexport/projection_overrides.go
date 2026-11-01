package csvexport

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// fieldNames includes only owned and explicitly selected foreign fields. It uses
// the captured source identity, never an unqualified global field lookup.
func fieldNames(v projectsnapshot.CompleteProject, root projectsnapshot.Identity) (map[string]string, error) {
	in, err := v.Input(root)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, r := range in.Content.Fields {
		names[r.ID] = value(r.SemanticID)
	}
	sourceFields := map[projectsnapshot.Identity]map[string]string{}
	for _, m := range in.Membership {
		if m.Kind != "field" {
			continue
		}
		fields, ok := sourceFields[m.Source]
		if !ok {
			c, err := v.Content(m.Source)
			if err != nil {
				return nil, err
			}
			fields = map[string]string{}
			for _, r := range c.Fields {
				fields[r.ID] = value(r.SemanticID)
			}
			sourceFields[m.Source] = fields
		}
		name, ok := fields[m.ID]
		if !ok {
			return nil, fmt.Errorf("CSV source field %s is missing", m.ID)
		}
		names[m.ID] = name
	}
	return names, nil
}

func (e *Export) projectOverrides(v projectsnapshot.CompleteProject, c projectsnapshot.Content) error {
	names, err := fieldNames(v, e.identity)
	if err != nil {
		return err
	}
	refs := overrideRefs(c.OverrideRefs)
	e.overrides = map[string][]OverrideRow{}
	e.baseMeta = map[string]FieldOverrideMeta{}
	e.baseRefs = map[string]string{}
	slices.SortFunc(c.Overrides, func(a, b projectsnapshot.OverrideRecord) int { return cmp.Compare(a.ID, b.ID) })
	for _, r := range c.Overrides {
		if r.EntityType != "" && r.EntityType != "model" && r.EntityType != "collection" {
			return fmt.Errorf("unsupported CSV override context %q", r.EntityType)
		}
		name, ok := names[r.FieldID]
		if !ok {
			return fmt.Errorf("CSV field %s is outside retained membership", r.FieldID)
		}
		row, err := overrideRow(r, name, refs[r.ID])
		if err != nil {
			return err
		}
		e.overrides[r.EntityType] = append(e.overrides[r.EntityType], row)
		if r.EntityType == "" {
			if _, exists := e.baseMeta[r.FieldID]; exists {
				return fmt.Errorf("duplicate CSV base override for %s", r.FieldID)
			}
			e.baseMeta[r.FieldID] = FieldOverrideMeta{CategoryID: value(r.CategoryID), SetValue: value(r.SetValue)}
			e.baseRefs[r.FieldID] = refs[r.ID]
		}
	}
	return nil
}

func overrideRefs(records []projectsnapshot.OverrideRefRecord) map[int64]string {
	slices.SortFunc(records, func(a, b projectsnapshot.OverrideRefRecord) int {
		if c := cmp.Compare(a.Position, b.Position); c != 0 {
			return c
		}
		if c := strings.Compare(a.RefType, b.RefType); c != 0 {
			return c
		}
		return strings.Compare(a.TargetID, b.TargetID)
	})
	parts := map[int64][]string{}
	for _, r := range records {
		parts[r.OverrideID] = append(parts[r.OverrideID], r.SemanticID)
	}
	out := map[int64]string{}
	for id, refs := range parts {
		out[id] = strings.Join(refs, ",")
	}
	return out
}

func overrideRow(r projectsnapshot.OverrideRecord, field, refs string) (OverrideRow, error) {
	out := OverrideRow{EntitySemanticID: r.EntityID, FieldSemanticID: field, Position: int(r.Position), CollectionOrder: int(r.CollectionOrder), CategoryID: value(r.CategoryID), ExpectedValueType: value(r.ExpectedValueType), SetValue: value(r.SetValue), PartOfCollectionID: value(r.PartOfCollectionID), IsRequired: value(r.IsRequired), MinOccurs: int(value(r.MinOccurs)), IsHidden: value(r.IsHidden), Visibility: value(r.Visibility), Refs: refs}
	if r.MaxOccurs != nil {
		max := int(*r.MaxOccurs)
		out.MaxOccurs = &max
	}
	for _, pair := range []struct {
		raw    []byte
		target any
	}{{r.DisplayName, &out.DisplayName}, {r.Description, &out.Description}, {r.CollectionName, &out.CollectionName}} {
		if err := decode(pair.raw, pair.target); err != nil {
			return out, err
		}
	}
	return out, nil
}
