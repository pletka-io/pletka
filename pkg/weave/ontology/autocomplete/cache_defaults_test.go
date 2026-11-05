package autocomplete

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
)

// defaultsFakeStore returns a fake store in which the defaults ontology
// exists with the given active version ID ("" = not imported).
func defaultsFakeStore(activeVersionID string) *fakeIndexStore {
	s := &fakeIndexStore{
		versions:      map[string]*domain.OntologyVersion{},
		ontologies:    map[string]*domain.Ontology{},
		byPrefix:      map[string]*domain.Ontology{},
		activeVersion: map[string]*domain.OntologyVersion{},
	}
	if activeVersionID == "" {
		return s
	}
	const ontID = "ont-defaults"
	s.byPrefix[domain.DefaultsOntologyPrefix] = &domain.Ontology{ID: ontID, Prefix: domain.DefaultsOntologyPrefix, Namespace: "https://example.org/defaults/"}
	s.activeVersion[ontID] = &domain.OntologyVersion{ID: activeVersionID, OntologyID: ontID, VersionString: "1.1", IsActive: true}
	s.versions[activeVersionID] = s.activeVersion[ontID]
	s.ontologies[ontID] = s.byPrefix[domain.DefaultsOntologyPrefix]
	return s
}

func TestResolve_AppendsDefaultsActiveVersion(t *testing.T) {
	tests := []struct {
		name     string
		defaults string // active defaults version ID, "" = none imported
		links    []domain.ResolvedOntologyVersion
		want     []string
	}{
		{
			name:     "defaults imported: appended once",
			defaults: "v-defaults",
			links:    []domain.ResolvedOntologyVersion{minimalVersionLink("v-own", true)},
			want:     []string{"v-own", "v-defaults"},
		},
		{
			name:     "defaults not imported: version set unchanged",
			defaults: "",
			links:    []domain.ResolvedOntologyVersion{minimalVersionLink("v-own", true)},
			want:     []string{"v-own"},
		},
		{
			name:     "project links defaults explicitly: no duplicate",
			defaults: "v-defaults",
			links:    []domain.ResolvedOntologyVersion{minimalVersionLink("v-own", true), minimalVersionLink("v-defaults", false)},
			want:     []string{"v-own", "v-defaults"},
		},
		{
			name:     "project with no links: defaults alone is not an index",
			defaults: "v-defaults",
			links:    nil,
			want:     nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewIndexCache(defaultsFakeStore(tc.defaults), &fakeProjectReader{links: tc.links}, testLogger())
			got, primary, err := c.resolve(context.Background(), Request{ProjectID: "p1"})
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("version set mismatch (-want +got):\n%s", diff)
			}
			if len(tc.links) > 0 && primary != "v-own" {
				t.Errorf("primary = %q, want v-own", primary)
			}
		})
	}
}

func TestResolve_PinnedVersionIgnoresDefaults(t *testing.T) {
	c := NewIndexCache(defaultsFakeStore("v-defaults"), &fakeProjectReader{}, testLogger())
	got, _, err := c.resolve(context.Background(), Request{VersionID: "v-pinned"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if diff := cmp.Diff([]string{"v-pinned"}, got); diff != "" {
		t.Errorf("pinned request must not gain defaults (-want +got):\n%s", diff)
	}
}
