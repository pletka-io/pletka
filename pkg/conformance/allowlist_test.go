package conformance

import (
	"strings"
	"testing"
)

// An entry naming a method that no longer exists grants a pass to nothing
// and makes the list look like it is shrinking when it is not.
func TestAllowlistHasNoStaleEntries(t *testing.T) {
	live := map[string]bool{}
	for _, s := range VersionedSlices {
		methods, err := scanSliceReads(repoRoot(t), s)
		if err != nil {
			t.Fatalf("scan %s: %v", s.Name, err)
		}
		for _, m := range methods {
			live[m.Key()] = true
		}
	}
	for key := range UnscopedReadAllowlist {
		if !live[key] {
			t.Errorf("allowlist entry %q names no read method that exists; "+
				"the method was renamed, deleted, or converted -- remove the entry", key)
		}
	}
}

// An entry without a reason is an exemption nobody can review.
func TestEveryAllowlistEntryGivesAReason(t *testing.T) {
	for key, ex := range UnscopedReadAllowlist {
		if len(strings.TrimSpace(ex.Reason)) < 30 {
			t.Errorf("%s: reason %q is too short to be a reason", key, ex.Reason)
		}
	}
}

// The spec is explicit: a permanent exemption covers the slice, never its
// callers. Without the obligation written down, the entry reads as blanket
// permission -- which is the #3616 error.
func TestPermanentEntriesCarryTheirCallerObligation(t *testing.T) {
	for key, ex := range UnscopedReadAllowlist {
		if ex.Permanent && strings.TrimSpace(ex.CallerObligation) == "" {
			t.Errorf("%s is permanent but names no caller obligation; "+
				"an exemption covers the slice, never its callers", key)
		}
	}
}

// A temporary entry is a promise that something will remove it.
func TestTemporaryEntriesNameTheConvertingSlice(t *testing.T) {
	for key, ex := range UnscopedReadAllowlist {
		if !ex.Permanent && strings.TrimSpace(ex.ConvertsWith) == "" {
			t.Errorf("%s is temporary but names nothing that will convert it; "+
				"a temporary exemption with no owner is a permanent one that has not admitted it", key)
		}
	}
}

// A permanent entry must not also claim something will convert it: the two
// fields contradict, and a reader cannot tell which the author meant.
func TestPermanentEntriesDoNotAlsoClaimAConversion(t *testing.T) {
	for key, ex := range UnscopedReadAllowlist {
		if ex.Permanent && strings.TrimSpace(ex.ConvertsWith) != "" {
			t.Errorf("%s is permanent but also names a converter (%q); pick one",
				key, ex.ConvertsWith)
		}
	}
}

// TestNoEntryWaitsOnTheCollectionConversion inverts the check for this slice,
// as Part 4 describes: from "this read has a version branch" to "this slice
// has no unscoped read it is still waiting to convert".
//
// A collection.* entry may survive, but only as a permanent exemption with a
// caller obligation, or as a temporary one waiting on a DIFFERENT slice
// (View and LookupView return resolved fields, which convert with the field
// slice). Nothing, in any slice, may still name the collection conversion as
// what will remove it -- that work is done.
func TestNoEntryWaitsOnTheCollectionConversion(t *testing.T) {
	for key, ex := range UnscopedReadAllowlist {
		if strings.Contains(ex.ConvertsWith, "collection slice conversion") {
			t.Errorf("%s still waits on the collection slice conversion, which is done", key)
		}
		if !strings.HasPrefix(key, "collection.") {
			continue
		}
		if ex.Permanent && strings.TrimSpace(ex.CallerObligation) == "" {
			t.Errorf("%s is permanent with no caller obligation; an exemption covers the slice, never its callers", key)
		}
	}
}
