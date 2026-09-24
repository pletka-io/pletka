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

// stampMatchedIDs writes plan.update[i] onto desired[i].ID wherever
// matchOverrides claimed an existing row for it, so a kept row carries its
// real id by the time ComputeDiff runs — reported as Changed, not
// Removed+Added. This matters for callers that don't already round-trip
// ids on every row: the raw PUT …/models/{id}/overrides and
// …/collections/{id}/overrides routes, which decode straight from client
// JSON, and the ops/MCP/CLI writers. (The editor itself is not one of
// these — it mints negative temporary ids for new rows and round-trips the
// real positive id for every persisted row it edits, so it never sent
// ID==0 for a kept row; only the second bug this task fixed, the "0"
// entity id on create entries, affected it.) Desired rows matchOverrides
// left unclaimed (plan.update[i]==0) are untouched: a genuinely new row
// keeps ID==0, and a row whose sent id doesn't match anything (wrong
// field, or unknown entirely) keeps that id, which ComputeDiff already
// treats as Added (see its doc comment).
//
// existing is a pre-transaction snapshot (SaveForEntity loads it before
// ReplaceForEntity opens its own tx): under concurrent saves of the same
// entity, matching against a snapshot that's gone stale by write time
// means a row that would previously have been deleted and reinserted is
// now updated in place instead — the placement id and its example values
// survive a race that used to lose them. That's a deliberate improvement,
// not a regression, but it is a lost-update semantics change on the
// unlocked PUT …/overrides routes; tracked in the platform backlog, not
// addressed here.
func stampMatchedIDs(desired []domain.FieldOverride, plan overridePlan) {
	// plan.update is built one entry per desired row, so the lengths agree;
	// the explicit bound keeps that a local fact rather than a caller's
	// promise, and lets a static analyzer see it.
	for i := 0; i < len(plan.update) && i < len(desired); i++ {
		if id := plan.update[i]; id != 0 {
			desired[i].ID = id
		}
	}
}
