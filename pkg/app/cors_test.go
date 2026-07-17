package app

import "testing"

// TestMatchOriginWildcard locks the suffix-anchored wildcard semantics for
// matchOrigin: "https://*.pletka.io" must match the exact base domain and
// its subdomains, but never an unrelated origin or a suffix-spoofing origin
// like "https://app.pletka.io.evil.com". It also locks the "host:*"
// port-wildcard dev pattern used by defaultAllowedOrigins.
func TestMatchOriginWildcard(t *testing.T) {
	cases := []struct {
		origin, pattern string
		want            bool
	}{
		// Brief's core cases.
		{"https://app.pletka.io", "https://*.pletka.io", true},
		{"https://evil.com", "https://*.pletka.io", false},
		{"https://app.pletka.io.evil.com", "https://*.pletka.io", false},
		{"https://pletka.io", "https://pletka.io", true},
		{"https://evil.com", "https://pletka.io", false},

		// Apex domain also matches the wildcard pattern (not just subdomains).
		{"https://pletka.io", "https://*.pletka.io", true},

		// Dev port-wildcard pattern must keep working.
		{"http://localhost:3333", "http://localhost:*", true},
		{"http://localhost:8080", "http://localhost:*", true},
		{"http://evil.com:3333", "http://localhost:*", false},
		{"http://127.0.0.1:5000", "http://127.0.0.1:*", true},

		// Single-level subdomain match.
		{"https://a.pletka.io", "https://*.pletka.io", true},

		// Deep (multi-level) subdomain: the implementation's suffix check
		// (host minus ".pletka.io" must not contain "/") does not restrict
		// to a single label, so "sub.a.pletka.io" DOES match. This is a
		// truthful assertion of current behavior, not an endorsement that
		// it's "direct subdomain only" as the code comment claims.
		{"https://sub.a.pletka.io", "https://*.pletka.io", true},

		// Scheme mismatch must not match even with a matching host suffix.
		{"http://app.pletka.io", "https://*.pletka.io", false},
	}
	for _, c := range cases {
		if got := matchOrigin(c.origin, c.pattern); got != c.want {
			t.Errorf("matchOrigin(%q, %q) = %v, want %v", c.origin, c.pattern, got, c.want)
		}
	}
}

// TestMatchOriginNoBareWildcard confirms a bare "*" pattern (old match-all
// behavior) is no longer honored — no config in this codebase configures a
// bare "*" AllowedOrigins entry, so removing match-all is a security
// improvement, not a behavior regression for any real deployment.
func TestMatchOriginNoBareWildcard(t *testing.T) {
	if matchOrigin("https://evil.com", "*") {
		t.Error("matchOrigin(\"https://evil.com\", \"*\") = true, want false (bare wildcard match-all removed)")
	}
}
