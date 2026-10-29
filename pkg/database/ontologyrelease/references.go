package ontologyrelease

import (
	"slices"
	"sort"

	"github.com/pletka-io/pletka/pkg/namespace"
)

func (p *publisher) references() error {
	// Body bindings are an alias registry, not an effective prefix map. Resolve
	// against retained terms using the same matcher as project publications.
	refs := []string{}
	for _, r := range p.body.Relations {
		refs = append(refs, r.Target)
	}
	for _, term := range p.body.Properties {
		if term.Inverse != "" {
			refs = append(refs, term.Inverse)
		}
	}
	refs = unique(refs)
	for _, external := range p.opts.ExternalReferences {
		if !slices.Contains(refs, external) {
			return invalid("external reference %s is not used by the body", external)
		}
	}
	owners := make([]string, 0, len(p.sources))
	for owner := range p.sources {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	for _, ref := range refs {
		requirement := Requirement{Kind: "reference", Name: ref}
		if slices.Contains(p.opts.ExternalReferences, ref) {
			requirement.Disposition = "external"
		} else {
			disposition, owner, err := p.classifyReference(ref, owners)
			if err != nil {
				return err
			}
			requirement.Disposition, requirement.OntologyID = disposition, owner
		}
		p.requirements = append(p.requirements, requirement)
	}
	return nil
}

func (p *publisher) classifyReference(ref string, owners []string) (string, string, error) {
	local := matchingTermCount(p.body, ref, p.body.Bindings)
	if local > 1 {
		return "", "", invalid("reference %s is ambiguous within the local ontology body", ref)
	}
	if local == 1 {
		return "local", "", nil
	}
	matchedOwner := ""
	for _, owner := range owners {
		body := p.sources[owner]
		aliases := append(slices.Clone(p.body.Bindings), body.Bindings...)
		matches := matchingTermCount(body, ref, aliases)
		if matches > 1 || (matches == 1 && matchedOwner != "") {
			return "", "", invalid("reference %s is ambiguous between pinned ontology terms", ref)
		}
		if matches == 1 {
			matchedOwner = owner
		}
	}
	if matchedOwner == "" {
		return "", "", invalid("reference %s is absent from the publication closure; provide an ontology pin or classify the exact reference as external", ref)
	}
	return "pinned", matchedOwner, nil
}

func matchingTermCount(b bodyDocument, ref string, aliases []namespace.Binding) int {
	seen := map[string]bool{}
	for _, terms := range [][]bodyTerm{b.Classes, b.Properties} {
		for _, term := range terms {
			if namespace.MatchesTerm(ref, term.URI, term.Prefix, term.LocalName, aliases) {
				seen[term.URI] = true
			}
		}
	}
	return len(seen)
}
