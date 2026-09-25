//go:build integration

package vocabulary_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

// TestRenderConceptListSKOS_Integration proves the endpoint renders a concept
// list as a SKOS concept scheme in Turtle, with the pletka: curie expanded to
// the configured namespace and hierarchy edges emitted (#3599).
func TestRenderConceptListSKOS_Integration(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed exec: %v\n%s", err, sql)
		}
	}
	exec(`INSERT INTO weave_actors (id, display_name, slug) VALUES ('sko','SKO','sko')`)
	exec(`INSERT INTO weave_projects (id, owner_id) VALUES ('SKOSP','sko')`)
	exec(`INSERT INTO weave_vocabularies (id, connector_type, project_id) VALUES ('SKV','local','SKOSP')`)
	exec(`INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label) VALUES ('SA','SKV','pletka:concept/SA','{"en":"Crimson"}')`)
	exec(`INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label) VALUES ('SB','SKV','pletka:concept/SB','{"en":"Red"}')`)
	exec(`INSERT INTO weave_concept_lists (id, project_id, semantic_id) VALUES ('SKCL','SKOSP','SKOSP.CL.1')`)
	exec(`INSERT INTO weave_concept_list_entries (id, concept_list_id, vocabulary_entry_id, position) VALUES ('J1','SKCL','SA',1)`)
	exec(`INSERT INTO weave_concept_list_entries (id, concept_list_id, vocabulary_entry_id, position) VALUES ('J2','SKCL','SB',2)`)
	exec(`INSERT INTO weave_concept_broader (id, concept_id, broader_id, scheme_id) VALUES ('E1','SA','SB','SKCL')`)

	svc := vocabulary.NewService(pool, nil)
	var buf bytes.Buffer
	if err := svc.RenderConceptListSKOS(ctx, "SKOSP", "SKCL", &buf); err != nil {
		t.Fatalf("RenderConceptListSKOS: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"@prefix skos: <http://www.w3.org/2004/02/skos/core#> .",
		"@prefix pletka: <https://vocab.pletka.io/> .",
		"<https://vocab.pletka.io/scheme/SKOSP.CL.1> a skos:ConceptScheme",
		"<https://vocab.pletka.io/concept/SA> a skos:Concept",
		"skos:prefLabel \"Crimson\"@en",
		"skos:broader <https://vocab.pletka.io/concept/SB>",
		"skos:narrower <https://vocab.pletka.io/concept/SA>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("SKOS output missing %q\n---\n%s", want, out)
		}
	}
}
