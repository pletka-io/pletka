package search

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParsePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		wantLocalNames []string
		wantPrefixes   []string
	}{
		{
			name:           "empty input",
			input:          "",
			wantLocalNames: []string{},
			wantPrefixes:   []string{},
		},
		{
			name:           "bare single element",
			input:          "P1",
			wantLocalNames: []string{"P1"},
			wantPrefixes:   []string{""},
		},
		{
			name:           "prefix single element",
			input:          "crm:P1",
			wantLocalNames: []string{"P1"},
			wantPrefixes:   []string{"crm"},
		},
		{
			name:           "bare multi element",
			input:          "P1->E33",
			wantLocalNames: []string{"P1", "E33"},
			wantPrefixes:   []string{"", ""},
		},
		{
			name:           "prefix multi element",
			input:          "crm:P1->crm:E33",
			wantLocalNames: []string{"P1", "E33"},
			wantPrefixes:   []string{"crm", "crm"},
		},
		{
			name:           "mixed prefix and bare",
			input:          "crm:P1->E33",
			wantLocalNames: []string{"P1", "E33"},
			wantPrefixes:   []string{"crm", ""},
		},
		{
			name:           "leading arrow stripped",
			input:          "->P1->E33",
			wantLocalNames: []string{"P1", "E33"},
			wantPrefixes:   []string{"", ""},
		},
		{
			// TrimSpace on input, then TrimSpace on each segment.
			// "  crm:P1  ->  E33  " → TrimSpace → "crm:P1  ->  E33"
			// split by "->" → ["crm:P1  ", "  E33  "]
			// each seg trimmed → ["crm:P1", "E33"]
			name:           "surrounding whitespace trimmed",
			input:          "  crm:P1  ->  E33  ",
			wantLocalNames: []string{"P1", "E33"},
			wantPrefixes:   []string{"crm", ""},
		},
		{
			// After stripping leading "->", input is "". Split by "->" gives [""].
			// Empty segment skipped → both slices remain empty.
			name:           "only arrow",
			input:          "->",
			wantLocalNames: []string{},
			wantPrefixes:   []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotLocal, gotPrefixes := parsePath(tc.input)

			if diff := cmp.Diff(tc.wantLocalNames, gotLocal); diff != "" {
				t.Errorf("parsePath(%q) localNames mismatch (-want +got):\n%s", tc.input, diff)
			}

			if diff := cmp.Diff(tc.wantPrefixes, gotPrefixes); diff != "" {
				t.Errorf("parsePath(%q) prefixes mismatch (-want +got):\n%s", tc.input, diff)
			}
		})
	}
}

func TestIntersect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base []string
		next []string
		want []string
		// sortBeforeCompare is true when map iteration order means the result
		// order is non-deterministic (overlap cases).
		sortBeforeCompare bool
	}{
		{
			// nil base = unconstrained; function returns next directly.
			name: "nil base returns next",
			base: nil,
			next: []string{"a", "b"},
			want: []string{"a", "b"},
		},
		{
			// empty (non-nil) base means the set is constrained to nothing.
			name: "empty base returns empty",
			base: []string{},
			next: []string{"a", "b"},
			want: []string{},
		},
		{
			name: "empty next returns empty",
			base: []string{"a"},
			next: []string{},
			want: []string{},
		},
		{
			name: "disjoint sets return empty",
			base: []string{"a", "b"},
			next: []string{"c", "d"},
			want: []string{},
		},
		{
			name:              "overlapping sets return intersection",
			base:              []string{"a", "b", "c"},
			next:              []string{"b", "c", "d"},
			want:              []string{"b", "c"},
			sortBeforeCompare: true,
		},
		{
			name:              "identical sets return all elements",
			base:              []string{"a", "b"},
			next:              []string{"a", "b"},
			want:              []string{"a", "b"},
			sortBeforeCompare: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := intersect(tc.base, tc.next)

			// Ensure non-nil result for comparison: intersect guarantees non-nil
			// for non-nil base inputs.
			if tc.base != nil && got == nil {
				t.Fatalf("intersect returned nil, want non-nil empty slice")
			}

			want := tc.want
			if tc.sortBeforeCompare {
				sortedGot := append([]string(nil), got...)
				sortedWant := append([]string(nil), want...)
				sort.Strings(sortedGot)
				sort.Strings(sortedWant)
				if diff := cmp.Diff(sortedWant, sortedGot); diff != "" {
					t.Errorf("intersect(%v, %v) mismatch after sort (-want +got):\n%s", tc.base, tc.next, diff)
				}
				return
			}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("intersect(%v, %v) mismatch (-want +got):\n%s", tc.base, tc.next, diff)
			}
		})
	}
}

func TestNormalizeScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string falls back to project",
			input: "",
			want:  "project",
		},
		{
			name:  "project returns project",
			input: "project",
			want:  "project",
		},
		{
			name:  "inherited returns inherited",
			input: "inherited",
			want:  "inherited",
		},
		{
			name:  "unknown value falls back to project",
			input: "garbage",
			want:  "project",
		},
		{
			// Strict string match: "Project" != "inherited", so falls back to "project".
			name:  "capital P Project falls back to project",
			input: "Project",
			want:  "project",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := normalizeScope(tc.input)
			if got != tc.want {
				t.Errorf("normalizeScope(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
