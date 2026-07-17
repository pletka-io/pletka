package serverruntime

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestApplyInstanceOverridesMergesConfiguredOrigins verifies that per-deploy
// config-supplied AllowedOrigins are merged with (not clobbered by) the
// derived https://<instance>.pletka.io origin.
func TestApplyInstanceOverridesMergesConfiguredOrigins(t *testing.T) {
	cfg := ApplyInstanceOverrides(Config{
		Instance:       "prod",
		AllowedOrigins: []string{"https://pletka.io"},
	})

	want := []string{"https://prod.pletka.io", "https://pletka.io"}
	if len(cfg.AllowedOrigins) != len(want) {
		t.Fatalf("AllowedOrigins = %#v, want %#v", cfg.AllowedOrigins, want)
	}
	for i := range want {
		if cfg.AllowedOrigins[i] != want[i] {
			t.Fatalf("AllowedOrigins = %#v, want %#v", cfg.AllowedOrigins, want)
		}
	}
}

// TestApplyInstanceOverridesDefaultsToDerivedOriginWhenEmpty verifies that
// with no config-supplied origins, the derived instance origin is still
// applied on its own.
func TestApplyInstanceOverridesDefaultsToDerivedOriginWhenEmpty(t *testing.T) {
	cfg := ApplyInstanceOverrides(Config{
		Instance:       "beta",
		AllowedOrigins: nil,
	})

	want := []string{"https://beta.pletka.io"}
	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != want[0] {
		t.Fatalf("AllowedOrigins = %#v, want %#v", cfg.AllowedOrigins, want)
	}
}

// TestApplyInstanceOverridesLeavesOriginsUnchangedWithoutInstance verifies
// the early-return path: with no Instance set, config-supplied
// AllowedOrigins pass through untouched and no derived origin is injected.
func TestApplyInstanceOverridesLeavesOriginsUnchangedWithoutInstance(t *testing.T) {
	cfg := ApplyInstanceOverrides(Config{
		Instance:       "",
		AllowedOrigins: []string{"https://x.example"},
	})

	want := []string{"https://x.example"}
	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != want[0] {
		t.Fatalf("AllowedOrigins = %#v, want %#v", cfg.AllowedOrigins, want)
	}
}

// TestApplyInstanceOverridesDedupesDerivedOrigin verifies that when the
// config-supplied AllowedOrigins already contains the derived instance
// origin, it appears exactly once in the result.
func TestApplyInstanceOverridesDedupesDerivedOrigin(t *testing.T) {
	cfg := ApplyInstanceOverrides(Config{
		Instance:       "prod",
		AllowedOrigins: []string{"https://prod.pletka.io"},
	})

	want := []string{"https://prod.pletka.io"}
	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != want[0] {
		t.Fatalf("AllowedOrigins = %#v, want %#v (no duplicate)", cfg.AllowedOrigins, want)
	}
}

// TestParseAllowedOrigins covers both sources viper.GetStringSlice can hand
// back for cors.allowed_origins: a YAML list (already one element per
// origin) and a comma-separated PLETKA_CORS_ALLOWED_ORIGINS env value
// (which viper.GetStringSlice returns as a single whitespace-split element,
// not comma-split — see parseAllowedOrigins doc comment).
func TestParseAllowedOrigins(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "comma-separated env value is split",
			in:   []string{"https://a.io,https://b.io"},
			want: []string{"https://a.io", "https://b.io"},
		},
		{
			name: "yaml list is left unchanged",
			in:   []string{"https://a.io", "https://b.io"},
			want: []string{"https://a.io", "https://b.io"},
		},
		{
			name: "whitespace is trimmed and empty entries dropped",
			in:   []string{"https://a.io, https://b.io ", ""},
			want: []string{"https://a.io", "https://b.io"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAllowedOrigins(tt.in)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("parseAllowedOrigins(%#v) mismatch (-want +got):\n%s", tt.in, diff)
			}
		})
	}
}
