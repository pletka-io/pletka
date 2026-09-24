// Package skos emits a concept list and its hierarchy as a SKOS concept
// scheme in Turtle. It is deliberately not wired through the generators
// Renderer registry: those renderers project a schema Snapshot (project /
// models / fields), which carries no vocabulary data. SKOS emission is fed
// from concept-list, entry, and broader-edge data instead (#3599).
package skos

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// skosNS is the canonical SKOS namespace, always bound in the output.
const skosNS = "http://www.w3.org/2004/02/skos/core#"

// Render writes scheme + its concepts as SKOS Turtle. Concept and broader
// endpoints are VocabularyEntry IDs; their IRIs come from the concepts slice,
// so a broader edge is only emitted when both endpoints are present there.
// namespaces maps a curie prefix (e.g. "pletka") to its IRI base; an absolute
// http(s) URI is emitted as-is.
//
// ponytail: in-scheme edges only — a cross-scheme broader whose target isn't in
// `concepts` is skipped. Pass the union of concepts to emit cross-scheme edges.
func Render(w io.Writer, scheme domain.ConceptList, concepts []domain.VocabularyEntry, edges []domain.ConceptBroaderEdge, namespaces map[string]string) error {
	var b strings.Builder

	fmt.Fprintf(&b, "@prefix skos: <%s> .\n", skosNS)
	for _, p := range sortedKeys(namespaces) {
		fmt.Fprintf(&b, "@prefix %s: <%s> .\n", p, namespaces[p])
	}
	b.WriteString("\n")

	schemeIRI := term("pletka:scheme/"+scheme.SemanticID, namespaces)
	fmt.Fprintf(&b, "%s a skos:ConceptScheme", schemeIRI)
	for _, lang := range sortedKeys(scheme.UIName) {
		fmt.Fprintf(&b, " ;\n    skos:prefLabel %s", literal(scheme.UIName[lang], lang))
	}
	b.WriteString(" .\n\n")

	// entry ID -> IRI, for resolving broader/narrower edge endpoints.
	iriByID := make(map[string]string, len(concepts))
	for _, c := range concepts {
		iriByID[c.ID] = term(c.URI, namespaces)
	}

	// edge (A narrower) --broader--> (B): A skos:broader B, B skos:narrower A.
	broaderOf := map[string][]string{}
	narrowerOf := map[string][]string{}
	for _, e := range edges {
		a, okA := iriByID[e.ConceptID]
		bIRI, okB := iriByID[e.BroaderID]
		if !okA || !okB {
			continue
		}
		broaderOf[e.ConceptID] = append(broaderOf[e.ConceptID], bIRI)
		narrowerOf[e.BroaderID] = append(narrowerOf[e.BroaderID], a)
	}

	ordered := append([]domain.VocabularyEntry(nil), concepts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].URI < ordered[j].URI })

	for _, c := range ordered {
		fmt.Fprintf(&b, "%s a skos:Concept ;\n    skos:inScheme %s", iriByID[c.ID], schemeIRI)
		for _, lang := range sortedKeys(c.Label) {
			fmt.Fprintf(&b, " ;\n    skos:prefLabel %s", literal(c.Label[lang], lang))
		}
		for _, lang := range sortedKeys(c.ScopeNote) {
			fmt.Fprintf(&b, " ;\n    skos:scopeNote %s", literal(c.ScopeNote[lang], lang))
		}
		for _, t := range sortedUnique(broaderOf[c.ID]) {
			fmt.Fprintf(&b, " ;\n    skos:broader %s", t)
		}
		for _, t := range sortedUnique(narrowerOf[c.ID]) {
			fmt.Fprintf(&b, " ;\n    skos:narrower %s", t)
		}
		b.WriteString(" .\n\n")
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// term renders a URI or curie as a Turtle IRI. An absolute http(s) URI is
// wrapped verbatim; a curie whose prefix is in namespaces is expanded.
func term(v string, namespaces map[string]string) string {
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return "<" + v + ">"
	}
	if i := strings.Index(v, ":"); i > 0 {
		if base, ok := namespaces[v[:i]]; ok {
			return "<" + base + v[i+1:] + ">"
		}
	}
	return "<" + v + ">"
}

// literal renders a language-tagged Turtle string literal.
func literal(value, lang string) string {
	esc := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r").Replace(value)
	if lang == "" {
		return "\"" + esc + "\""
	}
	return "\"" + esc + "\"@" + lang
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedUnique(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
