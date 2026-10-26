package ontologyrelease

import (
	"slices"
	"sort"

	"github.com/pletka-io/pletka/pkg/namespace"
)

func (p *publisher) references() error {
	// Share ADR-0004's effective-prefix rule with project snapshots. Provenance
	// ontology_id on a namespace binding is NOT proof of term ownership.
	effective, err := namespace.Effective(p.body.Bindings)
	if err != nil {
		return invalid("publication namespaces: %v", err)
	}
	mgr := namespace.NewStore()
	for _, b := range effective {
		if _, err := mgr.Put(b.Prefix, b.Namespace, 1); err != nil {
			return err
		}
	}
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
			uri, err := mgr.Expand(ref)
			if err != nil {
				return invalid("reference %s cannot be resolved: %v", ref, err)
			}
			if hasTerm(p.body, uri) {
				requirement.Disposition = "local"
			} else {
				for _, owner := range owners {
					if hasTerm(p.sources[owner], uri) {
						if requirement.OntologyID != "" {
							return invalid("reference %s is ambiguous between pinned ontology bodies", ref)
						}
						requirement.OntologyID = owner
						requirement.Disposition = "pinned"
					}
				}
				if requirement.OntologyID == "" {
					return invalid("reference %s is absent from the publication closure; provide an ontology pin or classify the exact reference as external", ref)
				}
			}
		}
		p.requirements = append(p.requirements, requirement)
	}
	return nil
}

func hasTerm(b bodyDocument, uri string) bool {
	for _, term := range b.Classes {
		if term.URI == uri {
			return true
		}
	}
	for _, term := range b.Properties {
		if term.URI == uri {
			return true
		}
	}
	return false
}
