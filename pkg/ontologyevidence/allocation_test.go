package ontologyevidence

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/namespace"
)

func TestBoundedHashPreservesExactBytes(t *testing.T) {
	for _, size := range []int{0, 1, 63, 64, 65, 32767, 32768, 32769, 1048576} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			// Includes multibyte UTF-8, NUL and invalid UTF-8; hashing must not
			// normalize bytes, including a code point split across chunks.
			data := strings.Repeat("\x00雪\xffé", size/7+1)[:size]
			want := fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
			if got := hash(data); got != want {
				t.Fatalf("digest changed: %s != %s", got, want)
			}
		})
	}
}

func TestAssessmentPreservesBothAliasRegistriesAndAmbiguity(t *testing.T) {
	project := []namespace.Binding{{Prefix: "alias", Namespace: "https://first/", Weight: 80}}
	bodyAliases := []namespace.Binding{{Prefix: "alias", Namespace: "https://second/", Weight: 20}}
	wantProject, wantBody := slices.Clone(project), slices.Clone(bodyAliases)
	terms := []term{{URI: "https://first/Thing"}, {URI: "https://second/Thing"}}
	bodies := []bodyDocument{{Classes: terms, Properties: terms, Bindings: bodyAliases}}
	inventory := []Body{{OntologyID: "o", VersionID: "v", Digest: "retained"}}
	for _, kind := range []string{classKind, propertyKind} {
		for _, tc := range []struct {
			name string
			ref  TermReference
			uris []string
		}{
			{"conflicting alias", TermReference{Kind: kind, Prefix: "alias", LocalName: "Thing"}, []string{"https://first/Thing", "https://second/Thing"}},
			{"project alias and exact URI", TermReference{Kind: kind, URI: "https://first/Thing", Prefix: "alias", LocalName: "Thing"}, []string{"https://first/Thing"}},
			{"body alias and exact URI", TermReference{Kind: kind, URI: "https://second/Thing", Prefix: "alias", LocalName: "Thing"}, []string{"https://second/Thing"}},
			{"unknown alias", TermReference{Kind: kind, Prefix: "unknown", LocalName: "Thing"}, []string{}},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				status, candidates := assess(tc.ref, inventory, bodies, project)
				wantStatus := Resolved
				if len(tc.uris) == 0 {
					wantStatus = Missing
				} else if len(tc.uris) > 1 {
					wantStatus = Ambiguous
				}
				if status != wantStatus || len(candidates) != len(tc.uris) {
					t.Fatalf("assessment changed: %s %+v", status, candidates)
				}
				for i, uri := range tc.uris {
					if candidates[i] != (Candidate{OntologyID: "o", VersionID: "v", Digest: "retained", URI: uri}) {
						t.Fatalf("candidate identity changed: %+v", candidates[i])
					}
				}
			})
		}
	}
	if diff := cmp.Diff(wantProject, project); diff != "" {
		t.Fatal(diff)
	}
	if diff := cmp.Diff(wantBody, bodyAliases); diff != "" {
		t.Fatal(diff)
	}
}

func BenchmarkEvidenceHash(b *testing.B) {
	data := strings.Repeat("x", 1024*1024)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if hash(data) == "" {
			b.Fatal("empty digest")
		}
	}
}

func BenchmarkAssessmentAliasRegistries(b *testing.B) {
	aliases := make([]namespace.Binding, 256)
	for i := range aliases {
		aliases[i] = namespace.Binding{Prefix: "alias", Namespace: fmt.Sprintf("https://example/%d/", i)}
	}
	bodies := []bodyDocument{{Classes: []term{{URI: "https://example/255/Thing"}}, Bindings: aliases}}
	inventory := []Body{{OntologyID: "o", VersionID: "v"}}
	ref := TermReference{Kind: classKind, Prefix: "alias", LocalName: "Thing"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if status, _ := assess(ref, inventory, bodies, aliases); status != Resolved {
			b.Fatal(status)
		}
	}
}
