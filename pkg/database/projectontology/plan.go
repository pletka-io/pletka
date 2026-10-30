// Package projectontology selects a project's published ontology closure without
// writing. Draft reads and release capture use the same selection algorithm.
package projectontology

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/ontologyartifact"
)

// Candidate retains only policy used to choose the defaults publication.
type Candidate struct {
	OntologyID, Prefix, VersionID string
	Active, Published             bool
}

// Reader has only the two SELECT capabilities needed by planning.
type Reader interface {
	Candidates(context.Context) ([]Candidate, error)
	Publication(context.Context, string) (ontologyartifact.Artifact, error)
}

// Plan chooses explicit linked/reference versions first, then defaults, before
// merging inherited closures. A conflicting source cannot silently select policy.
func Plan(ctx context.Context, r Reader, selected []string, inherited []ontologyartifact.Pin, hasReferences bool) ([]ontologyartifact.Artifact, error) {
	explicit := map[string]string{}
	direct := []ontologyartifact.Artifact{}
	for _, version := range selected {
		a, err := r.Publication(ctx, version)
		if err != nil {
			return nil, fmt.Errorf("read selected ontology %s: %w", version, err)
		}
		d, err := a.Decode()
		if err != nil {
			return nil, err
		}
		if d.VersionID != version {
			return nil, invalid("published ontology selection identity mismatch")
		}
		if previous, ok := explicit[d.OntologyID]; ok && previous != version {
			return nil, invalid("project selects conflicting ontology versions for %s", d.OntologyID)
		}
		explicit[d.OntologyID] = version
		direct = append(direct, a)
	}
	if len(selected) > 0 || hasReferences {
		a, err := defaults(ctx, r, explicit)
		if err != nil {
			return nil, err
		}
		if a.Digest != "" {
			direct = append(direct, a)
		}
	}
	p := planner{reader: r, versions: map[string]string{}, artifacts: map[string]ontologyartifact.Artifact{}, active: map[string]bool{}}
	for _, a := range direct {
		if err := p.add(ctx, a); err != nil {
			return nil, err
		}
	}
	for _, pin := range inherited {
		if err := p.pin(ctx, pin); err != nil {
			return nil, err
		}
	}
	owners := make([]string, 0, len(p.artifacts))
	for owner := range p.artifacts {
		owners = append(owners, owner)
	}
	slices.Sort(owners)
	out := make([]ontologyartifact.Artifact, 0, len(owners))
	for _, owner := range owners {
		out = append(out, p.artifacts[owner])
	}
	return out, nil
}

func defaults(ctx context.Context, r Reader, explicit map[string]string) (ontologyartifact.Artifact, error) {
	candidates, err := r.Candidates(ctx)
	if err != nil {
		return ontologyartifact.Artifact{}, err
	}
	// Match capture's deterministic first defaults owner, including an unpublished
	// or missing active selection: neither may disappear and trigger another choice.
	slices.SortFunc(candidates, func(a, b Candidate) int {
		if c := strings.Compare(a.OntologyID, b.OntologyID); c != 0 {
			return c
		}
		return strings.Compare(a.VersionID, b.VersionID)
	})
	owner := ""
	for _, c := range candidates {
		if c.Prefix == "defaults" {
			owner = c.OntologyID
			break
		}
	}
	if owner == "" {
		return ontologyartifact.Artifact{}, nil
	}
	version, hasExplicit := explicit[owner]
	for _, c := range candidates {
		if c.OntologyID != owner || c.VersionID == "" {
			continue
		}
		if (hasExplicit && c.VersionID == version) || (!hasExplicit && c.Active) {
			if !c.Published {
				return ontologyartifact.Artifact{}, invalid("defaults ontology %s must be published", c.VersionID)
			}
			return r.Publication(ctx, c.VersionID)
		}
	}
	return ontologyartifact.Artifact{}, invalid("defaults ontology has no selected published version")
}

type planner struct {
	reader    Reader
	versions  map[string]string
	artifacts map[string]ontologyartifact.Artifact
	active    map[string]bool
}

func (p *planner) pin(ctx context.Context, pin ontologyartifact.Pin) error {
	a, err := p.reader.Publication(ctx, pin.VersionID)
	if err != nil {
		return err
	}
	d, err := a.Decode()
	if err != nil {
		return err
	}
	if d.OntologyID != pin.OntologyID || d.VersionID != pin.VersionID || a.Digest != pin.Digest {
		return invalid("ontology dependency identity or digest mismatch")
	}
	return p.add(ctx, a)
}
func (p *planner) add(ctx context.Context, a ontologyartifact.Artifact) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d, err := a.Decode()
	if err != nil {
		return err
	}
	if p.active[d.OntologyID] {
		return invalid("cyclic project ontology closure")
	}
	if version, ok := p.versions[d.OntologyID]; ok {
		if version != d.VersionID || p.artifacts[d.OntologyID].Digest != a.Digest {
			return invalid("conflicting project ontology closure for %s", d.OntologyID)
		}
		return nil
	}
	p.versions[d.OntologyID], p.artifacts[d.OntologyID], p.active[d.OntologyID] = d.VersionID, a, true
	for _, pin := range d.Pins {
		if err := p.pin(ctx, pin); err != nil {
			return err
		}
	}
	delete(p.active, d.OntologyID)
	return nil
}
