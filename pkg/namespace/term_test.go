package namespace

import "testing"

func TestTermAliasRequiresExactExpansion(t *testing.T) {
	aliases := []Binding{{Prefix: "crm", Namespace: "https://example.org/long-namespace/"}, {Prefix: "crm", Namespace: "https://other.example/"}}
	for _, tc := range []struct {
		ref, uri string
		want     bool
	}{
		{"crm:E1", "https://example.org/long-namespace/E1", true},
		{"crm:E1", "https://other.example/E1", true},
		{"crm:E1", "https://example.org/long-namespace/E12", false},
		{"crm:E1", "https://example.org/long-namespace/extra/E1", false},
		{"crm:E1", "https://example.org/", false},
		{"crm:E1", "", false},
		{"https://example.org/E1", "https://example.org/E1", true},
		{"https://example.org/E1", "https://other.example/E1", false},
	} {
		if got := MatchesTerm(tc.ref, tc.uri, "", "", aliases); got != tc.want {
			t.Errorf("%s -> %s: %v, want %v", tc.ref, tc.uri, got, tc.want)
		}
	}
}

func BenchmarkTermAliases(b *testing.B) {
	aliases := []Binding{{Prefix: "crm", Namespace: "https://example.org/long-namespace/"}}
	b.ReportAllocs()
	for b.Loop() {
		if !MatchesTerm("crm:E1", "https://example.org/long-namespace/E1", "", "", aliases) {
			b.Fatal("missing term")
		}
	}
}
