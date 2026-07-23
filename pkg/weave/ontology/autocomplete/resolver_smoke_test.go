//go:build integration

package autocomplete_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
)

// TestScopeResolver_Smoke exercises the slice scope resolver against a
// live DB containing a project with linked ontology versions. It is a
// smoke check that the slice scope resolver behaves correctly on real data.
//
// Skipped when no DB is available (CI without postgres) or when the
// expected fixture project (LA) has no ontology coverage.
func TestScopeResolver_Smoke(t *testing.T) {
	pool := testPool(t)

	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)
	svc := weaveontology.NewService(store, ws.Projects(), nil)

	resolver, err := svc.NewScopeResolver(context.Background(), "LA")
	if err == autocomplete.ErrNoCoverage {
		t.Fatal("fixture regression: project LA has no ontology coverage (fixture LA links 5 ontology versions in test/fixtures/LA/project.yaml; this must not be empty)")
	}
	if err != nil {
		t.Fatalf("NewScopeResolver: %v", err)
	}

	cases := []struct {
		name       string
		candidates []string
		wantNil    bool
		wantPrefix string // expected qname prefix on a successful match
	}{
		{
			name:       "exact qname",
			candidates: []string{"crm:E21_Person"},
			wantPrefix: "crm",
		},
		{
			name:       "bare local name",
			candidates: []string{"E21_Person"},
			wantPrefix: "crm",
		},
		{
			name:       "scope id only (CRM short code)",
			candidates: []string{"E21"},
			wantPrefix: "crm",
		},
		{
			name:       "free text underscored",
			candidates: []string{"E21_Person"},
			wantPrefix: "crm",
		},
		{
			name:       "case insensitive normalization",
			candidates: []string{"e21_person"},
			wantPrefix: "crm",
		},
		{
			name:       "unknown returns nil",
			candidates: []string{"NotAClass_FooBar"},
			wantNil:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			match := resolver.Resolve(tc.candidates...)
			if tc.wantNil {
				if match != nil {
					t.Errorf("expected nil, got match=%+v", match)
				}
				return
			}
			if match == nil {
				t.Fatalf("expected match, got nil for candidates=%v", tc.candidates)
			}
			if match.Element.Prefix != tc.wantPrefix {
				t.Errorf("prefix=%q, want %q (qname=%q match=%s)", match.Element.Prefix, tc.wantPrefix, match.Element.URI, match.Match)
			}
			t.Logf("%s: %s qname=%s match=%s relevance=%.2f",
				tc.name, tc.candidates[0], match.Element.URI, match.Match, match.Relevance)
		})
	}
}

func testPool(t *testing.T) *pgxpool.Pool {
	return testdb.Pool(t)
}
