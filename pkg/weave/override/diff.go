package override

import (
	"reflect"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Diff describes the changes between two []FieldOverride snapshots for
// the same (entityType, entityID) pair. It is what the Service emits
// alongside the Store.ReplaceForEntity write so the changelog can
// record per-row Create/Update/Delete entries instead of one opaque
// "replaced N rows" event.
//
// Identity is keyed on the bigserial ID, with one exception: an id that
// names an existing row of a DIFFERENT field is not a match (see
// matchOverrides' same rule in reconcile.go — an id only claims a row when
// the field agrees). ReplaceForEntity deletes and re-inserts in that case,
// so ComputeDiff reports Removed+Added, never an Update naming a row that
// no longer exists.
//
// Existing rows arrive with non-zero IDs (assigned by Store.Create on
// first insert and kept by ReplaceForEntity, which updates matched rows in
// place). Diff is computed BEFORE the replace. IDs in the desired slice
// usually come from the client's last known view of the row, but can also
// be filled in by the service's own key-based match (SaveForEntity calls
// matchOverrides and stamps its result onto desired before diffing) —
// either way, ComputeDiff just reads whatever id ended up on the row.
//
// Rows with ID==0 in the desired slice are treated as Added — these
// are new draft rows the user just created (e.g., dropped a field via
// the search widget). Same field_id can appear N times in the same
// (entity_type, entity_id) scope when the user duplicates a field
// across categories within a single model — keying on FieldID would
// collapse those, so we key on ID.
//
// Position-only changes are recorded as Changed (not Removed+Added).
type Diff struct {
	Added   []domain.FieldOverride
	Removed []domain.FieldOverride
	Changed []DiffPair

	// AddedIdx holds, for each entry in Added at the same position, the
	// index into the desired slice ComputeDiff was given that produced it.
	// SaveForEntity uses this to patch the row's real post-insert id onto
	// Added[i] once ReplaceForEntity assigns one — reading the same
	// classification ComputeDiff already made, rather than a second,
	// hand-duplicated predicate that could drift from this one (fix round
	// 1, finding 6: two independently-maintained predicates agreeing only
	// by construction is exactly the kind of thing that stops agreeing).
	AddedIdx []int
}

// DiffPair holds the before/after state for a single Changed override.
type DiffPair struct {
	Before domain.FieldOverride
	After  domain.FieldOverride
}

// Empty reports whether the diff has no Added/Removed/Changed rows.
func (d Diff) Empty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// ComputeDiff returns the Diff between the existing override set
// (loaded from Store.ListForEntity) and the desired set submitted by
// the caller. Existing rows match by bigserial ID, provided the field
// agrees too (see the doc comment above); desired rows with ID==0, or
// with an id that doesn't match on those terms, are Added; existing rows
// no desired row claimed are Removed.
//
// Timestamps and StagingID are ignored when comparing rows for
// equality — see semanticallyEqual.
func ComputeDiff(existing, desired []domain.FieldOverride) Diff {
	oldByID := make(map[int64]domain.FieldOverride, len(existing))
	for _, o := range existing {
		if o.ID == 0 {
			// Defensive: existing rows should always have an assigned
			// ID. Skip rather than panic; the row will fall through to
			// Added on the new side.
			continue
		}
		oldByID[o.ID] = o
	}

	var diff Diff
	seenNew := make(map[int64]struct{}, len(desired))
	for i := range desired {
		n := desired[i]
		if n.ID == 0 {
			diff.Added = append(diff.Added, n)
			diff.AddedIdx = append(diff.AddedIdx, i)
			continue
		}
		o, ok := oldByID[n.ID]
		if !ok {
			// Caller sent an ID that doesn't exist in the existing set.
			// Treat as Added — ReplaceForEntity ignores ids it does not
			// know, so the stale ID is harmless.
			diff.Added = append(diff.Added, n)
			diff.AddedIdx = append(diff.AddedIdx, i)
			continue
		}
		if o.FieldID != n.FieldID {
			// Same id, different field: matchOverrides never lets this id
			// claim that row (an id only claims a row when the field
			// agrees — see reconcile.go), so ReplaceForEntity deletes the
			// old row and inserts a new one instead of updating it in
			// place. Report the same thing: leave the existing row
			// unclaimed (it falls through to Removed below) and treat n
			// as Added, rather than pairing them into a false Update that
			// would name a row the database no longer has.
			diff.Added = append(diff.Added, n)
			diff.AddedIdx = append(diff.AddedIdx, i)
			continue
		}
		seenNew[n.ID] = struct{}{}
		if !semanticallyEqual(o, n) {
			diff.Changed = append(diff.Changed, DiffPair{Before: o, After: n})
		}
	}
	for id, o := range oldByID {
		if _, ok := seenNew[id]; !ok {
			diff.Removed = append(diff.Removed, o)
		}
	}
	return diff
}

// semanticallyEqual returns true when two overrides describe the same
// state, ignoring fields that are write-side noise: ID, StagingID,
// CreatedAt, UpdatedAt. EntityType/EntityID/ProjectID always match by
// construction (caller scopes the diff), so leaving them in the
// comparison is harmless.
func semanticallyEqual(a, b domain.FieldOverride) bool {
	var zero time.Time
	a.ID, b.ID = 0, 0
	a.StagingID, b.StagingID = nil, nil
	a.CreatedAt, b.CreatedAt = zero, zero
	a.UpdatedAt, b.UpdatedAt = zero, zero
	return reflect.DeepEqual(a, b)
}
