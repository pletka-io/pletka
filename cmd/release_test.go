package cmd

import "testing"

// TestBackfillProgressed pins the materialize-missing drain progress guard:
// a stuck release change set keeps processed_at NULL forever, so the loop
// must stop on a pass that fails to shrink the remaining count rather than
// spinning on ProcessPending's returned count alone.
func TestBackfillProgressed(t *testing.T) {
	for _, tc := range []struct {
		name string
		prev int
		curr int
		want bool
	}{
		{"decreased -> progressed", 5, 3, true},
		{"decreased to zero -> progressed", 1, 0, true},
		{"unchanged -> stuck", 5, 5, false},
		{"increased -> stuck", 5, 6, false},
		{"both zero -> stuck (no-op case)", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := backfillProgressed(tc.prev, tc.curr); got != tc.want {
				t.Fatalf("backfillProgressed(%d, %d) = %v, want %v", tc.prev, tc.curr, got, tc.want)
			}
		})
	}
}

// TestFormatValidationFields pins the archive CLI's error rendering: field
// order must be deterministic (sorted), and the operator must see the actual
// field message rather than the generic "validation error" that
// release.ErrValidation.Error() alone would produce.
func TestFormatValidationFields(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields map[string][]string
		want   string
	}{
		{
			name:   "single field single message",
			fields: map[string][]string{"message": {"an archive message is required"}},
			want:   "message: an archive message is required",
		},
		{
			name:   "single field multiple messages",
			fields: map[string][]string{"version": {"required", "must be semver"}},
			want:   "version: required; must be semver",
		},
		{
			name: "multiple fields sorted by name",
			fields: map[string][]string{
				"version": {"required"},
				"message": {"an archive message is required"},
			},
			want: "message: an archive message is required, version: required",
		},
		{
			name:   "empty map",
			fields: map[string][]string{},
			want:   "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatValidationFields(tc.fields); got != tc.want {
				t.Fatalf("formatValidationFields(%v) = %q, want %q", tc.fields, got, tc.want)
			}
		})
	}
}
