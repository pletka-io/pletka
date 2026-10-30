// Package projectread assembles complete detached values on an existing read
// session. It owns neither a pool nor a transaction and exposes no partial result.
package projectread

import (
	"context"
	"errors"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

// Options configures resource budgets and draft-only module hosts at wiring time.
// Released identities always come from captured descriptors.
type Options struct {
	Limits Limits
	Hosts  releasedescriptor.Hosts
}

// DefaultOptions supplies bounded assembly with the established module hosts.
func DefaultOptions() Options {
	return Options{Limits: Limits{PageSize: 256, MaxRows: 1_000_000, MaxBytes: 256 << 20, MaxProjects: 256}, Hosts: releasedescriptor.Hosts{Project: releasedescriptor.DefaultModuleHost, Ontology: releasedescriptor.DefaultOntologyHost}}
}

// Assembler has configuration only. Each call owns its graph and budget.
type Assembler struct{ options Options }

// New requires explicit positive budgets; zero does not mean unlimited.
func New(options Options) (*Assembler, error) {
	if err := options.Limits.validate(); err != nil {
		return nil, err
	}
	return &Assembler{options: options}, nil
}

// Project loads complete own/source content sequentially through the supplied
// session. The caller authorizes the root; the session's factory authorizes every
// source. All database work and validation finish before the value is returned.
func (a *Assembler) Project(ctx context.Context, s *readsession.Session) (projectsnapshot.CompleteProject, error) {
	if a == nil {
		return projectsnapshot.CompleteProject{}, fmt.Errorf("project assembler is uninitialized")
	}
	if err := a.options.Limits.validate(); err != nil {
		return projectsnapshot.CompleteProject{}, err
	}
	if err := s.Check(ctx); err != nil {
		return projectsnapshot.CompleteProject{}, err
	}
	sources, err := s.Dependencies(ctx)
	if err != nil {
		return projectsnapshot.CompleteProject{}, err
	}
	if len(sources) >= a.options.Limits.MaxProjects {
		return projectsnapshot.CompleteProject{}, &BudgetError{Resource: "projects"}
	}
	b := budget{limits: a.options.Limits}
	inputs := make([]projectsnapshot.ProjectInput, 0, len(sources))
	for _, source := range sources {
		bound, err := s.ProjectDependency(ctx, source.ProjectID)
		if err != nil {
			return projectsnapshot.CompleteProject{}, err
		}
		in, err := a.load(ctx, bound, &b, nil)
		if err != nil {
			return projectsnapshot.CompleteProject{}, fmt.Errorf("assemble source %s@%s: %w", source.ProjectID, source.Version, err)
		}
		inputs = append(inputs, in)
	}
	root, err := a.load(ctx, s, &b, inputs)
	if err != nil {
		return projectsnapshot.CompleteProject{}, err
	}
	value, err := projectsnapshot.NewComplete(root, inputs)
	if err != nil {
		if s.Scope().IsRelease() {
			err = errors.Join(&readsession.NotInReleaseError{ProjectID: s.ProjectID(), Version: s.Scope().Version(), EntityID: "complete project graph"}, err)
		}
		return projectsnapshot.CompleteProject{}, fmt.Errorf("validate project %s: %w", s.ProjectID(), err)
	}
	if err := ctx.Err(); err != nil {
		return projectsnapshot.CompleteProject{}, err
	}
	return value, nil
}

func (a *Assembler) load(ctx context.Context, s *readsession.Session, b *budget, sources []projectsnapshot.ProjectInput) (projectsnapshot.ProjectInput, error) {
	var evidence projectsnapshot.Evidence
	var err error
	if s.Scope().IsRelease() {
		evidence, err = releasedEvidence(ctx, s)
		if err != nil {
			return projectsnapshot.ProjectInput{}, err
		}
	}
	content, counts, err := loadContent(ctx, s, b)
	if err != nil {
		return projectsnapshot.ProjectInput{}, err
	}
	if !s.Scope().IsRelease() {
		// Match capture's adoption deduplication and concrete source receipt pins.
		for i := range content.Adoptions {
			source, err := s.ProjectDependency(ctx, content.Adoptions[i].SourceProjectID)
			if err != nil {
				return projectsnapshot.ProjectInput{}, err
			}
			content.Adoptions[i].SourceVersion = source.Scope().Version()
		}
		evidence, err = a.draftEvidence(ctx, s, content, sources)
		if err != nil {
			return projectsnapshot.ProjectInput{}, err
		}
	}
	if err := b.evidence(evidence); err != nil {
		return projectsnapshot.ProjectInput{}, err
	}
	deps, err := s.Dependencies(ctx)
	if err != nil {
		return projectsnapshot.ProjectInput{}, err
	}
	members, err := s.Membership(ctx)
	if err != nil {
		return projectsnapshot.ProjectInput{}, err
	}
	in := projectsnapshot.ProjectInput{Content: content, Counts: counts, Evidence: evidence, Dependencies: make([]projectsnapshot.Identity, 0, len(deps)), Membership: make([]projectsnapshot.Membership, 0, len(members))}
	for _, source := range deps {
		id, err := projectsnapshot.Release(source.ProjectID, source.Version)
		if err != nil {
			return projectsnapshot.ProjectInput{}, err
		}
		in.Dependencies = append(in.Dependencies, id)
	}
	for _, member := range members {
		id, err := projectsnapshot.Release(member.Source.ProjectID, member.Source.Version)
		if err != nil {
			return projectsnapshot.ProjectInput{}, err
		}
		in.Membership = append(in.Membership, projectsnapshot.Membership{Kind: member.Kind, ID: member.ID, Source: id})
	}
	return in, nil
}
