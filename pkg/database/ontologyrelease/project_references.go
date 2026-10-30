package ontologyrelease

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/namespace"
	"github.com/pletka-io/pletka/pkg/ontologyartifact"
)

// TermReference preserves the authored input at a captured semantic location.
type TermReference = ontologyartifact.TermReference

// TermResolution retains the exact captured meaning of a semantic reference.
type TermResolution = ontologyartifact.TermResolution

func captureProjectReferences(ctx context.Context, q *sqlcgen.Queries, p *publisher, project, version string, refs []sqlcgen.WeaveProjectSemanticReferencesRow) error {
	rows, err := q.WeaveListReleaseNamespaceBindings(ctx, sqlcgen.WeaveListReleaseNamespaceBindingsParams{ProjectID: project, Version: version})
	if err != nil {
		return err
	}
	aliases := make([]namespace.Binding, 0, len(rows))
	for _, row := range rows {
		aliases = append(aliases, namespace.Binding{Prefix: row.Prefix, Namespace: row.Namespace})
	}
	resolutions := make([]TermResolution, 0, len(refs))
	for _, row := range refs {
		var ref TermReference
		if err := json.Unmarshal(row.Reference, &ref); err != nil {
			return err
		}
		resolution, err := resolveProjectReference(p, ref, aliases)
		if err != nil {
			return invalid("%s %s%s: %v", row.EntityKind, row.EntityID, row.Location, err)
		}
		resolution.EntityKind, resolution.EntityID, resolution.Location = row.EntityKind, row.EntityID, row.Location
		resolutions = append(resolutions, resolution)
	}
	data, err := json.Marshal(resolutions)
	if err != nil {
		return err
	}
	return q.WeaveCaptureProjectSemanticReferences(ctx, sqlcgen.WeaveCaptureProjectSemanticReferencesParams{ProjectID: project, Version: version, Resolutions: data})
}

func resolveProjectReference(p *publisher, ref TermReference, aliases []namespace.Binding) (TermResolution, error) {
	if ref.Kind != "class" && ref.Kind != "property" {
		return TermResolution{}, fmt.Errorf("unsupported semantic kind %q", ref.Kind)
	}
	if (ref.Prefix == "") != (ref.LocalName == "") {
		return TermResolution{}, fmt.Errorf("incomplete QName")
	}
	if ref.URI == "" && ref.Prefix == "" {
		return TermResolution{}, fmt.Errorf("empty semantic reference")
	}
	owners := make([]string, 0, len(p.sources))
	for owner := range p.sources {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	matches := make([]TermResolution, 0, 1)
	for _, owner := range owners {
		pin, body := p.pins[owner], p.sources[owner]
		if ref.VersionID != "" && ref.VersionID != pin.VersionID {
			continue
		}
		bindings := append(append([]namespace.Binding{}, aliases...), body.Bindings...)
		terms := body.Classes
		if ref.Kind == "property" {
			terms = body.Properties
		}
		seen := map[string]bool{}
		for _, term := range terms {
			if ref.URI != "" && !namespace.MatchesTerm(ref.URI, term.URI, term.Prefix, term.LocalName, bindings) {
				continue
			}
			if ref.Prefix != "" && !namespace.MatchesTerm(ref.Prefix+":"+ref.LocalName, term.URI, term.Prefix, term.LocalName, bindings) {
				continue
			}
			if seen[term.URI] {
				continue
			}
			seen[term.URI] = true
			matches = append(matches, TermResolution{Reference: ref, OntologyID: owner, VersionID: pin.VersionID, Digest: pin.Digest, URI: term.URI})
		}
	}
	if len(matches) != 1 {
		return TermResolution{}, fmt.Errorf("semantic reference %q (%s:%s, %s) matches %d terms in the selected published closure; correct the reference or link/pin its ontology", ref.URI, ref.Prefix, ref.LocalName, ref.Kind, len(matches))
	}
	return matches[0], nil
}
