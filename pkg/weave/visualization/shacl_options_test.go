package visualization

import (
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/weave/generators"
)

func TestParseSHACLOptions(t *testing.T) {
	for _, tt := range []struct {
		query string
		want  generators.SHACLOptions
	}{
		{"", generators.SHACLOptions{}},
		{"links=class&closed=0&targets=core&concepts=list", generators.SHACLOptions{}},
		{"links=iri", generators.SHACLOptions{LinksIRIOnly: true}},
		{"closed=1", generators.SHACLOptions{Closed: true}},
		{"targets=sparql", generators.SHACLOptions{SPARQLTargets: true}},
		{"concepts=advisory", generators.SHACLOptions{ConceptsAdvisory: true}},
	} {
		q, _ := url.ParseQuery(tt.query)
		got, err := parseSHACLOptions(q)
		if err != nil {
			t.Fatalf("%q: %v", tt.query, err)
		}
		if diff := cmp.Diff(tt.want, got); diff != "" {
			t.Fatalf("%q: %s", tt.query, diff)
		}
	}
}

func TestSHACLOptionRejectsUnknownValue(t *testing.T) {
	for _, query := range []string{"links=identity", "closed=yes", "targets=x", "concepts=all"} {
		q, _ := url.ParseQuery(query)
		if _, err := parseSHACLOptions(q); err == nil {
			t.Fatalf("%q: want an error", query)
		}
	}
}
