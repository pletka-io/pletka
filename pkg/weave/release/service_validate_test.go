package release

import "testing"

func TestValidateCreate_Version(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		version  string
		wantErr  bool
		wantHint string
	}{
		{"empty", "", true, "version is required"},
		{"whitespace only", "   ", true, "version is required"},
		{"shorthand 0.5", "0.5", true, "MAJOR.MINOR.PATCH"},
		{"shorthand 1.0", "1.0", true, "MAJOR.MINOR.PATCH"},
		{"label only", "v1", true, "MAJOR.MINOR.PATCH"},
		{"text", "alpha", true, "MAJOR.MINOR.PATCH"},
		{"valid 0.5.0", "0.5.0", false, ""},
		{"valid 1.2.3", "1.2.3", false, ""},
		{"valid pre-release", "1.2.3-rc.1", false, ""},
		{"valid build metadata", "1.2.3+build.42", false, ""},
		{"trims whitespace", "  1.0.0  ", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateCreate(CreateInput{Version: tc.version})
			if tc.wantErr {
				if len(errs["version"]) == 0 {
					t.Fatalf("expected version error, got none for %q", tc.version)
				}
				found := false
				for _, msg := range errs["version"] {
					if contains(msg, tc.wantHint) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got %v", tc.wantHint, errs["version"])
				}
				return
			}
			if len(errs) != 0 {
				t.Errorf("unexpected errors for %q: %v", tc.version, errs)
			}
		})
	}
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
