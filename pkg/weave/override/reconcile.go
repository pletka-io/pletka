package override

import (
	"cmp"
	"slices"

	"github.com/pletka-io/pletka/pkg/domain"
)

// overridePlan says what ReplaceForEntity does with each desired row.
type overridePlan struct {
	// update[i] is the existing row id desired[i] updates, or 0 to insert.
	update []int64
	// remove lists existing row ids no desired row claimed, ascending.
	remove []int64
}

// matchOverrides pairs desired rows with existing rows so a save keeps
// placement ids stable (example values anchor to them). Rows carrying a
// known id of the same field claim it; id-less rows reuse an unclaimed row with the same
// (field, category, collection) key, pairing duplicates by position.
func matchOverrides(existing, desired []domain.FieldOverride) overridePlan {
	plan := overridePlan{update: make([]int64, len(desired))}
	// fieldOf maps each existing row to its field: an id claims its row only
	// for the same field, since updates never change field_id.
	fieldOf := make(map[int64]string, len(existing))
	for _, e := range existing {
		fieldOf[e.ID] = e.FieldID
	}
	claimed := make(map[int64]bool, len(existing))
	for i, d := range desired {
		if f, known := fieldOf[d.ID]; d.ID > 0 && known && f == d.FieldID && !claimed[d.ID] {
			plan.update[i] = d.ID
			claimed[d.ID] = true
		}
	}

	byKey := map[string][]domain.FieldOverride{}
	for _, e := range existing {
		if !claimed[e.ID] {
			byKey[overrideKey(e)] = append(byKey[overrideKey(e)], e)
		}
	}
	waiting := map[string][]int{}
	for i, d := range desired {
		if plan.update[i] == 0 {
			waiting[overrideKey(d)] = append(waiting[overrideKey(d)], i)
		}
	}
	for key, idxs := range waiting {
		free := byKey[key]
		slices.SortFunc(free, func(a, b domain.FieldOverride) int {
			return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
		})
		slices.SortFunc(idxs, func(a, b int) int {
			return cmp.Or(cmp.Compare(desired[a].Position, desired[b].Position), cmp.Compare(a, b))
		})
		for n := 0; n < len(idxs) && n < len(free); n++ {
			plan.update[idxs[n]] = free[n].ID
			claimed[free[n].ID] = true
		}
	}

	for _, e := range existing {
		if !claimed[e.ID] {
			plan.remove = append(plan.remove, e.ID)
		}
	}
	slices.Sort(plan.remove)
	return plan
}

func overrideKey(o domain.FieldOverride) string {
	return o.FieldID + "\x00" + o.CategoryID + "\x00" + o.PartOfCollectionID
}
