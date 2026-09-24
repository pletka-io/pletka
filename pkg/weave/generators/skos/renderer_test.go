package skos

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestRender_ConceptSchemeWithHierarchy(t *testing.T) {
	ns := map[string]string{"pletka": "https://data.pletka.io/"}
	scheme := domain.ConceptList{
		Entity: domain.Entity{SemanticID: "SEM.CL.6", UIName: domain.Translations{"en": "Colors"}},
	}
	concepts := []domain.VocabularyEntry{
		{ID: "A", URI: "pletka:concept/A", Label: domain.Translations{"en": "Crimson"}, ScopeNote: domain.Translations{"en": "A deep red."}},
		{ID: "B", URI: "pletka:concept/B", Label: domain.Translations{"en": "Red"}},
	}
	// A (Crimson) is narrower than B (Red).
	edges := []domain.ConceptBroaderEdge{{ConceptID: "A", BroaderID: "B"}}

	var b strings.Builder
	if err := Render(&b, scheme, concepts, edges, ns); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := b.String()

	must := []string{
		"@prefix skos: <http://www.w3.org/2004/02/skos/core#> .",
		"@prefix pletka: <https://data.pletka.io/> .",
		"<https://data.pletka.io/scheme/SEM.CL.6> a skos:ConceptScheme",
		"a skos:Concept",
		"skos:inScheme <https://data.pletka.io/scheme/SEM.CL.6>",
		"skos:prefLabel \"Crimson\"@en",
		"skos:scopeNote \"A deep red.\"@en",
		"skos:broader <https://data.pletka.io/concept/B>",
		"skos:narrower <https://data.pletka.io/concept/A>",
	}
	for _, m := range must {
		if !strings.Contains(out, m) {
			t.Errorf("output missing %q\n---\n%s", m, out)
		}
	}
}

func TestRender_SkipsEdgeWithUnknownEndpoint(t *testing.T) {
	ns := map[string]string{"pletka": "https://data.pletka.io/"}
	scheme := domain.ConceptList{Entity: domain.Entity{SemanticID: "SEM.CL.1", UIName: domain.Translations{"en": "X"}}}
	concepts := []domain.VocabularyEntry{{ID: "A", URI: "pletka:concept/A", Label: domain.Translations{"en": "Red"}}}
	// broader points at B, which isn't in the concepts slice -> skipped.
	edges := []domain.ConceptBroaderEdge{{ConceptID: "A", BroaderID: "B"}}

	var b strings.Builder
	if err := Render(&b, scheme, concepts, edges, ns); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(b.String(), "skos:broader") {
		t.Errorf("edge with unknown endpoint should be skipped\n---\n%s", b.String())
	}
}
