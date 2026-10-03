package conformance

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// The ratchet. A read in a versioned slice either names the version it reads
// at or appears in the allowlist with a reason. Anything else fails here.
//
// The default is DENY: a method this test has never seen fails, rather than
// passing because it is unknown. A scanner that only checks what it already
// knows about is a list, not a ratchet.
func TestVersionedSliceReadsAreScopedOrAllowlisted(t *testing.T) {
	root := repoRoot(t)
	var offenders []string

	for _, s := range VersionedSlices {
		methods, err := scanSliceReads(root, s)
		if err != nil {
			t.Fatalf("scan %s: %v", s.Name, err)
		}
		for _, m := range methods {
			if m.HasScope {
				continue
			}
			if _, exempt := UnscopedReadAllowlist[m.Key()]; exempt {
				continue
			}
			offenders = append(offenders, fmt.Sprintf("  %s/service.go:%d  %s", s.Pkg, m.Line, m.Key()))
		}
	}

	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Fatalf("read methods in versioned slices that neither take auth.ReadScope "+
			"nor appear in UnscopedReadAllowlist:\n%s\n\n"+
			"A read in a versioned slice names the version it reads at. Either give it "+
			"an auth.ReadScope parameter and route it through the slice's scoped reader, "+
			"or add it to the allowlist with a reason saying why it is correct unscoped.",
			strings.Join(offenders, "\n"))
	}
}

// As slices convert, this inverts: from "this read has a version branch" to
// "this slice has no unscoped read". A slice with no allowlist entries left
// has converted, and nothing may add one back without this failing first.
func TestConvertedSlicesStayConverted(t *testing.T) {
	withEntries := map[string]bool{}
	for key := range UnscopedReadAllowlist {
		withEntries[strings.SplitN(key, ".", 2)[0]] = true
	}
	root := repoRoot(t)

	for _, s := range VersionedSlices {
		if withEntries[s.Name] {
			continue // still converting; the ratchet above covers it
		}
		methods, err := scanSliceReads(root, s)
		if err != nil {
			t.Fatalf("scan %s: %v", s.Name, err)
		}
		for _, m := range methods {
			if !m.HasScope {
				t.Errorf("%s/service.go:%d %s is unscoped, but %s has no allowlist "+
					"entries and is therefore converted. A converted slice does not "+
					"grow a new unscoped read.", s.Pkg, m.Line, m.Key(), s.Name)
			}
		}
	}
}
