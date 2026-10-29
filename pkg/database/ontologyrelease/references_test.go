package ontologyrelease

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/namespace"
)

func TestPublicationReferencesPreserveAliasesAndRejectAmbiguousTerms(t *testing.T) {
	term := bodyTerm{URI: "https://old.example/A", Prefix: "ex", LocalName: "A"}
	aliases := []namespace.Binding{
		{Prefix: "ex", Namespace: "https://old.example/", Weight: 40},
		{Prefix: "ex", Namespace: "https://new.example/", Weight: 40},
		{Prefix: "alias", Namespace: "https://old.example/"},
	}
	body := bodyDocument{Classes: []bodyTerm{term}, Bindings: aliases}
	other := bodyDocument{Classes: []bodyTerm{{URI: "https://new.example/A", Prefix: "ex", LocalName: "A"}}}
	ambiguous := body
	ambiguous.Classes = append([]bodyTerm{term}, other.Classes...)
	for _, tc := range []struct {
		name, ref, disposition, owner, failure string
		local                                  bodyDocument
		sources                                map[string]bodyDocument
	}{
		{name: "local QName with tied aliases", ref: "ex:A", local: body, disposition: "local"},
		{name: "exact local URI", ref: term.URI, local: body, disposition: "local"},
		{name: "declared alias", ref: "alias:A", local: body, disposition: "local"},
		{name: "source aliases", ref: "alias:A", sources: map[string]bodyDocument{"source": body}, disposition: "pinned", owner: "source"},
		{name: "phantom URI", ref: "https://new.example/A", local: body, failure: "absent"},
		{name: "unknown QName", ref: "unknown:A", local: body, failure: "absent"},
		{name: "local ambiguity", ref: "ex:A", local: ambiguous, failure: "ambiguous"},
		{name: "exact URI disambiguates", ref: term.URI, local: ambiguous, disposition: "local"},
		{name: "source body ambiguity", ref: "ex:A", sources: map[string]bodyDocument{"source": ambiguous}, failure: "ambiguous"},
		{name: "source owner ambiguity", ref: "ex:A", sources: map[string]bodyDocument{"first": body, "second": other}, failure: "ambiguous"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := publisher{body: tc.local, sources: tc.sources}
			p.body.Relations = append(p.body.Relations, struct {
				Target string `json:"target_qname"`
			}{tc.ref})
			err := p.references()
			if tc.failure != "" {
				if err == nil || !strings.Contains(err.Error(), tc.failure) {
					t.Fatalf("wanted %s, got %v", tc.failure, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(p.requirements) != 1 || p.requirements[0].Disposition != tc.disposition || p.requirements[0].OntologyID != tc.owner || p.requirements[0].Name != tc.ref {
				t.Fatalf("reference identity/disposition changed: %+v", p.requirements)
			}
		})
	}
}
